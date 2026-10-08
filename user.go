package starlings

import "time"

// User is a Discord account: a person or a bot.
type User struct {
	ID            Snowflake `json:"id"`
	Username      string    `json:"username"`
	Discriminator string    `json:"discriminator"` // "0" for accounts migrated to unique usernames
	GlobalName    string    `json:"global_name"`   // display name, may be empty
	Avatar        string    `json:"avatar"`        // image hash, empty if using a default avatar
	Bot           bool      `json:"bot"`
	System        bool      `json:"system"`
	MFAEnabled    bool      `json:"mfa_enabled"`
	Banner        string    `json:"banner"`
	AccentColor   int       `json:"accent_color"`
	Locale        string    `json:"locale"`
	Verified      bool      `json:"verified"`
	Email         string    `json:"email"`
	Flags         int       `json:"flags"`
	PremiumType   int       `json:"premium_type"`
	PublicFlags   int       `json:"public_flags"`

	ref bound `json:"-"` // the client this value came from
}

// Tag returns the name to show for a user: the global display name if they have
// one, otherwise the username, falling back to the legacy name#1234 form for
// accounts that have not migrated.
func (u *User) Tag() string {
	switch {
	case u.GlobalName != "":
		return u.GlobalName
	case u.Discriminator != "" && u.Discriminator != "0":
		return u.Username + "#" + u.Discriminator
	default:
		return u.Username
	}
}

// Mention returns the <@id> form that renders as a ping in message content.
func (u *User) Mention() string { return "<@" + u.ID.String() + ">" }

// AvatarURL returns a CDN link to the user's avatar, or to the default avatar
// Discord assigns when they have not set one. Pass a size that is a power of
// two between 16 and 4096; zero uses Discord's default.
func (u *User) AvatarURL(size int) string {
	if u.Avatar == "" {
		return defaultAvatarURL(u)
	}
	return cdnURL("avatars/"+u.ID.String()+"/"+u.Avatar, u.Avatar, size)
}

// Member is a user as seen inside one guild, carrying the nickname, roles and
// join date that only make sense in that guild's context.
//
// User is nil on a few gateway payloads where Discord already sent the user
// alongside the member, so check it before dereferencing.
type Member struct {
	User                       *User       `json:"user"`
	Nick                       string      `json:"nick"`
	Avatar                     string      `json:"avatar"` // per-guild avatar hash
	Roles                      []Snowflake `json:"roles"`
	JoinedAt                   time.Time   `json:"joined_at"`
	PremiumSince               *time.Time  `json:"premium_since"`
	Deaf                       bool        `json:"deaf"`
	Mute                       bool        `json:"mute"`
	Flags                      int         `json:"flags"`
	Pending                    bool        `json:"pending"`
	Permissions                Permissions `json:"permissions,string"`
	CommunicationDisabledUntil *time.Time  `json:"communication_disabled_until"`

	ref bound `json:"-"` // the client this value came from
}

// DisplayName returns the nickname if the member has one, otherwise the
// underlying user's name.
func (m *Member) DisplayName() string {
	if m.Nick != "" {
		return m.Nick
	}
	if m.User != nil {
		return m.User.Tag()
	}
	return ""
}

// AvatarURL returns this member's guild-specific avatar. When the member has
// not set one it falls back to the user's normal (or default) avatar. An empty
// string means the payload did not include enough user information.
func (m *Member) AvatarURL(guildID Snowflake, size int) string {
	if m == nil || m.User == nil {
		return ""
	}
	if m.Avatar == "" {
		return m.User.AvatarURL(size)
	}
	return cdnURL("guilds/"+guildID.String()+"/users/"+m.User.ID.String()+"/avatars/"+m.Avatar, m.Avatar, size)
}

// TimedOut reports whether the member is currently under a timeout.
func (m *Member) TimedOut() bool {
	return m.CommunicationDisabledUntil != nil && m.CommunicationDisabledUntil.After(time.Now())
}

// Role is a set of permissions that can be assigned to members of a guild.
type Role struct {
	ID          Snowflake   `json:"id"`
	Name        string      `json:"name"`
	Color       int         `json:"color"`
	Hoist       bool        `json:"hoist"` // shown separately in the member list
	Icon        string      `json:"icon"`
	Position    int         `json:"position"`
	Permissions Permissions `json:"permissions,string"`
	Managed     bool        `json:"managed"` // created by an integration, not editable
	Mentionable bool        `json:"mentionable"`

	ref bound `json:"-"` // the client this value came from
}

// Mention returns the <@&id> form that renders as a role ping.
func (r *Role) Mention() string { return "<@&" + r.ID.String() + ">" }
