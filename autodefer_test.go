package starlings

import (
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// callLog records the REST calls an interaction makes, in order.
type callLog struct {
	mu     sync.Mutex
	calls  []string
	bodies []string
}

func (l *callLog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}
	path := r.URL.Path[strings.Index(r.URL.Path, "/api/v10")+len("/api/v10"):]
	// Hide the token segment so expectations stay readable.
	parts := strings.Split(path, "/")
	switch {
	case len(parts) > 3 && parts[1] == "interactions":
		parts = append([]string{"", "interactions", "T"}, parts[4:]...)
	case len(parts) > 3 && parts[1] == "webhooks":
		parts[3] = "T"
	}
	entry := r.Method + " " + strings.Join(parts, "/")
	if len(body) > 0 {
		var probe struct {
			Type  int `json:"type"`
			Flags int `json:"flags"`
			Data  struct {
				Flags int `json:"flags"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &probe)
		probe.Flags |= probe.Data.Flags
		if probe.Type != 0 {
			entry += " type=" + itoa(probe.Type)
		}
		if probe.Flags != 0 {
			entry += " flags=" + itoa(probe.Flags)
		}
	}
	l.mu.Lock()
	l.calls = append(l.calls, entry)
	l.bodies = append(l.bodies, string(body))
	l.mu.Unlock()
	if r.Method == http.MethodPost && parts[1] == "webhooks" || r.Method == http.MethodPatch {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"5"}`))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (l *callLog) sentBodies() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.bodies, "\n")
}

func (l *callLog) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

func freshSnowflake() Snowflake {
	return Snowflake(uint64(time.Now().UnixMilli()-discordEpoch) << 22)
}

func autoDeferClient(t *testing.T, after time.Duration) (*Client, *callLog) {
	t.Helper()
	log := &callLog{}
	c := clientServedBy(log, WithAutoDefer(after))
	return c, log
}

func commandInteraction(c *Client, name string) *InteractionCreate {
	i := &InteractionCreate{Interaction: Interaction{
		ID: freshSnowflake(), ApplicationID: 2, Token: "secret", Type: InteractionApplicationCommand,
		Data: InteractionData{Name: name},
	}}
	i.bind(c)
	return i
}

func assertCalls(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestFastHandlerAnswersOnceWithoutDefer(t *testing.T) {
	c, log := autoDeferClient(t, 100*time.Millisecond)
	c.Slash("fast", "fast", func(i *InteractionCreate) {
		if err := i.Reply("hi"); err != nil {
			t.Error(err)
		}
	})
	c.routeInteraction(commandInteraction(c, "fast"))
	time.Sleep(200 * time.Millisecond)
	assertCalls(t, log.get(), "POST /interactions/T/callback type=4")
}

func TestSlowHandlerReplyEditsDeferredResponse(t *testing.T) {
	c, log := autoDeferClient(t, 30*time.Millisecond)
	errs := make(chan error, 1)
	c.Slash("slow", "slow", func(i *InteractionCreate) {
		time.Sleep(120 * time.Millisecond)
		errs <- i.Reply("done")
	})
	c.routeInteraction(commandInteraction(c, "slow"))
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	assertCalls(t, log.get(),
		"POST /interactions/T/callback type=5",
		"PATCH /webhooks/2/T/messages/@original")
}

func TestSlowEphemeralReplyReplacesPublicDefer(t *testing.T) {
	c, log := autoDeferClient(t, 30*time.Millisecond)
	errs := make(chan error, 1)
	c.Slash("secret", "secret", func(i *InteractionCreate) {
		time.Sleep(120 * time.Millisecond)
		errs <- i.ReplyEphemeral("only you")
	})
	c.routeInteraction(commandInteraction(c, "secret"))
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	assertCalls(t, log.get(),
		"POST /interactions/T/callback type=5",
		"DELETE /webhooks/2/T/messages/@original",
		"POST /webhooks/2/T flags=64")
}

func TestEphemeralRouteDefersPrivately(t *testing.T) {
	c, log := autoDeferClient(t, 30*time.Millisecond)
	errs := make(chan error, 1)
	c.Slash("private", "private", func(i *InteractionCreate) {
		time.Sleep(120 * time.Millisecond)
		errs <- i.ReplyEphemeral("only you")
	}).Ephemeral()
	c.routeInteraction(commandInteraction(c, "private"))
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	assertCalls(t, log.get(),
		"POST /interactions/T/callback type=5 flags=64",
		"PATCH /webhooks/2/T/messages/@original")
}

func TestSlowComponentUpdateEditsMessage(t *testing.T) {
	c, log := autoDeferClient(t, 30*time.Millisecond)
	errs := make(chan error, 1)
	c.OnComponent("page:next", func(i *InteractionCreate) {
		time.Sleep(120 * time.Millisecond)
		errs <- i.UpdateMessage(InteractionResponseData{Content: "page 2"})
	})
	i := &InteractionCreate{Interaction: Interaction{
		ID: freshSnowflake(), ApplicationID: 2, Token: "secret", Type: InteractionMessageComponent,
		Data: InteractionData{CustomID: "page:next"}, Message: &Message{ID: 9},
	}}
	i.bind(c)
	c.routeInteraction(i)
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	assertCalls(t, log.get(),
		"POST /interactions/T/callback type=6",
		"PATCH /webhooks/2/T/messages/@original")
}

func TestModalAfterAutoDeferIsAnError(t *testing.T) {
	c, _ := autoDeferClient(t, 30*time.Millisecond)
	errs := make(chan error, 1)
	c.Slash("form", "form", func(i *InteractionCreate) {
		time.Sleep(120 * time.Millisecond)
		errs <- i.Modal("f", "Form")
	})
	c.routeInteraction(commandInteraction(c, "form"))
	if err := <-errs; !errors.Is(err, ErrAutoDeferred) {
		t.Fatalf("Modal after auto-defer = %v, want ErrAutoDeferred", err)
	}
}

func TestNoAutoDeferRoute(t *testing.T) {
	c, log := autoDeferClient(t, 30*time.Millisecond)
	done := make(chan struct{})
	c.Slash("manual", "manual", func(i *InteractionCreate) {
		time.Sleep(100 * time.Millisecond)
		close(done)
	}).NoAutoDefer()
	c.routeInteraction(commandInteraction(c, "manual"))
	<-done
	if calls := log.get(); len(calls) != 0 {
		t.Fatalf("NoAutoDefer route made calls: %v", calls)
	}
}

func TestSecondReplyStillFailsAfterAutoDefer(t *testing.T) {
	c, _ := autoDeferClient(t, 30*time.Millisecond)
	errs := make(chan error, 2)
	c.Slash("twice", "twice", func(i *InteractionCreate) {
		time.Sleep(120 * time.Millisecond)
		errs <- i.Reply("one")
		errs <- i.Defer(false)
	})
	c.routeInteraction(commandInteraction(c, "twice"))
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	// A late Defer after the automatic one is harmless.
	if err := <-errs; err != nil {
		t.Fatalf("Defer after auto-defer = %v, want nil", err)
	}
}
