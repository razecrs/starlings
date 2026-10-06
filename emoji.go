package starlings

import (
	"context"
	"net/http"
)

// GuildEmoji is a custom emoji uploaded to a guild. It extends the minimal
// Emoji sent on reactions with authorship and role restrictions.
type GuildEmoji struct {
	ID            Snowflake   `json:"id"`
	Name          string      `json:"name"`
	Roles         []Snowflake `json:"roles"` // if set, only these roles may use it
	User          *User       `json:"user"`  // who uploaded it
	RequireColons bool        `json:"require_colons"`
	Managed       bool        `json:"managed"`
	Animated      bool        `json:"animated"`
	Available     bool        `json:"available"` // false when lost to a boost downgrade
}

// GuildEmojis lists a guild's custom emoji.
func (c *Client) GuildEmojis(ctx context.Context, guildID Snowflake) ([]GuildEmoji, error) {
	var out []GuildEmoji
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/emojis",
		Route:  "GET /guilds/" + guildID.String() + "/emojis",
	}, &out)
	return out, err
}

// GuildEmoji fetches one custom emoji.
func (c *Client) GuildEmoji(ctx context.Context, guildID, emojiID Snowflake) (*GuildEmoji, error) {
	var out GuildEmoji
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/emojis/" + emojiID.String(),
		Route:  "GET /guilds/" + guildID.String() + "/emojis/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateGuildEmoji uploads a custom emoji.
//
// image is a data URI - "data:image/png;base64,..." - of at most 256 KiB.
func (c *Client) CreateGuildEmoji(ctx context.Context, guildID Snowflake, name, image string, roles []Snowflake, reason string) (*GuildEmoji, error) {
	var out GuildEmoji
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/guilds/" + guildID.String() + "/emojis",
		Route:  "POST /guilds/" + guildID.String() + "/emojis",
		Body: map[string]any{
			"name":  name,
			"image": image,
			"roles": roles,
		},
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyGuildEmoji renames an emoji or changes which roles may use it.
func (c *Client) ModifyGuildEmoji(ctx context.Context, guildID, emojiID Snowflake, name string, roles []Snowflake, reason string) (*GuildEmoji, error) {
	body := map[string]any{}
	if name != "" {
		body["name"] = name
	}
	if roles != nil {
		body["roles"] = roles
	}

	var out GuildEmoji
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/emojis/" + emojiID.String(),
		Route:  "PATCH /guilds/" + guildID.String() + "/emojis/{id}",
		Body:   body,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteGuildEmoji removes a custom emoji.
func (c *Client) DeleteGuildEmoji(ctx context.Context, guildID, emojiID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/guilds/" + guildID.String() + "/emojis/" + emojiID.String(),
		Route:  "DELETE /guilds/" + guildID.String() + "/emojis/{id}",
		Reason: reason,
	}, nil)
}

// Application-owned emoji

// ApplicationEmojis lists emoji owned by the application rather than a guild.
// These work in every guild the bot is in, with no slot limit shared with
// servers.
func (c *Client) ApplicationEmojis(ctx context.Context, appID Snowflake) ([]GuildEmoji, error) {
	var out struct {
		Items []GuildEmoji `json:"items"`
	}
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/applications/" + appID.String() + "/emojis",
		Route:  "GET /applications/{id}/emojis",
	}, &out)
	return out.Items, err
}

// ApplicationEmoji fetches one application emoji.
func (c *Client) ApplicationEmoji(ctx context.Context, appID, emojiID Snowflake) (*GuildEmoji, error) {
	var out GuildEmoji
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/applications/" + appID.String() + "/emojis/" + emojiID.String(),
		Route:  "GET /applications/{id}/emojis/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateApplicationEmoji uploads an emoji owned by the application.
func (c *Client) CreateApplicationEmoji(ctx context.Context, appID Snowflake, name, image string) (*GuildEmoji, error) {
	var out GuildEmoji
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/applications/" + appID.String() + "/emojis",
		Route:  "POST /applications/{id}/emojis",
		Body:   map[string]any{"name": name, "image": image},
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyApplicationEmoji renames an application emoji.
func (c *Client) ModifyApplicationEmoji(ctx context.Context, appID, emojiID Snowflake, name string) (*GuildEmoji, error) {
	var out GuildEmoji
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/applications/" + appID.String() + "/emojis/" + emojiID.String(),
		Route:  "PATCH /applications/{id}/emojis/{id}",
		Body:   map[string]any{"name": name},
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteApplicationEmoji removes an application emoji.
func (c *Client) DeleteApplicationEmoji(ctx context.Context, appID, emojiID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/applications/" + appID.String() + "/emojis/" + emojiID.String(),
		Route:  "DELETE /applications/{id}/emojis/{id}",
	}, nil)
}

// Stickers

// Sticker is an image or animation that can be sent in a message.
type Sticker struct {
	ID          Snowflake `json:"id"`
	PackID      Snowflake `json:"pack_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Tags        string    `json:"tags"` // comma-separated autocomplete hints
	Type        int       `json:"type"`
	FormatType  int       `json:"format_type"`
	Available   bool      `json:"available"`
	GuildID     Snowflake `json:"guild_id"`
	User        *User     `json:"user"`
	SortValue   int       `json:"sort_value"`
}

// StickerPack is a set of stickers Discord ships.
type StickerPack struct {
	ID             Snowflake `json:"id"`
	Stickers       []Sticker `json:"stickers"`
	Name           string    `json:"name"`
	SKUID          Snowflake `json:"sku_id"`
	CoverStickerID Snowflake `json:"cover_sticker_id"`
	Description    string    `json:"description"`
	BannerAssetID  Snowflake `json:"banner_asset_id"`
}

// GuildStickers lists a guild's custom stickers.
func (c *Client) GuildStickers(ctx context.Context, guildID Snowflake) ([]Sticker, error) {
	var out []Sticker
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/stickers",
		Route:  "GET /guilds/" + guildID.String() + "/stickers",
	}, &out)
	return out, err
}

// GuildSticker fetches one guild sticker.
func (c *Client) GuildSticker(ctx context.Context, guildID, stickerID Snowflake) (*Sticker, error) {
	var out Sticker
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/stickers/" + stickerID.String(),
		Route:  "GET /guilds/" + guildID.String() + "/stickers/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifySticker edits a guild sticker's name, description or tags.
func (c *Client) ModifySticker(ctx context.Context, guildID, stickerID Snowflake, name, description, tags, reason string) (*Sticker, error) {
	body := map[string]any{}
	if name != "" {
		body["name"] = name
	}
	if description != "" {
		body["description"] = description
	}
	if tags != "" {
		body["tags"] = tags
	}

	var out Sticker
	err := c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/stickers/" + stickerID.String(),
		Route:  "PATCH /guilds/" + guildID.String() + "/stickers/{id}",
		Body:   body,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteSticker removes a guild sticker.
func (c *Client) DeleteSticker(ctx context.Context, guildID, stickerID Snowflake, reason string) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/guilds/" + guildID.String() + "/stickers/" + stickerID.String(),
		Route:  "DELETE /guilds/" + guildID.String() + "/stickers/{id}",
		Reason: reason,
	}, nil)
}

// Sticker fetches any sticker by ID, including ones from Discord's own packs.
func (c *Client) Sticker(ctx context.Context, stickerID Snowflake) (*Sticker, error) {
	var out Sticker
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/stickers/" + stickerID.String(),
		Route:  "GET /stickers/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// StickerPacks lists the sticker packs Discord offers.
func (c *Client) StickerPacks(ctx context.Context) ([]StickerPack, error) {
	var out struct {
		StickerPacks []StickerPack `json:"sticker_packs"`
	}
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/sticker-packs",
		Route:  "GET /sticker-packs",
	}, &out)
	return out.StickerPacks, err
}

// StickerPack fetches one sticker pack.
func (c *Client) StickerPack(ctx context.Context, packID Snowflake) (*StickerPack, error) {
	var out StickerPack
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/sticker-packs/" + packID.String(),
		Route:  "GET /sticker-packs/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
