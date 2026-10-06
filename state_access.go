package starlings

import "sort"

// StateStats is a cheap count-only snapshot of cache occupancy.
type StateStats struct {
	Guilds, Channels, Members, Users                  int
	Roles, Emojis, Stickers, ThreadMembers            int
	VoiceStates, Presences, Messages, MessageChannels int
}

func (s *State) Guild(id Snowflake) (Guild, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.guildLocked(id)
}

func (s *State) guildLocked(id Snowflake) (Guild, bool) {
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
	return cloneGuild(v), true
}

func (s *State) Channel(id Snowflake) (Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.channels[id]
	return cloneChannel(v), ok
}

func (s *State) Member(guildID, userID Snowflake) (Member, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.members[guildID][userID]
	return cloneMember(v), ok
}

func (s *State) User(id Snowflake) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.users[id]
	return cloneUser(v), ok
}

func (s *State) Role(guildID, roleID Snowflake) (Role, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.roles[guildID][roleID]
	return cloneRole(v), ok
}

func (s *State) Emoji(guildID, emojiID Snowflake) (GuildEmoji, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.emojis[guildID][emojiID]
	return cloneGuildEmoji(v), ok
}

func (s *State) Sticker(guildID, stickerID Snowflake) (Sticker, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.stickers[guildID][stickerID]
	return cloneSticker(v), ok
}

func (s *State) ThreadMember(threadID, userID Snowflake) (ThreadMember, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.threadMembers[threadID][userID]
	return cloneThreadMember(v), ok
}

func (s *State) VoiceState(guildID, userID Snowflake) (VoiceState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.voiceStates[guildID][userID]
	return cloneVoiceState(v), ok
}

func (s *State) Presence(guildID, userID Snowflake) (PresenceUpdate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.presences[guildID][userID]
	return clonePresence(v), ok
}

func (s *State) Message(channelID, messageID Snowflake) (Message, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cache := s.messages[channelID]
	if cache == nil {
		return Message{}, false
	}
	v, ok := cache.items[messageID]
	return cloneMessage(v), ok
}

func (s *State) Guilds() []Guild {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Guild, 0, len(s.guilds))
	for id := range s.guilds {
		v, _ := s.guildLocked(id)
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *State) Channels() []Channel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Channel, 0, len(s.channels))
	for _, v := range s.channels {
		out = append(out, cloneChannel(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Users returns snapshots of every cached user.
func (s *State) Users() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]User, 0, len(s.users))
	for _, v := range s.users {
		out = append(out, cloneUser(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *State) Members(guildID Snowflake) []Member {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Member, 0, len(s.members[guildID]))
	for _, v := range s.members[guildID] {
		out = append(out, cloneMember(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].User.ID < out[j].User.ID })
	return out
}

func (s *State) Roles(guildID Snowflake) []Role {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Role, 0, len(s.roles[guildID]))
	for _, v := range s.roles[guildID] {
		out = append(out, cloneRole(v))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Position == out[j].Position {
			return out[i].ID < out[j].ID
		}
		return out[i].Position < out[j].Position
	})
	return out
}

func (s *State) Emojis(guildID Snowflake) []GuildEmoji {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]GuildEmoji, 0, len(s.emojis[guildID]))
	for _, v := range s.emojis[guildID] {
		out = append(out, cloneGuildEmoji(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *State) Stickers(guildID Snowflake) []Sticker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Sticker, 0, len(s.stickers[guildID]))
	for _, v := range s.stickers[guildID] {
		out = append(out, cloneSticker(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *State) ThreadMembers(threadID Snowflake) []ThreadMember {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ThreadMember, 0, len(s.threadMembers[threadID]))
	for _, v := range s.threadMembers[threadID] {
		out = append(out, cloneThreadMember(v))
	}
	sort.Slice(out, func(i, j int) bool { return threadMemberUserID(out[i]) < threadMemberUserID(out[j]) })
	return out
}

func (s *State) VoiceStates(guildID Snowflake) []VoiceState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]VoiceState, 0, len(s.voiceStates[guildID]))
	for _, v := range s.voiceStates[guildID] {
		out = append(out, cloneVoiceState(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	return out
}

func (s *State) Presences(guildID Snowflake) []PresenceUpdate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PresenceUpdate, 0, len(s.presences[guildID]))
	for _, v := range s.presences[guildID] {
		out = append(out, clonePresence(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].User.ID < out[j].User.ID })
	return out
}

// Messages returns a channel's cached messages oldest first.
func (s *State) Messages(channelID Snowflake) []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cache := s.messages[channelID]
	if cache == nil {
		return nil
	}
	out := make([]Message, 0, len(cache.order))
	for _, id := range cache.order {
		if v, ok := cache.items[id]; ok {
			out = append(out, cloneMessage(v))
		}
	}
	return out
}

func (s *State) Stats() StateStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := StateStats{Guilds: len(s.guilds), Channels: len(s.channels), Users: len(s.users), MessageChannels: len(s.messages)}
	for _, x := range s.members {
		v.Members += len(x)
	}
	for _, x := range s.roles {
		v.Roles += len(x)
	}
	for _, x := range s.emojis {
		v.Emojis += len(x)
	}
	for _, x := range s.stickers {
		v.Stickers += len(x)
	}
	for _, x := range s.threadMembers {
		v.ThreadMembers += len(x)
	}
	for _, x := range s.voiceStates {
		v.VoiceStates += len(x)
	}
	for _, x := range s.presences {
		v.Presences += len(x)
	}
	for _, x := range s.messages {
		v.Messages += len(x.items)
	}
	return v
}
