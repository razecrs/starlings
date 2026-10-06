package starlings

import (
	"errors"
	"testing"
	"time"
)

func TestStateTracksAllRequestedResourcesAndSnapshots(t *testing.T) {
	s := newState()
	user := &User{ID: 20, Username: "before"}
	if err := s.Apply(&GuildCreate{Guild: Guild{
		ID: 1, OwnerID: 99,
		Roles:       []Role{{ID: 1, Name: "@everyone"}, {ID: 2, Name: "blue", Color: 0x123456, Position: 2}},
		Emojis:      []GuildEmoji{{ID: 3, Name: "bird", Roles: []Snowflake{2}}},
		Stickers:    []Sticker{{ID: 4, Name: "wing", User: user}},
		Channels:    []Channel{{ID: 10, GuildID: 1, Name: "old"}},
		Threads:     []Channel{{ID: 11, GuildID: 1, ParentID: 10, Type: ChannelPublicThread}},
		Members:     []Member{{User: user, Roles: []Snowflake{2}}},
		VoiceStates: []VoiceState{{GuildID: 1, ChannelID: 12, UserID: 20}},
		Presences:   []PresenceUpdate{{GuildID: 1, User: user, Status: "online"}},
	}}); err != nil {
		t.Fatal(err)
	}

	if got := s.Stats(); got.Guilds != 1 || got.Channels != 2 || got.Members != 1 ||
		got.Users != 1 || got.Roles != 2 || got.Emojis != 1 || got.Stickers != 1 ||
		got.VoiceStates != 1 || got.Presences != 1 {
		t.Fatalf("unexpected cache stats: %+v", got)
	}

	channelUpdate := &ChannelUpdate{Channel: Channel{ID: 10, GuildID: 1, Name: "new"}}
	if err := s.Apply(channelUpdate); err != nil {
		t.Fatal(err)
	}
	if channelUpdate.BeforeUpdate == nil || channelUpdate.BeforeUpdate.Name != "old" {
		t.Fatalf("channel before snapshot = %#v", channelUpdate.BeforeUpdate)
	}

	roleUpdate := &GuildRoleUpdate{GuildID: 1, Role: &Role{ID: 2, Name: "new blue", Color: 7, Position: 3}}
	if err := s.Apply(roleUpdate); err != nil {
		t.Fatal(err)
	}
	if roleUpdate.BeforeUpdate == nil || roleUpdate.BeforeUpdate.Name != "blue" {
		t.Fatalf("role before snapshot = %#v", roleUpdate.BeforeUpdate)
	}

	voiceUpdate := &VoiceStateUpdate{VoiceState: VoiceState{GuildID: 1, UserID: 20}}
	if err := s.Apply(voiceUpdate); err != nil {
		t.Fatal(err)
	}
	if voiceUpdate.BeforeUpdate == nil || voiceUpdate.BeforeUpdate.ChannelID != 12 {
		t.Fatalf("voice before snapshot = %#v", voiceUpdate.BeforeUpdate)
	}
	if _, ok := s.VoiceState(1, 20); ok {
		t.Fatal("disconnected voice state remained cached")
	}

	thread := ThreadMember{ID: 11, UserID: 20, Member: &Member{User: user}}
	if err := s.Apply(&ThreadMemberUpdate{ThreadMember: thread, GuildID: 1}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.ThreadMember(11, 20); !ok {
		t.Fatal("thread member was not cached")
	}

	guild, ok := s.Guild(1)
	if !ok || len(guild.Roles) != 2 || guild.Roles[1].Name != "new blue" || len(guild.Stickers) != 1 {
		t.Fatalf("composed guild snapshot is stale: %#v", guild)
	}
}

func TestStateBoundedMessagesAndDeleteSnapshots(t *testing.T) {
	config := DefaultStateConfig()
	config.MaxMessagesPerChannel = 2
	s := newStateWithConfig(config)
	for id := Snowflake(1); id <= 3; id++ {
		if err := s.Apply(&MessageCreate{Message: Message{ID: id, ChannelID: 9, Content: id.String()}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := s.Message(9, 1); ok {
		t.Fatal("oldest message was not evicted")
	}
	if got := s.Messages(9); len(got) != 2 || got[0].ID != 2 || got[1].ID != 3 {
		t.Fatalf("bounded messages = %#v", got)
	}

	update := &MessageUpdate{Message: Message{ID: 2, ChannelID: 9, Content: "edited", EditedTimestamp: timePtr(time.Now())}}
	if err := s.Apply(update); err != nil {
		t.Fatal(err)
	}
	if update.BeforeUpdate == nil || update.BeforeUpdate.Content != "2" {
		t.Fatalf("message before update = %#v", update.BeforeUpdate)
	}
	deleted := &MessageDelete{ID: 2, ChannelID: 9}
	if err := s.Apply(deleted); err != nil {
		t.Fatal(err)
	}
	if deleted.BeforeDelete == nil || deleted.BeforeDelete.Content != "edited" {
		t.Fatalf("message before delete = %#v", deleted.BeforeDelete)
	}
}

func TestStatePermissionOrderAndColor(t *testing.T) {
	s := newState()
	s.putGuild(Guild{
		ID: 1, OwnerID: 99,
		Roles: []Role{
			{ID: 1, Permissions: PermissionViewChannel | PermissionSendMessages},
			{ID: 2, Position: 2, Color: 0x112233, Permissions: PermissionAttachFiles},
			{ID: 3, Position: 4, Color: 0xabcdef},
		},
		Members: []Member{{User: &User{ID: 10}, Roles: []Snowflake{2, 3}}},
		Channels: []Channel{{ID: 5, GuildID: 1, PermissionOverwrites: []Overwrite{
			{ID: 1, Type: 0, Deny: PermissionSendMessages},
			{ID: 2, Type: 0, Deny: PermissionAttachFiles, Allow: PermissionSendMessages},
			{ID: 10, Type: 1, Deny: PermissionSendMessages, Allow: PermissionAttachFiles},
		}}},
	})
	permissions, err := s.Permissions(1, 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !permissions.Has(PermissionViewChannel|PermissionAttachFiles) || permissions.Has(PermissionSendMessages) {
		t.Fatalf("effective permissions = %s", permissions)
	}
	color, err := s.UserColor(1, 10)
	if err != nil || color != 0xabcdef {
		t.Fatalf("color = %#x, err=%v", color, err)
	}
	owner, err := s.Permissions(1, 5, 99)
	if err != nil || !owner.Has(PermissionAdministrator|PermissionBypassSlowmode) {
		t.Fatalf("owner permissions = %s, err=%v", owner, err)
	}
}

func TestManualStateUsesTheSameApplyPath(t *testing.T) {
	c := New("token", WithLogger(discardLogger()), WithStateMode(StateManual), WithGuard())
	guard := c.Guard()
	seen := false
	On(c, func(event *ChannelCreate) {
		if _, ok := c.State.Channel(event.ID); ok {
			t.Fatal("manual state changed before Apply")
		}
		if err := c.State.Apply(event); err != nil {
			t.Fatal(err)
		}
		_, seen = c.State.Channel(event.ID)
	})
	c.dispatch("CHANNEL_CREATE", []byte(`{"id":"2","guild_id":"1","name":"manual","type":0}`))
	if !seen {
		t.Fatal("manual Apply did not update state")
	}
	report := guard.Report()
	if len(report.Metrics) != 1 || report.Metrics[0].Implementation != GuardManual {
		t.Fatalf("guard report = %+v", report)
	}
	if err := c.State.Apply(&Resumed{}); !errors.Is(err, ErrUnsupportedStateEvent) {
		t.Fatalf("unsupported event error = %v", err)
	}
}

func TestStateConfigAvoidsUnusedEventDecoding(t *testing.T) {
	config := DefaultStateConfig()
	config.Presences = false
	config.Users = false
	c := New("token", WithLogger(discardLogger()), WithStateCache(config))
	if slot := c.slotFor("PRESENCE_UPDATE"); slot != nil {
		t.Fatalf("disabled presence cache installed a handler: %#v", slot)
	}
}

func TestGuildCreateReplacesStaleSnapshotResources(t *testing.T) {
	s := newState()
	s.putGuild(Guild{ID: 1, Channels: []Channel{{ID: 2, GuildID: 1}}, Members: []Member{{User: &User{ID: 3}}}})
	s.putGuild(Guild{ID: 1, Channels: []Channel{{ID: 4, GuildID: 1}}})
	if _, ok := s.Channel(2); ok {
		t.Fatal("stale channel survived a full guild snapshot")
	}
	if _, ok := s.Member(1, 3); ok {
		t.Fatal("stale member survived a full guild snapshot")
	}
	if _, ok := s.Channel(4); !ok {
		t.Fatal("new guild channel was not cached")
	}
}

func timePtr(value time.Time) *time.Time { return &value }
