package starlings

import (
	"context"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"
)

// SlashFunc handles one slash command invocation.
type SlashFunc func(i *InteractionCreate)

// slashEntry pairs a command's definition with its handler, so one call can
// both declare the command to Discord and say what it does.
type slashEntry struct {
	def          ApplicationCommand
	fn           SlashFunc
	autocomplete SlashFunc
	task         TaskFunc
	private      bool
	timeout      time.Duration
	noAutoDefer  bool
	require      Permissions
	botNeeds     Permissions
}

type commandKey struct {
	kind ApplicationCommandType
	name string
}

// SlashRoute is the optional fluent configuration returned by Slash and
// SlashCommand. Ignoring it is fine; keeping it makes common command policy
// readable without expanding back into a full ApplicationCommand literal.
type SlashRoute struct {
	client *Client
	key    commandKey
}

// Slash registers a slash command and its handler in one step:
//
//	bot.Slash("ping", "Check the bot is alive", func(i *starlings.InteractionCreate) {
//		i.Reply("pong")
//	})
//
// Add parameters with the option builders:
//
//	bot.Slash("say", "Repeat something", handler,
//		starlings.StringOption("text", "What to say", true))
//
// This only registers it in the program. Call SyncCommands once connected to
// publish the definitions to Discord, which is what makes them appear in the
// client.
//
// The handler can take any of the shapes described on Response: for example
// func() string, or func(*InteractionCreate, Args) (string, error), where the
// fields of Args become the command's options.
func (c *Client) Slash(name, description string, handler any, options ...CommandOption) *SlashRoute {
	def := ApplicationCommand{Type: CommandChat, Name: name, Description: description, Options: options}
	switch fn := handler.(type) {
	case nil:
		return c.SlashCommand(def, nil)
	case SlashFunc:
		return c.SlashCommand(def, fn)
	case func(*InteractionCreate):
		return c.SlashCommand(def, fn)
	}
	h := adaptHandler("/"+name, handler, argsFromOptions, description)
	if h.plan != nil {
		if len(options) > 0 {
			panic("starlings: /" + name + ": options come from the handler's struct; do not pass them as well")
		}
		def.Options = h.options
	}
	return c.SlashCommand(def, nil).Run(h.call)
}

// SlashCommand is Slash with a fully specified command, for the cases the
// shorthand does not cover - context-menu commands, permission gates,
// localisations.
func (c *Client) SlashCommand(cmd ApplicationCommand, fn SlashFunc) *SlashRoute {
	c = c.rootClient()
	if cmd.Type == 0 {
		cmd.Type = CommandChat
	}
	key := commandKey{cmd.Type, cmd.Name}
	if cmd.Type == CommandChat {
		key.name = strings.ToLower(cmd.Name)
	}
	c.slashMu.Lock()
	if c.slashes == nil {
		c.slashes = make(map[commandKey]slashEntry)
	}
	c.slashes[key] = slashEntry{def: cloneCommand(cmd), fn: fn}

	hook := !c.slashHooked
	c.slashHooked = true
	c.slashMu.Unlock()

	// However many commands are registered, they share one INTERACTION_CREATE
	// subscription.
	if hook {
		On(c, c.routeInteraction)
	}
	return &SlashRoute{client: c, key: key}
}

// Permissions limits the command to members with every supplied permission.
// Discord performs the visibility check before Starlings receives the command.
func (r *SlashRoute) Permissions(permissions Permissions) *SlashRoute {
	return r.update(func(entry *slashEntry) {
		value := permissions
		entry.def.DefaultMemberPermissions = &value
	})
}

// Contexts selects where Discord offers the command.
func (r *SlashRoute) Contexts(contexts ...InteractionContextType) *SlashRoute {
	return r.update(func(entry *slashEntry) {
		entry.def.Contexts = append([]InteractionContextType(nil), contexts...)
	})
}

// GuildOnly is shorthand for Contexts(InteractionContextGuild).
func (r *SlashRoute) GuildOnly() *SlashRoute {
	return r.Contexts(InteractionContextGuild)
}

// InstallTypes selects whether a command belongs to guild installs, user
// installs, or both.
func (r *SlashRoute) InstallTypes(types ...ApplicationIntegrationType) *SlashRoute {
	return r.update(func(entry *slashEntry) {
		entry.def.IntegrationTypes = append([]ApplicationIntegrationType(nil), types...)
	})
}

// Autocomplete registers the command's autocomplete handler separately from
// its execution handler. This separation prevents command side effects from
// running while a user is merely typing an option.
func (r *SlashRoute) Autocomplete(fn SlashFunc) *SlashRoute {
	return r.update(func(entry *slashEntry) { entry.autocomplete = fn })
}

func (r *SlashRoute) update(fn func(*slashEntry)) *SlashRoute {
	if r == nil || r.client == nil {
		return r
	}
	r.client.slashMu.Lock()
	entry, ok := r.client.slashes[r.key]
	if ok {
		fn(&entry)
		r.client.slashes[r.key] = entry
	}
	r.client.slashMu.Unlock()
	return r
}

// routeInteraction dispatches an interaction to the handler registered for its
// command name.
func (c *Client) routeInteraction(i *InteractionCreate) {
	c = c.rootClient()
	if i.Type == InteractionMessageComponent || i.Type == InteractionModalSubmit {
		if fn := c.componentRoute(i); fn != nil {
			kind := CallbackDeferredUpdateMessage
			if i.Type == InteractionModalSubmit && i.Message == nil {
				kind = CallbackDeferredChannelMessage // a modal opened from a command
			}
			i.armAutoDefer(c.autoDefer, kind, false)
			fn(i)
			i.stopAutoDefer()
		}
		return
	}
	if i.Type != InteractionApplicationCommand && i.Type != InteractionCommandAutocomplete {
		return // components and modals are routed by custom ID, not name
	}

	kind := ApplicationCommandType(i.Data.Type)
	if kind == 0 {
		kind = CommandChat
	}
	key := commandKey{kind, i.Data.Name}
	if kind == CommandChat {
		key.name = strings.ToLower(key.name)
	}
	c.slashMu.RLock()
	entry, ok := c.slashes[key]
	runner := c.tasks
	c.slashMu.RUnlock()

	if !ok {
		return
	}
	if i.Type == InteractionCommandAutocomplete {
		if entry.autocomplete != nil {
			entry.autocomplete(i)
		}
		return
	}
	if entry.task != nil {
		if !entry.checkPermissions(i) {
			return
		}
		c.runTask(runner, entry, i)
	} else if entry.fn != nil {
		if !entry.checkPermissions(i) {
			return
		}
		if !entry.noAutoDefer {
			i.armAutoDefer(c.autoDefer, CallbackDeferredChannelMessage, entry.private)
		}
		entry.fn(i)
		i.stopAutoDefer()
	}
}

// SlashDefinitions returns the registered command definitions, in no
// particular order.
func (c *Client) SlashDefinitions() []ApplicationCommand {
	c = c.rootClient()
	c.slashMu.RLock()
	defer c.slashMu.RUnlock()

	defs := make([]ApplicationCommand, 0, len(c.slashes))
	for _, e := range c.slashes {
		defs = append(defs, cloneCommand(e.def))
	}
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].Type != defs[j].Type {
			return defs[i].Type < defs[j].Type
		}
		return defs[i].Name < defs[j].Name
	})
	return defs
}

func cloneCommand(value ApplicationCommand) ApplicationCommand {
	value.NameLocalizations = maps.Clone(value.NameLocalizations)
	value.DescriptionLocalizations = maps.Clone(value.DescriptionLocalizations)
	value.Options = cloneCommandOptions(value.Options)
	value.Contexts = slices.Clone(value.Contexts)
	value.IntegrationTypes = slices.Clone(value.IntegrationTypes)
	value.DefaultMemberPermissions = cloneRef(value.DefaultMemberPermissions)
	value.DMPermission = cloneRef(value.DMPermission)
	value.DefaultPermission = cloneRef(value.DefaultPermission)
	return value
}

func cloneRef[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return Ref(*value)
}

func cloneCommandOptions(values []CommandOption) []CommandOption {
	out := slices.Clone(values)
	for n := range out {
		v := &out[n]
		v.NameLocalizations = maps.Clone(v.NameLocalizations)
		v.DescriptionLocalizations = maps.Clone(v.DescriptionLocalizations)
		v.Options = cloneCommandOptions(v.Options)
		v.ChannelTypes = slices.Clone(v.ChannelTypes)
		v.Choices = slices.Clone(v.Choices)
		for j := range v.Choices {
			v.Choices[j].NameLocalizations = maps.Clone(v.Choices[j].NameLocalizations)
		}
		v.MinValue, v.MaxValue = cloneRef(v.MinValue), cloneRef(v.MaxValue)
		v.MinLength, v.MaxLength = cloneRef(v.MinLength), cloneRef(v.MaxLength)
	}
	return out
}

// SyncCommands publishes every registered slash command to Discord, replacing
// whatever was there before.
//
// Pass a guild ID while developing: guild commands appear immediately, whereas
// global ones (guildID zero) can take up to an hour to propagate.
//
// The usual place to call this is a Ready handler, because it needs the
// application ID that READY provides:
//
//	bot.On(func(r *starlings.Ready) {
//		if err := bot.SyncCommands(context.Background(), guildID); err != nil {
//			log.Printf("syncing commands: %v", err)
//		}
//	})
func (c *Client) SyncCommands(ctx context.Context, guildID Snowflake) error {
	appID := c.ApplicationID()
	if appID.IsZero() {
		return errNotReady
	}

	defs := c.SlashDefinitions()
	if len(defs) == 0 {
		return nil
	}

	// Publishing replaces every command at once and is rate limited, so it
	// is skipped when Discord already has these exact definitions.
	var current []ApplicationCommand
	var err error
	if guildID.IsZero() {
		current, err = c.ApplicationCommands(ctx, appID)
	} else {
		current, err = c.GuildCommands(ctx, appID, guildID)
	}
	if err == nil && commandsMatch(defs, current) {
		c.log.Debug("starlings: commands already up to date", "count", len(defs))
		return nil
	}
	if guildID.IsZero() {
		_, err = c.SetApplicationCommands(ctx, appID, defs)
	} else {
		_, err = c.SetGuildCommands(ctx, appID, guildID, defs)
	}
	if err == nil {
		scope := "every guild"
		if !guildID.IsZero() {
			scope = "guild " + guildID.String()
		}
		c.log.Info("starlings: published commands", "count", len(defs), "to", scope)
	}
	return err
}
