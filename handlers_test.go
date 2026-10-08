package starlings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandlerShapes(t *testing.T) {
	cases := map[string]struct {
		handler any
		want    string
	}{
		"returns text":          {func() string { return "pong" }, `"content":"pong"`},
		"takes the interaction": {func(i *InteractionCreate) string { return "hi " + i.Invoker().ID.String() }, `"content":"hi 50"`},
		"returns an embed":      {func() *Embed { return NewEmbed("Card") }, `"title":"Card"`},
		"returns ephemeral":     {func() Response { return Ephemeral("secret") }, `"flags":64`},
		"returns text and nil":  {func() (string, error) { return "ok", nil }, `"content":"ok"`},
		"returns a user error":  {func() (string, error) { return "", UserErrorf("nope") }, `"content":"nope"`},
		"returns only an error": {func() error { return UserErrorf("denied") }, `"content":"denied"`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c, log := autoDeferClient(t, 0)
			c.Slash("x", "x", tc.handler)
			c.routeInteraction(argsInteraction(c, "x", `[]`, `{}`))
			if body := log.sentBodies(); !strings.Contains(body, tc.want) {
				t.Fatalf("sent %s, want it to contain %s", body, tc.want)
			}
		})
	}
}

func TestEmptyResultSendsNothing(t *testing.T) {
	c, log := autoDeferClient(t, 0)
	c.Slash("x", "x", func(i *InteractionCreate) string {
		_ = i.Reply("answered myself")
		return ""
	})
	c.routeInteraction(argsInteraction(c, "x", `[]`, `{}`))
	if calls := log.get(); len(calls) != 1 {
		t.Fatalf("calls = %v, want only the handler's own reply", calls)
	}
}

func TestBadHandlerShapesPanicAtRegistration(t *testing.T) {
	for name, handler := range map[string]any{
		"not a function":    "pong",
		"unknown parameter": func(int) string { return "" },
		"unsendable result": func() int { return 1 },
		"three results":     func() (string, string, error) { return "", "", nil },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("accepted a handler Starlings cannot call")
				}
			}()
			New(WithToken("token")).Slash("x", "x", handler)
		})
	}
}

func TestButtonFillsStructFromRoute(t *testing.T) {
	c, log := autoDeferClient(t, 0)
	c.Button("ticket:close:{id}", func(p struct{ ID int64 }) Response {
		return Update("closed #" + itoa(int(p.ID)))
	})
	c.routeInteraction(componentInteraction(c, "ticket:close:42", 9))
	if body := log.sentBodies(); !strings.Contains(body, `"type":7`) || !strings.Contains(body, "closed #42") {
		t.Fatalf("sent %s", body)
	}

	c.routeInteraction(componentInteraction(c, "ticket:close:nope", 9))
	if body := log.sentBodies(); !strings.Contains(body, `\"nope\" is not a valid id.`) {
		t.Fatalf("a forged parameter was not reported: %s", body)
	}
}

func TestModalFillsStructFromInputs(t *testing.T) {
	c, log := autoDeferClient(t, 0)
	type form struct {
		Subject string
		Details string `name:"details_text"`
	}
	c.Modal("ticket:open", func(f form) string { return f.Subject + "/" + f.Details })
	raw := `{"id":"` + freshSnowflake().String() + `","application_id":"2","token":"t","type":5,"guild_id":"1",` +
		`"member":{"user":{"id":"50"}},"data":{"custom_id":"ticket:open","components":[` +
		`{"type":18,"component":{"type":4,"custom_id":"subject","value":"Printer"}},` +
		`{"type":18,"component":{"type":4,"custom_id":"details_text","value":"On fire"}}]}}`
	var i InteractionCreate
	if err := jsonUnmarshalString(raw, &i); err != nil {
		t.Fatal(err)
	}
	i.bind(c)
	c.routeInteraction(&i)
	if body := log.sentBodies(); !strings.Contains(body, "Printer/On fire") {
		t.Fatalf("sent %s", body)
	}
}

func TestIntentsAreInferredFromHandlers(t *testing.T) {
	c := New(WithToken("token"))
	c.On(func(*GuildMemberAdd) {})
	c.On(func(*MessageReactionAdd) {})
	got := c.inferIntents()
	for _, want := range []Intent{IntentGuilds, IntentGuildMembers, IntentGuildMessageReactions} {
		if !got.Has(want) {
			t.Errorf("inferred intents miss %d", want)
		}
	}
	if got.Has(IntentMessageContent) {
		t.Error("message content was chosen without prefix commands")
	}
	c.Command("ping", func(*MessageCreate, []string) {})
	if !c.inferIntents().Has(IntentMessageContent) {
		t.Error("prefix commands need message content")
	}
}

func TestExplicitIntentsAreKept(t *testing.T) {
	c := New(WithToken("token"), WithIntents(IntentGuilds))
	c.On(func(*GuildMemberAdd) {})
	if err := c.prepareRun(); err != nil {
		t.Fatal(err)
	}
	if c.intents != IntentGuilds {
		t.Fatalf("intents = %d, want the explicit choice", c.intents)
	}
}

func TestCommandsMatchIgnoresDiscordDefaults(t *testing.T) {
	local := []ApplicationCommand{{Type: CommandChat, Name: "ping", Description: "Is the bot alive?"}}
	remote := []ApplicationCommand{{ID: 5, ApplicationID: 2, Type: CommandChat, Name: "ping", Description: "Is the bot alive?",
		Contexts: []InteractionContextType{0, 1, 2}, IntegrationTypes: []ApplicationIntegrationType{0}}}
	if !commandsMatch(local, remote) {
		t.Fatal("identical commands were reported as changed")
	}
	remote[0].Description = "old text"
	if commandsMatch(local, remote) {
		t.Fatal("a changed description was not noticed")
	}
}

func TestRunNeedsAToken(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "")
	os.Unsetenv("DISCORD_TOKEN")
	t.Chdir(t.TempDir())
	if err := New().prepareRun(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("prepareRun without a token = %v, want ErrNoToken", err)
	}
}

func TestDotEnvSuppliesToken(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "")
	os.Unsetenv("DISCORD_TOKEN")
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("# bot\nDISCORD_TOKEN=\"abc.def\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := New()
	if err := c.prepareRun(); err != nil {
		t.Fatal(err)
	}
	if c.token != "Bot abc.def" {
		t.Fatalf("token = %q", c.token)
	}
}

func TestGuardMeasuresEachCommand(t *testing.T) {
	c, _ := autoDeferClient(t, 0)
	c.guard = NewGuard()
	c.Slash("ping", "Ping", func() string { return "pong" })
	c.Slash("deny", "Deny", func() error { return UserErrorf("no") })
	c.routeInteraction(argsInteraction(c, "ping", `[]`, `{}`))
	c.routeInteraction(argsInteraction(c, "deny", `[]`, `{}`))
	report := c.guard.Report()
	seen := map[string]GuardMetric{}
	for _, m := range report.Metrics {
		seen[m.Feature] = m
	}
	if _, ok := seen["/ping"]; !ok {
		t.Fatalf("no metric for /ping in %v", report.Metrics)
	}
	if m := seen["/deny"]; m.Errors != 0 {
		t.Fatal("a user error was counted as a failure")
	}
}

func TestExplicitTurnsOffAutomaticBehaviour(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "from-env")
	c := New(Explicit())
	if c.token != "" || c.autoDefer != 0 || c.autoSync || c.recoverPanics || !c.intentsSet {
		t.Fatalf("Explicit left something on: token=%q defer=%v sync=%v recover=%v", c.token, c.autoDefer, c.autoSync, c.recoverPanics)
	}
	c = New(Explicit(), WithAutoDefer(time.Second))
	if c.autoDefer != time.Second {
		t.Fatal("an option after Explicit could not turn one behaviour back on")
	}
}

func TestPanicRecoveryCanBeTurnedOff(t *testing.T) {
	c, _ := autoDeferClient(t, 0)
	c.recoverPanics = false
	c.Slash("boom", "Boom", func() error { panic("boom") })
	defer func() {
		if recover() == nil {
			t.Fatal("the panic was recovered with recovery off")
		}
	}()
	c.routeInteraction(argsInteraction(c, "boom", `[]`, `{}`))
}
