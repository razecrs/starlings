package starlings

import "time"

// Apply mutates the cache for one gateway event. Automatic mode calls this
// before user handlers; manual mode lets applications choose exactly when.
func (s *State) Apply(event Event) (err error) {
	if s.guard != nil {
		started := time.Now()
		implementation := GuardAutomatic
		if s.config.Mode == StateManual {
			implementation = GuardManual
		}
		defer func() { s.guard.observe("state."+event.eventName(), implementation, time.Since(started), err) }()
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	switch e := event.(type) {
	case *Ready:
		s.resetShardLocked(e.Shard[0], e.Shard[1])
		if e.User != nil {
			// The bot's ID drives reaction "me" flags and CanModerate even
			// when users are not cached.
			s.selfID = e.User.ID
			if s.config.Users {
				s.users[e.User.ID] = cloneUser(*e.User)
			}
		}
		if s.config.Guilds {
			for _, guild := range e.Guilds {
				s.guilds[guild.ID] = cloneGuild(guild)
			}
		}
	case *GuildCreate:
		s.putGuildLocked(e.Guild)
	case *GuildUpdate:
		if old, ok := s.guildLocked(e.ID); ok {
			snapshot := old
			e.BeforeUpdate = &snapshot
		}
		if s.config.Guilds {
			s.guilds[e.ID] = mergeGuild(s.guilds[e.ID], e.Guild)
		}
	case *GuildDelete:
		if old, ok := s.guildLocked(e.ID); ok {
			snapshot := old
			e.BeforeDelete = &snapshot
		}
		if e.Unavailable {
			if guild, ok := s.guilds[e.ID]; ok {
				guild.Unavailable = true
				s.guilds[e.ID] = guild
			}
		} else {
			s.removeGuildLocked(e.ID)
		}
	case *ChannelCreate:
		s.putChannelLocked(e.Channel)
	case *ChannelUpdate:
		s.updateChannelLocked(&e.Channel, &e.BeforeUpdate)
	case *ChannelDelete:
		s.deleteChannelLocked(e.ID, &e.BeforeDelete)
	case *ThreadCreate:
		s.putChannelLocked(e.Channel)
	case *ThreadUpdate:
		s.updateChannelLocked(&e.Channel, &e.BeforeUpdate)
	case *ThreadDelete:
		if old, ok := s.channels[e.ID]; ok {
			snapshot := cloneChannel(old)
			e.BeforeDelete = &snapshot
		}
		delete(s.channels, e.ID)
		delete(s.threadMembers, e.ID)
		delete(s.threadGuild, e.ID)
		delete(s.threadParent, e.ID)
		delete(s.messages, e.ID)
		delete(s.messageGuild, e.ID)
	case *ThreadListSync:
		s.applyThreadSyncLocked(e)
	case *ThreadMemberUpdate:
		if !e.GuildID.IsZero() {
			s.threadGuild[e.ID] = e.GuildID
		}
		uid := threadMemberUserID(e.ThreadMember)
		if old, ok := s.threadMembers[e.ID][uid]; ok {
			snapshot := cloneThreadMember(old)
			e.BeforeUpdate = &snapshot
		}
		s.putThreadMemberLocked(e.ThreadMember)
	case *ThreadMembersUpdate:
		if !e.GuildID.IsZero() {
			s.threadGuild[e.ID] = e.GuildID
		}
		for _, old := range s.threadMembers[e.ID] {
			e.BeforeUpdate = append(e.BeforeUpdate, cloneThreadMember(old))
		}
		for _, member := range e.AddedMembers {
			s.putThreadMemberLocked(member)
		}
		for _, id := range e.RemovedMemberIDs {
			delete(s.threadMembers[e.ID], id)
		}
	case *GuildMemberAdd:
		s.putMemberLocked(e.GuildID, e.Member)
	case *GuildMemberUpdate:
		if e.User == nil {
			break
		}
		if old, ok := s.members[e.GuildID][e.User.ID]; ok {
			snapshot := cloneMember(old)
			e.BeforeUpdate = &snapshot
		}
		// The update is a full snapshot: nulls clear values rather than
		// meaning "unchanged", so it replaces the cached member.
		member := Member{
			User: e.User, Nick: e.Nick, Avatar: e.Avatar, Roles: e.Roles,
			PremiumSince: e.PremiumSince, Pending: e.Pending, Flags: e.Flags,
			CommunicationDisabledUntil: e.CommunicationDisabledUntil,
		}
		if e.JoinedAt != nil {
			member.JoinedAt = *e.JoinedAt
		}
		if old, ok := s.members[e.GuildID][e.User.ID]; ok {
			if e.JoinedAt == nil {
				member.JoinedAt = old.JoinedAt
			}
			member.Deaf, member.Mute = old.Deaf, old.Mute
		}
		if e.Deaf != nil {
			member.Deaf = *e.Deaf
		}
		if e.Mute != nil {
			member.Mute = *e.Mute
		}
		s.putMemberLocked(e.GuildID, member)
	case *GuildMemberRemove:
		if e.User == nil {
			break
		}
		if old, ok := s.members[e.GuildID][e.User.ID]; ok {
			snapshot := cloneMember(old)
			e.BeforeDelete = &snapshot
		}
		delete(s.members[e.GuildID], e.User.ID)
	case *GuildMembersChunk:
		for _, member := range e.Members {
			s.putMemberLocked(e.GuildID, member)
		}
		for i := range e.Presences {
			if e.Presences[i].GuildID.IsZero() {
				e.Presences[i].GuildID = e.GuildID
			}
			s.putPresenceLocked(e.Presences[i])
		}
	case *GuildRoleCreate:
		if e.Role != nil {
			s.putRoleLocked(e.GuildID, *e.Role)
		}
	case *GuildRoleUpdate:
		if e.Role == nil {
			break
		}
		if old, ok := s.roles[e.GuildID][e.Role.ID]; ok {
			snapshot := cloneRole(old)
			e.BeforeUpdate = &snapshot
		}
		s.putRoleLocked(e.GuildID, *e.Role)
	case *GuildRoleDelete:
		if old, ok := s.roles[e.GuildID][e.RoleID]; ok {
			snapshot := cloneRole(old)
			e.BeforeDelete = &snapshot
		}
		delete(s.roles[e.GuildID], e.RoleID)
	case *GuildEmojisUpdate:
		for _, old := range s.emojis[e.GuildID] {
			e.BeforeUpdate = append(e.BeforeUpdate, cloneGuildEmoji(old))
		}
		if s.config.Emojis {
			next := make(map[Snowflake]GuildEmoji, len(e.Emojis))
			for _, emoji := range e.Emojis {
				next[emoji.ID] = cloneGuildEmoji(emoji)
				s.putUserPointerLocked(emoji.User)
			}
			s.emojis[e.GuildID] = next
		}
	case *GuildStickersUpdate:
		for _, old := range s.stickers[e.GuildID] {
			e.BeforeUpdate = append(e.BeforeUpdate, cloneSticker(old))
		}
		if s.config.Stickers {
			next := make(map[Snowflake]Sticker, len(e.Stickers))
			for _, sticker := range e.Stickers {
				next[sticker.ID] = cloneSticker(sticker)
				s.putUserPointerLocked(sticker.User)
			}
			s.stickers[e.GuildID] = next
		}
	case *VoiceStateUpdate:
		if old, ok := s.voiceStates[e.GuildID][e.UserID]; ok {
			snapshot := cloneVoiceState(old)
			e.BeforeUpdate = &snapshot
		}
		s.putVoiceStateLocked(e.VoiceState)
	case *PresenceUpdate:
		if e.User == nil {
			break
		}
		if old, ok := s.presences[e.GuildID][e.User.ID]; ok {
			snapshot := clonePresence(old)
			e.BeforeUpdate = &snapshot
		}
		s.putPresenceLocked(*e)
	case *PresencesReplace:
		s.presences = make(map[Snowflake]map[Snowflake]PresenceUpdate)
		for _, presence := range *e {
			if presence.User != nil {
				s.putPresenceLocked(presence)
			}
		}
	case *MessageCreate:
		s.putMessageLocked(e.Message)
	case *MessageUpdate:
		if old, ok := s.messageLocked(e.ChannelID, e.ID); ok {
			snapshot := cloneMessage(old)
			e.BeforeUpdate = &snapshot
			s.putMessageLocked(mergeMessage(old, e.Message))
		} else {
			s.putMessageLocked(e.Message)
		}
	case *MessageDelete:
		if old, ok := s.messageLocked(e.ChannelID, e.ID); ok {
			snapshot := cloneMessage(old)
			e.BeforeDelete = &snapshot
		}
		s.deleteMessageLocked(e.ChannelID, e.ID)
	case *MessageDeleteBulk:
		for _, id := range e.IDs {
			if old, ok := s.messageLocked(e.ChannelID, id); ok {
				e.BeforeDelete = append(e.BeforeDelete, cloneMessage(old))
			}
			s.deleteMessageLocked(e.ChannelID, id)
		}
	case *MessageReactionAdd:
		if old, ok := s.messageLocked(e.ChannelID, e.MessageID); ok {
			snapshot := cloneMessage(old)
			e.BeforeUpdate = &snapshot
		}
		if e.Member != nil {
			s.putEventMemberLocked(e.GuildID, e.UserID, e.Member)
		}
		s.updateReactionLocked(e.ChannelID, e.MessageID, e.UserID, e.Emoji, 1)
	case *MessageReactionRemove:
		if old, ok := s.messageLocked(e.ChannelID, e.MessageID); ok {
			snapshot := cloneMessage(old)
			e.BeforeUpdate = &snapshot
		}
		s.updateReactionLocked(e.ChannelID, e.MessageID, e.UserID, e.Emoji, -1)
	case *MessageReactionRemoveAll:
		if message, ok := s.messageLocked(e.ChannelID, e.MessageID); ok {
			snapshot := cloneMessage(message)
			e.BeforeUpdate = &snapshot
			message.Reactions = nil
			s.putMessageLocked(message)
		}
	case *MessageReactionRemoveEmoji:
		if message, ok := s.messageLocked(e.ChannelID, e.MessageID); ok {
			snapshot := cloneMessage(message)
			e.BeforeUpdate = &snapshot
			for i := range message.Reactions {
				if sameEmoji(message.Reactions[i].Emoji, e.Emoji) {
					message.Reactions = append(message.Reactions[:i], message.Reactions[i+1:]...)
					break
				}
			}
			s.putMessageLocked(message)
		}
	case *ChannelPinsUpdate:
		if channel, ok := s.channels[e.ChannelID]; ok {
			snapshot := cloneChannel(channel)
			e.BeforeUpdate = &snapshot
			channel.LastPinTimestamp = e.LastPinTimestamp
			s.channels[e.ChannelID] = cloneChannel(channel)
		}
	case *ChannelInfo:
		for _, info := range e.Channels {
			channel, ok := s.channels[info.ID]
			if !ok {
				continue
			}
			if info.Status != nil {
				channel.Status = *info.Status
			}
			if info.VoiceStartTime != nil {
				started := time.Unix(*info.VoiceStartTime, 0).UTC()
				channel.VoiceStartTime = &started
			}
			s.channels[info.ID] = cloneChannel(channel)
		}
	case *VoiceChannelStatusUpdate:
		if channel, ok := s.channels[e.ID]; ok {
			snapshot := cloneChannel(channel)
			e.BeforeUpdate = &snapshot
			if e.Status == nil {
				channel.Status = ""
			} else {
				channel.Status = *e.Status
			}
			s.channels[e.ID] = cloneChannel(channel)
		}
	case *VoiceChannelStartTimeUpdate:
		if channel, ok := s.channels[e.ID]; ok {
			snapshot := cloneChannel(channel)
			e.BeforeUpdate = &snapshot
			if e.VoiceStartTime == nil {
				channel.VoiceStartTime = nil
			} else {
				started := time.Unix(*e.VoiceStartTime, 0).UTC()
				channel.VoiceStartTime = &started
			}
			s.channels[e.ID] = cloneChannel(channel)
		}
	case *TypingStart:
		if e.Member != nil {
			s.putEventMemberLocked(e.GuildID, e.UserID, e.Member)
		}
	case *GuildBanAdd:
		s.putUserPointerLocked(e.User)
	case *GuildBanRemove:
		s.putUserPointerLocked(e.User)
	case *UserUpdate:
		if old, ok := s.users[e.ID]; ok {
			snapshot := cloneUser(old)
			e.BeforeUpdate = &snapshot
		}
		if s.config.Users {
			s.users[e.ID] = cloneUser(e.User)
		}
	default:
		return ErrUnsupportedStateEvent
	}
	return nil
}

func (s *State) resetShardLocked(shardID, shardCount int) {
	if shardCount <= 1 {
		clear(s.guilds)
		clear(s.loadedGuilds)
		clear(s.channels)
		clear(s.members)
		clear(s.users)
		clear(s.roles)
		clear(s.emojis)
		clear(s.stickers)
		clear(s.threadMembers)
		clear(s.threadGuild)
		clear(s.threadParent)
		clear(s.voiceStates)
		clear(s.presences)
		clear(s.messages)
		clear(s.messageGuild)
		s.selfID = 0
		return
	}
	owned := func(guildID Snowflake) bool {
		return int((uint64(guildID)>>22)%uint64(shardCount)) == shardID
	}
	for guildID := range s.guilds {
		if owned(guildID) {
			delete(s.guilds, guildID)
		}
	}
	for guildID := range s.loadedGuilds {
		if owned(guildID) {
			delete(s.loadedGuilds, guildID)
		}
	}
	// Guild caching may be disabled while one of its resource indexes is on.
	for guildID := range s.members {
		if owned(guildID) {
			delete(s.members, guildID)
		}
	}
	for guildID := range s.roles {
		if owned(guildID) {
			delete(s.roles, guildID)
		}
	}
	for guildID := range s.emojis {
		if owned(guildID) {
			delete(s.emojis, guildID)
		}
	}
	for guildID := range s.stickers {
		if owned(guildID) {
			delete(s.stickers, guildID)
		}
	}
	for guildID := range s.voiceStates {
		if owned(guildID) {
			delete(s.voiceStates, guildID)
		}
	}
	for guildID := range s.presences {
		if owned(guildID) {
			delete(s.presences, guildID)
		}
	}
	for threadID, guildID := range s.threadGuild {
		if owned(guildID) {
			delete(s.threadGuild, threadID)
			delete(s.threadParent, threadID)
			delete(s.threadMembers, threadID)
		}
	}
	for channelID, guildID := range s.messageGuild {
		if owned(guildID) {
			delete(s.messageGuild, channelID)
			delete(s.messages, channelID)
		}
	}
	for channelID, channel := range s.channels {
		if owned(channel.GuildID) {
			delete(s.channels, channelID)
			delete(s.threadMembers, channelID)
			delete(s.threadGuild, channelID)
			delete(s.threadParent, channelID)
			delete(s.messages, channelID)
			delete(s.messageGuild, channelID)
		}
	}
}

func (s *State) putGuildLocked(guild Guild) {
	// GUILD_CREATE is a full snapshot, including when a previously unavailable
	// guild comes back. Replacing its resource indexes prevents stale entries
	// surviving a reconnect or outage.
	if _, loaded := s.loadedGuilds[guild.ID]; loaded {
		s.clearGuildResourcesLocked(guild.ID)
	}
	s.loadedGuilds[guild.ID] = struct{}{}
	if s.config.Guilds {
		// Resources live in dedicated indexes and guildLocked reconstructs a
		// snapshot from them. Keeping a second deep copy here doubled both the
		// allocations and memory of every GUILD_CREATE.
		base := guild
		base.Roles = nil
		base.Emojis = nil
		base.Stickers = nil
		base.VoiceStates = nil
		base.Members = nil
		base.Channels = nil
		base.Threads = nil
		base.Presences = nil
		s.guilds[guild.ID] = cloneGuild(base)
	}
	if s.config.Members {
		s.members[guild.ID] = make(map[Snowflake]Member, len(guild.Members))
	}
	if s.config.Roles {
		s.roles[guild.ID] = make(map[Snowflake]Role, len(guild.Roles))
	}
	if s.config.Emojis {
		s.emojis[guild.ID] = make(map[Snowflake]GuildEmoji, len(guild.Emojis))
	}
	if s.config.Stickers {
		s.stickers[guild.ID] = make(map[Snowflake]Sticker, len(guild.Stickers))
	}
	if s.config.VoiceStates {
		s.voiceStates[guild.ID] = make(map[Snowflake]VoiceState, len(guild.VoiceStates))
	}
	if s.config.Presences {
		s.presences[guild.ID] = make(map[Snowflake]PresenceUpdate, len(guild.Presences))
	}
	for _, channel := range guild.Channels {
		if channel.GuildID.IsZero() {
			channel.GuildID = guild.ID
		}
		s.putChannelLocked(channel)
	}
	for _, thread := range guild.Threads {
		if thread.GuildID.IsZero() {
			thread.GuildID = guild.ID
		}
		s.putChannelLocked(thread)
	}
	for _, member := range guild.Members {
		s.putMemberLocked(guild.ID, member)
	}
	for _, role := range guild.Roles {
		s.putRoleLocked(guild.ID, role)
	}
	if s.config.Emojis {
		for _, emoji := range guild.Emojis {
			if s.emojis[guild.ID] == nil {
				s.emojis[guild.ID] = make(map[Snowflake]GuildEmoji)
			}
			s.emojis[guild.ID][emoji.ID] = cloneGuildEmoji(emoji)
		}
	}
	if s.config.Stickers {
		for _, sticker := range guild.Stickers {
			if s.stickers[guild.ID] == nil {
				s.stickers[guild.ID] = make(map[Snowflake]Sticker)
			}
			s.stickers[guild.ID][sticker.ID] = cloneSticker(sticker)
		}
	}
	for _, voice := range guild.VoiceStates {
		if voice.GuildID.IsZero() {
			voice.GuildID = guild.ID
		}
		s.putVoiceStateLocked(voice)
	}
	for i := range guild.Presences {
		if guild.Presences[i].GuildID.IsZero() {
			guild.Presences[i].GuildID = guild.ID
		}
		s.putPresenceLocked(guild.Presences[i])
	}
}

func (s *State) removeGuildLocked(id Snowflake) {
	delete(s.guilds, id)
	delete(s.loadedGuilds, id)
	s.clearGuildResourcesLocked(id)
}

func (s *State) clearGuildResourcesLocked(id Snowflake) {
	delete(s.members, id)
	delete(s.roles, id)
	delete(s.emojis, id)
	delete(s.stickers, id)
	delete(s.voiceStates, id)
	delete(s.presences, id)
	for channelID, channel := range s.channels {
		if channel.GuildID == id {
			delete(s.channels, channelID)
			delete(s.threadMembers, channelID)
			delete(s.threadGuild, channelID)
			delete(s.threadParent, channelID)
			delete(s.messages, channelID)
			delete(s.messageGuild, channelID)
		}
	}
	for threadID, guildID := range s.threadGuild {
		if guildID == id {
			delete(s.threadGuild, threadID)
			delete(s.threadParent, threadID)
			delete(s.threadMembers, threadID)
		}
	}
	for channelID, guildID := range s.messageGuild {
		if guildID == id {
			delete(s.messageGuild, channelID)
			delete(s.messages, channelID)
		}
	}
}

func (s *State) putChannelLocked(channel Channel) {
	if s.config.ThreadMembers && channel.Type.IsThread() && !channel.GuildID.IsZero() {
		s.threadGuild[channel.ID] = channel.GuildID
		s.threadParent[channel.ID] = channel.ParentID
	}
	if s.config.Channels {
		s.channels[channel.ID] = cloneChannel(channel)
	}
}

func (s *State) updateChannelLocked(channel *Channel, before **Channel) {
	if old, ok := s.channels[channel.ID]; ok {
		snapshot := cloneChannel(old)
		*before = &snapshot
	}
	s.putChannelLocked(*channel)
}

func (s *State) deleteChannelLocked(id Snowflake, before **Channel) {
	if old, ok := s.channels[id]; ok {
		snapshot := cloneChannel(old)
		*before = &snapshot
	}
	delete(s.channels, id)
	delete(s.threadMembers, id)
	delete(s.threadGuild, id)
	delete(s.threadParent, id)
	delete(s.messages, id)
	delete(s.messageGuild, id)
}

func (s *State) putMemberLocked(guildID Snowflake, member Member) {
	if s.config.Members && member.User != nil {
		if s.members[guildID] == nil {
			s.members[guildID] = make(map[Snowflake]Member)
		}
		s.members[guildID][member.User.ID] = cloneMember(member)
	}
	s.putUserPointerLocked(member.User)
}

func (s *State) putUserPointerLocked(user *User) {
	if s.config.Users && user != nil {
		if old, ok := s.users[user.ID]; ok && user.Username == "" {
			s.users[user.ID] = mergeUser(old, *user)
		} else {
			s.users[user.ID] = cloneUser(*user)
		}
	}
}

func (s *State) putEventMemberLocked(guildID, userID Snowflake, member *Member) {
	if member == nil {
		return
	}
	copy := cloneMember(*member)
	if copy.User == nil && !userID.IsZero() {
		if user, ok := s.users[userID]; ok {
			copy.User = &user
		} else {
			copy.User = &User{ID: userID}
		}
	}
	s.putMemberLocked(guildID, copy)
}

func (s *State) putRoleLocked(guildID Snowflake, role Role) {
	if !s.config.Roles {
		return
	}
	if s.roles[guildID] == nil {
		s.roles[guildID] = make(map[Snowflake]Role)
	}
	s.roles[guildID][role.ID] = cloneRole(role)
}

func (s *State) putThreadMemberLocked(member ThreadMember) {
	if !s.config.ThreadMembers {
		return
	}
	uid := threadMemberUserID(member)
	if member.ID.IsZero() || uid.IsZero() {
		return
	}
	if s.threadMembers[member.ID] == nil {
		s.threadMembers[member.ID] = make(map[Snowflake]ThreadMember)
	}
	s.threadMembers[member.ID][uid] = cloneThreadMember(member)
	if member.Member != nil {
		s.putUserPointerLocked(member.Member.User)
	}
}

func (s *State) putVoiceStateLocked(voice VoiceState) {
	if !s.config.VoiceStates || voice.GuildID.IsZero() || voice.UserID.IsZero() {
		return
	}
	if voice.ChannelID.IsZero() {
		delete(s.voiceStates[voice.GuildID], voice.UserID)
		return
	}
	if s.voiceStates[voice.GuildID] == nil {
		s.voiceStates[voice.GuildID] = make(map[Snowflake]VoiceState)
	}
	s.voiceStates[voice.GuildID][voice.UserID] = cloneVoiceState(voice)
	if voice.Member != nil {
		s.putUserPointerLocked(voice.Member.User)
	}
}

func (s *State) putPresenceLocked(presence PresenceUpdate) {
	if presence.User == nil {
		return
	}
	s.putUserPointerLocked(presence.User)
	if !s.config.Presences || presence.GuildID.IsZero() {
		return
	}
	if s.presences[presence.GuildID] == nil {
		s.presences[presence.GuildID] = make(map[Snowflake]PresenceUpdate)
	}
	s.presences[presence.GuildID][presence.User.ID] = clonePresence(presence)
}

func (s *State) applyThreadSyncLocked(e *ThreadListSync) {
	if len(e.ChannelIDs) == 0 {
		for id, guildID := range s.threadGuild {
			if guildID == e.GuildID {
				s.deleteThreadLocked(id)
			}
		}
	} else {
		parents := make(map[Snowflake]struct{}, len(e.ChannelIDs))
		for _, id := range e.ChannelIDs {
			parents[id] = struct{}{}
		}
		for id, guildID := range s.threadGuild {
			if guildID != e.GuildID {
				continue
			}
			if _, ok := parents[s.threadParent[id]]; ok {
				s.deleteThreadLocked(id)
			}
		}
	}
	for _, thread := range e.Threads {
		s.putChannelLocked(thread)
	}
	for _, member := range e.Members {
		s.putThreadMemberLocked(member)
	}
}

func (s *State) deleteThreadLocked(id Snowflake) {
	delete(s.channels, id)
	delete(s.threadMembers, id)
	delete(s.threadGuild, id)
	delete(s.threadParent, id)
	delete(s.messages, id)
	delete(s.messageGuild, id)
}

func (s *State) putMessageLocked(message Message) {
	if s.config.MaxMessagesPerChannel == 0 || message.ChannelID.IsZero() || message.ID.IsZero() {
		return
	}
	cache := s.messages[message.ChannelID]
	if cache == nil {
		cache = &channelMessages{items: make(map[Snowflake]Message)}
		s.messages[message.ChannelID] = cache
	}
	if _, exists := cache.items[message.ID]; !exists {
		cache.order = append(cache.order, message.ID)
	}
	cache.items[message.ID] = cloneMessage(message)
	if !message.GuildID.IsZero() {
		s.messageGuild[message.ChannelID] = message.GuildID
	}
	for len(cache.order) > s.config.MaxMessagesPerChannel {
		delete(cache.items, cache.order[0])
		cache.order = cache.order[1:]
	}
	s.putUserPointerLocked(message.Author)
	if message.Member != nil && !message.GuildID.IsZero() {
		userID := Snowflake(0)
		if message.Author != nil {
			userID = message.Author.ID
		}
		s.putEventMemberLocked(message.GuildID, userID, message.Member)
	}
	for i := range message.Mentions {
		s.putUserPointerLocked(&message.Mentions[i])
	}
}

func (s *State) messageLocked(channelID, messageID Snowflake) (Message, bool) {
	cache := s.messages[channelID]
	if cache == nil {
		return Message{}, false
	}
	v, ok := cache.items[messageID]
	return v, ok
}

func (s *State) deleteMessageLocked(channelID, messageID Snowflake) {
	cache := s.messages[channelID]
	if cache == nil {
		return
	}
	delete(cache.items, messageID)
	for i, id := range cache.order {
		if id == messageID {
			cache.order = append(cache.order[:i], cache.order[i+1:]...)
			break
		}
	}
	if len(cache.items) == 0 {
		delete(s.messages, channelID)
		delete(s.messageGuild, channelID)
	}
}

func (s *State) updateReactionLocked(channelID, messageID, userID Snowflake, emoji Emoji, delta int) {
	message, ok := s.messageLocked(channelID, messageID)
	if !ok {
		return
	}
	for i := range message.Reactions {
		if sameEmoji(message.Reactions[i].Emoji, emoji) {
			message.Reactions[i].Count += delta
			if userID == s.selfID {
				message.Reactions[i].Me = delta > 0
			}
			if message.Reactions[i].Count <= 0 {
				message.Reactions = append(message.Reactions[:i], message.Reactions[i+1:]...)
			}
			s.putMessageLocked(message)
			return
		}
	}
	if delta > 0 {
		message.Reactions = append(message.Reactions, Reaction{Count: delta, Me: userID == s.selfID, Emoji: emoji})
		s.putMessageLocked(message)
	}
}

func sameEmoji(a, b Emoji) bool {
	if !a.ID.IsZero() || !b.ID.IsZero() {
		return a.ID == b.ID
	}
	return a.Name == b.Name
}

func threadMemberUserID(member ThreadMember) Snowflake {
	if !member.UserID.IsZero() {
		return member.UserID
	}
	if member.Member != nil && member.Member.User != nil {
		return member.Member.User.ID
	}
	return 0
}
