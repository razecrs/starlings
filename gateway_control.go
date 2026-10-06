package starlings

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// HandleGatewayFrame runs one already-decoded websocket frame through
// Starlings' normal dispatcher, including raw handlers and internal state
// consumers. It is intended for deterministic replay, testing, and advanced
// transports. Ordinary bots should use Run instead.
//
// Dispatch frames are safe to replay offline. Control opcodes such as heartbeat
// and reconnect retain their live gateway behavior and should not be injected
// without an attached transport.
func (c *Client) HandleGatewayFrame(ctx context.Context, frame []byte) error {
	if c == nil {
		return fmt.Errorf("starlings: nil client")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(frame) == 0 {
		return fmt.Errorf("starlings: empty gateway frame")
	}
	var replay gateway
	return replay.handleFrame(ctx, c, frame)
}

// SendGateway writes an arbitrary opcode and payload to every active shard.
// It is the low-level escape hatch used before Starlings has a typed helper.
func (c *Client) SendGateway(ctx context.Context, op Opcode, data any) error {
	return sendAll(ctx, c.gateways(), op, data)
}

// SendGuildGateway sends an arbitrary payload only through the shard that
// owns guildID.
func (c *Client) SendGuildGateway(ctx context.Context, guildID Snowflake, op Opcode, data any) error {
	g, err := c.gatewayForGuild(guildID)
	if err != nil {
		return err
	}
	return g.send(ctx, op, data)
}

// SendGatewayRaw writes an already encoded gateway frame to every shard.
func (c *Client) SendGatewayRaw(ctx context.Context, frame []byte) error {
	var errs []error
	for _, g := range c.gateways() {
		if err := g.sendRaw(ctx, frame); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// SendGuildGatewayRaw writes an encoded frame to the guild's owning shard.
func (c *Client) SendGuildGatewayRaw(ctx context.Context, guildID Snowflake, frame []byte) error {
	g, err := c.gatewayForGuild(guildID)
	if err != nil {
		return err
	}
	return g.sendRaw(ctx, frame)
}

// Presence is the status shown for the bot. Since is a Unix timestamp in
// milliseconds and only matters for an idle status.
type Presence struct {
	Since      *int64     `json:"since"`
	Activities []Activity `json:"activities"`
	Status     Status     `json:"status"`
	AFK        bool       `json:"afk"`
}

// UpdatePresence changes the bot's status without reconnecting.
func (c *Client) UpdatePresence(ctx context.Context, p Presence) error {
	if p.Activities == nil {
		// Discord's own clients always send an array, and a null activities
		// field makes them ignore the update.
		p.Activities = []Activity{}
	}
	return sendAll(ctx, c.gateways(), OpPresenceUpdate, p)
}

// SetStatus is the one-line presence toggle: it sets the status and clears
// the activity line. For "Playing ..." text, use UpdatePresence directly.
func (c *Client) SetStatus(ctx context.Context, status Status) error {
	return c.UpdatePresence(ctx, Presence{Status: status, Since: sinceFor(status)})
}

// SetActivity sets the status and the activity line in one call, e.g.
//
//	bot.SetActivity(ctx, starlings.StatusOnline, starlings.Playing("with starlings"))
func (c *Client) SetActivity(ctx context.Context, status Status, activity Activity) error {
	return c.UpdatePresence(ctx, Presence{Status: status, Since: sinceFor(status), Activities: []Activity{activity}})
}

// sinceFor is the presence "since" field the way Discord's own clients send
// it: the moment the idle state started, or zero for the other statuses.
func sinceFor(status Status) *int64 {
	since := int64(0)
	if status == StatusIdle {
		since = time.Now().UnixMilli()
	}
	return &since
}

// MemberRequest selects members to fetch through the gateway. Set Query to
// "" and Limit to 0 for the full member list, or set UserIDs for exact users.
type MemberRequest struct {
	GuildID   Snowflake   `json:"guild_id"`
	Query     *string     `json:"query,omitzero"`
	Limit     int         `json:"limit,omitzero"`
	Presences bool        `json:"presences,omitzero"`
	UserIDs   []Snowflake `json:"user_ids,omitzero"`
	Nonce     string      `json:"nonce,omitzero"`
}

// RequestMembers asks Discord to send GuildMembersChunk events. The
// GuildMembers intent is needed for a full member list.
func (c *Client) RequestMembers(ctx context.Context, req MemberRequest) error {
	if req.GuildID.IsZero() {
		return errors.New("starlings: member request needs a guild ID")
	}
	if req.Query != nil && len(req.UserIDs) > 0 {
		return errors.New("starlings: member request cannot use query and user IDs together")
	}
	if req.Query == nil && len(req.UserIDs) == 0 {
		return errors.New("starlings: member request needs a query or user IDs")
	}
	if req.Limit < 0 || req.Limit > 1000 {
		return errors.New("starlings: member request limit must be between 0 and 1000")
	}
	if len(req.UserIDs) > 100 {
		return errors.New("starlings: member request accepts at most 100 user IDs")
	}
	if len(req.Nonce) > 32 {
		return errors.New("starlings: member request nonce is longer than 32 bytes")
	}
	gateway, err := c.gatewayForGuild(req.GuildID)
	if err != nil {
		return err
	}
	return gateway.send(ctx, OpRequestGuildMembers, req)
}

// RequestAllMembers is the common full-list request.
func (c *Client) RequestAllMembers(ctx context.Context, guildID Snowflake, nonce string) error {
	query := ""
	return c.RequestMembers(ctx, MemberRequest{
		GuildID: guildID,
		Query:   &query,
		Nonce:   nonce,
	})
}

// RequestSoundboardSounds asks Discord for a guild's soundboard sounds.
// Results arrive as SoundboardSounds events.
func (c *Client) RequestSoundboardSounds(ctx context.Context, guildIDs ...Snowflake) error {
	if len(guildIDs) == 0 {
		return errors.New("starlings: soundboard request needs at least one guild ID")
	}
	for _, id := range guildIDs {
		if id.IsZero() {
			return errors.New("starlings: soundboard request has an empty guild ID")
		}
	}
	groups := make(map[*gateway][]Snowflake)
	for _, guildID := range guildIDs {
		gateway, err := c.gatewayForGuild(guildID)
		if err != nil {
			return err
		}
		groups[gateway] = append(groups[gateway], guildID)
	}
	for gateway, ids := range groups {
		if err := gateway.send(ctx, OpRequestSoundboardSounds, struct {
			GuildIDs []Snowflake `json:"guild_ids"`
		}{GuildIDs: ids}); err != nil {
			return err
		}
	}
	return nil
}

// ChannelInfoField selects ephemeral fields for RequestChannelInfo.
type ChannelInfoField string

const (
	ChannelInfoStatus         ChannelInfoField = "status"
	ChannelInfoVoiceStartTime ChannelInfoField = "voice_start_time"
)

// RequestChannelInfo fetches ephemeral voice-channel data for a guild.
// Results arrive as ChannelInfo events.
func (c *Client) RequestChannelInfo(ctx context.Context, guildID Snowflake, fields ...ChannelInfoField) error {
	if guildID.IsZero() {
		return errors.New("starlings: channel info request needs a guild ID")
	}
	if len(fields) == 0 {
		return errors.New("starlings: channel info request needs at least one field")
	}
	for _, field := range fields {
		if field != ChannelInfoStatus && field != ChannelInfoVoiceStartTime {
			return errors.New("starlings: unknown channel info field " + string(field))
		}
	}
	gateway, err := c.gatewayForGuild(guildID)
	if err != nil {
		return err
	}
	return gateway.send(ctx, OpRequestChannelInfo, struct {
		GuildID Snowflake          `json:"guild_id"`
		Fields  []ChannelInfoField `json:"fields"`
	}{GuildID: guildID, Fields: fields})
}
