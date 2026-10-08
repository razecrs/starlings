// Package voice connects Starlings bots to Discord voice channels.
//
// It uses voice gateway v8, negotiates Discord's AEAD transport encryption,
// and joins DAVE end-to-end encrypted calls automatically. It is a separate
// package so bots without voice never link the voice and DAVE dependencies:
//
//	conn, err := voice.Connect(ctx, bot, guildID, channelID)
//	if err != nil {
//		return err
//	}
//	defer conn.Close(ctx)
//	provider, err := conn.PlayFile(ctx, "song.mp3")
package voice

import (
	"context"
	"errors"
	"sync"

	"github.com/disgoorg/disgo/discord"
	discordgateway "github.com/disgoorg/disgo/gateway"
	discordvoice "github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
	"github.com/thomas-vilte/dave-go/session"

	"github.com/razecrs/starlings"
)

// ErrNotReady is returned when Connect runs before the bot has received
// READY, because Discord needs the bot's user ID for the voice handshake.
var ErrNotReady = errors.New("voice: the client has not received READY yet")

// OpusProvider supplies one 20 ms Opus frame at a time. Discord voice uses
// stereo Opus at 48 kHz. Return io.EOF when playback is done.
type OpusProvider interface {
	ProvideOpusFrame() ([]byte, error)
	Close()
}

// OpusPacket is one decoded Discord voice packet. Opus still contains encoded
// audio, so callers can store it directly or hand it to an Opus decoder.
type OpusPacket struct {
	UserID    starlings.Snowflake
	Sequence  uint16
	Timestamp uint32
	SSRC      uint32
	Opus      []byte
}

// Connection is a live Discord voice connection.
type Connection struct {
	mu      sync.RWMutex
	conn    discordvoice.Conn
	starlog *starlings.Starlog
	once    sync.Once
}

// Connect joins a voice channel and completes the separate voice gateway and
// UDP handshakes. The bot must be READY and needs Connect and Speak in the
// channel.
//
// Connect waits for gateway events, so call it outside a gateway event
// handler, or start a goroutine from the handler.
func Connect(ctx context.Context, c *starlings.Client, guildID, channelID starlings.Snowflake) (*Connection, error) {
	if c == nil {
		return nil, errors.New("voice: nil client")
	}
	if guildID.IsZero() || channelID.IsZero() {
		return nil, errors.New("voice: a connection needs a guild and channel ID")
	}
	m, err := managerFor(c)
	if err != nil {
		return nil, err
	}
	conn := m.manager.CreateConn(snowflake.ID(guildID))
	if err := conn.Open(ctx, snowflake.ID(channelID), false, false); err != nil {
		m.manager.RemoveConn(snowflake.ID(guildID))
		return nil, err
	}
	return &Connection{conn: conn, starlog: c.Starlog()}, nil
}

// Play starts reading Opus frames from provider. Calling it again stops and
// closes the previous provider.
func (v *Connection) Play(provider OpusProvider) error {
	if v == nil {
		return errors.New("voice: connection is closed")
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.conn == nil {
		return errors.New("voice: connection is closed")
	}
	if provider == nil {
		return errors.New("voice: nil Opus provider")
	}
	v.conn.SetOpusFrameProvider(provider)
	if source, ok := provider.(starlings.StarlogPlaybackSource); ok && v.starlog != nil {
		v.starlog.FollowPlayback(source)
	}
	return nil
}

// Receive starts delivering other users' Opus packets. The callback runs on
// the voice receive loop, so hand expensive work to another goroutine.
func (v *Connection) Receive(callback func(*OpusPacket) error) error {
	if v == nil {
		return errors.New("voice: connection is closed")
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.conn == nil {
		return errors.New("voice: connection is closed")
	}
	if callback == nil {
		return errors.New("voice: nil receive callback")
	}
	v.conn.SetOpusFrameReceiver(opusReceiver{callback: callback})
	return nil
}

// Close stops playback and leaves the voice channel. It is safe to call more
// than once.
func (v *Connection) Close(ctx context.Context) {
	if v == nil {
		return
	}
	v.once.Do(func() {
		v.mu.Lock()
		defer v.mu.Unlock()
		if v.conn != nil {
			v.conn.Close(ctx)
			v.conn = nil
		}
	})
}

// manager is stored as a client extension: one per client, shared by every
// shard, and closed when the client closes.
type manager struct {
	manager discordvoice.Manager
	once    sync.Once
}

func (m *manager) Close(ctx context.Context) {
	m.once.Do(func() { m.manager.Close(ctx) })
}

type extensionKey struct{}

func managerFor(c *starlings.Client) (*manager, error) {
	v, err := c.Extension(extensionKey{}, func() (any, error) {
		self := c.Self()
		if self == nil {
			return nil, ErrNotReady
		}
		m := &manager{}
		m.manager = discordvoice.NewManager(
			func(ctx context.Context, guildID snowflake.ID, channelID *snowflake.ID, mute, deaf bool) error {
				var channel starlings.Snowflake
				if channelID != nil {
					channel = starlings.Snowflake(*channelID)
				}
				return c.JoinVoice(ctx, starlings.Snowflake(guildID), channel, mute, deaf)
			},
			snowflake.ID(self.ID),
			discordvoice.WithLogger(c.Logger()),
			discordvoice.WithDaveSessionCreateFunc(session.CreateFunc()),
			discordvoice.WithConnConfigOpts(
				discordvoice.WithConnAudioSenderCreateFunc(newAudioSender),
			),
		)
		// The voice handshake depends on seeing state and server updates in
		// gateway order, so these run before application handlers even when
		// events are dispatched asynchronously.
		starlings.OnOrdered(c, func(update *starlings.VoiceStateUpdate) {
			m.manager.HandleVoiceStateUpdate(toVoiceStateUpdate(update))
		})
		starlings.OnOrdered(c, func(update *starlings.VoiceServerUpdate) {
			m.manager.HandleVoiceServerUpdate(toVoiceServerUpdate(update))
		})
		return m, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*manager), nil
}

type opusReceiver struct {
	callback func(*OpusPacket) error
}

func (r opusReceiver) ReceiveOpusFrame(userID snowflake.ID, packet *discordvoice.Packet) error {
	return r.callback(&OpusPacket{
		UserID:    starlings.Snowflake(userID),
		Sequence:  packet.Sequence,
		Timestamp: packet.Timestamp,
		SSRC:      packet.SSRC,
		Opus:      append([]byte(nil), packet.Opus...),
	})
}

func (opusReceiver) CleanupUser(snowflake.ID) {}
func (opusReceiver) Close()                   {}

func toVoiceStateUpdate(update *starlings.VoiceStateUpdate) discordgateway.EventVoiceStateUpdate {
	var channelID *snowflake.ID
	if !update.ChannelID.IsZero() {
		id := snowflake.ID(update.ChannelID)
		channelID = &id
	}
	return discordgateway.EventVoiceStateUpdate{VoiceState: discord.VoiceState{
		GuildID:    snowflake.ID(update.GuildID),
		ChannelID:  channelID,
		UserID:     snowflake.ID(update.UserID),
		SessionID:  update.SessionID,
		GuildDeaf:  update.Deaf,
		GuildMute:  update.Mute,
		SelfDeaf:   update.SelfDeaf,
		SelfMute:   update.SelfMute,
		SelfStream: update.SelfStream,
		SelfVideo:  update.SelfVideo,
		Suppress:   update.Suppress,
	}}
}

func toVoiceServerUpdate(update *starlings.VoiceServerUpdate) discordgateway.EventVoiceServerUpdate {
	var endpoint *string
	if update.Endpoint != "" {
		value := update.Endpoint
		endpoint = &value
	}
	return discordgateway.EventVoiceServerUpdate{
		Token:    update.Token,
		GuildID:  snowflake.ID(update.GuildID),
		Endpoint: endpoint,
	}
}
