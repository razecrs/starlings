package starlings

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Auto-archive durations Discord accepts, in minutes.
const (
	ArchiveOneHour   = 60
	ArchiveOneDay    = 1440
	ArchiveThreeDays = 4320
	ArchiveOneWeek   = 10080
)

// ThreadMember records that a user is following a thread.
type ThreadMember struct {
	ID            Snowflake `json:"id"` // the thread
	UserID        Snowflake `json:"user_id"`
	JoinTimestamp time.Time `json:"join_timestamp"`
	Flags         int       `json:"flags"`
	Member        *Member   `json:"member"`
}

// ThreadParams configures a new thread.
type ThreadParams struct {
	Name                string      `json:"name"`
	AutoArchiveDuration int         `json:"auto_archive_duration,omitzero"`
	Type                ChannelType `json:"type,omitzero"`
	Invitable           *bool       `json:"invitable,omitzero"`
	RateLimitPerUser    *int        `json:"rate_limit_per_user,omitzero"`
}

// StartThread creates a thread that is not attached to a message.
//
// Set Type to ChannelPrivateThread for a private one; the default is a public
// thread.
func (c *Client) StartThread(ctx context.Context, channelID Snowflake, params ThreadParams, reason string) (*Channel, error) {
	var out Channel
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/channels/" + channelID.String() + "/threads",
		Route:  "POST /channels/" + channelID.String() + "/threads",
		Body:   params,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// StartThreadFromMessage creates a thread hanging off an existing message.
func (c *Client) StartThreadFromMessage(ctx context.Context, channelID, messageID Snowflake, params ThreadParams, reason string) (*Channel, error) {
	var out Channel
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/channels/" + channelID.String() + "/messages/" + messageID.String() + "/threads",
		Route:  "POST /channels/" + channelID.String() + "/messages/{id}/threads",
		Body:   params,
		Reason: reason,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ActiveThreads lists a guild's threads that have not been archived.
func (c *Client) ActiveThreads(ctx context.Context, guildID Snowflake) ([]Channel, []ThreadMember, error) {
	var out struct {
		Threads []Channel      `json:"threads"`
		Members []ThreadMember `json:"members"`
	}
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/threads/active",
		Route:  "GET /guilds/" + guildID.String() + "/threads/active",
	}, &out)
	return out.Threads, out.Members, err
}

// ArchivedThreadsQuery pages backwards through archived threads.
type ArchivedThreadsQuery struct {
	Before time.Time
	Limit  int
}

// archivedThreadPage is what both archive listings return.
type archivedThreadPage struct {
	Threads []Channel `json:"threads"`
	HasMore bool      `json:"has_more"`
}

// archiveQuery renders the shared paging parameters.
func archiveQuery(q ArchivedThreadsQuery) string {
	v := url.Values{}
	if !q.Before.IsZero() {
		v.Set("before", q.Before.Format(time.RFC3339))
	}
	if q.Limit > 0 {
		v.Set("limit", itoa(q.Limit))
	}
	return query(v)
}

// PublicArchivedThreads lists archived public threads in a channel, newest
// first. The boolean reports whether more pages remain.
//
// The two archive listings repeat their request literal rather than sharing
// one, because internal/apidoc reads Route straight out of the AST: a route
// passed in as a variable measures as uncovered even though it is not.
func (c *Client) PublicArchivedThreads(ctx context.Context, channelID Snowflake, q ArchivedThreadsQuery) ([]Channel, bool, error) {
	var out archivedThreadPage
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/channels/" + channelID.String() + "/threads/archived/public" + archiveQuery(q),
		Route:  "GET /channels/" + channelID.String() + "/threads/archived/public",
	}, &out)
	return out.Threads, out.HasMore, err
}

// PrivateArchivedThreads lists archived private threads the bot can see.
func (c *Client) PrivateArchivedThreads(ctx context.Context, channelID Snowflake, q ArchivedThreadsQuery) ([]Channel, bool, error) {
	var out archivedThreadPage
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/channels/" + channelID.String() + "/threads/archived/private" + archiveQuery(q),
		Route:  "GET /channels/" + channelID.String() + "/threads/archived/private",
	}, &out)
	return out.Threads, out.HasMore, err
}

// JoinThread adds the bot to a thread, which is required before it can post.
func (c *Client) JoinThread(ctx context.Context, threadID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/channels/" + threadID.String() + "/thread-members/@me",
		Route:  "PUT /channels/" + threadID.String() + "/thread-members/@me",
	}, nil)
}

// LeaveThread removes the bot from a thread.
func (c *Client) LeaveThread(ctx context.Context, threadID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/channels/" + threadID.String() + "/thread-members/@me",
		Route:  "DELETE /channels/" + threadID.String() + "/thread-members/@me",
	}, nil)
}

// AddThreadMember adds someone else to a thread.
func (c *Client) AddThreadMember(ctx context.Context, threadID, userID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPut,
		Path:   "/channels/" + threadID.String() + "/thread-members/" + userID.String(),
		Route:  "PUT /channels/" + threadID.String() + "/thread-members/{id}",
	}, nil)
}

// RemoveThreadMember removes someone from a thread.
func (c *Client) RemoveThreadMember(ctx context.Context, threadID, userID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/channels/" + threadID.String() + "/thread-members/" + userID.String(),
		Route:  "DELETE /channels/" + threadID.String() + "/thread-members/{id}",
	}, nil)
}

// ThreadMember looks up one member of a thread.
func (c *Client) ThreadMember(ctx context.Context, threadID, userID Snowflake, withMember bool) (*ThreadMember, error) {
	v := url.Values{}
	if withMember {
		v.Set("with_member", "true")
	}

	var out ThreadMember
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/channels/" + threadID.String() + "/thread-members/" + userID.String() + query(v),
		Route:  "GET /channels/" + threadID.String() + "/thread-members/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ThreadMembers lists everyone in a thread.
//
// Populating Member on each entry needs the privileged GuildMembers intent.
func (c *Client) ThreadMembers(ctx context.Context, threadID Snowflake, withMember bool) ([]ThreadMember, error) {
	v := url.Values{}
	if withMember {
		v.Set("with_member", "true")
	}

	var out []ThreadMember
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/channels/" + threadID.String() + "/thread-members" + query(v),
		Route:  "GET /channels/" + threadID.String() + "/thread-members",
	}, &out)
	return out, err
}
