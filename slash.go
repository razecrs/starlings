package starlings

import (
	"context"
	"strings"
)

// SlashFunc handles one slash command invocation.
type SlashFunc func(i *InteractionCreate)

// slashEntry pairs a command's definition with its handler, so one call can
// both declare the command to Discord and say what it does.
type slashEntry struct {
	def ApplicationCommand
	fn  SlashFunc
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
func (c *Client) Slash(name, description string, fn SlashFunc, options ...CommandOption) {
	c.SlashCommand(ApplicationCommand{
		Type:        CommandChat,
		Name:        name,
		Description: description,
		Options:     options,
	}, fn)
}

// SlashCommand is Slash with a fully specified command, for the cases the
// shorthand does not cover - context-menu commands, permission gates,
// localisations.
func (c *Client) SlashCommand(cmd ApplicationCommand, fn SlashFunc) {
	c.slashMu.Lock()
	if c.slashes == nil {
		c.slashes = make(map[string]slashEntry)
	}
	c.slashes[strings.ToLower(cmd.Name)] = slashEntry{def: cmd, fn: fn}

	hook := !c.slashHooked
	c.slashHooked = true
	c.slashMu.Unlock()

	// However many commands are registered, they share one INTERACTION_CREATE
	// subscription.
	if hook {
		On(c, c.routeInteraction)
	}
}

// routeInteraction dispatches an interaction to the handler registered for its
// command name.
func (c *Client) routeInteraction(i *InteractionCreate) {
	switch i.Type {
	case InteractionApplicationCommand, InteractionCommandAutocomplete:
	default:
		return // components and modals are routed by custom ID, not name
	}

	c.slashMu.RLock()
	entry, ok := c.slashes[strings.ToLower(i.Data.Name)]
	c.slashMu.RUnlock()

	if ok {
		entry.fn(i)
	}
}

// SlashDefinitions returns the registered command definitions, in no
// particular order.
func (c *Client) SlashDefinitions() []ApplicationCommand {
	c.slashMu.RLock()
	defer c.slashMu.RUnlock()

	defs := make([]ApplicationCommand, 0, len(c.slashes))
	for _, e := range c.slashes {
		defs = append(defs, e.def)
	}
	return defs
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

	var err error
	if guildID.IsZero() {
		_, err = c.SetApplicationCommands(ctx, appID, defs)
	} else {
		_, err = c.SetGuildCommands(ctx, appID, guildID, defs)
	}
	return err
}
