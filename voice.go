package starlings

import (
	"context"
	"errors"
	"sync"

	"github.com/disgoorg/disgo/discord"
	discordgateway "github.com/disgoorg/disgo/gateway"
	discordvoice "github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
	"github.com/thomas-vilte/dave-go/session"
)

// OpusProvider supplies one 20 ms Opus frame at a time. Discord voice uses
// stereo Opus at 48 kHz. Return io.EOF when playback is done.
type OpusProvider interface {
	ProvideOpusFrame() ([]byte, error)
	Close()
}

// OpusPacket is one decoded Discord voice packet. Opus still contains encoded
// audio, so callers can store it directly or hand it to an Opus decoder.
type OpusPacket struct {
	UserID    Snowflake
	Sequence  uint16
	Timestamp uint32
	SSRC      uint32
	Opus      []byte
}

// VoiceConnection is a live Discord voice connection. It uses voice gateway
// v8, negotiates Discord's modern AEAD transport encryption, and participates
// in DAVE end-to-end encryption automatically.
type VoiceConnection struct {
	mu      sync.RWMutex
	conn    discordvoice.Conn
	starlog *Starlog
	once    sync.Once
}

// ConnectVoice joins a voice channel and completes the separate voice gateway
// and UDP handshakes. The bot must be READY and needs Connect and Speak in the
// channel.
//
// ConnectVoice waits for gateway events, so call it outside a gateway event
// handler (or launch a goroutine from the handler).
func (c *Client) ConnectVoice(ctx context.Context, guildID, channelID Snowflake) (*VoiceConnection, error) {
	if guildID.IsZero() || channelID.IsZero() {
		return nil, errors.New("starlings: voice connection needs a guild and channel ID")
	}

	manager, err := c.voiceManager()
	if err != nil {
		return nil, err
	}
	conn := manager.manager.CreateConn(snowflake.ID(guildID))
	if err := conn.Open(ctx, snowflake.ID(channelID), false, false); err != nil {
		manager.manager.RemoveConn(snowflake.ID(guildID))
		return nil, err
	}
	return &VoiceConnection{conn: conn, starlog: c.rootClient().starlog}, nil
}

// Play starts reading Opus frames from provider. Calling it again stops and
// closes the previous provider.
func (v *VoiceConnection) Play(provider OpusProvider) error {
	if v == nil {
		return errors.New("starlings: voice connection is closed")
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.conn == nil {
		return errors.New("starlings: voice connection is closed")
	}
	if provider == nil {
		return errors.New("starlings: nil Opus provider")
	}
	v.conn.SetOpusFrameProvider(provider)
	if source, ok := provider.(StarlogPlaybackSource); ok && v.starlog != nil {
		v.starlog.FollowPlayback(source)
	}
	return nil
}

// Receive starts delivering other users' Opus packets. The callback runs on
// the voice receive loop, so hand expensive work to another goroutine.
func (v *VoiceConnection) Receive(callback func(*OpusPacket) error) error {
	if v == nil {
		return errors.New("starlings: voice connection is closed")
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.conn == nil {
		return errors.New("starlings: voice connection is closed")
	}
	if callback == nil {
		return errors.New("starlings: nil voice receive callback")
	}
	v.conn.SetOpusFrameReceiver(opusReceiver{callback: callback})
	return nil
}

// Close stops playback and leaves the voice channel. It is safe to call more
// than once.
func (v *VoiceConnection) Close(ctx context.Context) {
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

type voiceManager struct {
	manager discordvoice.Manager
	once    sync.Once
}

type opusReceiver struct {
	callback func(*OpusPacket) error
}

func (r opusReceiver) ReceiveOpusFrame(userID snowflake.ID, packet *discordvoice.Packet) error {
	return r.callback(&OpusPacket{
		UserID:    Snowflake(userID),
		Sequence:  packet.Sequence,
		Timestamp: packet.Timestamp,
		SSRC:      packet.SSRC,
		Opus:      append([]byte(nil), packet.Opus...),
	})
}

func (opusReceiver) CleanupUser(snowflake.ID) {}
func (opusReceiver) Close()                   {}

func (c *Client) voiceManager() (*voiceManager, error) {
	c.voiceMu.Lock()
	defer c.voiceMu.Unlock()
	if c.voice != nil {
		return c.voice, nil
	}
	self := c.Self()
	if self == nil {
		return nil, errNotReady
	}

	vm := &voiceManager{}
	vm.manager = discordvoice.NewManager(
		func(ctx context.Context, guildID snowflake.ID, channelID *snowflake.ID, mute, deaf bool) error {
			var channel Snowflake
			if channelID != nil {
				channel = Snowflake(*channelID)
			}
			return c.JoinVoice(ctx, Snowflake(guildID), channel, mute, deaf)
		},
		snowflake.ID(self.ID),
		discordvoice.WithLogger(c.log),
		discordvoice.WithDaveSessionCreateFunc(session.CreateFunc()),
		discordvoice.WithConnConfigOpts(
			discordvoice.WithConnAudioSenderCreateFunc(newStarlingsAudioSender),
		),
	)

	internalOn(c, func(update *VoiceStateUpdate) {
		vm.manager.HandleVoiceStateUpdate(toVoiceStateUpdate(update))
	})
	internalOn(c, func(update *VoiceServerUpdate) {
		vm.manager.HandleVoiceServerUpdate(toVoiceServerUpdate(update))
	})
	c.voice = vm
	return vm, nil
}

func (v *voiceManager) close(ctx context.Context) {
	v.once.Do(func() { v.manager.Close(ctx) })
}

func toVoiceStateUpdate(update *VoiceStateUpdate) discordgateway.EventVoiceStateUpdate {
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

func toVoiceServerUpdate(update *VoiceServerUpdate) discordgateway.EventVoiceServerUpdate {
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
