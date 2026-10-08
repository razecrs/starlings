package starlings

import (
	"errors"
	"time"
)

var (
	ErrGuildNotCached   = errors.New("starlings: guild is not cached")
	ErrChannelNotCached = errors.New("starlings: channel is not cached")
	ErrMemberNotCached  = errors.New("starlings: member is not cached")
)

// BasePermissions calculates a member's guild-wide permissions before channel
// overwrites. Guild owners and administrators receive every permission.
func (s *State) BasePermissions(guildID, userID Snowflake) (value Permissions, err error) {
	if s.guard != nil {
		started := time.Now()
		s.guard.cacheUse("guilds", "members", "roles")
		defer func() { s.guard.observe("state.base_permissions", GuardAutomatic, time.Since(started), err) }()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.basePermissionsLocked(guildID, userID)
}

// Permissions calculates a member's effective permissions in a channel,
// including @everyone, aggregate role, and member-specific overwrites.
func (s *State) Permissions(guildID, channelID, userID Snowflake) (value Permissions, err error) {
	if s.guard != nil {
		started := time.Now()
		s.guard.cacheUse("guilds", "channels", "members", "roles")
		defer func() { s.guard.observe("state.permissions", GuardAutomatic, time.Since(started), err) }()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.permissionsLocked(guildID, channelID, userID)
}

func (s *State) permissionsLocked(guildID, channelID, userID Snowflake) (Permissions, error) {
	base, err := s.basePermissionsLocked(guildID, userID)
	if err != nil {
		return 0, err
	}
	if base&PermissionAdministrator != 0 {
		return ^Permissions(0), nil
	}
	channel, ok := s.channels[channelID]
	if !ok {
		return 0, ErrChannelNotCached
	}
	// Threads inherit their parent's permissions; Discord does not give them an
	// independent overwrite set.
	if channel.Type.IsThread() && len(channel.PermissionOverwrites) == 0 && !channel.ParentID.IsZero() {
		if parent, found := s.channels[channel.ParentID]; found {
			channel = parent
		}
	}

	permissions := base
	for _, overwrite := range channel.PermissionOverwrites {
		if overwrite.Type == 0 && overwrite.ID == guildID {
			permissions = permissions.Remove(overwrite.Deny).Add(overwrite.Allow)
			break
		}
	}

	member := s.members[guildID][userID]
	roleIDs := make(map[Snowflake]struct{}, len(member.Roles))
	for _, id := range member.Roles {
		roleIDs[id] = struct{}{}
	}
	var deny, allow Permissions
	for _, overwrite := range channel.PermissionOverwrites {
		if overwrite.Type != 0 || overwrite.ID == guildID {
			continue
		}
		if _, ok := roleIDs[overwrite.ID]; ok {
			deny |= overwrite.Deny
			allow |= overwrite.Allow
		}
	}
	permissions = permissions.Remove(deny).Add(allow)
	for _, overwrite := range channel.PermissionOverwrites {
		if overwrite.Type == 1 && overwrite.ID == userID {
			permissions = permissions.Remove(overwrite.Deny).Add(overwrite.Allow)
			break
		}
	}
	// Overwrites cannot give a timed-out member back what the timeout removed.
	if member.TimedOut() {
		permissions &= timedOutPermissions
	}
	return permissions, nil
}

// ChannelPermissions is the short form when the channel is cached; it derives
// the guild ID automatically.
func (s *State) ChannelPermissions(channelID, userID Snowflake) (value Permissions, err error) {
	if s.guard != nil {
		started := time.Now()
		s.guard.cacheUse("guilds", "channels", "members", "roles")
		defer func() { s.guard.observe("state.permissions", GuardAutomatic, time.Since(started), err) }()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	channel, ok := s.channels[channelID]
	if !ok {
		return 0, ErrChannelNotCached
	}
	return s.permissionsLocked(channel.GuildID, channelID, userID)
}

// MessagePermissions is the convenient form for a cached message author.
func (s *State) MessagePermissions(message *Message) (Permissions, error) {
	if message == nil || message.Author == nil {
		return 0, ErrMemberNotCached
	}
	return s.Permissions(message.GuildID, message.ChannelID, message.Author.ID)
}

func (s *State) basePermissionsLocked(guildID, userID Snowflake) (Permissions, error) {
	guild, ok := s.guilds[guildID]
	if !ok {
		return 0, ErrGuildNotCached
	}
	if guild.OwnerID == userID {
		return ^Permissions(0), nil
	}
	member, ok := s.members[guildID][userID]
	if !ok {
		return 0, ErrMemberNotCached
	}
	var permissions Permissions
	if everyone, ok := s.roles[guildID][guildID]; ok {
		permissions = everyone.Permissions
	}
	for _, id := range member.Roles {
		if role, found := s.roles[guildID][id]; found {
			permissions |= role.Permissions
		}
	}
	if permissions&PermissionAdministrator != 0 {
		return ^Permissions(0), nil
	}
	if member.TimedOut() {
		permissions &= timedOutPermissions
	}
	return permissions, nil
}

// timedOutPermissions is all Discord leaves a timed-out member who is neither
// the owner nor an administrator.
const timedOutPermissions = PermissionViewChannel | PermissionReadMessageHistory

// UserColor returns the member's display colour from their highest coloured
// role. Zero means Discord's default text colour.
func (s *State) UserColor(guildID, userID Snowflake) (value int, err error) {
	if s.guard != nil {
		started := time.Now()
		s.guard.cacheUse("guilds", "members", "roles")
		defer func() { s.guard.observe("state.user_color", GuardAutomatic, time.Since(started), err) }()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.guilds[guildID]; !ok {
		return 0, ErrGuildNotCached
	}
	member, ok := s.members[guildID][userID]
	if !ok {
		return 0, ErrMemberNotCached
	}
	bestPosition, bestID, color := -1, Snowflake(0), 0
	for _, id := range member.Roles {
		role, found := s.roles[guildID][id]
		if !found || role.Color == 0 {
			continue
		}
		if role.Position > bestPosition || (role.Position == bestPosition && role.ID > bestID) {
			bestPosition, bestID, color = role.Position, role.ID, role.Color
		}
	}
	return color, nil
}

// MessageColor returns the cached author's display colour.
func (s *State) MessageColor(message *Message) (int, error) {
	if message == nil || message.Author == nil {
		return 0, ErrMemberNotCached
	}
	return s.UserColor(message.GuildID, message.Author.ID)
}
