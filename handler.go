package starlings

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"
)

// UserError is an error whose message is meant for the person who used the
// command. When an error-returning handler returns one, Starlings shows the
// message to that person privately. Every other error is logged and replaced
// with a generic message, so internal details never reach Discord.
type UserError struct {
	Message string
}

func (e *UserError) Error() string { return e.Message }

// UserMessage returns the text shown to the user.
func (e *UserError) UserMessage() string { return e.Message }

// UserErrorf builds a UserError with fmt.Sprintf formatting:
//
//	return starlings.UserErrorf("Warning #%d does not exist.", id)
func UserErrorf(format string, args ...any) error {
	return &UserError{Message: fmt.Sprintf(format, args...)}
}

// userFacing is implemented by errors whose message is safe to show to the
// user, such as UserError and ModerationError.
type userFacing interface{ UserMessage() string }

// UserMessage returns the denial as shown to the user.
func (e *ModerationError) UserMessage() string { return e.Reason.String() }

// genericFailure is shown for errors that are not meant for the user.
const genericFailure = "Something went wrong while running that. It has been logged."

// HandlerFunc is an interaction handler that reports failure by returning an
// error. See UserError for how errors are shown.
type HandlerFunc func(*InteractionCreate) error

// Run sets an error-returning handler for the command, replacing the one
// given to Slash:
//
//	bot.Slash("kick", "Kick a member", nil).Require(starlings.PermissionKickMembers).
//		Run(func(i *starlings.InteractionCreate) error { ... })
func (r *SlashRoute) Run(fn HandlerFunc) *SlashRoute {
	if fn == nil {
		panic("starlings: nil handler")
	}
	return r.update(func(e *slashEntry) {
		e.fn = func(i *InteractionCreate) { i.c.runHandler(e.def.Name, i, fn) }
	})
}

// Require limits the command to members with every supplied permission. It
// sets Discord's default member permissions, and also checks the permissions
// Discord reports with each use, because server admins can override the
// default. Members without them get a private message naming what is missing.
func (r *SlashRoute) Require(permissions Permissions) *SlashRoute {
	r.Permissions(permissions)
	return r.update(func(e *slashEntry) { e.require = permissions })
}

// BotNeeds checks, before the handler runs, that the bot has every supplied
// permission in the channel where the command was used. Without them the user
// gets a private message naming what is missing, instead of a failed request
// halfway through the handler.
func (r *SlashRoute) BotNeeds(permissions Permissions) *SlashRoute {
	return r.update(func(e *slashEntry) { e.botNeeds = permissions })
}

// checkPermissions answers the interaction and reports false when the member
// or the bot lacks a permission the route requires.
func (e *slashEntry) checkPermissions(i *InteractionCreate) bool {
	if e.require != 0 {
		var have Permissions
		if i.Member != nil {
			have = i.Member.Permissions
		}
		if missing := have.Missing(e.require); missing != 0 && i.Member != nil {
			i.c.replyFailure(i, "You need "+missing.HumanString()+" to use this.")
			return false
		}
	}
	if e.botNeeds != 0 && i.GuildID != 0 {
		if missing := i.AppPermissions.Missing(e.botNeeds); missing != 0 {
			i.c.replyFailure(i, "I need "+missing.HumanString()+" here to do that.")
			return false
		}
	}
	return true
}

// runHandler calls an error-returning handler and reports its result.
func (c *Client) runHandler(name string, i *InteractionCreate, fn HandlerFunc) {
	err := func() (err error) {
		defer func() {
			if v := recover(); v != nil {
				err = fmt.Errorf("panic: %v\n%s", v, debug.Stack())
			}
		}()
		return fn(i)
	}()
	if err == nil {
		return
	}
	var user userFacing
	if errors.As(err, &user) {
		c.replyFailure(i, user.UserMessage())
		return
	}
	c.log.Error("starlings: command failed", "command", name, "user", i.Invoker().ID, "err", err)
	c.replyFailure(i, genericFailure)
}

// replyFailure shows text privately to the user, whatever state the
// interaction is in.
func (c *Client) replyFailure(i *InteractionCreate, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	data := InteractionResponseData{Content: text, Flags: MessageFlagEphemeral, AllowedMentions: NoMentions()}
	err := i.respond(ctx, InteractionResponse{Type: CallbackChannelMessageWithSource, Data: &data}, nil)
	if errors.Is(err, ErrInteractionAlreadyAnswered) {
		if i.Deferred() && atomic.LoadInt32(&i.finished) == 0 {
			// Replace the "thinking" placeholder rather than leave it spinning.
			_ = i.DeleteResponse(ctx)
		}
		_, err = i.Followup(ctx, data)
	}
	if err != nil {
		c.log.Warn("starlings: could not report a command failure", "err", err)
	}
}
