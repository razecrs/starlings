package starlings

import "time"

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
func (e *Embed) AddField(name, value string, inline bool) *Embed {
	e.Fields = append(e.Fields, EmbedField{Name: name, Value: value, Inline: inline})
	return e
}

// SetFooter sets the footer and returns the embed.
func (e *Embed) SetFooter(text, iconURL string) *Embed {
	e.Footer = &EmbedFooter{Text: text, IconURL: iconURL}
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
	e.Author = &EmbedAuthor{Name: name, URL: url, IconURL: iconURL}
	return e
}
