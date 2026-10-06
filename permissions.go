package starlings

import "strconv"

// Permissions is a bitmask of what an account may do. Discord sends it as a
// decimal string because the mask has outgrown 53 bits, so struct fields that
// hold one need the ",string" JSON tag option.
type Permissions uint64

const (
	PermissionCreateInstantInvite Permissions = 1 << iota
	PermissionKickMembers
	PermissionBanMembers
	PermissionAdministrator
	PermissionManageChannels
	PermissionManageGuild
	PermissionAddReactions
	PermissionViewAuditLog
	PermissionPrioritySpeaker
	PermissionStream
	PermissionViewChannel
	PermissionSendMessages
	PermissionSendTTSMessages
	PermissionManageMessages
	PermissionEmbedLinks
	PermissionAttachFiles
	PermissionReadMessageHistory
	PermissionMentionEveryone
	PermissionUseExternalEmojis
	PermissionViewGuildInsights
	PermissionConnect
	PermissionSpeak
	PermissionMuteMembers
	PermissionDeafenMembers
	PermissionMoveMembers
	PermissionUseVAD
	PermissionChangeNickname
	PermissionManageNicknames
	PermissionManageRoles
	PermissionManageWebhooks
	PermissionManageGuildExpressions
	PermissionUseApplicationCommands
	PermissionRequestToSpeak
	PermissionManageEvents
	PermissionManageThreads
	PermissionCreatePublicThreads
	PermissionCreatePrivateThreads
	PermissionUseExternalStickers
	PermissionSendMessagesInThreads
	PermissionUseEmbeddedActivities
	PermissionModerateMembers
	PermissionViewCreatorMonetizationAnalytics
	PermissionUseSoundboard
	PermissionCreateGuildExpressions
	PermissionCreateEvents
	PermissionUseExternalSounds
	PermissionSendVoiceMessages
	_ // bit 47 is currently unassigned
	PermissionSetVoiceChannelStatus
	PermissionSendPolls
	PermissionUseExternalApps
	PermissionPinMessages
	PermissionBypassSlowmode
)

// Has reports whether every permission in other is granted by p.
// Administrator short-circuits the check, matching how Discord evaluates it.
func (p Permissions) Has(other Permissions) bool {
	if p&PermissionAdministrator != 0 {
		return true
	}
	return p&other == other
}

// Add returns p with the given permissions granted.
func (p Permissions) Add(other Permissions) Permissions { return p | other }

// Remove returns p with the given permissions revoked.
func (p Permissions) Remove(other Permissions) Permissions { return p &^ other }

// String renders the bitmask as the decimal string Discord expects in request
// bodies. The mask has outgrown 53 bits, so it never travels as a number.
func (p Permissions) String() string { return strconv.FormatUint(uint64(p), 10) }
