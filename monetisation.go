package starlings

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// EntitlementType says how a user came to hold an entitlement.
type EntitlementType int

const (
	EntitlementPurchase                EntitlementType = 1
	EntitlementPremiumSubscription     EntitlementType = 2
	EntitlementDeveloperGift           EntitlementType = 3
	EntitlementTestModePurchase        EntitlementType = 4
	EntitlementFreePurchase            EntitlementType = 5
	EntitlementUserGift                EntitlementType = 6
	EntitlementPremiumPurchase         EntitlementType = 7
	EntitlementApplicationSubscription EntitlementType = 8
)

// SKUType describes how a premium offering is purchased.
type SKUType int

const (
	SKUDurable           SKUType = 2
	SKUConsumable        SKUType = 3
	SKUSubscription      SKUType = 5
	SKUSubscriptionGroup SKUType = 6
)

// SKUFlags describes availability and who receives a SKU's benefits.
type SKUFlags int

const (
	SKUAvailable         SKUFlags = 1 << 2
	SKUGuildSubscription SKUFlags = 1 << 7
	SKUUserSubscription  SKUFlags = 1 << 8
)

// SKU is a premium offering belonging to an application.
type SKU struct {
	ID            Snowflake `json:"id"`
	Type          SKUType   `json:"type"`
	ApplicationID Snowflake `json:"application_id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	Flags         SKUFlags  `json:"flags"`
}

// Has reports whether all of flags are set.
func (f SKUFlags) Has(flags SKUFlags) bool { return f&flags == flags }

// SKUs lists every premium offering belonging to an application.
func (c *Client) SKUs(ctx context.Context, appID Snowflake) ([]SKU, error) {
	var out []SKU
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/applications/" + appID.String() + "/skus",
		Route:  "GET /applications/{id}/skus",
	}, &out)
	return out, err
}

// EntitlementsQuery narrows a list of entitlements.
type EntitlementsQuery struct {
	UserID       Snowflake
	SKUIDs       []Snowflake
	Before       Snowflake
	After        Snowflake
	Limit        int // 1-100
	GuildID      Snowflake
	ExcludeEnded bool
	// IncludeDeleted overrides Discord's default of excluding deleted grants.
	IncludeDeleted bool
}

// Entitlements lists who has access to an application's premium SKUs.
func (c *Client) Entitlements(ctx context.Context, appID Snowflake, q EntitlementsQuery) ([]Entitlement, error) {
	v := url.Values{}
	if !q.UserID.IsZero() {
		v.Set("user_id", q.UserID.String())
	}
	if len(q.SKUIDs) > 0 {
		ids := make([]string, len(q.SKUIDs))
		for i, s := range q.SKUIDs {
			ids[i] = s.String()
		}
		v.Set("sku_ids", strings.Join(ids, ","))
	}
	if !q.Before.IsZero() {
		v.Set("before", q.Before.String())
	}
	if !q.After.IsZero() {
		v.Set("after", q.After.String())
	}
	if q.Limit > 0 {
		v.Set("limit", itoa(q.Limit))
	}
	if !q.GuildID.IsZero() {
		v.Set("guild_id", q.GuildID.String())
	}
	if q.ExcludeEnded {
		v.Set("exclude_ended", "true")
	}
	if q.IncludeDeleted {
		v.Set("exclude_deleted", "false")
	}

	var out []Entitlement
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/applications/" + appID.String() + "/entitlements" + query(v),
		Route:  "GET /applications/{id}/entitlements",
	}, &out)
	return out, err
}

// Entitlement fetches one entitlement.
func (c *Client) Entitlement(ctx context.Context, appID, entitlementID Snowflake) (*Entitlement, error) {
	var out Entitlement
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/applications/" + appID.String() + "/entitlements/" + entitlementID.String(),
		Route:  "GET /applications/{id}/entitlements/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateTestEntitlement grants a SKU to a user or guild without payment, for
// testing premium features. ownerType is 1 for a guild, 2 for a user.
func (c *Client) CreateTestEntitlement(ctx context.Context, appID, skuID, ownerID Snowflake, ownerType int) (*Entitlement, error) {
	var out Entitlement
	err := c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path:   "/applications/" + appID.String() + "/entitlements",
		Route:  "POST /applications/{id}/entitlements",
		Body: map[string]any{
			"sku_id":     skuID,
			"owner_id":   ownerID,
			"owner_type": ownerType,
		},
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTestEntitlement revokes a test entitlement.
func (c *Client) DeleteTestEntitlement(ctx context.Context, appID, entitlementID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodDelete,
		Path:   "/applications/" + appID.String() + "/entitlements/" + entitlementID.String(),
		Route:  "DELETE /applications/{id}/entitlements/{id}",
	}, nil)
}

// ConsumeEntitlement marks a one-off purchase as used up, which is what makes
// a consumable buyable again.
func (c *Client) ConsumeEntitlement(ctx context.Context, appID, entitlementID Snowflake) error {
	return c.rest.do(ctx, request{
		Method: http.MethodPost,
		Path: "/applications/" + appID.String() + "/entitlements/" +
			entitlementID.String() + "/consume",
		Route: "POST /applications/{id}/entitlements/{id}/consume",
	}, nil)
}

// Subscription is a recurring purchase of one or more SKUs.
type Subscription struct {
	ID                 Snowflake          `json:"id"`
	UserID             Snowflake          `json:"user_id"`
	SKUIDs             []Snowflake        `json:"sku_ids"`
	EntitlementIDs     []Snowflake        `json:"entitlement_ids"`
	RenewalSKUIDs      []Snowflake        `json:"renewal_sku_ids"`
	CurrentPeriodStart time.Time          `json:"current_period_start"`
	CurrentPeriodEnd   time.Time          `json:"current_period_end"`
	Status             SubscriptionStatus `json:"status"`
	CanceledAt         *time.Time         `json:"canceled_at"`
	Country            string             `json:"country"`
}

// SubscriptionStatus is the billing lifecycle state. Grant perks from
// entitlements, not this value: billing retries can make the period and status
// temporarily appear inconsistent.
type SubscriptionStatus int

const (
	SubscriptionActive   SubscriptionStatus = 0
	SubscriptionInactive SubscriptionStatus = 1
	SubscriptionEnding   SubscriptionStatus = 2
)

// SubscriptionsQuery pages through a SKU's subscriptions.
type SubscriptionsQuery struct {
	Before Snowflake
	After  Snowflake
	Limit  int
	UserID Snowflake
}

// Subscriptions lists the subscriptions for one SKU.
func (c *Client) Subscriptions(ctx context.Context, skuID Snowflake, q SubscriptionsQuery) ([]Subscription, error) {
	v := url.Values{}
	if !q.Before.IsZero() {
		v.Set("before", q.Before.String())
	}
	if !q.After.IsZero() {
		v.Set("after", q.After.String())
	}
	if q.Limit > 0 {
		v.Set("limit", itoa(q.Limit))
	}
	if !q.UserID.IsZero() {
		v.Set("user_id", q.UserID.String())
	}

	var out []Subscription
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/skus/" + skuID.String() + "/subscriptions" + query(v),
		Route:  "GET /skus/{id}/subscriptions",
	}, &out)
	return out, err
}

// Subscription fetches one subscription.
func (c *Client) Subscription(ctx context.Context, skuID, subscriptionID Snowflake) (*Subscription, error) {
	var out Subscription
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/skus/" + skuID.String() + "/subscriptions/" + subscriptionID.String(),
		Route:  "GET /skus/{id}/subscriptions/{id}",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UserEntitlements lists the current user's entitlements for an application.
// Needs a bearer token.
func (c *Client) UserEntitlements(ctx context.Context, bearerToken string, appID Snowflake) ([]Entitlement, error) {
	var out []Entitlement
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/users/@me/applications/" + appID.String() + "/entitlements",
		Route:  "GET /users/@me/applications/{id}/entitlements",
		Auth:   BearerToken(bearerToken),
	}, &out)
	return out, err
}

// GuildJoinRequest is someone waiting to be approved into a guild that has
// membership screening with approval enabled.
type GuildJoinRequest struct {
	ID              Snowflake `json:"id"`
	UserID          Snowflake `json:"user_id"`
	GuildID         Snowflake `json:"guild_id"`
	Status          string    `json:"status"`
	CreatedAt       string    `json:"created_at"`
	RejectionReason string    `json:"rejection_reason,omitzero"`
	User            *User     `json:"user"`
}

// GuildJoinRequests lists pending membership applications.
func (c *Client) GuildJoinRequests(ctx context.Context, guildID Snowflake, status string, limit int) ([]GuildJoinRequest, error) {
	v := url.Values{}
	if status != "" {
		v.Set("status", status)
	}
	if limit > 0 {
		v.Set("limit", itoa(limit))
	}

	var out struct {
		Requests []GuildJoinRequest `json:"guild_join_requests"`
	}
	err := c.rest.do(ctx, request{
		Method: http.MethodGet,
		Path:   "/guilds/" + guildID.String() + "/requests" + query(v),
		Route:  "GET /guilds/" + guildID.String() + "/requests",
	}, &out)
	return out.Requests, err
}

// ResolveJoinRequest approves or rejects a membership application. action is
// "APPROVED" or "REJECTED".
func (c *Client) ResolveJoinRequest(ctx context.Context, guildID, requestID Snowflake, action, rejectionReason string) error {
	body := map[string]any{"action": action}
	if rejectionReason != "" {
		body["rejection_reason"] = rejectionReason
	}
	return c.rest.do(ctx, request{
		Method: http.MethodPatch,
		Path:   "/guilds/" + guildID.String() + "/requests/" + requestID.String(),
		Route:  "PATCH /guilds/" + guildID.String() + "/requests/{id}",
		Body:   body,
	}, nil)
}
