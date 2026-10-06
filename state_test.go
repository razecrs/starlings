package starlings

import "testing"

func TestGuildCreateInfersNestedChannelGuildIDs(t *testing.T) {
	state := newState()
	event := &GuildCreate{Guild: Guild{
		ID:       42,
		Channels: []Channel{{ID: 100, Type: ChannelGuildText}},
		Threads:  []Channel{{ID: 101, Type: ChannelPublicThread, ParentID: 100}},
	}}
	if err := state.Apply(event); err != nil {
		t.Fatal(err)
	}
	channel, ok := state.Channel(100)
	if !ok || channel.GuildID != 42 {
		t.Fatalf("channel=%#v found=%t", channel, ok)
	}
	thread, ok := state.Channel(101)
	if !ok || thread.GuildID != 42 {
		t.Fatalf("thread=%#v found=%t", thread, ok)
	}
	guild, ok := state.Guild(42)
	if !ok || len(guild.Channels) != 1 || len(guild.Threads) != 1 {
		t.Fatalf("guild=%#v found=%t", guild, ok)
	}
}

func TestStateTracksGuildObjects(t *testing.T) {
	s := newState()
	s.putGuild(Guild{
		ID:       1,
		Name:     "birds",
		Channels: []Channel{{ID: 2, GuildID: 1, Name: "nest"}},
		Members:  []Member{{User: &User{ID: 3, Username: "swift"}, Roles: []Snowflake{4}}},
	})

	g, ok := s.Guild(1)
	if !ok || g.Name != "birds" {
		t.Fatalf("guild = %#v, %v", g, ok)
	}
	ch, ok := s.Channel(2)
	if !ok || ch.Name != "nest" {
		t.Fatalf("channel = %#v, %v", ch, ok)
	}
	m, ok := s.Member(1, 3)
	if !ok || m.User.Username != "swift" {
		t.Fatalf("member = %#v, %v", m, ok)
	}
	if u, ok := s.User(3); !ok || u.Username != "swift" {
		t.Fatalf("user = %#v, %v", u, ok)
	}
	g.Channels[0].Name = "changed"
	againGuild, _ := s.Guild(1)
	if againGuild.Channels[0].Name != "nest" {
		t.Fatalf("caller mutated cached guild: %#v", againGuild)
	}

	m.User.Username = "changed"
	m.Roles[0] = 99
	again, _ := s.Member(1, 3)
	if again.User.Username != "swift" || again.Roles[0] != 4 {
		t.Fatalf("caller mutated cached member: %#v", again)
	}
}

func TestStateUnavailableGuildIsRetained(t *testing.T) {
	s := newState()
	s.putGuild(Guild{ID: 1, Channels: []Channel{{ID: 2, GuildID: 1}}})
	s.markGuildUnavailable(1)
	g, ok := s.Guild(1)
	if !ok || !g.Unavailable {
		t.Fatalf("unavailable guild = %#v, %v", g, ok)
	}
	s.removeGuild(1)
	if _, ok := s.Guild(1); ok {
		t.Fatal("deleted guild remained cached")
	}
	if _, ok := s.Channel(2); ok {
		t.Fatal("deleted guild channel remained cached")
	}
}

func TestClientStateUpdatesBeforeUserHandler(t *testing.T) {
	c := New("token", WithLogger(discardLogger()))
	seen := false
	On(c, func(e *ChannelCreate) {
		ch, ok := c.State.Channel(e.ID)
		seen = ok && ch.Name == "general"
	})
	c.dispatch("CHANNEL_CREATE", []byte(`{"id":"2","guild_id":"1","name":"general","type":0}`))
	if !seen {
		t.Fatal("state was not updated before application handler")
	}
}
