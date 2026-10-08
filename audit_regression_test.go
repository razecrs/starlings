package starlings

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestRouteTextStrictParsing(t *testing.T) {
	for _, tc := range []struct {
		value     any
		good, bad string
	}{
		{int64(0), "1548760732286451903", "1548760732286451903junk"},
		{uint64(0), "18446744073709551615", "-1"},
		{int8(0), "127", "128"},
		{uint8(0), "255", "256"},
		{int64(0), "10", "10 20"},
		{int64(0), "008", "0xff"},
		{false, "true", "true!"},
		{float32(0), "1.25", "1e100"},
		{float64(0), "1e3", "NaN"},
		{float64(0), "1.5", "+Inf"},
		{float64(0), "-2", "2x"},
	} {
		v := reflect.New(reflect.TypeOf(tc.value)).Elem()
		if err := setText(v, v.Kind(), tc.good); err != nil {
			t.Errorf("%T rejected %q: %v", tc.value, tc.good, err)
		}
		before := v.Interface()
		if err := setText(v, v.Kind(), tc.bad); err == nil {
			t.Errorf("%T accepted %q", tc.value, tc.bad)
		}
		if !reflect.DeepEqual(before, v.Interface()) {
			t.Errorf("invalid input %q mutated the destination", tc.bad)
		}
	}
}

func TestAdaptNilHandlerDiagnostic(t *testing.T) {
	for _, handler := range []any{nil, (func() string)(nil), (func(*InteractionCreate) error)(nil)} {
		func() {
			defer func() {
				message, _ := recover().(string)
				if !strings.Contains(message, "/test: handler must not be nil") {
					t.Errorf("panic = %q", message)
				}
			}()
			adaptHandler("/test", handler, argsFromOptions, "test")
		}()
	}
}

func TestRequireFailsClosedWithoutMembership(t *testing.T) {
	for _, guild := range []Snowflake{0, 1} {
		c, log := autoDeferClient(t, 0)
		called := false
		c.Slash("secure", "secure", func() string { called = true; return "secret" }).Require(PermissionBanMembers)
		i := commandInteraction(c, "secure")
		i.GuildID = guild
		i.User = &User{ID: 50}
		c.routeInteraction(i)
		if called || !strings.Contains(log.sentBodies(), "You need") {
			t.Fatalf("guild %d allowed an invocation without membership", guild)
		}
	}
}

func TestRunReplacesTask(t *testing.T) {
	c, log := autoDeferClient(t, 0)
	c.SlashTask("x", "x", func(context.Context, *InteractionCreate) (string, error) {
		t.Error("replaced task was called")
		return "old", nil
	}).Run(func(i *InteractionCreate) error { return i.Reply("new") })
	c.routeInteraction(commandInteraction(c, "x"))
	if !strings.Contains(log.sentBodies(), `"content":"new"`) {
		t.Fatal(log.sentBodies())
	}
}

func TestHandlerFailureWithoutInvoker(t *testing.T) {
	c, log := autoDeferClient(t, 0)
	c.Slash("x", "x", func() error { return errors.New("private failure") })
	c.routeInteraction(commandInteraction(c, "x"))
	if body := log.sentBodies(); !strings.Contains(body, genericFailure) || strings.Contains(body, "private failure") {
		t.Fatalf("unsafe or missing failure reply: %s", body)
	}
}

func TestGuildInfoAndSnapshotIsolation(t *testing.T) {
	s := newState()
	s.putGuild(Guild{ID: 1, OwnerID: 2, Name: "guild", MemberCount: 12,
		Features: []string{"FEATURE"},
		Members:  []Member{{User: &User{ID: 3, Username: "before"}, Roles: []Snowflake{4}}},
		Roles:    []Role{{ID: 4, Name: "role"}},
		Channels: []Channel{{ID: 5, GuildID: 1, PermissionOverwrites: []Overwrite{{ID: 4}}}},
	})
	info, ok := s.GuildInfo(1)
	if !ok || info.OwnerID != 2 || info.MemberCount != 12 || s.MemberCount(1) != 1 || s.MemberCount(99) != 0 {
		t.Fatal("incorrect metadata/count")
	}
	if len(info.Members)+len(info.Channels)+len(info.Roles) != 0 {
		t.Fatal("metadata included resources")
	}
	info.Features[0] = "changed"
	g, _ := s.Guild(1)
	g.Members[0].User.Username = "changed"
	g.Members[0].Roles[0] = 99
	g.Channels[0].PermissionOverwrites[0].ID = 99
	g.Roles[0].Name = "changed"
	again, _ := s.Guild(1)
	if again.Features[0] != "FEATURE" || again.Members[0].User.Username != "before" || again.Members[0].Roles[0] != 4 || again.Channels[0].PermissionOverwrites[0].ID != 4 || again.Roles[0].Name != "role" {
		t.Fatal("returned snapshot aliased the cache")
	}
}

func TestGuardComposedReadsDoNotInventHits(t *testing.T) {
	g := NewGuard(WithGuardWarmup(0))
	s := newState()
	s.guard = g
	s.putGuild(Guild{ID: 1, Members: []Member{{User: &User{ID: 2}}}})
	for range 20 {
		s.Member(1, 99)
		s.Guilds()
	}
	if m := g.cache["members"]; m.calls != 20 || m.hits != 0 || m.uses != 20 {
		t.Fatalf("dependency reads polluted hit ratio: %+v", m)
	}
	findings := guardCacheFindings(s.Config(), s.Stats(), map[string]guardCacheMetric{"members": *g.cache["members"]}, IntentsAll)
	found := false
	for _, f := range findings {
		if f.Area == "cache.members" && strings.Contains(f.Evidence, "0/20 lookups hit") {
			found = true
		}
	}
	if !found {
		t.Fatal("missing actual miss-rate warning")
	}
}

func TestDownloadAttachmentChecksRedirects(t *testing.T) {
	for _, target := range []string{
		"http://127.0.0.1/private?secret=hidden",
		"https://cdn.discordapp.com.evil.test/file",
		"https://media.discordapp.net/file",
	} {
		t.Run(target, func(t *testing.T) {
			requests := 0
			c := clientServedBy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if requests == 1 {
					w.Header().Set("Location", target)
					w.WriteHeader(http.StatusFound)
					return
				}
				_, _ = w.Write([]byte("ok"))
			}))
			data, err := c.DownloadAttachment(context.Background(), Attachment{URL: "https://cdn.discordapp.com/file?secret=hidden"}, 10)
			if trustedDiscordCDN(target) {
				if err != nil || string(data) != "ok" || requests != 2 {
					t.Fatalf("trusted redirect: data=%q err=%v requests=%d", data, err, requests)
				}
			} else if !errors.Is(err, ErrUntrustedAttachmentURL) || requests != 1 || strings.Contains(err.Error(), "hidden") {
				t.Fatalf("untrusted redirect: err=%v requests=%d", err, requests)
			}
			if c.rest.http.CheckRedirect != nil {
				t.Fatal("download changed shared HTTP client policy")
			}
		})
	}
}

func TestDurationRejectsNonFiniteAndOverflow(t *testing.T) {
	for _, raw := range []string{"NaN", "+Inf", "-Inf", "1e100", "9223372037", "9999999999d", "100000d100000d", "9223372036854775808ns"} {
		if value, err := ParseDuration(raw); err == nil {
			t.Errorf("accepted %q as %s", raw, value)
		}
	}
	for raw, want := range map[string]time.Duration{
		"1w2d": 9 * 24 * time.Hour, "1.5": 1500 * time.Millisecond,
		"1h30m": 90 * time.Minute, "10us": 10 * time.Microsecond,
		"1d1ns": 24*time.Hour + time.Nanosecond, "-1h": -time.Hour,
	} {
		if got, err := ParseDuration(raw); err != nil || got != want {
			t.Errorf("%q = %v, %v; want %s", raw, got, err, want)
		}
	}
}

func TestGuardUsesInferredIntents(t *testing.T) {
	c := New(WithToken("token"), WithGuard(), WithAutoSync(false), WithLogger(discardLogger()))
	On(c, func(*GuildMemberAdd) {})
	if err := c.prepareRun(); err != nil {
		t.Fatal(err)
	}
	if c.guard.intents != c.intents || !c.guard.intents.Has(IntentGuildMembers) {
		t.Fatalf("Guard intents %d != active intents %d", c.guard.intents, c.intents)
	}
}

func TestMemberSnapshotsSortedByID(t *testing.T) {
	s := newState()
	s.putGuild(Guild{ID: 1, Members: []Member{
		{User: &User{ID: 9}}, {User: &User{ID: 2}}, {User: &User{ID: 7}},
	}})
	g, _ := s.Guild(1)
	for _, members := range [][]Member{s.Members(1), g.Members} {
		for n, id := range []Snowflake{2, 7, 9} {
			if members[n].User.ID != id {
				t.Fatalf("unsorted members: %+v", members)
			}
		}
	}
}

func TestCommandNamesMatchPreviousValidator(t *testing.T) {
	previous := regexp.MustCompile(`^[-_\p{L}\p{N}\p{Devanagari}\p{Thai}]{1,32}$`)
	// Exercise every Unicode code point, including script combining marks.
	for r := rune(0); r <= 0x10ffff; r++ {
		checkCommandNameEquivalence(t, previous, string(r))
	}
	for _, name := range []string{"", "ping", "Ping", "ไทย", "हिन्दी", "héllo", "你好", "😀", "has space", "has.dot", "\xff", strings.Repeat("a", 32), strings.Repeat("a", 33)} {
		checkCommandNameEquivalence(t, previous, name)
	}
}

func checkCommandNameEquivalence(t *testing.T, previous *regexp.Regexp, name string) {
	t.Helper()
	want := previous.MatchString(name) && strings.ToLower(name) == name
	got := true
	checkName(name, func(string, ...any) { got = false })
	if got != want {
		t.Fatalf("name %q: accepted=%t want=%t", name, got, want)
	}
}

func TestChildShardsPreservePanicPolicy(t *testing.T) {
	for _, recovery := range []bool{true, false} {
		c := New(WithToken("token"), WithPanicRecovery(recovery), WithLogger(discardLogger()), WithAutoDefer(time.Second))
		for _, shard := range c.makeShards(3, "wss://gateway.example") {
			if shard.recoverPanics != recovery || shard.autoDefer != c.autoDefer || shard.requestTimeout != c.requestTimeout {
				t.Fatalf("shard %d lost client policies", shard.shard[0])
			}
			panicked := func() (panicked bool) {
				defer func() { panicked = recover() != nil }()
				invokeApplicationHandler(shard, "MESSAGE_CREATE", func(*MessageCreate) { panic("test panic") }, &MessageCreate{})
				return false
			}()
			if panicked == recovery {
				t.Fatalf("shard %d: propagated panic=%t, recovery=%t", shard.shard[0], panicked, recovery)
			}
		}
	}
}
