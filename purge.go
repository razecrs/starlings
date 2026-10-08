package starlings

import (
	"context"
	"time"
)

// PurgeOptions chooses which recent messages Purge deletes.
type PurgeOptions struct {
	// Count is how many matching messages to delete.
	Count int
	// Match selects messages; nil matches every message.
	Match func(*Message) bool
	// MaxScan caps how many messages are read while looking for matches.
	// Zero means 100, or 500 when Match is set.
	MaxScan int
	// MaxOld caps how many messages older than 14 days are deleted one at a
	// time, since Discord's bulk delete refuses them. Zero means 20; a
	// negative value skips them.
	MaxOld int
	// Reason is recorded in the audit log.
	Reason string
}

// PurgeResult reports what Purge did.
type PurgeResult struct {
	Scanned    int // messages read
	Matched    int // messages that matched, up to Count
	Deleted    int // messages deleted, in bulk and one at a time
	OldDeleted int // of Deleted, older than 14 days
	OldSkipped int // older messages left because of MaxOld
}

// bulkDeleteAge is just under Discord's 14-day bulk-delete limit, so a
// message close to the limit does not make the whole request fail.
const bulkDeleteAge = 14*24*time.Hour - time.Minute

// Purge deletes recent messages in a channel. Messages under 14 days old are
// removed with bulk deletes; older ones are removed one at a time, up to
// MaxOld, because Discord refuses them in bulk. The bot needs Manage
// Messages and Read Message History.
func (c *Client) Purge(ctx context.Context, channelID Snowflake, opts PurgeOptions) (PurgeResult, error) {
	var res PurgeResult
	if opts.Count <= 0 {
		return res, nil
	}
	maxScan := opts.MaxScan
	if maxScan <= 0 {
		maxScan = 100
		if opts.Match != nil {
			maxScan = 500
		}
	}
	maxOld := opts.MaxOld
	if maxOld == 0 {
		maxOld = 20
	}

	cutoff := time.Now().Add(-bulkDeleteAge)
	var recent []Snowflake
	var old []Snowflake
	for msg, err := range c.MessageHistory(ctx, channelID, 0) {
		if err != nil {
			return res, err
		}
		res.Scanned++
		if opts.Match == nil || opts.Match(&msg) {
			res.Matched++
			if msg.ID.Time().After(cutoff) {
				recent = append(recent, msg.ID)
			} else {
				old = append(old, msg.ID)
			}
		}
		if res.Matched >= opts.Count || res.Scanned >= maxScan {
			break
		}
	}

	for len(recent) > 0 {
		batch := recent[:min(len(recent), 100)]
		recent = recent[len(batch):]
		var err error
		if len(batch) == 1 {
			err = c.DeleteMessage(ctx, channelID, batch[0], opts.Reason)
		} else {
			err = c.BulkDeleteMessages(ctx, channelID, batch, opts.Reason)
		}
		if err != nil {
			return res, err
		}
		res.Deleted += len(batch)
	}
	for n, id := range old {
		if maxOld < 0 || n >= maxOld {
			res.OldSkipped = len(old) - n
			break
		}
		err := c.DeleteMessage(ctx, channelID, id, opts.Reason)
		if IsDiscordCode(err, ErrorUnknownMessage) {
			continue // already gone
		}
		if err != nil {
			return res, err
		}
		res.Deleted++
		res.OldDeleted++
	}
	return res, nil
}

// Purge deletes recent messages in this channel. See Client.Purge.
func (ch *Channel) Purge(opts PurgeOptions) (PurgeResult, error) {
	c, ctx, err := ch.ref.use()
	if err != nil {
		return PurgeResult{}, err
	}
	return c.Purge(ctx, ch.ID, opts)
}
