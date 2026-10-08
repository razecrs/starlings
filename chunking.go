package starlings

import (
	"context"
	"time"
)

// WithMemberChunking asks Discord for the full member list of every guild
// whose GUILD_CREATE did not include all members. Discord only sends members
// up front for small guilds, so without it State.Members is incomplete in
// larger ones.
//
// It needs the privileged IntentGuildMembers. Requests go through the
// gateway's command limit, so a bot in many guilds fills its cache over a few
// minutes instead of being disconnected.
func WithMemberChunking(enabled bool) Option {
	return func(c *Client) { c.chunkMembers = enabled }
}

func (c *Client) installMemberChunking() {
	OnOrdered(c, func(g *GuildCreate) {
		if g.Unavailable || g.MemberCount <= len(g.Members) || !c.intents.Has(IntentGuildMembers) {
			return
		}
		guildID := g.ID
		// The send may wait for the gateway's command window, and ordered
		// handlers must not block the read loop.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			query := ""
			if err := c.RequestMembers(ctx, MemberRequest{GuildID: guildID, Query: &query}); err != nil {
				c.log.Warn("starlings: requesting guild members", "guild", guildID, "err", err)
			}
		}()
	})
}
