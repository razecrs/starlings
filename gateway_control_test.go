package starlings

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGatewayControlsNeedConnection(t *testing.T) {
	c := testClient()
	ctx := context.Background()

	if err := c.UpdatePresence(ctx, Presence{Status: StatusOnline}); !errors.Is(err, errNotReady) {
		t.Errorf("UpdatePresence() error = %v, want errNotReady", err)
	}
	if err := c.RequestAllMembers(ctx, 1, "test"); !errors.Is(err, errNotReady) {
		t.Errorf("RequestAllMembers() error = %v, want errNotReady", err)
	}
	if err := c.RequestSoundboardSounds(ctx, 1); !errors.Is(err, errNotReady) {
		t.Errorf("RequestSoundboardSounds() error = %v, want errNotReady", err)
	}
	if err := c.RequestChannelInfo(ctx, 1, ChannelInfoStatus); !errors.Is(err, errNotReady) {
		t.Errorf("RequestChannelInfo() error = %v, want errNotReady", err)
	}
	if err := c.JoinVoice(ctx, 1, 2, false, false); !errors.Is(err, errNotReady) {
		t.Errorf("JoinVoice() error = %v, want errNotReady", err)
	}
}

func TestMemberRequestValidation(t *testing.T) {
	c := testClient()
	ctx := context.Background()
	query := "a"

	tests := []struct {
		name string
		req  MemberRequest
		want string
	}{
		{"guild", MemberRequest{Query: &query}, "guild ID"},
		{"selector", MemberRequest{GuildID: 1}, "query or user IDs"},
		{"both selectors", MemberRequest{GuildID: 1, Query: &query, UserIDs: []Snowflake{2}}, "together"},
		{"limit", MemberRequest{GuildID: 1, Query: &query, Limit: 1001}, "limit"},
		{"nonce", MemberRequest{GuildID: 1, Query: &query, Nonce: strings.Repeat("x", 33)}, "nonce"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.RequestMembers(ctx, tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("RequestMembers() error = %v, want text %q", err, tt.want)
			}
		})
	}
}

func TestSoundboardRequestValidation(t *testing.T) {
	c := testClient()
	ctx := context.Background()

	if err := c.RequestSoundboardSounds(ctx); err == nil {
		t.Fatal("empty request should fail")
	}
	if err := c.RequestSoundboardSounds(ctx, 0); err == nil {
		t.Fatal("zero guild ID should fail")
	}
}

func TestChannelInfoRequestValidation(t *testing.T) {
	c := testClient()
	ctx := context.Background()

	if err := c.RequestChannelInfo(ctx, 0, ChannelInfoStatus); err == nil {
		t.Fatal("zero guild ID should fail")
	}
	if err := c.RequestChannelInfo(ctx, 1); err == nil {
		t.Fatal("empty field list should fail")
	}
	if err := c.RequestChannelInfo(ctx, 1, "made_up"); err == nil {
		t.Fatal("unknown field should fail")
	}
}
