package starlings

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Embed is the rich card that can accompany a message: a title, description,
// coloured bar, fields, images and footer.
//
// Discord caps the total text across every embed on one message at 6000
// characters, on top of the individual limits noted below.
type Embed struct {
	Title       string         `json:"title,omitempty"`       // max 256
	Type        string         `json:"type,omitempty"`        // always "rich" for bot-sent embeds
	Description string         `json:"description,omitempty"` // max 4096
	URL         string         `json:"url,omitempty"`         // makes the title a link
	Timestamp   *time.Time     `json:"timestamp,omitempty"`
	Color       int            `json:"color,omitzero"` // 0xRRGGBB
	Footer      *EmbedFooter   `json:"footer,omitempty"`
	Image       *EmbedMedia    `json:"image,omitempty"`
	Thumbnail   *EmbedMedia    `json:"thumbnail,omitempty"`
	Video       *EmbedMedia    `json:"video,omitempty"`
	Provider    *EmbedProvider `json:"provider,omitempty"`
	Author      *EmbedAuthor   `json:"author,omitempty"`
	Fields      []EmbedField   `json:"fields,omitempty"` // max 25
}

// EmbedFooter is the small line at the bottom of an embed.
type EmbedFooter struct {
	Text    string `json:"text"` // max 2048
	IconURL string `json:"icon_url,omitempty"`
}

// EmbedMedia is an image, thumbnail or video attached to an embed. Only URL
// matters when sending; Discord fills in the rest.
type EmbedMedia struct {
	URL      string `json:"url"`
	ProxyURL string `json:"proxy_url,omitempty"`
	Height   int    `json:"height,omitzero"`
	Width    int    `json:"width,omitzero"`
}

// EmbedProvider names the source of an auto-generated embed.
type EmbedProvider struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

// EmbedAuthor is the small line above the title.
type EmbedAuthor struct {
	Name    string `json:"name"` // max 256
	URL     string `json:"url,omitempty"`
	IconURL string `json:"icon_url,omitempty"`
}

// EmbedField is one name/value pair in an embed's body.
type EmbedField struct {
	Name   string `json:"name"`  // max 256
	Value  string `json:"value"` // max 1024
	Inline bool   `json:"inline,omitzero"`
}

// AddField appends a field and returns the embed, so calls can be chained.
// Text is shortened to Discord's limits, an empty name or value is filled
// with an invisible character, and fields past Discord's 25 are dropped.
func (e *Embed) AddField(name, value string, inline bool) *Embed {
	if len(e.Fields) >= embedFieldsLimit {
		return e
	}
	e.Fields = append(e.Fields, EmbedField{
		Name:   fieldText(name, embedFieldNameLimit),
		Value:  fieldText(value, embedFieldValueLimit),
		Inline: inline,
	})
	return e
}

// SetFooter sets the footer and returns the embed.
func (e *Embed) SetFooter(text, iconURL string) *Embed {
	e.Footer = &EmbedFooter{Text: clampText(text, embedFooterLimit), IconURL: iconURL}
	return e
}

// SetImage sets the main image and returns the embed.
func (e *Embed) SetImage(url string) *Embed {
	e.Image = &EmbedMedia{URL: url}
	return e
}

// SetThumbnail sets the corner thumbnail and returns the embed.
func (e *Embed) SetThumbnail(url string) *Embed {
	e.Thumbnail = &EmbedMedia{URL: url}
	return e
}

// SetAuthor sets the author line and returns the embed.
func (e *Embed) SetAuthor(name, url, iconURL string) *Embed {
	e.Author = &EmbedAuthor{Name: clampText(name, embedAuthorLimit), URL: url, IconURL: iconURL}
	return e
}

// Colours for Embed.Color, matching Discord's own palette.
const (
	ColorBlurple = 0x5865F2
	ColorGreen   = 0x57F287
	ColorYellow  = 0xFEE75C
	ColorRed     = 0xED4245
	ColorFuchsia = 0xEB459E
	ColorGrey    = 0x99AAB5
)

// Discord's per-field embed limits. The helpers below shorten text to fit,
// ending it with an ellipsis, so a long user-supplied value cannot make the
// whole message fail. Setting the struct fields directly sends them as they
// are.
const (
	embedTitleLimit       = 256
	embedDescriptionLimit = 4096
	embedFieldNameLimit   = 256
	embedFieldValueLimit  = 1024
	embedFooterLimit      = 2048
	embedAuthorLimit      = 256
	embedFieldsLimit      = 25
)

// NewEmbed starts an embed with a title:
//
//	e := starlings.NewEmbed("Warning issued").
//		SetDescription(reason).
//		SetColor(starlings.ColorYellow).
//		AddField("Moderator", mod.Mention(), true)
func NewEmbed(title string) *Embed {
	return &Embed{Title: clampText(title, embedTitleLimit)}
}

// SetDescription sets the main text and returns the embed.
func (e *Embed) SetDescription(text string) *Embed {
	e.Description = clampText(text, embedDescriptionLimit)
	return e
}

// SetColor sets the coloured bar, as 0xRRGGBB, and returns the embed.
func (e *Embed) SetColor(color int) *Embed {
	e.Color = color
	return e
}

// SetURL makes the title a link and returns the embed.
func (e *Embed) SetURL(url string) *Embed {
	e.URL = url
	return e
}

// SetTimestamp shows t in the footer, in each reader's time zone.
func (e *Embed) SetTimestamp(t time.Time) *Embed {
	e.Timestamp = &t
	return e
}

// clampText shortens s to limit characters, marking the cut with an
// ellipsis.
func clampText(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit-1]) + "…"
}

// fieldText fills an empty field part, which Discord rejects, with an
// invisible character.
func fieldText(s string, limit int) string {
	if strings.TrimSpace(s) == "" {
		return "\u200b"
	}
	return clampText(s, limit)
}
