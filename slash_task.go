package starlings

import (
	"context"
	"time"
)

// TaskFunc does bounded background work for a slash command. Starlings sends
// the acknowledgement before calling it and edits the original response with
// its result. Return an error to log it and show a generic failure message.
// Use Slash for manual response ownership, modals, files, or multiple replies.
type TaskFunc func(context.Context, *InteractionCreate) (string, error)

type taskRunner struct {
	ctx    context.Context
	cancel context.CancelFunc
	slots  chan struct{}
}

// WithTaskConcurrency caps in-flight SlashTask handlers across all shards.
// The default is 32. Busy requests receive an ephemeral reply instead of
// entering an unbounded queue. A nonpositive limit is a programming error.
func WithTaskConcurrency(limit int) Option {
	if limit < 1 {
		panic("starlings: task concurrency must be positive")
	}
	return func(c *Client) { c.taskLimit = limit }
}

// SlashTask registers a command that acknowledges immediately, runs work
// outside the gateway loop, and sends the returned text without mentions.
// Tasks get a one-minute deadline by default and cancellation on shutdown.
// The callback must honor its context; Go cannot forcibly stop user code.
func (c *Client) SlashTask(name, description string, fn TaskFunc, options ...CommandOption) *SlashRoute {
	if fn == nil {
		panic("starlings: nil task handler")
	}
	c = c.rootClient()
	c.slashMu.Lock()
	if c.tasks == nil {
		ctx, cancel := context.WithCancel(context.Background())
		limit := c.taskLimit
		if limit == 0 {
			limit = 32
		}
		c.tasks = &taskRunner{ctx: ctx, cancel: cancel, slots: make(chan struct{}, limit)}
	}
	c.slashMu.Unlock()
	return c.Slash(name, description, nil, options...).update(func(e *slashEntry) {
		e.task, e.timeout = fn, time.Minute
	})
}

// Ephemeral makes SlashTask's deferred response private to the invoker.
// Manual Slash handlers choose visibility on their own response methods.
func (r *SlashRoute) Ephemeral() *SlashRoute {
	return r.update(func(e *slashEntry) { e.private = true })
}

// Timeout selects the deadline passed to SlashTask. It must be positive and
// less than the interaction token's fifteen-minute lifetime.
func (r *SlashRoute) Timeout(timeout time.Duration) *SlashRoute {
	if timeout <= 0 || timeout >= 15*time.Minute {
		panic("starlings: task timeout must be between zero and fifteen minutes")
	}
	return r.update(func(e *slashEntry) { e.timeout = timeout })
}

func (c *Client) stopTasks() {
	c = c.rootClient()
	c.slashMu.RLock()
	runner := c.tasks
	c.slashMu.RUnlock()
	if runner != nil {
		runner.cancel()
	}
}

func (c *Client) runTask(runner *taskRunner, entry slashEntry, i *InteractionCreate) {
	if runner.ctx.Err() != nil {
		return
	}
	select {
	case runner.slots <- struct{}{}:
	default:
		if err := i.ReplyEphemeral("I'm busy right now. Try again shortly."); err != nil {
			c.log.Error("starlings: replying to busy task", "err", err)
		}
		return
	}
	ack, cancel := context.WithTimeout(runner.ctx, 2*time.Second)
	data := &InteractionResponseData{}
	if entry.private {
		data.Flags = MessageFlagEphemeral
	}
	err := i.Respond(ack, InteractionResponse{Type: CallbackDeferredChannelMessage, Data: data})
	cancel()
	if err != nil {
		<-runner.slots
		c.log.Error("starlings: acknowledging task", "command", entry.def.Name, "err", err)
		return
	}
	go func() {
		defer func() { <-runner.slots }()
		ctx, cancel := context.WithTimeout(runner.ctx, entry.timeout)
		defer cancel()
		var started time.Time
		if c.guard != nil {
			started = time.Now()
		}
		content, err := entry.task(ctx, i)
		if c.guard != nil {
			c.guard.Observe("task."+entry.def.Name, GuardApplication, time.Since(started), err)
		}
		if err != nil || ctx.Err() != nil {
			if err == nil {
				err = ctx.Err()
			}
			c.log.Error("starlings: task failed", "command", entry.def.Name, "err", err)
			content = "That didn't finish successfully. Please try again."
		}
		if runner.ctx.Err() != nil {
			return
		}
		// A task timeout should not also prevent the user seeing its failure.
		responseCtx, stop := context.WithTimeout(runner.ctx, 10*time.Second)
		defer stop()
		_, err = i.EditResponse(responseCtx, InteractionResponseData{Content: content, AllowedMentions: NoMentions()})
		if err != nil {
			c.log.Error("starlings: completing task", "command", entry.def.Name, "err", err)
		}
	}()
}
