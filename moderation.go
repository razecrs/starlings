package starlings

import "errors"

// ModerationError explains why one member cannot act on another. Its message
// is written for the person who ran the command, so it can be shown as is.
type ModerationError struct {
	Reason ModerationDenial
}

func (e *ModerationError) Error() string { return e.Reason.String() }

// ModerationDenial is the reason CanModerate refused.
type ModerationDenial uint8

const (
	DenyTargetIsSelf ModerationDenial = iota + 1
	DenyTargetIsOwner
	DenyTargetIsBot
	DenyActorTooLow
	DenyBotTooLow
	DenyActorTimedOut
)

func (d ModerationDenial) String() string {
	switch d {
	case DenyTargetIsSelf:
		return "You cannot do that to yourself."
	case DenyTargetIsOwner:
		return "The server owner cannot be moderated."
	case DenyTargetIsBot:
		return "I cannot do that to myself."
	case DenyActorTooLow:
		return "Your highest role must be above theirs."
	case DenyBotTooLow:
		return "My highest role must be above theirs. Move my role higher in Server Settings > Roles."
	case DenyActorTimedOut:
		return "You cannot moderate while you are timed out."
	}
	return "That action is not allowed."
}

// CanModerate reports whether actor may kick, ban, time out, or change the
// roles of target, and whether the bot can carry it out. It applies Discord's
// role hierarchy: the owner outranks everyone, and otherwise a member's
// highest role must be strictly above the target's. It does not check the
// specific permission; combine it with BasePermissions or Permissions for that.
//
// A refusal is a *ModerationError. ErrGuildNotCached and ErrMemberNotCached
// mean the cache cannot answer, not that the action is refused.
func (s *State) CanModerate(guildID, actorID, targetID Snowflake) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	guild, ok := s.guilds[guildID]
	if !ok {
		return ErrGuildNotCached
	}
	deny := func(d ModerationDenial) error { return &ModerationError{Reason: d} }
	switch {
	case actorID == targetID:
		return deny(DenyTargetIsSelf)
	case targetID == guild.OwnerID:
		return deny(DenyTargetIsOwner)
	case targetID == s.selfID && !s.selfID.IsZero():
		return deny(DenyTargetIsBot)
	}

	target, ok := s.members[guildID][targetID]
	if !ok {
		return ErrMemberNotCached
	}
	targetTop := s.topPositionLocked(guildID, target.Roles)

	if actorID != guild.OwnerID {
		actor, ok := s.members[guildID][actorID]
		if !ok {
			return ErrMemberNotCached
		}
		if actor.TimedOut() {
			return deny(DenyActorTimedOut)
		}
		if s.topPositionLocked(guildID, actor.Roles) <= targetTop {
			return deny(DenyActorTooLow)
		}
	}
	if !s.selfID.IsZero() && s.selfID != guild.OwnerID {
		bot, ok := s.members[guildID][s.selfID]
		if !ok {
			return ErrMemberNotCached
		}
		if s.topPositionLocked(guildID, bot.Roles) <= targetTop {
			return deny(DenyBotTooLow)
		}
	}
	return nil
}

func (s *State) topPositionLocked(guildID Snowflake, roles []Snowflake) int {
	top := 0
	for _, id := range roles {
		if role, ok := s.roles[guildID][id]; ok && role.Position > top {
			top = role.Position
		}
	}
	return top
}

// IsModerationDenial reports whether err is a CanModerate refusal rather than
// a cache miss.
func IsModerationDenial(err error) bool {
	var m *ModerationError
	return errors.As(err, &m)
}
