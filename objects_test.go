package starlings

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// restRecorder answers every request with an empty object and records them.
type restRecorder struct {
	callLog
}

func (r *restRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/users/@me/channels") {
		r.callLog.mu.Lock()
		r.calls = append(r.calls, "POST /users/@me/channels")
		r.callLog.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"777","type":1}`))
		return
	}
	if strings.Contains(req.URL.Path, "/members/") && req.Method == http.MethodGet {
		r.callLog.mu.Lock()
		r.calls = append(r.calls, "GET "+req.URL.Path[strings.Index(req.URL.Path, "/guilds"):])
		r.callLog.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"id":"9","username":"x"},"roles":[]}`))
		return
	}
	r.callLog.ServeHTTP(w, req)
}

func TestInteractionMemberActsOnItself(t *testing.T) {
	rec := &restRecorder{}
	c := clientServedBy(rec)
	got := make(chan error, 1)
	Slash(c, "ban", "Ban", func(_ *InteractionCreate, a banArgs) error {
		got <- a.User.Ban(a.Reason, time.Hour)
		return nil
	})
	c.routeInteraction(argsInteraction(c, "ban",
		`[{"name":"user","type":6,"value":"9"},{"name":"reason","type":3,"value":"spam"}]`,
		`{"users":{"9":{"id":"9","username":"target"}},"members":{"9":{"roles":[]}}}`))
	if err := <-got; err != nil {
		t.Fatal(err)
	}
	calls := rec.get()
	if len(calls) != 1 || calls[0] != "PUT /guilds/1/bans/9" {
		t.Fatalf("calls = %v", calls)
	}
	if !strings.Contains(rec.sentBodies(), `"delete_message_seconds":3600`) {
		t.Fatalf("ban body = %s", rec.sentBodies())
	}
}

func TestStateValuesAreBound(t *testing.T) {
	rec := &restRecorder{}
	c := clientServedBy(rec, WithStateMode(StateManual))
	if err := c.State.Apply(&GuildCreate{Guild: Guild{ID: 1, Members: []Member{{User: &User{ID: 9}}}}}); err != nil {
		t.Fatal(err)
	}
	m, ok := c.State.Member(1, 9)
	if !ok {
		t.Fatal("member not cached")
	}
	if err := m.Kick("bye"); err != nil {
		t.Fatal(err)
	}
	if calls := rec.get(); len(calls) != 1 || calls[0] != "DELETE /guilds/1/members/9" {
		t.Fatalf("calls = %v", calls)
	}
}

func TestMessageEventRepliesThroughMessage(t *testing.T) {
	rec := &restRecorder{}
	c := clientServedBy(rec)
	e := &MessageCreate{Message: Message{ID: 5, ChannelID: 42, GuildID: 1, Author: &User{ID: 9}, Member: &Member{}}}
	e.bind(c)
	msg := e.Message // a copy, as a handler might keep
	if _, err := msg.Reply("hi"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.sentBodies(), `"message_id":"5"`) {
		t.Fatalf("reply body = %s", rec.sentBodies())
	}
	if msg.Member.User == nil || msg.Member.User.ID != 9 || msg.Member.GuildID() != 1 {
		t.Fatal("the message's member was not completed and bound")
	}
}

func TestUserSendReusesDMChannel(t *testing.T) {
	rec := &restRecorder{}
	c := clientServedBy(rec)
	u := &User{ID: 9}
	u.bindTo(c)
	for range 2 {
		if _, err := u.Send("hello"); err != nil {
			t.Fatal(err)
		}
	}
	dms := 0
	for _, call := range rec.get() {
		if call == "POST /users/@me/channels" {
			dms++
		}
	}
	if dms != 1 {
		t.Fatalf("created the DM channel %d times, want 1: %v", dms, rec.get())
	}
}

func TestRESTResultsAreBound(t *testing.T) {
	rec := &restRecorder{}
	c := clientServedBy(rec)
	m, err := c.GuildMember(t.Context(), 1, 9)
	if err != nil {
		t.Fatal(err)
	}
	if m.GuildID() != 1 {
		t.Fatalf("member guild = %d, want 1", m.GuildID())
	}
	if err := m.AddRole(3, ""); err != nil {
		t.Fatal(err)
	}
	if calls := rec.get(); calls[len(calls)-1] != "PUT /guilds/1/members/9/roles/3" {
		t.Fatalf("calls = %v", calls)
	}
}

func TestUnboundValuesSayHowToFixIt(t *testing.T) {
	m := &Member{User: &User{ID: 9}}
	if err := m.Kick(""); !errors.Is(err, ErrUnbound) {
		t.Fatalf("Kick on a literal = %v, want ErrUnbound", err)
	}
	var sendable Sendable = &Channel{ID: 1}
	if _, err := sendable.Send("x"); !errors.Is(err, ErrUnbound) {
		t.Fatalf("Send on a literal = %v, want ErrUnbound", err)
	}
}

func TestMessageLink(t *testing.T) {
	msg := Message{ID: 3, ChannelID: 2, GuildID: 1}
	if got := msg.Link(); got != "https://discord.com/channels/1/2/3" {
		t.Fatalf("Link = %s", got)
	}
	msg.GuildID = 0
	if got := msg.Link(); got != "https://discord.com/channels/@me/2/3" {
		t.Fatalf("DM Link = %s", got)
	}
}
