package starlings

import (
	"context"
	"net/http"
)

// SoundboardSound is a clip that can be played into a voice channel.
//
// Playing one needs no voice connection: Discord mixes it server-side when you
// call SendSoundboardSound, so a bot can be a soundboard without implementing
// any of the voice protocol.
type SoundboardSound struct {
	SoundID   Snowflake `json:"sound_id"`
	Name      string    `json:"name"`
	Volume    float64   `json:"volume"` // 0 to 1
	EmojiID   Snowflake `json:"emoji_id,omitzero"`
	EmojiName string    `json:"emoji_name,omitzero"`
	GuildID   Snowflake `json:"guild_id,omitzero"`
	Available bool      `json:"available"` // false when lost to a boost downgrade
	User      *User     `json:"user,omitzero"`
}

// Emoji renders the sound's icon for display, preferring a custom emoji and
// falling back to the unicode one. Returns "" when the sound has neither.
func (s *SoundboardSound) Emoji() string {
	switch {
	case !s.EmojiID.IsZero():
		return "<:" + s.EmojiName + ":" + s.EmojiID.String() + ">"
	case s.EmojiName != "":
		return s.EmojiName
	default:
		return ""
	}
}

// GuildSoundboardSounds lists a guild's soundboard sounds.
func (c *Client) GuildSoundboardSounds(ctx context.Context, guildID Snowflake) ([]SoundboardSound, error) {
	var out struct {
		Items []SoundboardSound `json:"items"`
	}
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/soundboard-sounds",
		Route:  "GET /guilds/" + guildID.String() + "/soundboard-sounds",
	}, &out)
	return out.Items, err
}

// GuildSoundboardSound fetches one sound.
func (c *Client) GuildSoundboardSound(ctx context.Context, guildID, soundID Snowflake) (*SoundboardSound, error) {
	var out SoundboardSound
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/soundboard-sounds/" + soundID.String(),
		Route:  "GET /guilds/" + guildID.String() + "/soundboard-sounds/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DefaultSoundboardSounds lists the sounds Discord ships, which every guild
// can play without uploading anything.
func (c *Client) DefaultSoundboardSounds(ctx context.Context) ([]SoundboardSound, error) {
	var out []SoundboardSound
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/soundboard-default-sounds",
		Route:  "GET /soundboard-default-sounds",
	}, &out)
	return out, err
}

// NewSoundboardSound is the payload for uploading a sound.
type NewSoundboardSound struct {
	Name string `json:"name"`
	// Sound is a data URI of an MP3 or OGG clip, at most 5.2 seconds long and
	// 512 KiB.
	Sound     string    `json:"sound"`
	Volume    *float64  `json:"volume,omitzero"`
	EmojiID   Snowflake `json:"emoji_id,omitzero"`
	EmojiName string    `json:"emoji_name,omitzero"`
}

// CreateSoundboardSound uploads a sound to a guild.
func (c *Client) CreateSoundboardSound(ctx context.Context, guildID Snowflake, s NewSoundboardSound, reason string) (*SoundboardSound, error) {
	var out SoundboardSound
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/guilds/" + guildID.String() + "/soundboard-sounds",
		Route:  "POST /guilds/" + guildID.String() + "/soundboard-sounds",
		Body:   s,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifySoundboardSound edits a sound's name, volume or emoji.
func (c *Client) ModifySoundboardSound(ctx context.Context, guildID, soundID Snowflake, s NewSoundboardSound, reason string) (*SoundboardSound, error) {
	var out SoundboardSound
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/soundboard-sounds/" + soundID.String(),
		Route:  "PATCH /guilds/" + guildID.String() + "/soundboard-sounds/{id}",
		Body:   s,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteSoundboardSound removes a sound from a guild.
func (c *Client) DeleteSoundboardSound(ctx context.Context, guildID, soundID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/guilds/" + guildID.String() + "/soundboard-sounds/" + soundID.String(),
		Route:  "DELETE /guilds/" + guildID.String() + "/soundboard-sounds/{id}",
		Reason: reason,
	}, nil)
}

// SendSoundboardSound plays a sound into a voice channel.
//
// The bot must be connected to that voice channel - but "connected" here means
// a gateway voice-state update, not an actual voice websocket, so this works
// without implementing the voice protocol.
//
// sourceGuildID is required only when playing a sound that belongs to a
// different guild than the channel; pass zero otherwise.
func (c *Client) SendSoundboardSound(ctx context.Context, channelID, soundID, sourceGuildID Snowflake) error {
	body := map[string]any{"sound_id": soundID}
	if !sourceGuildID.IsZero() {
		body["source_guild_id"] = sourceGuildID
	}
	return c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/channels/" + channelID.String() + "/send-soundboard-sound",
		Route:  "POST /channels/" + channelID.String() + "/send-soundboard-sound",
		Body:   body,
	}, nil)
}

// JoinVoice puts the bot into a voice channel, or moves it if it is already in
// one. Passing a zero channelID disconnects it.
//
// This is the gateway's Voice State Update (opcode 4). It is all that is
// needed to play soundboard sounds; a full voice connection is only required
// to stream audio yourself.
func (c *Client) JoinVoice(ctx context.Context, guildID, channelID Snowflake, mute, deaf bool) error {
	var ch *Snowflake
	if !channelID.IsZero() {
		ch = &channelID
	}
	gateway, err := c.gatewayForGuild(guildID)
	if err != nil {
		return err
	}
	return gateway.send(ctx, OpVoiceStateUpdate, voiceStateUpdate{
		GuildID:   guildID,
		ChannelID: ch,
		SelfMute:  mute,
		SelfDeaf:  deaf,
	})
}

// LeaveVoice disconnects the bot from voice in a guild.
func (c *Client) LeaveVoice(ctx context.Context, guildID Snowflake) error {
	return c.JoinVoice(ctx, guildID, 0, false, false)
}

// voiceStateUpdate is the opcode 4 payload. ChannelID is a pointer so that
// disconnecting can send an explicit null.
type voiceStateUpdate struct {
	GuildID   Snowflake  `json:"guild_id"`
	ChannelID *Snowflake `json:"channel_id"`
	SelfMute  bool       `json:"self_mute"`
	SelfDeaf  bool       `json:"self_deaf"`
}
