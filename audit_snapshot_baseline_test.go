package starlings

import (
	"reflect"
	"sort"
	"testing"
)

// Reference from the pre-audit implementation. Keeping both paths in one
// benchmark process controls for package initialization and GC headroom.
func (s *State) originalSnapshotForBenchmark(id Snowflake) (Guild, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.guilds[id]
	if !ok {
		return Guild{}, false
	}
	if s.config.Roles {
		v.Roles = make([]Role, 0, len(s.roles[id]))
		for _, role := range s.roles[id] {
			v.Roles = append(v.Roles, cloneRole(role))
		}
		sort.Slice(v.Roles, func(i, j int) bool {
			if v.Roles[i].Position == v.Roles[j].Position {
				return v.Roles[i].ID < v.Roles[j].ID
			}
			return v.Roles[i].Position < v.Roles[j].Position
		})
	}
	if s.config.Channels {
		v.Channels = make([]Channel, 0)
		v.Threads = make([]Channel, 0)
		for _, channel := range s.channels {
			if channel.GuildID != id {
				continue
			}
			if channel.Type.IsThread() {
				v.Threads = append(v.Threads, cloneChannel(channel))
			} else {
				v.Channels = append(v.Channels, cloneChannel(channel))
			}
		}
		sort.Slice(v.Channels, func(i, j int) bool {
			if v.Channels[i].Position == v.Channels[j].Position {
				return v.Channels[i].ID < v.Channels[j].ID
			}
			return v.Channels[i].Position < v.Channels[j].Position
		})
		sort.Slice(v.Threads, func(i, j int) bool { return v.Threads[i].ID < v.Threads[j].ID })
	}
	if s.config.Members {
		v.Members = make([]Member, 0, len(s.members[id]))
		for _, member := range s.members[id] {
			v.Members = append(v.Members, cloneMember(member))
		}
		sort.Slice(v.Members, func(i, j int) bool { return v.Members[i].User.ID < v.Members[j].User.ID })
	}
	if s.config.Emojis {
		v.Emojis = make([]GuildEmoji, 0, len(s.emojis[id]))
		for _, emoji := range s.emojis[id] {
			v.Emojis = append(v.Emojis, cloneGuildEmoji(emoji))
		}
		sort.Slice(v.Emojis, func(i, j int) bool { return v.Emojis[i].ID < v.Emojis[j].ID })
	}
	if s.config.Stickers {
		v.Stickers = make([]Sticker, 0, len(s.stickers[id]))
		for _, sticker := range s.stickers[id] {
			v.Stickers = append(v.Stickers, cloneSticker(sticker))
		}
		sort.Slice(v.Stickers, func(i, j int) bool { return v.Stickers[i].ID < v.Stickers[j].ID })
	}
	if s.config.VoiceStates {
		v.VoiceStates = make([]VoiceState, 0, len(s.voiceStates[id]))
		for _, voice := range s.voiceStates[id] {
			v.VoiceStates = append(v.VoiceStates, cloneVoiceState(voice))
		}
		sort.Slice(v.VoiceStates, func(i, j int) bool { return v.VoiceStates[i].UserID < v.VoiceStates[j].UserID })
	}
	if s.config.Presences {
		v.Presences = make([]PresenceUpdate, 0, len(s.presences[id]))
		for _, presence := range s.presences[id] {
			v.Presences = append(v.Presences, clonePresence(presence))
		}
		sort.Slice(v.Presences, func(i, j int) bool { return v.Presences[i].User.ID < v.Presences[j].User.ID })
	}
	out := cloneGuild(v)
	out.bindTo(s.client)
	return out, true
}

func BenchmarkGuildSnapshotOriginal(b *testing.B) {
	s, id := snapshotBenchmarkState(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		g, ok := s.originalSnapshotForBenchmark(id)
		if !ok || len(g.Members) != 2000 {
			b.Fatal("incomplete snapshot")
		}
	}
}

func TestSnapshotBenchmarkEquivalence(t *testing.T) {
	s := newState()
	s.putGuild(Guild{ID: 1, Members: []Member{{User: &User{ID: 9}, Roles: []Snowflake{2}}, {User: &User{ID: 3}}}, Roles: []Role{{ID: 2}}, Channels: []Channel{{ID: 4, GuildID: 1}}, Features: []string{"A"}})
	before, _ := s.originalSnapshotForBenchmark(1)
	after, _ := s.Guild(1)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("benchmark paths returned different snapshots")
	}
}
