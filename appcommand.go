package starlings

import (
	"context"
	"net/http"
)

// ApplicationCommandType selects where a command appears in the Discord UI.
type ApplicationCommandType int

// ApplicationIntegrationType says how an application was installed.
type ApplicationIntegrationType int

const (
	IntegrationGuildInstall ApplicationIntegrationType = iota
	IntegrationUserInstall
)

// InteractionContextType is where a command may be used.
type InteractionContextType int

const (
	InteractionContextGuild InteractionContextType = iota
	InteractionContextBotDM
	InteractionContextPrivateChannel
)

type EntryPointHandlerType int

const (
	EntryPointAppHandler EntryPointHandlerType = iota + 1
	EntryPointDiscordLaunchActivity
)

const (
	// CommandChat is a slash command, typed after "/".
	CommandChat ApplicationCommandType = iota + 1
	// CommandUser appears when right-clicking a user.
	CommandUser
	// CommandMessage appears when right-clicking a message.
	CommandMessage
	// CommandPrimaryEntryPoint is the main way to launch an Activity.
	CommandPrimaryEntryPoint
)

// ApplicationCommand is a command registered with Discord.
//
// Registering is separate from handling: this declares the command so it shows
// up in the client, while HandleCommand says what to do when it is used.
type ApplicationCommand struct {
	ID            Snowflake              `json:"id,omitzero"`
	ApplicationID Snowflake              `json:"application_id,omitzero"`
	GuildID       Snowflake              `json:"guild_id,omitzero"`
	Type          ApplicationCommandType `json:"type,omitzero"`

	// Name must be 1-32 characters. For CommandChat it must be lowercase and
	// contain no spaces.
	Name              string            `json:"name"`
	NameLocalizations map[string]string `json:"name_localizations,omitzero"`
	NameLocalized     string            `json:"name_localized,omitzero"`

	// Description is required for CommandChat and must be empty for the
	// right-click command types.
	Description              string            `json:"description,omitzero"` // max 100
	DescriptionLocalizations map[string]string `json:"description_localizations,omitzero"`
	DescriptionLocalized     string            `json:"description_localized,omitzero"`

	Options          []CommandOption              `json:"options,omitzero"` // max 25
	IntegrationTypes []ApplicationIntegrationType `json:"integration_types,omitzero"`
	Contexts         []InteractionContextType     `json:"contexts,omitzero"`
	Handler          EntryPointHandlerType        `json:"handler,omitzero"`

	// DefaultMemberPermissions restricts who sees the command. Discord sends
	// it as a decimal string, and nil means no restriction.
	DefaultMemberPermissions *Permissions `json:"default_member_permissions,string,omitzero"`

	NSFW              bool      `json:"nsfw,omitzero"`
	Version           Snowflake `json:"version,omitzero"`
	DMPermission      *bool     `json:"dm_permission,omitzero"`      // deprecated; use Contexts
	DefaultPermission *bool     `json:"default_permission,omitzero"` // deprecated
}

// CommandOption is one parameter of a slash command, or a subcommand.
type CommandOption struct {
	Type                     OptionType        `json:"type"`
	Name                     string            `json:"name"`
	NameLocalizations        map[string]string `json:"name_localizations,omitzero"`
	NameLocalized            string            `json:"name_localized,omitzero"`
	Description              string            `json:"description"`
	DescriptionLocalizations map[string]string `json:"description_localizations,omitzero"`
	DescriptionLocalized     string            `json:"description_localized,omitzero"`
	Required                 bool              `json:"required,omitzero"`

	// Choices restricts the option to a fixed set. Max 25.
	Choices []CommandChoice `json:"choices,omitzero"`
	// Options holds nested parameters, for OptionSubCommand and
	// OptionSubCommandGroup.
	Options []CommandOption `json:"options,omitzero"`

	// ChannelTypes narrows an OptionChannel to particular kinds of channel.
	ChannelTypes []ChannelType `json:"channel_types,omitzero"`

	MinValue  *float64 `json:"min_value,omitzero"`
	MaxValue  *float64 `json:"max_value,omitzero"`
	MinLength *int     `json:"min_length,omitzero"`
	MaxLength *int     `json:"max_length,omitzero"`

	// Autocomplete asks Discord to send an autocomplete interaction as the
	// user types. It cannot be combined with Choices.
	Autocomplete bool `json:"autocomplete,omitzero"`
}

// CommandChoice is one preset value for an option. Value must be a string or a
// number, matching the option's type.
type CommandChoice struct {
	Name              string            `json:"name"`
	NameLocalizations map[string]string `json:"name_localizations,omitzero"`
	NameLocalized     string            `json:"name_localized,omitzero"`
	Value             any               `json:"value"`
}

// StringOption builds a required-or-optional text parameter.
func StringOption(name, description string, required bool) CommandOption {
	return CommandOption{Type: OptionString, Name: name, Description: description, Required: required}
}

// IntOption builds an integer parameter.
func IntOption(name, description string, required bool) CommandOption {
	return CommandOption{Type: OptionInteger, Name: name, Description: description, Required: required}
}

// BoolOption builds a true/false parameter.
func BoolOption(name, description string, required bool) CommandOption {
	return CommandOption{Type: OptionBoolean, Name: name, Description: description, Required: required}
}

// UserOption builds a user picker.
func UserOption(name, description string, required bool) CommandOption {
	return CommandOption{Type: OptionUser, Name: name, Description: description, Required: required}
}

// ChannelOption builds a channel picker.
func ChannelOption(name, description string, required bool) CommandOption {
	return CommandOption{Type: OptionChannel, Name: name, Description: description, Required: required}
}

// RoleOption builds a role picker.
func RoleOption(name, description string, required bool) CommandOption {
	return CommandOption{Type: OptionRole, Name: name, Description: description, Required: required}
}

// Registration

// Commands lists an application's global commands.
func (c *Client) ApplicationCommands(ctx context.Context, appID Snowflake) ([]ApplicationCommand, error) {
	var out []ApplicationCommand
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/applications/" + appID.String() + "/commands",
		Route:  "GET /applications/{id}/commands",
	}, &out)
	return out, err
}

// CreateApplicationCommand registers one global command.
//
// Global commands can take up to an hour to appear. While developing, register
// against a single guild with CreateGuildCommand instead - those update
// instantly.
func (c *Client) CreateApplicationCommand(ctx context.Context, appID Snowflake, cmd ApplicationCommand) (*ApplicationCommand, error) {
	var out ApplicationCommand
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/applications/" + appID.String() + "/commands",
		Route:  "POST /applications/{id}/commands",
		Body:   cmd,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SetApplicationCommands replaces every global command in one call. Commands
// absent from the list are deleted.
func (c *Client) SetApplicationCommands(ctx context.Context, appID Snowflake, cmds []ApplicationCommand) ([]ApplicationCommand, error) {
	var out []ApplicationCommand
	err := c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/applications/" + appID.String() + "/commands",
		Route:  "PUT /applications/{id}/commands",
		Body:   cmds,
	}, &out)
	return out, err
}

// DeleteApplicationCommand removes one global command.
func (c *Client) DeleteApplicationCommand(ctx context.Context, appID, commandID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/applications/" + appID.String() + "/commands/" + commandID.String(),
		Route:  "DELETE /applications/{id}/commands/{id}",
	}, nil)
}

// GuildCommands lists an application's commands in one guild.
func (c *Client) GuildCommands(ctx context.Context, appID, guildID Snowflake) ([]ApplicationCommand, error) {
	var out []ApplicationCommand
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/applications/" + appID.String() + "/guilds/" + guildID.String() + "/commands",
		Route:  "GET /applications/{id}/guilds/{id}/commands",
	}, &out)
	return out, err
}

// CreateGuildCommand registers a command in one guild, where it appears
// immediately rather than after Discord's global propagation delay.
func (c *Client) CreateGuildCommand(ctx context.Context, appID, guildID Snowflake, cmd ApplicationCommand) (*ApplicationCommand, error) {
	var out ApplicationCommand
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/applications/" + appID.String() + "/guilds/" + guildID.String() + "/commands",
		Route:  "POST /applications/{id}/guilds/{id}/commands",
		Body:   cmd,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SetGuildCommands replaces every command in one guild.
func (c *Client) SetGuildCommands(ctx context.Context, appID, guildID Snowflake, cmds []ApplicationCommand) ([]ApplicationCommand, error) {
	var out []ApplicationCommand
	err := c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/applications/" + appID.String() + "/guilds/" + guildID.String() + "/commands",
		Route:  "PUT /applications/{id}/guilds/{id}/commands",
		Body:   cmds,
	}, &out)
	return out, err
}

// DeleteGuildCommand removes one guild command.
func (c *Client) DeleteGuildCommand(ctx context.Context, appID, guildID, commandID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path: "/applications/" + appID.String() + "/guilds/" + guildID.String() +
			"/commands/" + commandID.String(),
		Route: "DELETE /applications/{id}/guilds/{id}/commands/{id}",
	}, nil)
}
