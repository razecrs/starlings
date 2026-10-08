package starlings

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

// defaultAutoDefer leaves about 800 ms of Discord's three-second window for
// the acknowledgement itself to arrive.
const defaultAutoDefer = 2200 * time.Millisecond

// WithAutoDefer sets how long a slash command, button, select, or modal
// handler may run before Starlings acknowledges the interaction for it. Fast
// handlers answer normally in one request. A slow handler's later Reply or
// UpdateMessage edits the deferred response instead, so it never fails with
// "unknown interaction". The default is 2.2 seconds; zero turns it off.
//
// It applies to interactions received over the gateway. Interactions served
// by InteractionHandler must answer before the handler returns.
func WithAutoDefer(after time.Duration) Option {
	return func(c *Client) { c.autoDefer = max(after, 0) }
}

// NoAutoDefer turns automatic deferral off for one command, for a handler
// that may open a modal after slow work. A modal must be the first response.
func (r *SlashRoute) NoAutoDefer() *SlashRoute {
	return r.update(func(e *slashEntry) { e.noAutoDefer = true })
}

// ErrAutoDeferred is returned when a handler opens a modal or answers
// autocomplete after Starlings already deferred the interaction for it.
var ErrAutoDeferred = errors.New("starlings: the interaction was already deferred because the handler was slow; " +
	"open modals before slow work, or use NoAutoDefer")

type autoDeferState struct {
	kind      CallbackType
	ephemeral bool
	fired     atomic.Bool
	timer     *time.Timer
	done      chan struct{} // closed once the deferral has been decided and sent
	won       bool          // the deferral was the initial response
	err       error         // the deferral request's error
}

// armAutoDefer schedules a deferral unless the handler answers first.
func (i *InteractionCreate) armAutoDefer(after time.Duration, kind CallbackType, ephemeral bool) {
	if after <= 0 || i.respondHTTP != nil || i.c == nil {
		return
	}
	// Discord's clock starts when the interaction is created, not when it
	// reaches the bot, so a delayed delivery leaves less time.
	if created := i.ID.Time(); !i.ID.IsZero() {
		after = min(after, max(time.Until(created.Add(after)), 0))
	}
	st := &autoDeferState{kind: kind, ephemeral: ephemeral, done: make(chan struct{})}
	i.auto = st
	st.timer = time.AfterFunc(after, func() {
		defer close(st.done)
		st.fired.Store(true)
		if !i.claimAnswer(kind) {
			st.fired.Store(false)
			return
		}
		st.won = true
		var data *InteractionResponseData
		if ephemeral {
			data = &InteractionResponseData{Flags: MessageFlagEphemeral}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		st.err = i.sendCallback(ctx, InteractionResponse{Type: kind, Data: data}, nil)
		if st.err != nil {
			i.c.log.Warn("starlings: automatic defer failed", "interaction", i.ID, "err", st.err)
		}
	})
}

// stopAutoDefer cancels a pending deferral once the handler has answered.
func (i *InteractionCreate) stopAutoDefer() {
	if st := i.auto; st != nil && i.Answered() && !st.fired.Load() {
		st.timer.Stop()
	}
}

// afterAutoDefer delivers a response that arrived after Starlings deferred.
func (i *InteractionCreate) afterAutoDefer(ctx context.Context, st *autoDeferState, resp InteractionResponse, files []File) error {
	select {
	case <-st.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	if !st.won {
		return ErrInteractionAlreadyAnswered
	}
	if st.err != nil {
		return fmt.Errorf("starlings: the automatic defer failed, so this response cannot be delivered: %w", st.err)
	}
	var data InteractionResponseData
	if resp.Data != nil {
		data = *resp.Data
	}
	switch resp.Type {
	case CallbackDeferredChannelMessage, CallbackDeferredUpdateMessage:
		return nil // already done

	case CallbackChannelMessageWithSource:
		if st.kind == CallbackDeferredUpdateMessage {
			// A component was acknowledged silently; a reply is a new message.
			_, err := i.FollowupFiles(ctx, data, files...)
			return err
		}
		if wantEphemeral := data.Flags&MessageFlagEphemeral != 0; wantEphemeral != st.ephemeral {
			// Visibility is fixed when deferring. Replace the placeholder with
			// a follow-up that has the visibility the handler asked for.
			if err := i.DeleteResponse(ctx); err != nil {
				return err
			}
			_, err := i.FollowupFiles(ctx, data, files...)
			return err
		}
		data.Flags &^= MessageFlagEphemeral
		_, err := i.EditResponseFiles(ctx, data, files...)
		return err

	case CallbackUpdateMessage:
		data.Flags &^= MessageFlagEphemeral
		_, err := i.EditResponseFiles(ctx, data, files...)
		return err
	}
	return ErrAutoDeferred
}
