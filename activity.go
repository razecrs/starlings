package starlings

import (
	"context"
	"net/http"
	"net/url"
)

// ActivityInstance is a running session of an embedded Activity - the games
// and apps that run inside a voice channel.
type ActivityInstance struct {
	ApplicationID Snowflake `json:"application_id"`
	InstanceID    string    `json:"instance_id"`
	LaunchID      Snowflake `json:"launch_id"`
	Location      struct {
		ID        string    `json:"id"`
		Kind      string    `json:"kind"` // "gc" guild channel, "pc" private channel
		ChannelID Snowflake `json:"channel_id"`
		GuildID   Snowflake `json:"guild_id,omitzero"`
	} `json:"location"`
	Users []Snowflake `json:"users"`
}

// ActivityInstance reads a running Activity session, which is how a game
// server confirms a client's claimed session really exists and sees who is in
// it.
func (c *Client) ActivityInstance(ctx context.Context, appID Snowflake, instanceID string) (*ActivityInstance, error) {
	var out ActivityInstance
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path: "/applications/" + appID.String() + "/activity-instances/" +
			url.PathEscape(instanceID),
		Route: "GET /applications/{id}/activity-instances/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AttachmentUpload is an ephemeral CDN URL an Activity can share images
// through, since an Activity cannot post messages itself.
type AttachmentUpload struct {
	URL string `json:"url"`
}

// UploadActivityAttachment uploads an image on behalf of an embedded Activity
// and returns a temporary CDN URL for it.
//
// The URL expires, so it is meant to be handed straight to whatever will
// display it rather than stored.
func (c *Client) UploadActivityAttachment(ctx context.Context, appID Snowflake, file File) (*AttachmentUpload, error) {
	var out struct {
		Attachment AttachmentUpload `json:"attachment"`
	}
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/applications/" + appID.String() + "/attachment",
		Route:  "POST /applications/{id}/attachment",
		Files:  []File{file},
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Attachment, nil
}

// Invite target users

// InviteTargetUser is someone explicitly allowed to use a targeted invite.
type InviteTargetUser struct {
	UserID Snowflake `json:"user_id"`
	User   *User     `json:"user,omitzero"`
}

// InviteTargetUsers lists who may use a targeted invite.
func (c *Client) InviteTargetUsers(ctx context.Context, code string, limit int, after Snowflake) ([]InviteTargetUser, error) {
	v := url.Values{}
	if limit > 0 {
		v.Set("limit", itoa(limit))
	}
	if !after.IsZero() {
		v.Set("after", after.String())
	}

	var out struct {
		Users []InviteTargetUser `json:"users"`
	}
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/invites/" + url.PathEscape(code) + "/target-users" + query(v),
		Route:  "GET /invites/{code}/target-users",
	}, &out)
	return out.Users, err
}

// SetInviteTargetUsers replaces the whole allow-list for a targeted invite.
func (c *Client) SetInviteTargetUsers(ctx context.Context, code string, userIDs []Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/invites/" + url.PathEscape(code) + "/target-users",
		Route:  "PUT /invites/{code}/target-users",
		Body:   map[string]any{"user_ids": userIDs},
	}, nil)
}

// AddInviteTargetUser adds one user to a targeted invite's allow-list.
func (c *Client) AddInviteTargetUser(ctx context.Context, code string, userID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/invites/" + url.PathEscape(code) + "/target-users/" + userID.String(),
		Route:  "PUT /invites/{code}/target-users/{id}",
	}, nil)
}

// RemoveInviteTargetUser removes one user from the allow-list.
func (c *Client) RemoveInviteTargetUser(ctx context.Context, code string, userID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/invites/" + url.PathEscape(code) + "/target-users/" + userID.String(),
		Route:  "DELETE /invites/{code}/target-users/{id}",
	}, nil)
}

// InviteTargetUsersJob is the handle for a bulk allow-list change, which
// Discord processes asynchronously.
type InviteTargetUsersJob struct {
	JobID    string `json:"job_id"`
	Status   string `json:"status"`
	Progress int    `json:"progress,omitzero"`
}

// BulkAddInviteTargetUsers adds many users at once. The change is applied in
// the background; poll InviteTargetUsersJobStatus to see when it lands.
func (c *Client) BulkAddInviteTargetUsers(ctx context.Context, code string, userIDs []Snowflake) (*InviteTargetUsersJob, error) {
	var out InviteTargetUsersJob
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/invites/" + url.PathEscape(code) + "/target-users/bulk-add",
		Route:  "POST /invites/{code}/target-users/bulk-add",
		Body:   map[string]any{"user_ids": userIDs},
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// BulkDeleteInviteTargetUsers removes many users at once.
func (c *Client) BulkDeleteInviteTargetUsers(ctx context.Context, code string, userIDs []Snowflake) (*InviteTargetUsersJob, error) {
	var out InviteTargetUsersJob
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/invites/" + url.PathEscape(code) + "/target-users/bulk-delete",
		Route:  "POST /invites/{code}/target-users/bulk-delete",
		Body:   map[string]any{"user_ids": userIDs},
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// InviteTargetUsersJobStatus reports how a bulk allow-list change is going.
func (c *Client) InviteTargetUsersJobStatus(ctx context.Context, code string) (*InviteTargetUsersJob, error) {
	var out InviteTargetUsersJob
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/invites/" + url.PathEscape(code) + "/target-users/job-status",
		Route:  "GET /invites/{code}/target-users/job-status",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
