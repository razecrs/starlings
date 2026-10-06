package starlings

import (
	"context"
	"strings"
	"testing"
)

// frameWithContent builds a MESSAGE_CREATE frame carrying the given text.
func frameWithContent(content string, fromBot bool) []byte {
	bot := "false"
	if fromBot {
		bot = "true"
	}
	return []byte(`{"t":"MESSAGE_CREATE","s":1,"op":0,"d":{` +
		`"id":"1","channel_id":"2","content":"` + content + `",` +
		`"author":{"id":"3","username":"raze","bot":` + bot + `}}}`)
}

func deliver(t *testing.T, c *Client, frame []byte) {
	t.Helper()
	var g gateway
	if err := g.handleFrame(context.Background(), c, frame); err != nil {
		t.Fatalf("handleFrame: %v", err)
	}
}

func TestCommandDispatch(t *testing.T) {
	c := testClient()

	var gotArgs []string
	called := 0
	c.Command("say", func(m *MessageCreate, args []string) {
		called++
		gotArgs = args
	})

	deliver(t, c, frameWithContent("!say hello there", false))

	if called != 1 {
		t.Fatalf("command ran %d times, want 1", called)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "hello" || gotArgs[1] != "there" {
		t.Errorf("args = %v, want [hello there]", gotArgs)
	}
}

func TestCommandIgnoresBotsAndNonCommands(t *testing.T) {
	c := testClient()
	called := 0
	c.Command("ping", func(*MessageCreate, []string) { called++ })

	// A bot's own message must not trigger a command, or a bot that replies
	// to its own trigger word loops forever.
	deliver(t, c, frameWithContent("!ping", true))
	// Ordinary chatter, and a different command, are both ignored.
	deliver(t, c, frameWithContent("ping", false))
	deliver(t, c, frameWithContent("!pong", false))

	if called != 0 {
		t.Errorf("command ran %d times, want 0", called)
	}

	deliver(t, c, frameWithContent("!PING", false))
	if called != 1 {
		t.Errorf("command names should match case-insensitively, ran %d times", called)
	}
}

func TestCommandCustomPrefix(t *testing.T) {
	c := New("token", WithLogger(discardLogger()), WithPrefix("?"))
	called := 0
	c.Command("hi", func(*MessageCreate, []string) { called++ })

	if got := c.Prefix(); got != "?" {
		t.Errorf("Prefix() = %q, want ?", got)
	}

	deliver(t, c, frameWithContent("!hi", false))
	if called != 0 {
		t.Error("default prefix should not fire when a custom one is set")
	}

	deliver(t, c, frameWithContent("?hi", false))
	if called != 1 {
		t.Errorf("custom prefix did not fire, called = %d", called)
	}
}

// TestCommandsShareOneHandler checks that N commands still register a single
// MESSAGE_CREATE subscription rather than one each.
func TestCommandsShareOneHandler(t *testing.T) {
	c := testClient()
	for _, name := range []string{"a", "b", "c", "d"} {
		c.Command(name, func(*MessageCreate, []string) {})
	}

	slot := c.slotFor("MESSAGE_CREATE")
	if slot == nil {
		t.Fatal("no MESSAGE_CREATE handler registered")
	}
	if userHandlers := len(slot.handlers) - slot.internal; userHandlers != 1 {
		t.Errorf("registered %d user handlers for 4 commands, want 1", userHandlers)
	}
	if len(c.Commands()) != 4 {
		t.Errorf("Commands() = %v, want 4 entries", c.Commands())
	}
}

func TestIntentWarnings(t *testing.T) {
	t.Run("missing event intent", func(t *testing.T) {
		c := New("token", WithLogger(discardLogger())) // no intents at all
		On(c, func(*MessageCreate) {})

		warnings := strings.Join(c.intentWarnings(), "\n")
		if !strings.Contains(warnings, "MESSAGE_CREATE") {
			t.Errorf("expected a MESSAGE_CREATE warning, got:\n%s", warnings)
		}
		if !strings.Contains(warnings, "IntentGuildMessages") {
			t.Errorf("warning should name the intent to enable, got:\n%s", warnings)
		}
	})

	t.Run("missing message content", func(t *testing.T) {
		c := New("token", WithLogger(discardLogger()), WithIntents(IntentGuildMessages))
		On(c, func(*MessageCreate) {})

		warnings := strings.Join(c.intentWarnings(), "\n")
		if strings.Contains(warnings, "will never fire") {
			t.Errorf("the event intent is set, so it should not warn about delivery:\n%s", warnings)
		}
		if !strings.Contains(warnings, "IntentMessageContent") {
			t.Errorf("expected a MessageContent warning, got:\n%s", warnings)
		}
	})

	t.Run("correctly configured", func(t *testing.T) {
		c := New("token", WithLogger(discardLogger()),
			WithIntents(IntentGuildMessages|IntentMessageContent))
		On(c, func(*MessageCreate) {})

		if w := c.intentWarnings(); len(w) != 0 {
			t.Errorf("a correct setup should warn about nothing, got %v", w)
		}
	})

	t.Run("events needing no intent", func(t *testing.T) {
		c := New("token", WithLogger(discardLogger()))
		On(c, func(*Ready) {})
		On(c, func(*InteractionCreate) {})

		if w := c.intentWarnings(); len(w) != 0 {
			t.Errorf("Ready and InteractionCreate need no intent, got %v", w)
		}
	})
}

func TestMustID(t *testing.T) {
	if got := MustID("175928847299117063"); got != 175928847299117063 {
		t.Errorf("MustID = %d", got)
	}

	defer func() {
		if recover() == nil {
			t.Error("MustID should panic on a non-numeric ID")
		}
	}()
	MustID("not-an-id")
}

// TestEveryEventIsRouted guards the type switch in Client.On against drifting
// out of sync with the event types: adding an event and forgetting to register
// it makes bot.On panic at runtime instead of failing here.
func TestEveryEventIsRouted(t *testing.T) {
	// One instance of every event type, built through the same generic path
	// Client.On uses.
	checks := []func(*Client){
		func(c *Client) { c.On(func(*Ready) {}) },
		func(c *Client) { c.On(func(*ChannelInfo) {}) },
		func(c *Client) { c.On(func(*PresenceUpdate) {}) },
		func(c *Client) { c.On(func(*ThreadMembersUpdate) {}) },
		func(c *Client) { c.On(func(*GuildAuditLogEntryCreate) {}) },
		func(c *Client) { c.On(func(*AutoModerationActionExecution) {}) },
		func(c *Client) { c.On(func(*EntitlementCreate) {}) },
		func(c *Client) { c.On(func(*MessagePollVoteAdd) {}) },
		func(c *Client) { c.On(func(*RateLimited) {}) },
		func(c *Client) { c.On(func(*VoiceChannelEffectSend) {}) },
		func(c *Client) { c.On(func(*GuildSoundboardSoundCreate) {}) },
		func(c *Client) { c.On(func(*ApplicationCommandPermissionsUpdate) {}) },
	}
	for i, check := range checks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("handler %d is not registered in Client.On: %v", i, r)
				}
			}()
			check(testClient())
		}()
	}
}

// TestNewEventsHaveIntents checks the events added alongside the router are
// gated correctly, so the startup warning stays truthful.
func TestNewEventsHaveIntents(t *testing.T) {
	cases := map[string]Intent{
		"PRESENCE_UPDATE":                  IntentGuildPresences,
		"GUILD_BAN_ADD":                    IntentGuildModeration,
		"INVITE_CREATE":                    IntentGuildInvites,
		"WEBHOOKS_UPDATE":                  IntentGuildWebhooks,
		"AUTO_MODERATION_ACTION_EXECUTION": IntentAutoModerationExecution,
		"VOICE_STATE_UPDATE":               IntentGuildVoiceStates,
		"THREAD_CREATE":                    IntentGuilds,
	}
	for event, want := range cases {
		got, ok := eventIntents[event]
		if !ok {
			t.Errorf("%s has no intent mapping, so the guard cannot warn about it", event)
			continue
		}
		if !slicesContains(got, want) {
			t.Errorf("%s maps to %v, expected it to include %v", event, got, want)
		}
	}
}

func slicesContains(xs []Intent, want Intent) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
