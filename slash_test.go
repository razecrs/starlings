package starlings

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"strings"
	"testing"
)

// interactionFrame builds an INTERACTION_CREATE for a slash command with the
// given options, as Discord sends them.
func interactionFrame(name, options string) []byte {
	return interactionFrameOfType(name, options, InteractionApplicationCommand)
}

func interactionFrameOfType(name, options string, interactionType InteractionType) []byte {
	if options == "" {
		options = "[]"
	}
	return []byte(`{"t":"INTERACTION_CREATE","s":1,"op":0,"d":{` +
		`"id":"100","application_id":"200","type":` + itoa(int(interactionType)) + `,"token":"tok",` +
		`"data":{"id":"300","name":"` + name + `","type":1,"options":` + options + `}}}`)
}

func TestSlashRouting(t *testing.T) {
	c := testClient()

	called := ""
	c.Slash("ping", "check", func(i *InteractionCreate) { called = "ping" })
	c.Slash("say", "repeat", func(i *InteractionCreate) { called = "say" })

	deliver(t, c, interactionFrame("say", ""))
	if called != "say" {
		t.Errorf("routed to %q, want say", called)
	}

	called = ""
	deliver(t, c, interactionFrame("nosuchcommand", ""))
	if called != "" {
		t.Errorf("unknown command routed to %q, want no handler", called)
	}
}

func TestSlashAutocompleteIsSeparateFromExecution(t *testing.T) {
	c := testClient()
	executions := 0
	autocompletes := 0
	c.Slash("search", "find", func(*InteractionCreate) { executions++ }).
		Autocomplete(func(*InteractionCreate) { autocompletes++ })

	deliver(t, c, interactionFrameOfType("search", "", InteractionCommandAutocomplete))
	if executions != 0 || autocompletes != 1 {
		t.Fatalf("autocomplete routed executions=%d autocompletes=%d, want 0/1", executions, autocompletes)
	}

	deliver(t, c, interactionFrame("search", ""))
	if executions != 1 || autocompletes != 1 {
		t.Fatalf("command routed executions=%d autocompletes=%d, want 1/1", executions, autocompletes)
	}
}

func TestSlashRouteErgonomics(t *testing.T) {
	c := testClient()
	want := PermissionBanMembers | PermissionModerateMembers
	c.Slash("ban", "Ban a member", func(*InteractionCreate) {}).
		Permissions(want).
		GuildOnly().
		InstallTypes(IntegrationGuildInstall)

	defs := c.SlashDefinitions()
	if len(defs) != 1 || defs[0].DefaultMemberPermissions == nil || *defs[0].DefaultMemberPermissions != want {
		t.Fatalf("permissions = %#v, want %v", defs, want)
	}
	if len(defs[0].Contexts) != 1 || defs[0].Contexts[0] != InteractionContextGuild {
		t.Fatalf("contexts = %v, want guild only", defs[0].Contexts)
	}
	if len(defs[0].IntegrationTypes) != 1 || defs[0].IntegrationTypes[0] != IntegrationGuildInstall {
		t.Fatalf("integration types = %v, want guild install", defs[0].IntegrationTypes)
	}
}

// TestSlashCommandsShareOneHandler mirrors the prefix-command guarantee: many
// commands, one gateway subscription.
func TestSlashCommandsShareOneHandler(t *testing.T) {
	c := testClient()
	for _, n := range []string{"a", "b", "c"} {
		c.Slash(n, "d", func(*InteractionCreate) {})
	}

	slot := c.slotFor("INTERACTION_CREATE")
	if slot == nil {
		t.Fatal("no INTERACTION_CREATE handler registered")
	}
	if len(slot.handlers) != 1 {
		t.Errorf("registered %d handlers for 3 commands, want 1", len(slot.handlers))
	}
	if len(c.SlashDefinitions()) != 3 {
		t.Errorf("SlashDefinitions() has %d entries, want 3", len(c.SlashDefinitions()))
	}
}

func TestSlashOptionAccessors(t *testing.T) {
	c := testClient()

	var got *InteractionCreate
	c.Slash("test", "d", func(i *InteractionCreate) { got = i })

	opts := `[{"name":"text","type":3,"value":"hello"},` +
		`{"name":"count","type":4,"value":7},` +
		`{"name":"flag","type":5,"value":true},` +
		`{"name":"who","type":6,"value":"1472713114905743633"}]`
	deliver(t, c, interactionFrame("test", opts))

	if got == nil {
		t.Fatal("handler was not called")
	}
	if v := got.Data.Option("text").String(); v != "hello" {
		t.Errorf("text = %q, want hello", v)
	}
	if v := got.Data.Option("count").Int(); v != 7 {
		t.Errorf("count = %d, want 7", v)
	}
	if v := got.Data.Option("flag").Bool(); !v {
		t.Error("flag = false, want true")
	}
	if v := got.Data.Option("who").Snowflake(); v != 1472713114905743633 {
		t.Errorf("who = %d", v)
	}

	// A missing option must be safe to call straight through, so handlers can
	// read optional parameters without a nil check every time.
	if v := got.Data.Option("absent").String(); v != "" {
		t.Errorf("absent option = %q, want empty", v)
	}
	if v := got.Data.Option("absent").Int(); v != 0 {
		t.Errorf("absent option = %d, want 0", v)
	}
}

// TestSlashSubcommandOptionLookup checks that Option reaches into subcommands,
// so a handler need not know how deeply Discord nested the parameter.
func TestSlashSubcommandOptionLookup(t *testing.T) {
	c := testClient()
	var got *InteractionCreate
	c.Slash("config", "d", func(i *InteractionCreate) { got = i })

	opts := `[{"name":"set","type":1,"options":[{"name":"key","type":3,"value":"theme"}]}]`
	deliver(t, c, interactionFrame("config", opts))

	if got == nil {
		t.Fatal("handler was not called")
	}
	if v := got.Data.Option("key").String(); v != "theme" {
		t.Errorf("nested option = %q, want theme", v)
	}
}

// TestRespondOnlyOnce pins Discord's rule that an interaction takes exactly
// one initial response.
func TestRespondOnlyOnce(t *testing.T) {
	i := &InteractionCreate{}
	i.bind(testClient())

	// The first attempt gets past the guard and fails at the network, which
	// is fine - what matters is that the guard was consumed.
	_ = i.Respond(context.Background(), InteractionResponse{Type: CallbackPong})

	err := i.Respond(context.Background(), InteractionResponse{Type: CallbackPong})
	if !errors.Is(err, ErrInteractionAlreadyAnswered) {
		t.Fatalf("second response returned %v, want ErrInteractionAlreadyAnswered", err)
	}
}

func TestApplicationCommandJSON(t *testing.T) {
	cmd := ApplicationCommand{
		Type:        CommandChat,
		Name:        "say",
		Description: "Repeat something",
		Options: []CommandOption{
			StringOption("text", "What to say", true),
			UserOption("target", "Who to say it to", false),
		},
	}

	b, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"name":"say"`,
		`"type":1`,
		`"required":true`,
		`"type":3`, // OptionString
		`"type":6`, // OptionUser
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("marshalled command %s\nis missing %s", b, want)
		}
	}

	// An unset ID must not be sent as 0, which Discord rejects.
	if strings.Contains(string(b), `"id"`) {
		t.Errorf("empty ID should be omitted, got %s", b)
	}
}

func TestSyncCommandsNeedsReady(t *testing.T) {
	c := testClient()
	c.Slash("ping", "d", func(*InteractionCreate) {})

	if err := c.SyncCommands(context.Background(), 0); !errors.Is(err, errNotReady) {
		t.Errorf("SyncCommands before READY returned %v, want errNotReady", err)
	}
}
