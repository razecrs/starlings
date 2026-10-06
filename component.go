package starlings

// ComponentType identifies which interactive element a Component describes.
type ComponentType int

const (
	ComponentActionRow ComponentType = iota + 1
	ComponentButton
	ComponentStringSelect
	ComponentTextInput
	ComponentUserSelect
	ComponentRoleSelect
	ComponentMentionableSelect
	ComponentChannelSelect
	ComponentSection
	ComponentTextDisplay
	ComponentThumbnail
	ComponentMediaGallery
	ComponentFile
	ComponentSeparator
	_ // 15 is not currently assigned by Discord
	_ // 16 is not currently assigned by Discord
	ComponentContainer
	ComponentLabel
	ComponentFileUpload
	_ // 20 is not currently assigned by Discord
	ComponentRadioGroup
	ComponentCheckboxGroup
	ComponentCheckbox
)

// ButtonStyle sets a button's colour and behaviour. ButtonLink renders as a
// hyperlink and produces no interaction event.
type ButtonStyle int

const (
	ButtonPrimary ButtonStyle = iota + 1
	ButtonSecondary
	ButtonSuccess
	ButtonDanger
	ButtonLink
	ButtonPremium
)

// SeparatorSpacing controls the vertical space around a separator.
type SeparatorSpacing int

const (
	SeparatorSpacingSmall SeparatorSpacing = iota + 1
	SeparatorSpacingLarge
)

// TextInputStyle controls whether a text input is one line or a paragraph.
type TextInputStyle int

const (
	TextInputShort TextInputStyle = iota + 1
	TextInputParagraph
)

// SelectDefaultValue preselects an entity in an auto-populated select menu.
// Type is "user", "role", or "channel".
type SelectDefaultValue struct {
	ID   Snowflake `json:"id"`
	Type string    `json:"type"`
}

// UnfurledMedia is media Discord fetches and renders inside a Components V2
// layout. For uploads, URL uses the attachment://filename form.
type UnfurledMedia struct {
	ID                 Snowflake `json:"id,omitzero"`
	URL                string    `json:"url"`
	ProxyURL           string    `json:"proxy_url,omitempty"`
	Height             *int      `json:"height,omitempty"`
	Width              *int      `json:"width,omitempty"`
	ContentType        string    `json:"content_type,omitempty"`
	AttachmentID       Snowflake `json:"attachment_id,omitzero"`
	Placeholder        string    `json:"placeholder,omitempty"`
	PlaceholderVersion int       `json:"placeholder_version,omitzero"`
	Flags              int       `json:"flags,omitzero"`
}

// MediaGalleryItem is one image or video in a media gallery.
type MediaGalleryItem struct {
	Media       UnfurledMedia `json:"media"`
	Description string        `json:"description,omitempty"`
	Spoiler     bool          `json:"spoiler,omitzero"`
}

// Component is one entry in a message's interactive layout. Discord models
// every kind of component with a single JSON shape, so this struct is a union:
// which fields are meaningful depends on Type.
//
// Legacy messages use action rows at the top level. Components V2 messages
// may instead use sections, text, galleries, files, separators and containers.
type Component struct {
	Type ComponentType `json:"type"`
	ID   int           `json:"id,omitzero"`

	// Action rows only.
	Components []Component `json:"components,omitempty"`

	// Buttons and select menus.
	CustomID string `json:"custom_id,omitempty"` // echoed back on interaction, max 100 chars
	Disabled bool   `json:"disabled,omitzero"`

	// Buttons only.
	Style ButtonStyle `json:"style,omitzero"`
	Label string      `json:"label,omitempty"`
	Emoji *Emoji      `json:"emoji,omitempty"`
	URL   string      `json:"url,omitempty"`   // ButtonLink only
	SKUID Snowflake   `json:"sku_id,omitzero"` // ButtonPremium only

	// Select menus only.
	Options       []SelectOption       `json:"options,omitempty"`
	DefaultValues []SelectDefaultValue `json:"default_values,omitempty"`
	ChannelTypes  []ChannelType        `json:"channel_types,omitempty"`
	Placeholder   string               `json:"placeholder,omitempty"`
	MinValues     *int                 `json:"min_values,omitempty"`
	MaxValues     int                  `json:"max_values,omitzero"`

	// Text inputs (modals) only.
	Value     any      `json:"value,omitempty"`
	Values    []string `json:"values,omitempty"`
	Required  *bool    `json:"required,omitempty"`
	Default   *bool    `json:"default,omitempty"`
	MinLength *int     `json:"min_length,omitempty"`
	MaxLength int      `json:"max_length,omitzero"`

	// Components V2 display and layout fields.
	Content     string             `json:"content,omitempty"`
	Accessory   *Component         `json:"accessory,omitempty"`
	Media       *UnfurledMedia     `json:"media,omitempty"`
	File        *UnfurledMedia     `json:"file,omitempty"`
	Items       []MediaGalleryItem `json:"items,omitempty"`
	Component   *Component         `json:"component,omitempty"`
	Description string             `json:"description,omitempty"`
	Spoiler     bool               `json:"spoiler,omitzero"`
	Divider     *bool              `json:"divider,omitempty"`
	Spacing     SeparatorSpacing   `json:"spacing,omitzero"`
	AccentColor *int               `json:"accent_color,omitempty"`
	FileTypes   []string           `json:"file_types,omitempty"`
	Name        string             `json:"name,omitempty"`
	Size        int                `json:"size,omitzero"`
}

// SelectOption is one choice in a string select menu.
type SelectOption struct {
	Label       string `json:"label"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Emoji       *Emoji `json:"emoji,omitempty"`
	Default     bool   `json:"default,omitzero"`
}

// ActionRow wraps components in the row container Discord requires at the top
// level of a message.
func ActionRow(components ...Component) Component {
	return Component{Type: ComponentActionRow, Components: components}
}

// Button builds a clickable button. The customID is echoed back to your
// interaction handler when someone presses it.
func Button(style ButtonStyle, label, customID string) Component {
	return Component{Type: ComponentButton, Style: style, Label: label, CustomID: customID}
}

// LinkButton builds a button that opens a URL. It never fires an interaction,
// so it has no custom ID.
func LinkButton(label, url string) Component {
	return Component{Type: ComponentButton, Style: ButtonLink, Label: label, URL: url}
}

// StringSelect builds a select menu whose choices are supplied by the bot.
func StringSelect(customID string, options ...SelectOption) Component {
	return Component{Type: ComponentStringSelect, CustomID: customID, Options: options}
}

// EntitySelect builds a user, role, mentionable, or channel select menu.
func EntitySelect(kind ComponentType, customID string) Component {
	return Component{Type: kind, CustomID: customID}
}

// TextInput builds a modal text field. Wrap it in Label for modern modals or
// ActionRow for the legacy modal layout.
func TextInput(style TextInputStyle, customID, placeholder string, required bool) Component {
	return Component{
		Type: ComponentTextInput, Style: ButtonStyle(style), CustomID: customID,
		Placeholder: placeholder, Required: &required,
	}
}

// PremiumButton builds a button that opens Discord's SKU purchase flow.
func PremiumButton(skuID Snowflake) Component {
	return Component{Type: ComponentButton, Style: ButtonPremium, SKUID: skuID}
}

// TextDisplay adds markdown text to a Components V2 message.
func TextDisplay(content string) Component {
	return Component{Type: ComponentTextDisplay, Content: content}
}

// Section lays out one to three text displays beside a button or thumbnail.
func Section(accessory Component, text ...Component) Component {
	return Component{Type: ComponentSection, Components: text, Accessory: &accessory}
}

// Container groups Components V2 elements into a visually distinct block.
func Container(components ...Component) Component {
	return Component{Type: ComponentContainer, Components: components}
}

// AccentContainer builds a container with its left accent colour set.
func AccentContainer(color int, components ...Component) Component {
	return Component{Type: ComponentContainer, Components: components, AccentColor: &color}
}

// Thumbnail displays remote or attached media, usually as a Section accessory.
func Thumbnail(url, description string) Component {
	return Component{Type: ComponentThumbnail, Media: &UnfurledMedia{URL: url}, Description: description}
}

// MediaGallery displays up to ten image or video items.
func MediaGallery(items ...MediaGalleryItem) Component {
	return Component{Type: ComponentMediaGallery, Items: items}
}

// GalleryItem builds one media gallery entry.
func GalleryItem(url, description string) MediaGalleryItem {
	return MediaGalleryItem{Media: UnfurledMedia{URL: url}, Description: description}
}

// FileDisplay renders an uploaded attachment in a Components V2 layout.
func FileDisplay(filename string) Component {
	return Component{Type: ComponentFile, File: &UnfurledMedia{URL: "attachment://" + filename}}
}

// Separator adds an optional rule and vertical spacing to a V2 layout.
func Separator(divider bool, spacing SeparatorSpacing) Component {
	return Component{Type: ComponentSeparator, Divider: &divider, Spacing: spacing}
}

// Label wraps one modal input with its visible label and description.
func Label(label, description string, input Component) Component {
	return Component{Type: ComponentLabel, Label: label, Description: description, Component: &input}
}

// FileUpload builds a modal file picker.
func FileUpload(customID string, required bool) Component {
	return Component{Type: ComponentFileUpload, CustomID: customID, Required: &required}
}

// RadioGroup builds a single-choice modal input.
func RadioGroup(customID string, options ...SelectOption) Component {
	return Component{Type: ComponentRadioGroup, CustomID: customID, Options: options}
}

// CheckboxGroup builds a multiple-choice modal input.
func CheckboxGroup(customID string, options ...SelectOption) Component {
	return Component{Type: ComponentCheckboxGroup, CustomID: customID, Options: options}
}

// Checkbox builds a single boolean modal input.
func Checkbox(customID string, checked bool) Component {
	return Component{Type: ComponentCheckbox, CustomID: customID, Default: &checked}
}

// StringValue returns a submitted text-input value without a type assertion.
func (c Component) StringValue() (string, bool) {
	v, ok := c.Value.(string)
	return v, ok
}

// BoolValue returns a submitted checkbox value without a type assertion.
func (c Component) BoolValue() (bool, bool) {
	v, ok := c.Value.(bool)
	return v, ok
}

// SelectedValues returns values submitted by a select, file upload, or
// checkbox group inside a modal.
func (c Component) SelectedValues() []string {
	return append([]string(nil), c.Values...)
}
