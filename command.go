package starlings

import (
	"sort"
	"strings"
)

// DefaultPrefix is what Command matches when WithPrefix is not used.
const DefaultPrefix = "!"

// CommandFunc handles one prefix command. args holds the whitespace-separated
// words after the command name, so `!say hello there` arrives as
// args = ["hello", "there"].
type CommandFunc func(m *MessageCreate, args []string)

// Command registers a text command triggered by the bot's prefix:
//
//	bot.Command("ping", func(m *starlings.MessageCreate, args []string) {
//		m.Reply("pong")
//	})
//
// That responds to `!ping`. Change the prefix with WithPrefix.
//
// Messages from bots - including this one - are ignored, so a command that
// replies cannot trigger itself. Names are matched case-insensitively.
//
// Reading message text needs the privileged MessageContent intent unless the
// message mentions the bot or is a DM. Run warns when that intent is missing,
// because the symptom otherwise is a bot that silently ignores every command.
func (c *Client) Command(name string, fn CommandFunc) *CommandRoute {
	c.cmdMu.Lock()
	if c.cmds == nil {
		c.cmds = make(map[string]CommandFunc)
	}
	c.cmds[strings.ToLower(name)] = fn

	// Only the first Command call subscribes to MESSAGE_CREATE; later ones
	// just add to the table, so N commands still cost one handler.
	hook := !c.cmdHooked
	c.cmdHooked = true
	c.cmdMu.Unlock()

	if hook {
		On(c, c.runCommand)
	}
	return &CommandRoute{client: c, fn: fn}
}

// CommandRoute adds optional conveniences to a prefix command. Ignoring the
// value returned by Command keeps the one-line form.
type CommandRoute struct {
	client *Client
	fn     CommandFunc
}

// Aliases registers more names for the same handler.
func (r *CommandRoute) Aliases(names ...string) *CommandRoute {
	if r == nil || r.client == nil || r.fn == nil {
		return r
	}
	r.client.cmdMu.Lock()
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			r.client.cmds[name] = r.fn
		}
	}
	r.client.cmdMu.Unlock()
	return r
}

// Prefix returns the command prefix in use.
func (c *Client) Prefix() string {
	if c.prefix == "" {
		return DefaultPrefix
	}
	return c.prefix
}

// runCommand is the single MESSAGE_CREATE handler behind every registered
// command.
func (c *Client) runCommand(m *MessageCreate) {
	if m.IsFromBot() {
		return
	}

	rest, ok := strings.CutPrefix(m.Content, c.Prefix())
	if !ok {
		return
	}

	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return
	}

	c.cmdMu.RLock()
	fn := c.cmds[strings.ToLower(fields[0])]
	c.cmdMu.RUnlock()

	if fn != nil {
		fn(m, fields[1:])
	}
}

// Commands lists the registered command names, which is handy for building a
// help command.
func (c *Client) Commands() []string {
	c.cmdMu.RLock()
	defer c.cmdMu.RUnlock()

	names := make([]string, 0, len(c.cmds))
	for name := range c.cmds {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
