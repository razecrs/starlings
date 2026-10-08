package starlings

import (
	"encoding/json/v2"
	"errors"
	"strings"
	"testing"
	"time"
)

type banArgs struct {
	User   *Member       `desc:"Who to ban"`
	Reason string        `desc:"Why" max:"400"`
	Delete time.Duration `desc:"Delete recent messages" optional:"" choices:"None=0s|Last hour=1h|Last day=24h"`
	Count  int           `desc:"How many" optional:"" min:"1" max:"100"`
	Where  *Channel      `desc:"Channel" optional:"" channel:"text,thread"`
}

func TestSlashArgsBuildsDiscordSchema(t *testing.T) {
	c := New("token")
	Slash(c, "ban", "Ban a member", func(*InteractionCreate, banArgs) error { return nil })
	defs := c.SlashDefinitions()
	if len(defs) != 1 {
		t.Fatalf("definitions = %d", len(defs))
	}
	opts := defs[0].Options
	names := make([]string, len(opts))
	for n, o := range opts {
		names[n] = o.Name
	}
	if got := strings.Join(names, ","); got != "user,reason,delete,count,where" {
		t.Fatalf("option order = %s", got)
	}
	if opts[0].Type != OptionUser || !opts[0].Required || opts[0].Description != "Who to ban" {
		t.Fatalf("user option = %+v", opts[0])
	}
	if opts[1].MaxLength == nil || *opts[1].MaxLength != 400 {
		t.Fatalf("reason max length = %v", opts[1].MaxLength)
	}
	if opts[2].Type != OptionString || opts[2].Required || len(opts[2].Choices) != 3 || opts[2].Choices[1].Value != "1h" {
		t.Fatalf("delete option = %+v", opts[2])
	}
	if opts[3].Type != OptionInteger || *opts[3].MinValue != 1 || *opts[3].MaxValue != 100 {
		t.Fatalf("count option = %+v", opts[3])
	}
	if len(opts[4].ChannelTypes) != 4 {
		t.Fatalf("channel types = %v", opts[4].ChannelTypes)
	}
}

func TestSlashArgsPutsRequiredOptionsFirst(t *testing.T) {
	type args struct {
		Note string `optional:""`
		User *User
	}
	c := New("token")
	Slash(c, "note", "Add a note", func(*InteractionCreate, args) error { return nil })
	opts := c.SlashDefinitions()[0].Options
	if opts[0].Name != "user" || opts[1].Name != "note" {
		t.Fatalf("Discord requires required options first, got %s then %s", opts[0].Name, opts[1].Name)
	}
}

func TestSlashArgsRejectsInvalidDefinitions(t *testing.T) {
	cases := map[string]func(c *Client){
		"uppercase command": func(c *Client) {
			Slash(c, "Ban", "x", func(*InteractionCreate, struct{}) error { return nil })
		},
		"empty description": func(c *Client) {
			Slash(c, "ban", "", func(*InteractionCreate, struct{}) error { return nil })
		},
		"unsupported field": func(c *Client) {
			type args struct{ M map[string]int }
			Slash(c, "ban", "x", func(*InteractionCreate, args) error { return nil })
		},
		"bad choice": func(c *Client) {
			type args struct {
				N int `choices:"One=uno"`
			}
			Slash(c, "ban", "x", func(*InteractionCreate, args) error { return nil })
		},
	}
	for name, register := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("invalid definition was accepted")
				}
			}()
			register(New("token"))
		})
	}
}

func argsInteraction(c *Client, name, options, resolved string) *InteractionCreate {
	raw := `{"id":"` + freshSnowflake().String() + `","application_id":"2","token":"t","type":2,"guild_id":"1",` +
		`"member":{"user":{"id":"50"},"permissions":"0"},"app_permissions":"8",` +
		`"data":{"name":"` + name + `","options":` + options + `,"resolved":` + resolved + `}}`
	var i InteractionCreate
	if err := jsonUnmarshalString(raw, &i); err != nil {
		panic(err)
	}
	i.bind(c)
	return &i
}

func TestSlashArgsDecodesValues(t *testing.T) {
	c, _ := autoDeferClient(t, 0)
	got := make(chan banArgs, 1)
	Slash(c, "ban", "Ban a member", func(_ *InteractionCreate, a banArgs) error {
		got <- a
		return nil
	})
	c.routeInteraction(argsInteraction(c, "ban",
		`[{"name":"user","type":6,"value":"9"},{"name":"reason","type":3,"value":"spam"},{"name":"delete","type":3,"value":"1d"}]`,
		`{"users":{"9":{"id":"9","username":"target"}},"members":{"9":{"roles":[]}}}`))
	a := <-got
	if a.User == nil || a.User.User == nil || a.User.User.ID != 9 || a.Reason != "spam" || a.Delete != 24*time.Hour || a.Count != 0 {
		t.Fatalf("decoded %+v", a)
	}
}

func TestSlashArgsExplainsNonMemberTarget(t *testing.T) {
	c, log := autoDeferClient(t, 0)
	called := false
	Slash(c, "ban", "Ban a member", func(*InteractionCreate, banArgs) error {
		called = true
		return nil
	})
	c.routeInteraction(argsInteraction(c, "ban",
		`[{"name":"user","type":6,"value":"9"},{"name":"reason","type":3,"value":"spam"}]`,
		`{"users":{"9":{"id":"9","username":"target"}}}`))
	if called {
		t.Fatal("handler ran for a user who is not in the server")
	}
	assertCalls(t, log.get(), "POST /interactions/T/callback type=4 flags=64")
}

func TestRequireChecksPermissionsAtRuntime(t *testing.T) {
	c, log := autoDeferClient(t, 0)
	called := false
	c.Slash("purge", "Purge", nil).Require(PermissionManageMessages).Run(func(*InteractionCreate) error {
		called = true
		return nil
	})
	if p := c.SlashDefinitions()[0].DefaultMemberPermissions; p == nil || *p != PermissionManageMessages {
		t.Fatal("Require did not set the default member permissions")
	}
	c.routeInteraction(argsInteraction(c, "purge", `[]`, `{}`))
	if called {
		t.Fatal("handler ran for a member without Manage Messages")
	}
	assertCalls(t, log.get(), "POST /interactions/T/callback type=4 flags=64")
}

func TestHandlerErrorsReachUserSafely(t *testing.T) {
	c, log := autoDeferClient(t, 0)
	c.Slash("fail", "Fail", nil).Run(func(*InteractionCreate) error {
		return errors.New("database password is hunter2")
	})
	c.Slash("deny", "Deny", nil).Run(func(*InteractionCreate) error {
		return UserErrorf("Warning #%d does not exist.", 7)
	})
	c.Slash("boom", "Boom", nil).Run(func(*InteractionCreate) error {
		var m map[string]int
		m["x"] = 1 // panics
		return nil
	})
	for _, name := range []string{"fail", "deny", "boom"} {
		c.routeInteraction(argsInteraction(c, name, `[]`, `{}`))
	}
	if calls := log.get(); len(calls) != 3 {
		t.Fatalf("calls = %v", calls)
	}
	sent := log.sentBodies()
	if strings.Contains(sent, "hunter2") || strings.Contains(sent, "panic") {
		t.Fatalf("internal error text reached Discord: %s", sent)
	}
	if !strings.Contains(sent, "Warning #7 does not exist.") {
		t.Fatalf("user error was not shown: %s", sent)
	}
	if strings.Count(sent, genericFailure) != 2 {
		t.Fatalf("generic failure shown %d times, want 2: %s", strings.Count(sent, genericFailure), sent)
	}
}

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"90":    90 * time.Second,
		"10m":   10 * time.Minute,
		"1h30m": 90 * time.Minute,
		"3d":    72 * time.Hour,
		"1w2d":  9 * 24 * time.Hour,
		"1.5h":  90 * time.Minute,
		"2h 5m": 125 * time.Minute,
	}
	for in, want := range cases {
		if got, err := ParseDuration(in); err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "soon", "5y", "h"} {
		if _, err := ParseDuration(bad); err == nil {
			t.Errorf("ParseDuration(%q) accepted bad input", bad)
		}
	}
}

func TestSnakeCase(t *testing.T) {
	for in, want := range map[string]string{"User": "user", "UserID": "user_id", "DeleteHistory": "delete_history", "HTTPCode": "http_code"} {
		if got := snakeCase(in); got != want {
			t.Errorf("snakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func jsonUnmarshalString(s string, v any) error { return json.Unmarshal([]byte(s), v) }
