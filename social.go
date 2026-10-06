package starlings

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
)

// LobbyFlags controls application-level lobby behavior. Discord currently
// reserves the values, so Starlings keeps this typed without inventing names.
type LobbyFlags uint32

// LobbyMemberFlags controls what a member may do in a lobby.
type LobbyMemberFlags int

const LobbyMemberCanLinkChannel LobbyMemberFlags = 1 << 0

// Lobby is an application-owned group of users that can message each other.
type Lobby struct {
	ID                       Snowflake         `json:"id"`
	ApplicationID            Snowflake         `json:"application_id"`
	Metadata                 map[string]string `json:"metadata"`
	Members                  []LobbyMember     `json:"members"`
	LinkedChannel            *Channel          `json:"linked_channel"`
	Flags                    LobbyFlags        `json:"flags"`
	OverrideEventWebhooksURL string            `json:"override_event_webhooks_url"`
}

// LobbyMember is one user's application-specific identity inside a lobby.
type LobbyMember struct {
	ID             Snowflake         `json:"id"`
	Metadata       map[string]string `json:"metadata"`
	Flags          LobbyMemberFlags  `json:"flags"`
	AdditionalName string            `json:"additional_name"`
}

// CanLinkChannel reports whether the member may link a guild text channel.
func (m LobbyMember) CanLinkChannel() bool { return m.Flags&LobbyMemberCanLinkChannel != 0 }

// LobbyMemberInput adds or updates a lobby member. Set ClearAdditionalName to
// explicitly send null; leaving both fields zero preserves the existing name.
type LobbyMemberInput struct {
	ID                  Snowflake
	Metadata            map[string]string
	Flags               LobbyMemberFlags
	AdditionalName      string
	ClearAdditionalName bool
	Remove              bool // only used by BulkUpdateLobbyMembers
}

// LobbyCreate configures a server-managed lobby.
type LobbyCreate struct {
	Metadata                 map[string]string
	Members                  []LobbyMemberInput
	IdleTimeoutSeconds       int
	Flags                    LobbyFlags
	OverrideEventWebhooksURL string
}

// LobbyEdit changes only the non-zero fields. Pointer collections let an
// empty value mean "replace with empty". ClearMetadata and
// ClearOverrideEventWebhooksURL explicitly send JSON null.
type LobbyEdit struct {
	Metadata                      *map[string]string
	Members                       *[]LobbyMemberInput
	IdleTimeoutSeconds            *int
	Flags                         *LobbyFlags
	OverrideEventWebhooksURL      *string
	ClearMetadata                 bool
	ClearOverrideEventWebhooksURL bool
}

// LobbyJoin configures the user-authenticated create-or-join operation.
type LobbyJoin struct {
	Secret             string
	LobbyMetadata      map[string]string
	MemberMetadata     map[string]string
	IdleTimeoutSeconds int
	Flags              LobbyFlags
}

// LobbyMessage is a message sent within a Social SDK lobby.
type LobbyMessage struct {
	ID                 Snowflake           `json:"id"`
	Type               MessageType         `json:"type"`
	Content            string              `json:"content"`
	LobbyID            Snowflake           `json:"lobby_id"`
	ChannelID          Snowflake           `json:"channel_id"`
	Author             *User               `json:"author"`
	LobbyMember        *MessageLobbyMember `json:"lobby_member"`
	Metadata           map[string]string   `json:"metadata"`
	ModerationMetadata map[string]string   `json:"moderation_metadata"`
	Flags              MessageFlags        `json:"flags"`
	ApplicationID      Snowflake           `json:"application_id"`
}

// LobbyMessageData is everything Discord accepts when sending a lobby
// message. Content alone is enough for ordinary chat.
type LobbyMessageData struct {
	Content           string             `json:"content,omitzero"`
	Embeds            []Embed            `json:"embeds,omitzero"`
	AllowedMentions   *AllowedMentions   `json:"allowed_mentions,omitzero"`
	StickerIDs        []Snowflake        `json:"sticker_ids,omitzero"`
	Components        []Component        `json:"components,omitzero"`
	Flags             MessageFlags       `json:"flags,omitzero"`
	Attachments       []Attachment       `json:"attachments,omitzero"`
	Poll              *Poll              `json:"poll,omitzero"`
	SharedClientTheme *SharedClientTheme `json:"shared_client_theme,omitzero"`
	MessageRef        *MessageRef        `json:"message_reference,omitzero"`
	Nonce             any                `json:"nonce,omitzero"`
	EnforceNonce      bool               `json:"enforce_nonce,omitzero"`
	TTS               bool               `json:"tts,omitzero"`
	Metadata          map[string]string  `json:"metadata,omitzero"`
}

// SocialClient performs operations as a user authorized with the
// sdk.social_layer OAuth2 scope. It shares the parent client's HTTP transport
// and rate limiter but never its bot authorization header.
type SocialClient struct {
	c    *Client
	auth string
}

// Social creates a user-scoped Social SDK client from an OAuth2 access token.
func (c *Client) Social(accessToken string) *SocialClient {
	return &SocialClient{c: c.rootClient(), auth: BearerToken(accessToken)}
}

// CreateLobby creates a server-managed lobby with the bot token.
func (c *Client) CreateLobby(ctx context.Context, data LobbyCreate) (*Lobby, error) {
	body, err := lobbyCreateBody(data)
	if err != nil {
		return nil, err
	}
	var out Lobby
	err = c.rest.do(ctx, request{Method: http.MethodPost, Path: "/lobbies", Route: "POST /lobbies", Body: body}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Lobby fetches a server-managed lobby.
func (c *Client) Lobby(ctx context.Context, lobbyID Snowflake) (*Lobby, error) {
	path := "/lobbies/" + lobbyID.String()
	var out Lobby
	err := c.rest.do(ctx, request{Method: http.MethodGet, Path: path, Route: "GET /lobbies/" + lobbyID.String()}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// EditLobby modifies a server-managed lobby.
func (c *Client) EditLobby(ctx context.Context, lobbyID Snowflake, data LobbyEdit) (*Lobby, error) {
	body, err := lobbyEditBody(data)
	if err != nil {
		return nil, err
	}
	path := "/lobbies/" + lobbyID.String()
	var out Lobby
	err = c.rest.do(ctx, request{Method: http.MethodPatch, Path: path, Route: "PATCH /lobbies/" + lobbyID.String(), Body: body}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteLobby permanently removes a server-managed lobby. Discord treats an
// already-deleted lobby as success.
func (c *Client) DeleteLobby(ctx context.Context, lobbyID Snowflake) error {
	path := "/lobbies/" + lobbyID.String()
	return c.rest.do(ctx, request{Method: http.MethodDelete, Path: path, Route: "DELETE /lobbies/" + lobbyID.String()}, nil)
}

// AddLobbyMember adds a user or updates their lobby metadata.
func (c *Client) AddLobbyMember(ctx context.Context, lobbyID, userID Snowflake, data LobbyMemberInput) (*LobbyMember, error) {
	data.ID = userID
	body, err := lobbyMemberBody(data, false, false)
	if err != nil {
		return nil, err
	}
	path := "/lobbies/" + lobbyID.String() + "/members/" + userID.String()
	var out LobbyMember
	err = c.rest.do(ctx, request{Method: http.MethodPut, Path: path, Route: "PUT /lobbies/" + lobbyID.String() + "/members/{id}", Body: body}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// BulkUpdateLobbyMembers adds, updates, or removes up to 25 users atomically.
func (c *Client) BulkUpdateLobbyMembers(ctx context.Context, lobbyID Snowflake, members []LobbyMemberInput) ([]LobbyMember, error) {
	if len(members) < 1 || len(members) > 25 {
		return nil, fmt.Errorf("starlings: lobby bulk update needs 1-25 members, got %d", len(members))
	}
	body := make([]map[string]any, len(members))
	for i, member := range members {
		var err error
		body[i], err = lobbyMemberBody(member, true, true)
		if err != nil {
			return nil, fmt.Errorf("starlings: lobby member %d: %w", i, err)
		}
	}
	path := "/lobbies/" + lobbyID.String() + "/members/bulk"
	var out []LobbyMember
	err := c.rest.do(ctx, request{Method: http.MethodPost, Path: path, Route: "POST /lobbies/" + lobbyID.String() + "/members/bulk", Body: body}, &out)
	return out, err
}

// RemoveLobbyMember removes a user. Removing an absent user is successful.
func (c *Client) RemoveLobbyMember(ctx context.Context, lobbyID, userID Snowflake) error {
	path := "/lobbies/" + lobbyID.String() + "/members/" + userID.String()
	return c.rest.do(ctx, request{Method: http.MethodDelete, Path: path, Route: "DELETE /lobbies/" + lobbyID.String() + "/members/{id}"}, nil)
}

// LobbyInviteForUser creates a one-use invite to the lobby's linked channel.
func (c *Client) LobbyInviteForUser(ctx context.Context, lobbyID, userID Snowflake) (string, error) {
	path := "/lobbies/" + lobbyID.String() + "/members/" + userID.String() + "/invites"
	var out struct {
		Code string `json:"code"`
	}
	err := c.rest.do(ctx, request{Method: http.MethodPost, Path: path, Route: "POST /lobbies/" + lobbyID.String() + "/members/{id}/invites"}, &out)
	return out.Code, err
}

// UpdateLobbyMessageModeration sets application-visible moderation metadata.
func (c *Client) UpdateLobbyMessageModeration(ctx context.Context, lobbyID, messageID Snowflake, metadata map[string]string) error {
	if err := validateModerationMetadata(metadata); err != nil {
		return err
	}
	path := "/lobbies/" + lobbyID.String() + "/messages/" + messageID.String() + "/moderation-metadata"
	return c.rest.do(ctx, request{Method: http.MethodPut, Path: path, Route: "PUT /lobbies/" + lobbyID.String() + "/messages/{id}/moderation-metadata", Body: metadata}, nil)
}

// UpdateDirectMessageModeration sets moderation metadata for a Social SDK DM.
func (c *Client) UpdateDirectMessageModeration(ctx context.Context, user1, user2, messageID Snowflake, metadata map[string]string) error {
	if err := validateModerationMetadata(metadata); err != nil {
		return err
	}
	path := "/partner-sdk/dms/" + user1.String() + "/" + user2.String() + "/messages/" + messageID.String() + "/moderation-metadata"
	return c.rest.do(ctx, request{Method: http.MethodPut, Path: path, Route: "PUT /partner-sdk/dms/{id}/{id}/messages/{id}/moderation-metadata", Body: metadata}, nil)
}

// CreateOrJoinLobby joins the OAuth user to the lobby identified by Secret,
// creating it when necessary.
func (s *SocialClient) CreateOrJoinLobby(ctx context.Context, data LobbyJoin) (*Lobby, error) {
	if data.Secret == "" || len(data.Secret) > 250 {
		return nil, fmt.Errorf("starlings: lobby secret must be 1-250 bytes")
	}
	if err := validateIdleTimeout(data.IdleTimeoutSeconds); err != nil {
		return nil, err
	}
	if err := validateLobbyMetadata(data.LobbyMetadata); err != nil {
		return nil, fmt.Errorf("starlings: lobby metadata: %w", err)
	}
	if err := validateLobbyMetadata(data.MemberMetadata); err != nil {
		return nil, fmt.Errorf("starlings: member metadata: %w", err)
	}
	body := map[string]any{"secret": data.Secret}
	putNonZero(body, "idle_timeout_seconds", data.IdleTimeoutSeconds)
	putNonZero(body, "lobby_metadata", data.LobbyMetadata)
	putNonZero(body, "member_metadata", data.MemberMetadata)
	putNonZero(body, "flags", data.Flags)
	var out Lobby
	err := s.c.rest.do(ctx, request{Method: http.MethodPut, Path: "/lobbies", Route: "PUT /lobbies", Body: body, Auth: s.auth}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// LeaveLobby removes the OAuth user from a lobby.
func (s *SocialClient) LeaveLobby(ctx context.Context, lobbyID Snowflake) error {
	path := "/lobbies/" + lobbyID.String() + "/members/@me"
	return s.c.rest.do(ctx, request{Method: http.MethodDelete, Path: path, Route: "DELETE /lobbies/" + lobbyID.String() + "/members/@me", Auth: s.auth}, nil)
}

// LinkLobbyChannel links a guild text channel to a lobby.
func (s *SocialClient) LinkLobbyChannel(ctx context.Context, lobbyID, channelID Snowflake) (*Lobby, error) {
	path := "/lobbies/" + lobbyID.String() + "/channel-linking"
	var out Lobby
	err := s.c.rest.do(ctx, request{Method: http.MethodPatch, Path: path, Route: "PATCH /lobbies/" + lobbyID.String() + "/channel-linking", Body: map[string]any{"channel_id": channelID}, Auth: s.auth}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UnlinkLobbyChannel removes the lobby's current channel link.
func (s *SocialClient) UnlinkLobbyChannel(ctx context.Context, lobbyID Snowflake) (*Lobby, error) {
	path := "/lobbies/" + lobbyID.String() + "/channel-linking"
	var out Lobby
	err := s.c.rest.do(ctx, request{Method: http.MethodPatch, Path: path, Route: "PATCH /lobbies/" + lobbyID.String() + "/channel-linking", Body: map[string]any{}, Auth: s.auth}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// LobbyInvite creates a one-use invite to the linked channel for the OAuth user.
func (s *SocialClient) LobbyInvite(ctx context.Context, lobbyID Snowflake) (string, error) {
	path := "/lobbies/" + lobbyID.String() + "/members/@me/invites"
	var out struct {
		Code string `json:"code"`
	}
	err := s.c.rest.do(ctx, request{Method: http.MethodPost, Path: path, Route: "POST /lobbies/" + lobbyID.String() + "/members/@me/invites", Auth: s.auth}, &out)
	return out.Code, err
}

// LobbyMessages returns the newest messages, up to 200.
func (s *SocialClient) LobbyMessages(ctx context.Context, lobbyID Snowflake, limit int) ([]LobbyMessage, error) {
	if limit < 0 || limit > 200 {
		return nil, fmt.Errorf("starlings: lobby message limit must be 1-200, got %d", limit)
	}
	v := url.Values{}
	if limit > 0 {
		v.Set("limit", itoa(limit))
	}
	path := "/lobbies/" + lobbyID.String() + "/messages" + query(v)
	var out []LobbyMessage
	err := s.c.rest.do(ctx, request{Method: http.MethodGet, Path: path, Route: "GET /lobbies/" + lobbyID.String() + "/messages", Auth: s.auth}, &out)
	return out, err
}

// SendLobbyMessage sends plain text as the OAuth user.
func (s *SocialClient) SendLobbyMessage(ctx context.Context, lobbyID Snowflake, content string) (*LobbyMessage, error) {
	return s.SendLobbyMessageComplex(ctx, lobbyID, LobbyMessageData{Content: content})
}

// SendLobbyMessageComplex sends a rich Social SDK lobby message.
func (s *SocialClient) SendLobbyMessageComplex(ctx context.Context, lobbyID Snowflake, data LobbyMessageData) (*LobbyMessage, error) {
	if len(data.Content) > 4000 {
		return nil, fmt.Errorf("starlings: lobby message content exceeds 4000 bytes")
	}
	if err := validateLobbyMetadata(data.Metadata); err != nil {
		return nil, fmt.Errorf("starlings: message metadata: %w", err)
	}
	path := "/lobbies/" + lobbyID.String() + "/messages"
	var out LobbyMessage
	err := s.c.rest.do(ctx, request{Method: http.MethodPost, Path: path, Route: "POST /lobbies/" + lobbyID.String() + "/messages", Body: data, Auth: s.auth}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ExternalAuthType identifies the game account provider behind a provisional user.
type ExternalAuthType string

const (
	ExternalAuthOIDC                      ExternalAuthType = "OIDC"
	ExternalAuthEpicAccessToken           ExternalAuthType = "EPIC_ONLINE_SERVICES_ACCESS_TOKEN"
	ExternalAuthEpicIDToken               ExternalAuthType = "EPIC_ONLINE_SERVICES_ID_TOKEN"
	ExternalAuthSteamSessionTicket        ExternalAuthType = "STEAM_SESSION_TICKET"
	ExternalAuthUnityIDToken              ExternalAuthType = "UNITY_SERVICES_ID_TOKEN"
	ExternalAuthDiscordBotAccessToken     ExternalAuthType = "DISCORD_BOT_ISSUED_ACCESS_TOKEN"
	ExternalAuthAppleIDToken              ExternalAuthType = "APPLE_ID_TOKEN"
	ExternalAuthPlayStationNetworkIDToken ExternalAuthType = "PLAYSTATION_NETWORK_ID_TOKEN"
)

// ProvisionalCredentials authenticates an external game identity. Keep
// ClientSecret on a trusted server; public clients should use Discord's native
// SDK helper instead.
type ProvisionalCredentials struct {
	ClientID          Snowflake        `json:"client_id"`
	ClientSecret      string           `json:"client_secret,omitzero"`
	ExternalAuthToken string           `json:"external_auth_token"`
	ExternalAuthType  ExternalAuthType `json:"external_auth_type"`
}

// ProvisionalToken is a short-lived sdk.social_layer OAuth2 token.
type ProvisionalToken struct {
	TokenType    string   `json:"token_type"`
	AccessToken  string   `json:"access_token"`
	ExpiresIn    int      `json:"expires_in"`
	Scope        string   `json:"scope"`
	IDToken      string   `json:"id_token"`
	RefreshToken string   `json:"refresh_token"`
	Scopes       []string `json:"scopes"`
	ExpiresAtS   *int64   `json:"expires_at_s"`
}

// ExchangeProvisionalToken exchanges an external identity credential without
// attaching the client's bot authorization.
func (c *Client) ExchangeProvisionalToken(ctx context.Context, credentials ProvisionalCredentials) (*ProvisionalToken, error) {
	if err := validateProvisionalCredentials(credentials); err != nil {
		return nil, err
	}
	var out ProvisionalToken
	err := c.rest.do(ctx, request{Method: http.MethodPost, Path: "/partner-sdk/token", Route: "POST /partner-sdk/token", Body: credentials, NoAuth: true}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UnmergeProvisionalAccount detaches an external identity using confidential
// client credentials.
func (c *Client) UnmergeProvisionalAccount(ctx context.Context, credentials ProvisionalCredentials) error {
	if err := validateProvisionalCredentials(credentials); err != nil {
		return err
	}
	return c.rest.do(ctx, request{Method: http.MethodPost, Path: "/partner-sdk/provisional-accounts/unmerge", Route: "POST /partner-sdk/provisional-accounts/unmerge", Body: credentials, NoAuth: true}, nil)
}

// ExchangeBotProvisionalToken creates or refreshes a provisional user using
// a bot-authorized application identity.
func (c *Client) ExchangeBotProvisionalToken(ctx context.Context, externalUserID, preferredGlobalName string, provisionalUserID Snowflake) (*ProvisionalToken, error) {
	if externalUserID == "" || len(externalUserID) > 1024 {
		return nil, fmt.Errorf("starlings: external user ID must be 1-1024 bytes")
	}
	if len(preferredGlobalName) > 32 {
		return nil, fmt.Errorf("starlings: preferred global name exceeds 32 bytes")
	}
	body := map[string]any{"external_user_id": externalUserID}
	putNonZero(body, "preferred_global_name", preferredGlobalName)
	if !provisionalUserID.IsZero() {
		body["provisional_user_id"] = provisionalUserID
	}
	var out ProvisionalToken
	err := c.rest.do(ctx, request{Method: http.MethodPost, Path: "/partner-sdk/token/bot", Route: "POST /partner-sdk/token/bot", Body: body}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UnmergeBotProvisionalAccount detaches an external identity using bot auth.
func (c *Client) UnmergeBotProvisionalAccount(ctx context.Context, externalUserID string) error {
	if externalUserID == "" || len(externalUserID) > 1024 {
		return fmt.Errorf("starlings: external user ID must be 1-1024 bytes")
	}
	return c.rest.do(ctx, request{Method: http.MethodPost, Path: "/partner-sdk/provisional-accounts/unmerge/bot", Route: "POST /partner-sdk/provisional-accounts/unmerge/bot", Body: map[string]any{"external_user_id": externalUserID}}, nil)
}

func lobbyCreateBody(data LobbyCreate) (map[string]any, error) {
	if err := validateIdleTimeout(data.IdleTimeoutSeconds); err != nil {
		return nil, err
	}
	if err := validateLobbyMetadata(data.Metadata); err != nil {
		return nil, err
	}
	if len(data.Members) > 25 {
		return nil, fmt.Errorf("starlings: a lobby may contain at most 25 initial members")
	}
	body := map[string]any{}
	putNonZero(body, "metadata", data.Metadata)
	putNonZero(body, "idle_timeout_seconds", data.IdleTimeoutSeconds)
	putNonZero(body, "flags", data.Flags)
	putNonZero(body, "override_event_webhooks_url", data.OverrideEventWebhooksURL)
	if len(data.Members) > 0 {
		members := make([]map[string]any, len(data.Members))
		for i, member := range data.Members {
			var err error
			members[i], err = lobbyMemberBody(member, true, false)
			if err != nil {
				return nil, fmt.Errorf("starlings: lobby member %d: %w", i, err)
			}
		}
		body["members"] = members
	}
	return body, nil
}

func lobbyEditBody(data LobbyEdit) (map[string]any, error) {
	body := map[string]any{}
	if data.ClearMetadata {
		body["metadata"] = nil
	} else if data.Metadata != nil {
		if err := validateLobbyMetadata(*data.Metadata); err != nil {
			return nil, err
		}
		body["metadata"] = *data.Metadata
	}
	if data.Members != nil {
		if len(*data.Members) > 25 {
			return nil, fmt.Errorf("starlings: a lobby may contain at most 25 members")
		}
		members := make([]map[string]any, len(*data.Members))
		for i, member := range *data.Members {
			var err error
			members[i], err = lobbyMemberBody(member, true, false)
			if err != nil {
				return nil, fmt.Errorf("starlings: lobby member %d: %w", i, err)
			}
		}
		body["members"] = members
	}
	if data.IdleTimeoutSeconds != nil {
		if err := validateIdleTimeout(*data.IdleTimeoutSeconds); err != nil {
			return nil, err
		}
		body["idle_timeout_seconds"] = *data.IdleTimeoutSeconds
	}
	if data.Flags != nil {
		body["flags"] = *data.Flags
	}
	if data.ClearOverrideEventWebhooksURL {
		body["override_event_webhooks_url"] = nil
	} else if data.OverrideEventWebhooksURL != nil {
		body["override_event_webhooks_url"] = *data.OverrideEventWebhooksURL
	}
	return body, nil
}

func lobbyMemberBody(data LobbyMemberInput, includeID, bulk bool) (map[string]any, error) {
	if includeID && data.ID.IsZero() {
		return nil, fmt.Errorf("member ID is required")
	}
	if err := validateLobbyMetadata(data.Metadata); err != nil {
		return nil, err
	}
	if len(data.AdditionalName) > 80 {
		return nil, fmt.Errorf("additional name exceeds 80 bytes")
	}
	body := map[string]any{}
	if includeID {
		body["id"] = data.ID
	}
	putNonZero(body, "metadata", data.Metadata)
	putNonZero(body, "flags", data.Flags)
	if data.ClearAdditionalName {
		body["additional_name"] = nil
	} else {
		putNonZero(body, "additional_name", data.AdditionalName)
	}
	if bulk && data.Remove {
		body["remove_member"] = true
	}
	return body, nil
}

func validateLobbyMetadata(metadata map[string]string) error {
	if len(metadata) > 25 {
		return fmt.Errorf("metadata has %d keys; maximum is 25", len(metadata))
	}
	total := 0
	for key, value := range metadata {
		if len(key) > 1024 || len(value) > 1024 {
			return fmt.Errorf("metadata key and value must each be at most 1024 bytes")
		}
		total += len(key) + len(value)
	}
	if total > 1000 {
		return fmt.Errorf("metadata is %d bytes; maximum total is 1000", total)
	}
	return nil
}

func validateModerationMetadata(metadata map[string]string) error {
	if len(metadata) > 5 {
		return fmt.Errorf("starlings: moderation metadata has %d keys; maximum is 5", len(metadata))
	}
	for key, value := range metadata {
		if len(key) > 1024 || len(value) > 2000 {
			return fmt.Errorf("starlings: moderation metadata keys are limited to 1024 bytes and values to 2000")
		}
	}
	return nil
}

func validateIdleTimeout(seconds int) error {
	if seconds != 0 && (seconds < 5 || seconds > 604800) {
		return fmt.Errorf("starlings: lobby idle timeout must be 5-604800 seconds, got %d", seconds)
	}
	return nil
}

func validateProvisionalCredentials(credentials ProvisionalCredentials) error {
	if credentials.ClientID.IsZero() || credentials.ExternalAuthToken == "" || credentials.ExternalAuthType == "" {
		return fmt.Errorf("starlings: provisional credentials need client ID, external auth type, and external auth token")
	}
	if len(credentials.ClientSecret) > 1024 || len(credentials.ExternalAuthToken) > 10240 {
		return fmt.Errorf("starlings: provisional credentials exceed Discord's size limits")
	}
	return nil
}

func putNonZero[K comparable, V any](m map[K]any, key K, value V) {
	if !reflect.ValueOf(&value).Elem().IsZero() {
		m[key] = value
	}
}
