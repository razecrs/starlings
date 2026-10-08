package starlings

import (
	"bufio"
	"context"
	"encoding/json/v2"
	"errors"
	"os"
	"slices"
	"strings"
	"time"
)

// ErrNoToken is returned by Run when the client was created without a token
// and DISCORD_TOKEN is not set in the environment or a .env file.
var ErrNoToken = errors.New("starlings: no bot token: set DISCORD_TOKEN in the environment or in a .env file " +
	"(copy it from discord.com/developers/applications > your app > Bot)")

// WithAutoSync controls automatic command publishing. When it is on, the
// default, Run publishes the registered slash commands after connecting, but
// only when they differ from what Discord already has. Commands go to the
// guild in DISCORD_GUILD_ID when it is set, where changes appear at once, and
// to every guild otherwise. Turn it off to call SyncCommands yourself.
func WithAutoSync(enabled bool) Option {
	return func(c *Client) { c.autoSync = enabled }
}

// prepareRun fills in what a client created without full configuration
// still needs: the token, inferred intents, and command publishing.
func (c *Client) prepareRun() error {
	if c.token == "" || c.token == "Bot " {
		return ErrNoToken
	}
	if !c.intentsSet {
		c.intents = c.inferIntents()
		var privileged []string
		for _, p := range []struct {
			intent Intent
			name   string
		}{{IntentGuildMembers, "Server Members"}, {IntentGuildPresences, "Presence"}, {IntentMessageContent, "Message Content"}} {
			if c.intents.Has(p.intent) {
				privileged = append(privileged, p.name)
			}
		}
		if len(privileged) > 0 {
			c.log.Info("starlings: your handlers need privileged intents; enable them under Bot in the developer portal",
				"intents", strings.Join(privileged, ", "))
		}
	}
	if c.autoSync && c.syncGuild == nil && len(c.SlashDefinitions()) > 0 {
		var guild Snowflake
		if raw := os.Getenv("DISCORD_GUILD_ID"); raw != "" {
			id, err := ParseSnowflake(raw)
			if err != nil {
				return errors.New("starlings: DISCORD_GUILD_ID is not a valid ID")
			}
			guild = id
		}
		c.syncGuild = &guild
		c.installCommandSync()
	}
	return nil
}

// installCommandSync publishes commands after the first READY.
func (c *Client) installCommandSync() {
	guildID := *c.syncGuild
	internalOn(c, func(ready *Ready) {
		if ready.Client().ApplicationID().IsZero() {
			return
		}
		c.syncOnce.Do(func() {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := ready.Client().SyncCommands(ctx, guildID); err != nil {
					c.log.Error("starlings: publishing commands", "err", err)
				}
			}()
		})
	})
}

// inferIntents chooses intents from the registered handlers: Guilds always,
// the intents each handled event needs, and message content for prefix
// commands. Privileged intents chosen here must still be enabled in the
// developer portal; Discord refuses the connection otherwise, and Run
// returns a FatalError saying so.
func (c *Client) inferIntents() Intent {
	intents := IntentGuilds
	root := c.rootClient()
	for event, slot := range *root.slots.Load() {
		if len(slot.handlers) == slot.internal {
			continue
		}
		for _, intent := range eventIntents[event] {
			intents |= intent
		}
	}
	root.cmdMu.RLock()
	prefix := root.cmdHooked
	root.cmdMu.RUnlock()
	if prefix {
		intents |= IntentGuildMessages | IntentDirectMessages | IntentMessageContent
	}
	return intents
}

// loadDotEnv sets variables from a KEY=VALUE file without overriding ones
// already in the environment. A missing file is not an error.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"' || value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}

// commandsMatch reports whether Discord already has exactly these commands.
// Discord fills in defaults when it returns commands, so only the fields an
// application sets are compared.
func commandsMatch(local, remote []ApplicationCommand) bool {
	if len(local) != len(remote) {
		return false
	}
	key := func(cmd ApplicationCommand) string {
		kind := cmd.Type
		if kind == 0 {
			kind = CommandChat
		}
		return itoa(int(kind)) + "/" + cmd.Name
	}
	byKey := make(map[string]ApplicationCommand, len(remote))
	for _, cmd := range remote {
		byKey[key(cmd)] = cmd
	}
	for _, want := range local {
		got, ok := byKey[key(want)]
		if !ok || commandShape(want, want) != commandShape(got, want) {
			return false
		}
	}
	return true
}

// commandShape renders the comparable part of cmd. like is the local
// definition: optional fields it leaves unset are not compared.
func commandShape(cmd, like ApplicationCommand) string {
	shape := struct {
		Description string
		Options     []CommandOption
		Permissions *Permissions
		Contexts    []InteractionContextType
		Integration []ApplicationIntegrationType
		NSFW        bool
	}{
		Description: cmd.Description,
		Options:     normalizeOptions(cmd.Options),
		Permissions: cmd.DefaultMemberPermissions,
		NSFW:        cmd.NSFW,
	}
	if len(like.Contexts) > 0 {
		shape.Contexts = slices.Sorted(slices.Values(cmd.Contexts))
	}
	if len(like.IntegrationTypes) > 0 {
		shape.Integration = slices.Sorted(slices.Values(cmd.IntegrationTypes))
	}
	data, _ := json.Marshal(shape)
	return string(data)
}

func normalizeOptions(options []CommandOption) []CommandOption {
	out := make([]CommandOption, len(options))
	for n, o := range options {
		o.NameLocalized, o.DescriptionLocalized = "", ""
		o.Options = normalizeOptions(o.Options)
		if len(o.Options) == 0 {
			o.Options = nil
		}
		if len(o.Choices) == 0 {
			o.Choices = nil
		}
		for k := range o.Choices {
			o.Choices[k].NameLocalized = ""
		}
		out[n] = o
	}
	return out
}
