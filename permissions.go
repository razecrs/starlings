package starlings

import (
	"strconv"
	"strings"
)

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

// Missing returns the permissions required contains but p does not. An
// administrator is never missing a permission.
func (p Permissions) Missing(required Permissions) Permissions {
	if p&PermissionAdministrator != 0 {
		return 0
	}
	return required &^ p
}

// Names returns stable Go-style names for every known bit in p, in bit order.
// Unknown future bits are preserved by appending their decimal value.
func (p Permissions) Names() []string {
	names := make([]string, 0, 8)
	remaining := p
	for _, named := range permissionNames {
		if p&named.permission != 0 {
			names = append(names, named.name)
			remaining &^= named.permission
		}
	}
	if remaining != 0 {
		names = append(names, "Unknown("+remaining.String()+")")
	}
	return names
}

// HumanString returns known permission names joined for logs and user-facing
// diagnostics. Zero is rendered as "None". String remains the decimal wire
// form Discord expects.
func (p Permissions) HumanString() string {
	names := p.Names()
	if len(names) == 0 {
		return "None"
	}
	for i, name := range names {
		names[i] = splitPermissionName(name)
	}
	return strings.Join(names, ", ")
}

func splitPermissionName(name string) string {
	var out strings.Builder
	runes := []rune(name)
	for i, current := range runes {
		if i > 0 && current >= 'A' && current <= 'Z' {
			previous := runes[i-1]
			nextLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			if previous >= 'a' && previous <= 'z' || previous >= 'A' && previous <= 'Z' && nextLower {
				out.WriteByte(' ')
			}
		}
		out.WriteRune(current)
	}
	return out.String()
}

// String renders the bitmask as the decimal string Discord expects in request
// bodies. The mask has outgrown 53 bits, so it never travels as a number.
func (p Permissions) String() string { return strconv.FormatUint(uint64(p), 10) }

var permissionNames = []struct {
	permission Permissions
	name       string
}{
	{PermissionCreateInstantInvite, "CreateInstantInvite"},
	{PermissionKickMembers, "KickMembers"},
	{PermissionBanMembers, "BanMembers"},
	{PermissionAdministrator, "Administrator"},
	{PermissionManageChannels, "ManageChannels"},
	{PermissionManageGuild, "ManageGuild"},
	{PermissionAddReactions, "AddReactions"},
	{PermissionViewAuditLog, "ViewAuditLog"},
	{PermissionPrioritySpeaker, "PrioritySpeaker"},
	{PermissionStream, "Stream"},
	{PermissionViewChannel, "ViewChannel"},
	{PermissionSendMessages, "SendMessages"},
	{PermissionSendTTSMessages, "SendTTSMessages"},
	{PermissionManageMessages, "ManageMessages"},
	{PermissionEmbedLinks, "EmbedLinks"},
	{PermissionAttachFiles, "AttachFiles"},
	{PermissionReadMessageHistory, "ReadMessageHistory"},
	{PermissionMentionEveryone, "MentionEveryone"},
	{PermissionUseExternalEmojis, "UseExternalEmojis"},
	{PermissionViewGuildInsights, "ViewGuildInsights"},
	{PermissionConnect, "Connect"},
	{PermissionSpeak, "Speak"},
	{PermissionMuteMembers, "MuteMembers"},
	{PermissionDeafenMembers, "DeafenMembers"},
	{PermissionMoveMembers, "MoveMembers"},
	{PermissionUseVAD, "UseVAD"},
	{PermissionChangeNickname, "ChangeNickname"},
	{PermissionManageNicknames, "ManageNicknames"},
	{PermissionManageRoles, "ManageRoles"},
	{PermissionManageWebhooks, "ManageWebhooks"},
	{PermissionManageGuildExpressions, "ManageGuildExpressions"},
	{PermissionUseApplicationCommands, "UseApplicationCommands"},
	{PermissionRequestToSpeak, "RequestToSpeak"},
	{PermissionManageEvents, "ManageEvents"},
	{PermissionManageThreads, "ManageThreads"},
	{PermissionCreatePublicThreads, "CreatePublicThreads"},
	{PermissionCreatePrivateThreads, "CreatePrivateThreads"},
	{PermissionUseExternalStickers, "UseExternalStickers"},
	{PermissionSendMessagesInThreads, "SendMessagesInThreads"},
	{PermissionUseEmbeddedActivities, "UseEmbeddedActivities"},
	{PermissionModerateMembers, "ModerateMembers"},
	{PermissionViewCreatorMonetizationAnalytics, "ViewCreatorMonetizationAnalytics"},
	{PermissionUseSoundboard, "UseSoundboard"},
	{PermissionCreateGuildExpressions, "CreateGuildExpressions"},
	{PermissionCreateEvents, "CreateEvents"},
	{PermissionUseExternalSounds, "UseExternalSounds"},
	{PermissionSendVoiceMessages, "SendVoiceMessages"},
	{PermissionSetVoiceChannelStatus, "SetVoiceChannelStatus"},
	{PermissionSendPolls, "SendPolls"},
	{PermissionUseExternalApps, "UseExternalApps"},
	{PermissionPinMessages, "PinMessages"},
	{PermissionBypassSlowmode, "BypassSlowmode"},
}
