package starlings

func cloneUser(value User) User { return value }

func cloneMember(value Member) Member {
	if value.User != nil {
		user := cloneUser(*value.User)
		value.User = &user
	}
	value.Roles = append([]Snowflake(nil), value.Roles...)
	if value.PremiumSince != nil {
		t := *value.PremiumSince
		value.PremiumSince = &t
	}
	if value.CommunicationDisabledUntil != nil {
		t := *value.CommunicationDisabledUntil
		value.CommunicationDisabledUntil = &t
	}
	return value
}

func cloneRole(value Role) Role { return value }

func cloneGuildEmoji(value GuildEmoji) GuildEmoji {
	value.Roles = append([]Snowflake(nil), value.Roles...)
	if value.User != nil {
		user := cloneUser(*value.User)
		value.User = &user
	}
	return value
}

func cloneSticker(value Sticker) Sticker {
	if value.User != nil {
		user := cloneUser(*value.User)
		value.User = &user
	}
	return value
}

func cloneThreadMember(value ThreadMember) ThreadMember {
	if value.Member != nil {
		member := cloneMember(*value.Member)
		value.Member = &member
	}
	return value
}

func cloneVoiceState(value VoiceState) VoiceState {
	if value.Member != nil {
		member := cloneMember(*value.Member)
		value.Member = &member
	}
	return value
}

func clonePresence(value PresenceUpdate) PresenceUpdate {
	if value.User != nil {
		user := cloneUser(*value.User)
		value.User = &user
	}
	value.Activities = append([]Activity(nil), value.Activities...)
	value.BeforeUpdate = nil
	return value
}

func cloneChannel(value Channel) Channel {
	value.PermissionOverwrites = append([]Overwrite(nil), value.PermissionOverwrites...)
	value.Recipients = append([]User(nil), value.Recipients...)
	value.AvailableTags = append([]ForumTag(nil), value.AvailableTags...)
	value.AppliedTags = append([]Snowflake(nil), value.AppliedTags...)
	if value.LastPinTimestamp != nil {
		t := *value.LastPinTimestamp
		value.LastPinTimestamp = &t
	}
	if value.VoiceStartTime != nil {
		t := *value.VoiceStartTime
		value.VoiceStartTime = &t
	}
	if value.ThreadMetadata != nil {
		meta := *value.ThreadMetadata
		if meta.CreateTimestamp != nil {
			t := *meta.CreateTimestamp
			meta.CreateTimestamp = &t
		}
		value.ThreadMetadata = &meta
	}
	if value.DefaultReaction != nil {
		reaction := *value.DefaultReaction
		value.DefaultReaction = &reaction
	}
	return value
}

func cloneGuild(value Guild) Guild {
	value.Roles = append([]Role(nil), value.Roles...)
	if value.Emojis != nil {
		emojis := make([]GuildEmoji, len(value.Emojis))
		for i := range value.Emojis {
			emojis[i] = cloneGuildEmoji(value.Emojis[i])
		}
		value.Emojis = emojis
	}
	if value.Stickers != nil {
		stickers := make([]Sticker, len(value.Stickers))
		for i := range value.Stickers {
			stickers[i] = cloneSticker(value.Stickers[i])
		}
		value.Stickers = stickers
	}
	value.Features = append([]string(nil), value.Features...)
	value.VoiceStates = cloneVoiceStates(value.VoiceStates)
	value.Members = cloneMembers(value.Members)
	value.Channels = cloneChannels(value.Channels)
	value.Threads = cloneChannels(value.Threads)
	value.Presences = clonePresences(value.Presences)
	value.StageInstances = append([]StageInstance(nil), value.StageInstances...)
	if value.GuildScheduledEvents != nil {
		events := make([]ScheduledEvent, len(value.GuildScheduledEvents))
		for i, event := range value.GuildScheduledEvents {
			events[i] = event
			if event.ScheduledEndTime != nil {
				t := *event.ScheduledEndTime
				events[i].ScheduledEndTime = &t
			}
			if event.EntityMetadata != nil {
				metadata := *event.EntityMetadata
				events[i].EntityMetadata = &metadata
			}
			if event.Creator != nil {
				user := cloneUser(*event.Creator)
				events[i].Creator = &user
			}
		}
		value.GuildScheduledEvents = events
	}
	if value.SoundboardSounds != nil {
		sounds := make([]SoundboardSound, len(value.SoundboardSounds))
		copy(sounds, value.SoundboardSounds)
		for i := range sounds {
			if sounds[i].User != nil {
				user := cloneUser(*sounds[i].User)
				sounds[i].User = &user
			}
		}
		value.SoundboardSounds = sounds
	}
	return value
}

func cloneMembers(values []Member) []Member {
	if values == nil {
		return nil
	}
	out := make([]Member, len(values))
	for i := range values {
		out[i] = cloneMember(values[i])
	}
	return out
}

func cloneChannels(values []Channel) []Channel {
	if values == nil {
		return nil
	}
	out := make([]Channel, len(values))
	for i := range values {
		out[i] = cloneChannel(values[i])
	}
	return out
}

func cloneVoiceStates(values []VoiceState) []VoiceState {
	if values == nil {
		return nil
	}
	out := make([]VoiceState, len(values))
	for i := range values {
		out[i] = cloneVoiceState(values[i])
	}
	return out
}

func clonePresences(values []PresenceUpdate) []PresenceUpdate {
	if values == nil {
		return nil
	}
	out := make([]PresenceUpdate, len(values))
	for i := range values {
		out[i] = clonePresence(values[i])
	}
	return out
}

func cloneMessage(value Message) Message {
	if value.Author != nil {
		user := cloneUser(*value.Author)
		value.Author = &user
	}
	if value.Member != nil {
		member := cloneMember(*value.Member)
		value.Member = &member
	}
	if value.EditedTimestamp != nil {
		t := *value.EditedTimestamp
		value.EditedTimestamp = &t
	}
	value.Mentions = append([]User(nil), value.Mentions...)
	value.MentionRoles = append([]Snowflake(nil), value.MentionRoles...)
	value.MentionChannels = append([]ChannelMention(nil), value.MentionChannels...)
	value.Attachments = append([]Attachment(nil), value.Attachments...)
	for i := range value.Attachments {
		if value.Attachments[i].Application != nil {
			app := cloneMessageApplication(*value.Attachments[i].Application)
			value.Attachments[i].Application = &app
		}
		if value.Attachments[i].ClipCreatedAt != nil {
			t := *value.Attachments[i].ClipCreatedAt
			value.Attachments[i].ClipCreatedAt = &t
		}
		value.Attachments[i].ClipParticipants = append([]User(nil), value.Attachments[i].ClipParticipants...)
	}
	value.Embeds = cloneEmbeds(value.Embeds)
	value.Reactions = append([]Reaction(nil), value.Reactions...)
	for i := range value.Reactions {
		value.Reactions[i].BurstColors = append([]string(nil), value.Reactions[i].BurstColors...)
	}
	value.Components = cloneComponents(value.Components)
	value.StickerItems = append([]StickerItem(nil), value.StickerItems...)
	if value.Stickers != nil {
		value.Stickers = append([]Sticker(nil), value.Stickers...)
		for i := range value.Stickers {
			value.Stickers[i] = cloneSticker(value.Stickers[i])
		}
	}
	if value.PurchaseNotification != nil {
		v := *value.PurchaseNotification
		if v.GuildProductPurchase != nil {
			purchase := *v.GuildProductPurchase
			v.GuildProductPurchase = &purchase
		}
		value.PurchaseNotification = &v
	}
	if value.LobbyMember != nil {
		v := *value.LobbyMember
		value.LobbyMember = &v
	}
	if value.Activity != nil {
		v := *value.Activity
		value.Activity = &v
	}
	if value.Application != nil {
		v := cloneMessageApplication(*value.Application)
		value.Application = &v
	}
	if value.Interaction != nil {
		v := *value.Interaction
		if v.User != nil {
			u := cloneUser(*v.User)
			v.User = &u
		}
		if v.Member != nil {
			m := cloneMember(*v.Member)
			v.Member = &m
		}
		value.Interaction = &v
	}
	value.InteractionMeta = cloneInteractionMetadata(value.InteractionMeta)
	if value.Thread != nil {
		v := cloneChannel(*value.Thread)
		value.Thread = &v
	}
	if value.RoleSubscription != nil {
		v := *value.RoleSubscription
		value.RoleSubscription = &v
	}
	value.Resolved = cloneResolvedData(value.Resolved)
	if value.Poll != nil {
		value.Poll = clonePoll(value.Poll)
	}
	if value.Call != nil {
		v := *value.Call
		v.Participants = append([]Snowflake(nil), v.Participants...)
		if v.EndedTimestamp != nil {
			t := *v.EndedTimestamp
			v.EndedTimestamp = &t
		}
		value.Call = &v
	}
	if value.Snapshots != nil {
		value.Snapshots = append([]MessageSnapshot(nil), value.Snapshots...)
		for i := range value.Snapshots {
			if value.Snapshots[i].Message != nil {
				v := cloneMessage(*value.Snapshots[i].Message)
				value.Snapshots[i].Message = &v
			}
		}
	}
	if value.SharedClientTheme != nil {
		v := *value.SharedClientTheme
		if v.BackgroundGradientPresetID != nil {
			n := *v.BackgroundGradientPresetID
			v.BackgroundGradientPresetID = &n
		}
		if v.BackgroundGradientAngle != nil {
			n := *v.BackgroundGradientAngle
			v.BackgroundGradientAngle = &n
		}
		value.SharedClientTheme = &v
	}
	if value.ReferencedMsg != nil {
		message := cloneMessage(*value.ReferencedMsg)
		value.ReferencedMsg = &message
	}
	if value.MessageRef != nil {
		ref := *value.MessageRef
		if ref.FailIfNotExists != nil {
			b := *ref.FailIfNotExists
			ref.FailIfNotExists = &b
		}
		value.MessageRef = &ref
	}
	return value
}

func cloneMessageApplication(value MessageApplication) MessageApplication {
	if value.Bot != nil {
		user := cloneUser(*value.Bot)
		value.Bot = &user
	}
	return value
}

func cloneResolvedData(value *ResolvedData) *ResolvedData {
	if value == nil {
		return nil
	}
	out := &ResolvedData{}
	if value.Users != nil {
		out.Users = make(map[Snowflake]*User, len(value.Users))
		for id, user := range value.Users {
			if user != nil {
				v := cloneUser(*user)
				out.Users[id] = &v
			}
		}
	}
	if value.Members != nil {
		out.Members = make(map[Snowflake]*Member, len(value.Members))
		for id, member := range value.Members {
			if member != nil {
				v := cloneMember(*member)
				out.Members[id] = &v
			}
		}
	}
	if value.Roles != nil {
		out.Roles = make(map[Snowflake]*Role, len(value.Roles))
		for id, role := range value.Roles {
			if role != nil {
				v := cloneRole(*role)
				out.Roles[id] = &v
			}
		}
	}
	if value.Channels != nil {
		out.Channels = make(map[Snowflake]*Channel, len(value.Channels))
		for id, channel := range value.Channels {
			if channel != nil {
				v := cloneChannel(*channel)
				out.Channels[id] = &v
			}
		}
	}
	if value.Messages != nil {
		out.Messages = make(map[Snowflake]*Message, len(value.Messages))
		for id, message := range value.Messages {
			if message != nil {
				v := cloneMessage(*message)
				out.Messages[id] = &v
			}
		}
	}
	if value.Attachments != nil {
		out.Attachments = make(map[Snowflake]*Attachment, len(value.Attachments))
		for id, attachment := range value.Attachments {
			if attachment == nil {
				continue
			}
			v := *attachment
			v.ClipParticipants = append([]User(nil), attachment.ClipParticipants...)
			if attachment.ClipCreatedAt != nil {
				t := *attachment.ClipCreatedAt
				v.ClipCreatedAt = &t
			}
			if attachment.Application != nil {
				app := cloneMessageApplication(*attachment.Application)
				v.Application = &app
			}
			out.Attachments[id] = &v
		}
	}
	return out
}

func cloneInteractionMetadata(value *MessageInteractionMetadata) *MessageInteractionMetadata {
	if value == nil {
		return nil
	}
	out := *value
	if value.User != nil {
		u := cloneUser(*value.User)
		out.User = &u
	}
	if value.TargetUser != nil {
		u := cloneUser(*value.TargetUser)
		out.TargetUser = &u
	}
	if value.AuthorizingIntegrationOwners != nil {
		out.AuthorizingIntegrationOwners = make(map[string]Snowflake, len(value.AuthorizingIntegrationOwners))
		for k, v := range value.AuthorizingIntegrationOwners {
			out.AuthorizingIntegrationOwners[k] = v
		}
	}
	out.TriggeringInteractionMetadata = cloneInteractionMetadata(value.TriggeringInteractionMetadata)
	return &out
}

func clonePoll(value *Poll) *Poll {
	if value == nil {
		return nil
	}
	out := *value
	if value.Question.Emoji != nil {
		e := *value.Question.Emoji
		out.Question.Emoji = &e
	}
	out.Answers = append([]PollAnswer(nil), value.Answers...)
	for i := range out.Answers {
		if out.Answers[i].Media.Emoji != nil {
			e := *out.Answers[i].Media.Emoji
			out.Answers[i].Media.Emoji = &e
		}
	}
	if value.Expiry != nil {
		t := *value.Expiry
		out.Expiry = &t
	}
	if value.Results != nil {
		r := *value.Results
		r.AnswerCounts = append([]PollAnswerCount(nil), value.Results.AnswerCounts...)
		out.Results = &r
	}
	return &out
}

func cloneComponents(values []Component) []Component {
	if values == nil {
		return nil
	}
	out := make([]Component, len(values))
	for i, value := range values {
		out[i] = value
		out[i].Components = cloneComponents(value.Components)
		if value.Accessory != nil {
			accessory := cloneComponents([]Component{*value.Accessory})[0]
			out[i].Accessory = &accessory
		}
		if value.Media != nil {
			media := *value.Media
			out[i].Media = &media
		}
		if value.File != nil {
			media := *value.File
			out[i].File = &media
		}
		if value.Component != nil {
			child := cloneComponents([]Component{*value.Component})[0]
			out[i].Component = &child
		}
		out[i].Items = append([]MediaGalleryItem(nil), value.Items...)
		if value.AccentColor != nil {
			n := *value.AccentColor
			out[i].AccentColor = &n
		}
		if value.Divider != nil {
			b := *value.Divider
			out[i].Divider = &b
		}
		if value.Default != nil {
			b := *value.Default
			out[i].Default = &b
		}
		out[i].FileTypes = append([]string(nil), value.FileTypes...)
		out[i].Values = append([]string(nil), value.Values...)
		out[i].DefaultValues = append([]SelectDefaultValue(nil), value.DefaultValues...)
		out[i].ChannelTypes = append([]ChannelType(nil), value.ChannelTypes...)
		if value.Emoji != nil {
			emoji := *value.Emoji
			out[i].Emoji = &emoji
		}
		if value.MinValues != nil {
			n := *value.MinValues
			out[i].MinValues = &n
		}
		if value.Required != nil {
			b := *value.Required
			out[i].Required = &b
		}
		if value.MinLength != nil {
			n := *value.MinLength
			out[i].MinLength = &n
		}
		if value.Options != nil {
			out[i].Options = append([]SelectOption(nil), value.Options...)
			for j := range out[i].Options {
				if out[i].Options[j].Emoji != nil {
					emoji := *out[i].Options[j].Emoji
					out[i].Options[j].Emoji = &emoji
				}
			}
		}
	}
	return out
}

func cloneEmbeds(values []Embed) []Embed {
	if values == nil {
		return nil
	}
	out := make([]Embed, len(values))
	for i, value := range values {
		out[i] = value
		out[i].Fields = append([]EmbedField(nil), value.Fields...)
		if value.Timestamp != nil {
			t := *value.Timestamp
			out[i].Timestamp = &t
		}
		if value.Footer != nil {
			v := *value.Footer
			out[i].Footer = &v
		}
		if value.Image != nil {
			v := *value.Image
			out[i].Image = &v
		}
		if value.Thumbnail != nil {
			v := *value.Thumbnail
			out[i].Thumbnail = &v
		}
		if value.Video != nil {
			v := *value.Video
			out[i].Video = &v
		}
		if value.Provider != nil {
			v := *value.Provider
			out[i].Provider = &v
		}
		if value.Author != nil {
			v := *value.Author
			out[i].Author = &v
		}
	}
	return out
}

func mergeMember(old, update Member) Member {
	if update.User != nil {
		old.User = update.User
	}
	old.Nick = update.Nick
	if update.Avatar != "" {
		old.Avatar = update.Avatar
	}
	if update.Roles != nil {
		old.Roles = update.Roles
	}
	if !update.JoinedAt.IsZero() {
		old.JoinedAt = update.JoinedAt
	}
	if update.PremiumSince != nil {
		old.PremiumSince = update.PremiumSince
	}
	if update.CommunicationDisabledUntil != nil {
		old.CommunicationDisabledUntil = update.CommunicationDisabledUntil
	}
	return old
}

func mergeUser(old, update User) User {
	if update.Username != "" {
		old.Username = update.Username
	}
	if update.Discriminator != "" {
		old.Discriminator = update.Discriminator
	}
	if update.GlobalName != "" {
		old.GlobalName = update.GlobalName
	}
	if update.Avatar != "" {
		old.Avatar = update.Avatar
	}
	if update.Banner != "" {
		old.Banner = update.Banner
	}
	if update.AccentColor != 0 {
		old.AccentColor = update.AccentColor
	}
	if update.Locale != "" {
		old.Locale = update.Locale
	}
	if update.Email != "" {
		old.Email = update.Email
	}
	if update.Flags != 0 {
		old.Flags = update.Flags
	}
	if update.PremiumType != 0 {
		old.PremiumType = update.PremiumType
	}
	if update.PublicFlags != 0 {
		old.PublicFlags = update.PublicFlags
	}
	old.ID = update.ID
	return cloneUser(old)
}

func mergeGuild(old, update Guild) Guild {
	if old.ID.IsZero() {
		return cloneGuild(update)
	}
	roles, emojis, stickers, features := old.Roles, old.Emojis, old.Stickers, old.Features
	voices, members := old.VoiceStates, old.Members
	channels, threads, presences := old.Channels, old.Threads, old.Presences
	stages, events, sounds := old.StageInstances, old.GuildScheduledEvents, old.SoundboardSounds
	old = update
	if update.Roles == nil {
		old.Roles = roles
	}
	if update.Emojis == nil {
		old.Emojis = emojis
	}
	if update.Stickers == nil {
		old.Stickers = stickers
	}
	if update.Features == nil {
		old.Features = features
	}
	if update.VoiceStates == nil {
		old.VoiceStates = voices
	}
	if update.Members == nil {
		old.Members = members
	}
	if update.Channels == nil {
		old.Channels = channels
	}
	if update.Threads == nil {
		old.Threads = threads
	}
	if update.Presences == nil {
		old.Presences = presences
	}
	if update.StageInstances == nil {
		old.StageInstances = stages
	}
	if update.GuildScheduledEvents == nil {
		old.GuildScheduledEvents = events
	}
	if update.SoundboardSounds == nil {
		old.SoundboardSounds = sounds
	}
	return cloneGuild(old)
}

func mergeMessage(old, update Message) Message {
	if update.ChannelID.IsZero() {
		update.ChannelID = old.ChannelID
	}
	if update.GuildID.IsZero() {
		update.GuildID = old.GuildID
	}
	if update.Author == nil {
		update.Author = old.Author
	}
	if update.Member == nil {
		update.Member = old.Member
	}
	if update.Timestamp.IsZero() {
		update.Timestamp = old.Timestamp
	}
	if update.EditedTimestamp == nil {
		update.EditedTimestamp = old.EditedTimestamp
		update.Content = old.Content
	}
	if update.Mentions == nil {
		update.Mentions = old.Mentions
	}
	if update.MentionRoles == nil {
		update.MentionRoles = old.MentionRoles
	}
	if update.Attachments == nil {
		update.Attachments = old.Attachments
	}
	if update.Embeds == nil {
		update.Embeds = old.Embeds
	}
	if update.Reactions == nil {
		update.Reactions = old.Reactions
	}
	if update.Components == nil {
		update.Components = old.Components
	}
	if update.StickerItems == nil {
		update.StickerItems = old.StickerItems
	}
	if update.Stickers == nil {
		update.Stickers = old.Stickers
	}
	if update.PurchaseNotification == nil {
		update.PurchaseNotification = old.PurchaseNotification
	}
	if update.LobbyMember == nil {
		update.LobbyMember = old.LobbyMember
	}
	if update.MentionChannels == nil {
		update.MentionChannels = old.MentionChannels
	}
	if update.Poll == nil {
		update.Poll = old.Poll
	}
	if update.Thread == nil {
		update.Thread = old.Thread
	}
	if update.Snapshots == nil {
		update.Snapshots = old.Snapshots
	}
	if update.InteractionMeta == nil {
		update.InteractionMeta = old.InteractionMeta
	}
	if update.Activity == nil {
		update.Activity = old.Activity
	}
	if update.Application == nil {
		update.Application = old.Application
	}
	if update.Interaction == nil {
		update.Interaction = old.Interaction
	}
	if update.RoleSubscription == nil {
		update.RoleSubscription = old.RoleSubscription
	}
	if update.Resolved == nil {
		update.Resolved = old.Resolved
	}
	if update.Call == nil {
		update.Call = old.Call
	}
	if update.SharedClientTheme == nil {
		update.SharedClientTheme = old.SharedClientTheme
	}
	if update.Nonce == nil {
		update.Nonce = old.Nonce
	}
	if update.ReferencedMsg == nil {
		update.ReferencedMsg = old.ReferencedMsg
	}
	if update.MessageRef == nil {
		update.MessageRef = old.MessageRef
	}
	return update
}
