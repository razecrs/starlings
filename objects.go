package starlings

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Values that Starlings hands out - from events, State, and REST calls - know
// which client they came from, so they can act on themselves:
//
//	member.Timeout(10*time.Minute, "spam")
//	msg.Reply("done")
//	channel.Send("hello")
//
// Each method is a thin call to the matching Client method, which remains
// available for full control. A value built by hand has no client; its
// methods return ErrUnbound.

// ErrUnbound is returned by a resource method when the value was not
// produced by a client, such as a struct literal. Use the Client method.
var ErrUnbound = errors.New("starlings: this value did not come from a client; use the Client method instead")

// bound links a resource to its client. It is copied with the value.
type bound struct {
	c       *Client
	ctx     context.Context
	guildID Snowflake
}

func (b *bound) set(c *Client, guildID Snowflake) {
	b.c = c
	if guildID != 0 {
		b.guildID = guildID
	}
}

// use returns the client and the context for one call.
func (b bound) use() (*Client, context.Context, error) {
	if b.c == nil {
		return nil, nil, ErrUnbound
	}
	if b.ctx != nil {
		return b.c, b.ctx, nil
	}
	return b.c, context.Background(), nil
}

// Sendable is anything a message can be sent to: a channel, a thread, or a
// user or member (by direct message).
type Sendable interface {
	Send(content string) (*Message, error)
	SendComplex(data SendData) (*Message, error)
}

// Mentionable is anything that can be mentioned in message text.
type Mentionable interface {
	Mention() string
}

var (
	_ Sendable    = (*Channel)(nil)
	_ Sendable    = (*User)(nil)
	_ Sendable    = (*Member)(nil)
	_ Mentionable = (*User)(nil)
	_ Mentionable = (*Member)(nil)
	_ Mentionable = (*Role)(nil)
	_ Mentionable = (*Channel)(nil)
)

// User

func (u *User) bindTo(c *Client) {
	if u != nil {
		u.ref.set(c, 0)
	}
}

// WithContext returns a copy of u whose methods use ctx.
func (u *User) WithContext(ctx context.Context) *User {
	out := *u
	out.ref.ctx = ctx
	return &out
}

// Send sends a direct message. The DM channel is looked up once and reused.
func (u *User) Send(content string) (*Message, error) {
	return u.SendComplex(SendData{Content: content})
}

// SendComplex sends a direct message with embeds, components, or other
// fields.
func (u *User) SendComplex(data SendData) (*Message, error) {
	c, ctx, err := u.ref.use()
	if err != nil {
		return nil, err
	}
	channelID, err := c.dmChannel(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return c.SendComplex(ctx, channelID, data)
}

// Member

func (m *Member) bindTo(c *Client, guildID Snowflake) {
	if m == nil {
		return
	}
	m.ref.set(c, guildID)
	m.User.bindTo(c)
}

// GuildID returns the guild the member belongs to, when Starlings knows it.
func (m *Member) GuildID() Snowflake { return m.ref.guildID }

// WithContext returns a copy of m whose methods use ctx.
func (m *Member) WithContext(ctx context.Context) *Member {
	out := *m
	out.ref.ctx = ctx
	return &out
}

// Mention returns the member's mention text.
func (m *Member) Mention() string {
	if m.User == nil {
		return ""
	}
	return m.User.Mention()
}

func (m *Member) target() (*Client, context.Context, Snowflake, Snowflake, error) {
	c, ctx, err := m.ref.use()
	if err != nil {
		return nil, nil, 0, 0, err
	}
	if m.User == nil || m.ref.guildID == 0 {
		return nil, nil, 0, 0, ErrUnbound
	}
	return c, ctx, m.ref.guildID, m.User.ID, nil
}

// Ban bans the member. deleteHistory removes their messages from that far
// back, up to seven days; zero keeps them.
func (m *Member) Ban(reason string, deleteHistory time.Duration) error {
	c, ctx, guildID, userID, err := m.target()
	if err != nil {
		return err
	}
	return c.CreateBan(ctx, guildID, userID, int(deleteHistory/time.Second), reason)
}

// Kick removes the member from the guild.
func (m *Member) Kick(reason string) error {
	c, ctx, guildID, userID, err := m.target()
	if err != nil {
		return err
	}
	return c.Kick(ctx, guildID, userID, reason)
}

// Timeout stops the member from talking for d, up to MaxTimeout.
func (m *Member) Timeout(d time.Duration, reason string) error {
	c, ctx, guildID, userID, err := m.target()
	if err != nil {
		return err
	}
	if d <= 0 {
		_, err = c.ClearTimeout(ctx, guildID, userID, reason)
		return err
	}
	_, err = c.Timeout(ctx, guildID, userID, time.Now().Add(d), reason)
	return err
}

// ClearTimeout lifts the member's timeout.
func (m *Member) ClearTimeout(reason string) error { return m.Timeout(0, reason) }

// AddRole gives the member a role.
func (m *Member) AddRole(roleID Snowflake, reason string) error {
	c, ctx, guildID, userID, err := m.target()
	if err != nil {
		return err
	}
	return c.AddMemberRole(ctx, guildID, userID, roleID, reason)
}

// RemoveRole takes a role from the member.
func (m *Member) RemoveRole(roleID Snowflake, reason string) error {
	c, ctx, guildID, userID, err := m.target()
	if err != nil {
		return err
	}
	return c.RemoveMemberRole(ctx, guildID, userID, roleID, reason)
}

// SetNick changes the member's nickname. An empty nick removes it.
func (m *Member) SetNick(nick, reason string) error {
	c, ctx, guildID, userID, err := m.target()
	if err != nil {
		return err
	}
	_, err = c.ModifyMember(ctx, guildID, userID, ModifyMember{Nick: &nick}, reason)
	return err
}

// Send sends the member a direct message.
func (m *Member) Send(content string) (*Message, error) {
	return m.SendComplex(SendData{Content: content})
}

// SendComplex sends the member a direct message with embeds or components.
func (m *Member) SendComplex(data SendData) (*Message, error) {
	if m.User == nil {
		return nil, ErrUnbound
	}
	u := *m.User
	u.ref = m.ref
	return u.SendComplex(data)
}

// CanModerate reports whether this member may moderate target, using the
// cached role hierarchy. See State.CanModerate.
func (m *Member) CanModerate(target *Member) error {
	c, _, guildID, userID, err := m.target()
	if err != nil {
		return err
	}
	if target == nil || target.User == nil {
		return ErrMemberNotCached
	}
	if c.State == nil {
		return ErrGuildNotCached
	}
	return c.State.CanModerate(guildID, userID, target.User.ID)
}

// Channel

func (ch *Channel) bindTo(c *Client) {
	if ch != nil {
		ch.ref.set(c, ch.GuildID)
	}
}

// WithContext returns a copy of ch whose methods use ctx.
func (ch *Channel) WithContext(ctx context.Context) *Channel {
	out := *ch
	out.ref.ctx = ctx
	return &out
}

// Send posts a message in the channel.
func (ch *Channel) Send(content string) (*Message, error) {
	return ch.SendComplex(SendData{Content: content})
}

// SendComplex posts a message with embeds, components, or other fields.
func (ch *Channel) SendComplex(data SendData) (*Message, error) {
	c, ctx, err := ch.ref.use()
	if err != nil {
		return nil, err
	}
	return c.SendComplex(ctx, ch.ID, data)
}

// Typing shows the typing indicator for about ten seconds.
func (ch *Channel) Typing() error {
	c, ctx, err := ch.ref.use()
	if err != nil {
		return err
	}
	return c.Typing(ctx, ch.ID)
}

// Delete deletes the channel.
func (ch *Channel) Delete(reason string) error {
	c, ctx, err := ch.ref.use()
	if err != nil {
		return err
	}
	_, err = c.DeleteChannel(ctx, ch.ID, reason)
	return err
}

// Message

func (msg *Message) bindTo(c *Client) {
	if msg == nil {
		return
	}
	msg.ref.set(c, msg.GuildID)
	msg.Author.bindTo(c)
	msg.Member.bindTo(c, msg.GuildID)
	if msg.Member != nil && msg.Member.User == nil && msg.Author != nil {
		// Discord leaves the user out of a message's member object.
		msg.Member.User = msg.Author
	}
}

// WithContext returns a copy of msg whose methods use ctx.
func (msg *Message) WithContext(ctx context.Context) *Message {
	out := *msg
	out.ref.ctx = ctx
	return &out
}

// Reply answers the message as a Discord reply.
func (msg *Message) Reply(content string) (*Message, error) {
	return msg.ReplyComplex(SendData{Content: content})
}

// ReplyComplex replies with embeds or components.
func (msg *Message) ReplyComplex(data SendData) (*Message, error) {
	c, ctx, err := msg.ref.use()
	if err != nil {
		return nil, err
	}
	if data.MessageRef == nil {
		data.MessageRef = &MessageRef{MessageID: msg.ID, ChannelID: msg.ChannelID}
	}
	return c.SendComplex(ctx, msg.ChannelID, data)
}

// Edit replaces the content of one of the bot's own messages.
func (msg *Message) Edit(content string) (*Message, error) {
	c, ctx, err := msg.ref.use()
	if err != nil {
		return nil, err
	}
	return c.EditMessage(ctx, msg.ChannelID, msg.ID, SendData{Content: content})
}

// Delete deletes the message.
func (msg *Message) Delete() error {
	c, ctx, err := msg.ref.use()
	if err != nil {
		return err
	}
	return c.DeleteMessage(ctx, msg.ChannelID, msg.ID, "")
}

// React adds the bot's reaction. emoji is a Unicode emoji or name:id for a
// custom one.
func (msg *Message) React(emoji string) error {
	c, ctx, err := msg.ref.use()
	if err != nil {
		return err
	}
	return c.React(ctx, msg.ChannelID, msg.ID, Emoji{Name: emoji})
}

// Unreact removes the bot's reaction.
func (msg *Message) Unreact(emoji string) error {
	c, ctx, err := msg.ref.use()
	if err != nil {
		return err
	}
	return c.Unreact(ctx, msg.ChannelID, msg.ID, Emoji{Name: emoji})
}

// Pin pins the message in its channel.
func (msg *Message) Pin(reason string) error {
	c, ctx, err := msg.ref.use()
	if err != nil {
		return err
	}
	return c.PinMessage(ctx, msg.ChannelID, msg.ID, reason)
}

// Unpin unpins the message.
func (msg *Message) Unpin(reason string) error {
	c, ctx, err := msg.ref.use()
	if err != nil {
		return err
	}
	return c.UnpinMessage(ctx, msg.ChannelID, msg.ID, reason)
}

// Link returns the message's jump URL.
func (msg *Message) Link() string {
	guild := "@me"
	if msg.GuildID != 0 {
		guild = msg.GuildID.String()
	}
	return "https://discord.com/channels/" + guild + "/" + msg.ChannelID.String() + "/" + msg.ID.String()
}

// Role

func (r *Role) bindTo(c *Client, guildID Snowflake) {
	if r != nil {
		r.ref.set(c, guildID)
	}
}

// WithContext returns a copy of r whose methods use ctx.
func (r *Role) WithContext(ctx context.Context) *Role {
	out := *r
	out.ref.ctx = ctx
	return &out
}

// Delete deletes the role.
func (r *Role) Delete(reason string) error {
	c, ctx, err := r.ref.use()
	if err != nil {
		return err
	}
	if r.ref.guildID == 0 {
		return ErrUnbound
	}
	return c.DeleteRole(ctx, r.ref.guildID, r.ID, reason)
}

// Guild

func (g *Guild) bindTo(c *Client) {
	if g == nil {
		return
	}
	g.ref.set(c, g.ID)
	for n := range g.Members {
		g.Members[n].bindTo(c, g.ID)
	}
	for n := range g.Channels {
		g.Channels[n].ref.set(c, g.ID)
	}
	for n := range g.Roles {
		g.Roles[n].bindTo(c, g.ID)
	}
}

// WithContext returns a copy of g whose methods use ctx.
func (g *Guild) WithContext(ctx context.Context) *Guild {
	out := *g
	out.ref.ctx = ctx
	return &out
}

// Member returns a member from the cache, or from Discord when it is not
// cached.
func (g *Guild) Member(userID Snowflake) (*Member, error) {
	c, ctx, err := g.ref.use()
	if err != nil {
		return nil, err
	}
	return c.FetchMember(ctx, g.ID, userID)
}

// Ban bans a user by ID, whether or not they are a member.
func (g *Guild) Ban(userID Snowflake, reason string, deleteHistory time.Duration) error {
	c, ctx, err := g.ref.use()
	if err != nil {
		return err
	}
	return c.CreateBan(ctx, g.ID, userID, int(deleteHistory/time.Second), reason)
}

// Unban lifts a ban.
func (g *Guild) Unban(userID Snowflake, reason string) error {
	c, ctx, err := g.ref.use()
	if err != nil {
		return err
	}
	return c.RemoveBan(ctx, g.ID, userID, reason)
}

// FetchMember returns a member from State, or from Discord when it is not
// cached. Use State.Member when a network request is not acceptable.
func (c *Client) FetchMember(ctx context.Context, guildID, userID Snowflake) (*Member, error) {
	if c.State != nil {
		if m, ok := c.State.Member(guildID, userID); ok {
			return &m, nil
		}
	}
	return c.GuildMember(ctx, guildID, userID)
}

// FetchChannel returns a channel from State, or from Discord when it is not
// cached.
func (c *Client) FetchChannel(ctx context.Context, channelID Snowflake) (*Channel, error) {
	if c.State != nil {
		if ch, ok := c.State.Channel(channelID); ok {
			return &ch, nil
		}
	}
	return c.Channel(ctx, channelID)
}

// FetchGuild returns a guild from State, or from Discord when it is not
// cached.
func (c *Client) FetchGuild(ctx context.Context, guildID Snowflake) (*Guild, error) {
	if c.State != nil {
		if g, ok := c.State.Guild(guildID); ok {
			return &g, nil
		}
	}
	return c.Guild(ctx, guildID)
}

// dmChannel returns the DM channel for a user, creating it once.
func (c *Client) dmChannel(ctx context.Context, userID Snowflake) (Snowflake, error) {
	root := c.rootClient()
	if id, ok := root.dms.Load(userID); ok {
		return id.(Snowflake), nil
	}
	ch, err := c.CreateDM(ctx, userID)
	if err != nil {
		return 0, err
	}
	root.dms.Store(userID, ch.ID)
	return ch.ID, nil
}

// Events that carry resources bind them too, so their methods work.

func (e *MessageCreate) bind(c *Client) { e.eventBase.bind(c); e.Message.bindTo(c) }
func (e *MessageUpdate) bind(c *Client) { e.eventBase.bind(c); e.Message.bindTo(c) }
func (e *GuildCreate) bind(c *Client)   { e.eventBase.bind(c); e.Guild.bindTo(c) }
func (e *GuildUpdate) bind(c *Client)   { e.eventBase.bind(c); e.Guild.bindTo(c) }
func (e *ChannelCreate) bind(c *Client) { e.eventBase.bind(c); e.Channel.bindTo(c) }
func (e *ChannelUpdate) bind(c *Client) { e.eventBase.bind(c); e.Channel.bindTo(c) }
func (e *ChannelDelete) bind(c *Client) { e.eventBase.bind(c); e.Channel.bindTo(c) }
func (e *ThreadCreate) bind(c *Client)  { e.eventBase.bind(c); e.Channel.bindTo(c) }
func (e *ThreadUpdate) bind(c *Client)  { e.eventBase.bind(c); e.Channel.bindTo(c) }
func (e *TypingStart) bind(c *Client)   { e.eventBase.bind(c); e.Member.bindTo(c, e.GuildID) }

func (e *GuildMemberAdd) bind(c *Client) {
	e.eventBase.bind(c)
	e.Member.bindTo(c, e.GuildID)
}

func (e *GuildMemberUpdate) bind(c *Client) {
	e.eventBase.bind(c)
	e.User.bindTo(c)
}

func (e *GuildMemberRemove) bind(c *Client) {
	e.eventBase.bind(c)
	e.User.bindTo(c)
}

func (e *GuildRoleCreate) bind(c *Client) { e.eventBase.bind(c); e.Role.bindTo(c, e.GuildID) }
func (e *GuildRoleUpdate) bind(c *Client) { e.eventBase.bind(c); e.Role.bindTo(c, e.GuildID) }

func (e *InteractionCreate) bind(c *Client) {
	e.eventBase.bind(c)
	e.Member.bindTo(c, e.GuildID)
	e.User.bindTo(c)
	e.Message.bindTo(c)
	if r := e.Data.Resolved; r != nil {
		for _, u := range r.Users {
			u.bindTo(c)
		}
		for _, m := range r.Members {
			m.bindTo(c, e.GuildID)
		}
		for _, role := range r.Roles {
			role.bindTo(c, e.GuildID)
		}
		for _, ch := range r.Channels {
			ch.bindTo(c)
		}
		for _, msg := range r.Messages {
			msg.bindTo(c)
		}
	}
}

// bindResult binds the resources a REST call decoded. guildID comes from the
// request path, since member and role objects do not carry it.
func bindResult(c *Client, out any, guildID Snowflake) {
	switch v := out.(type) {
	case *Message:
		v.bindTo(c)
	case *[]Message:
		for n := range *v {
			(*v)[n].bindTo(c)
		}
	case *Channel:
		v.bindTo(c)
	case *[]Channel:
		for n := range *v {
			(*v)[n].bindTo(c)
		}
	case *User:
		v.bindTo(c)
	case *[]User:
		for n := range *v {
			(*v)[n].bindTo(c)
		}
	case *Member:
		v.bindTo(c, guildID)
	case *[]Member:
		for n := range *v {
			(*v)[n].bindTo(c, guildID)
		}
	case *Role:
		v.bindTo(c, guildID)
	case *[]Role:
		for n := range *v {
			(*v)[n].bindTo(c, guildID)
		}
	case *Guild:
		v.bindTo(c)
	}
}

// guildFromPath returns the guild ID in a /guilds/{id}/... request path.
func guildFromPath(path string) Snowflake {
	rest, ok := strings.CutPrefix(path, "/guilds/")
	if !ok {
		return 0
	}
	if end := strings.IndexAny(rest, "/?"); end >= 0 {
		rest = rest[:end]
	}
	id, _ := ParseSnowflake(rest)
	return id
}
