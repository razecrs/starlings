package starlings

import "encoding/json/jsontext"

// InteractionType says what triggered an interaction.
type InteractionType int

const (
	InteractionPing InteractionType = iota + 1
	InteractionApplicationCommand
	InteractionMessageComponent    // a button press or select menu choice
	InteractionCommandAutocomplete // the user is still typing an option
	InteractionModalSubmit
)

// Interaction is a user action Discord is asking the bot to respond to: a
// slash command, a button press, an autocomplete request or a modal submit.
//
// Discord expects a reply within three seconds. If the work takes longer, call
// Ctx.Defer first and Ctx.Followup when you have an answer.
type Interaction struct {
	ID                           Snowflake              `json:"id"`
	ApplicationID                Snowflake              `json:"application_id"`
	Type                         InteractionType        `json:"type"`
	Data                         InteractionData        `json:"data"`
	Guild                        *Guild                 `json:"guild"`
	GuildID                      Snowflake              `json:"guild_id"`
	ChannelID                    Snowflake              `json:"channel_id"`
	Channel                      *Channel               `json:"channel"`
	Member                       *Member                `json:"member"` // set in guilds
	User                         *User                  `json:"user"`   // set in DMs
	Token                        string                 `json:"token"`  // valid for 15 minutes
	Version                      int                    `json:"version"`
	Message                      *Message               `json:"message"` // set for component interactions
	AppPermissions               Permissions            `json:"app_permissions,string"`
	Locale                       string                 `json:"locale"`
	GuildLocale                  string                 `json:"guild_locale"`
	Context                      InteractionContextType `json:"context"`
	AuthorizingIntegrationOwners map[string]Snowflake   `json:"authorizing_integration_owners"`
	Entitlements                 []Entitlement          `json:"entitlements"`
	AttachmentSizeLimit          int                    `json:"attachment_size_limit"`
}

// AuthorizingOwner returns the user or guild that installed the app for this
// interaction. The bool is false when Discord did not supply that owner type.
func (i *Interaction) AuthorizingOwner(kind ApplicationIntegrationType) (Snowflake, bool) {
	id, ok := i.AuthorizingIntegrationOwners[itoa(int(kind))]
	return id, ok
}

// Invoker returns the user behind the interaction, whether it came from a
// guild or a DM.
func (i *Interaction) Invoker() *User {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User
	}
	return i.User
}

// InteractionData is the payload of an interaction. Which fields are set
// depends on Interaction.Type.
type InteractionData struct {
	// Application commands.
	ID       Snowflake           `json:"id"`
	Name     string              `json:"name"`
	Type     int                 `json:"type"`
	Resolved *ResolvedData       `json:"resolved"`
	Options  []InteractionOption `json:"options"`
	TargetID Snowflake           `json:"target_id"` // user/message context menu commands
	GuildID  Snowflake           `json:"guild_id"`

	// Message components and modals.
	CustomID      string        `json:"custom_id"`
	ComponentType ComponentType `json:"component_type"`
	Values        []string      `json:"values"`     // select menu choices
	Components    []Component   `json:"components"` // modal submissions
}

// ComponentID returns the numeric component id used by Components V2. ID is
// also the application-command snowflake because Discord uses the same JSON
// field for both interaction variants.
func (d InteractionData) ComponentID() int { return int(d.ID) }

// Component returns a submitted modal component by custom ID, walking through
// labels and action rows so handlers do not need to know the layout shape.
func (d *InteractionData) Component(customID string) *Component {
	return findComponent(d.Components, customID)
}

func findComponent(components []Component, customID string) *Component {
	for i := range components {
		component := &components[i]
		if component.CustomID == customID {
			return component
		}
		if found := findComponent(component.Components, customID); found != nil {
			return found
		}
		if component.Component != nil {
			if component.Component.CustomID == customID {
				return component.Component
			}
			if found := findComponent(component.Component.Components, customID); found != nil {
				return found
			}
		}
	}
	return nil
}

// ResolvedData holds the full objects for IDs referenced by command options,
// so a command taking a user option does not need a second API call to learn
// anything about them.
type ResolvedData struct {
	Users       map[Snowflake]*User       `json:"users"`
	Members     map[Snowflake]*Member     `json:"members"`
	Roles       map[Snowflake]*Role       `json:"roles"`
	Channels    map[Snowflake]*Channel    `json:"channels"`
	Messages    map[Snowflake]*Message    `json:"messages"`
	Attachments map[Snowflake]*Attachment `json:"attachments"`
}

// OptionType is the declared type of a slash command option.
type OptionType int

const (
	OptionSubCommand OptionType = iota + 1
	OptionSubCommandGroup
	OptionString
	OptionInteger
	OptionBoolean
	OptionUser
	OptionChannel
	OptionRole
	OptionMentionable
	OptionNumber
	OptionAttachment
)

// InteractionOption is one argument supplied to a slash command. Value holds
// the raw JSON, which the typed accessors below interpret.
type InteractionOption struct {
	Name    string              `json:"name"`
	Type    OptionType          `json:"type"`
	Value   jsontext.Value      `json:"value"`
	Options []InteractionOption `json:"options"` // set for subcommands and groups
	Focused bool                `json:"focused"` // autocomplete: the option being typed
}
