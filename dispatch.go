package starlings

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"sync/atomic"
	"time"
)

// RawEvent exposes the untouched data object from any gateway dispatch. Data
// is owned by the event and remains valid after the handler returns.
type RawEvent struct {
	Name   string
	Data   jsontext.Value
	Client *Client
}

type rawHandler struct {
	id uint64
	fn func(*RawEvent)
}

func (c *Client) emitSynthetic(event Event) {
	name := event.eventName()
	if !c.wants(name) {
		return
	}
	data, err := json.Marshal(event)
	if err != nil {
		c.log.Error("starlings: encoding synthetic event", "event", name, "err", err)
		return
	}
	c.dispatch(name, data)
}

// OnRaw listens to every gateway dispatch, including events Starlings does not
// yet model. It returns an idempotent unsubscribe function.
func (c *Client) OnRaw(fn func(*RawEvent)) func() {
	c = c.rootClient()
	id := c.handlerSeq.Add(1)
	c.handlerMu.Lock()
	old := c.rawHandlers.Load()
	next := append([]rawHandler(nil), (*old)...)
	next = append(next, rawHandler{id: id, fn: fn})
	c.rawHandlers.Store(&next)
	c.handlerMu.Unlock()
	var removed atomic.Bool
	return func() {
		if !removed.CompareAndSwap(false, true) {
			return
		}
		c.handlerMu.Lock()
		old := c.rawHandlers.Load()
		next := make([]rawHandler, 0, len(*old)-1)
		for _, h := range *old {
			if h.id != id {
				next = append(next, h)
			}
		}
		c.rawHandlers.Store(&next)
		c.handlerMu.Unlock()
	}
}

func (c *Client) wants(name string) bool {
	return c.slotFor(name) != nil || len(*c.rootClient().rawHandlers.Load()) > 0 || internalEvent(name)
}

// eventSlot holds every handler registered for one Discord event name, plus a
// closure that decodes the payload and calls them.
//
// The closure is rebuilt whenever a handler is added, which keeps the hot path
// free of reflection: dispatching an event is a map lookup, one allocation for
// the decoded event, and a direct call through a typed slice.
type eventSlot struct {
	handlers []any // []func(*E), kept so the closure can be rebuilt
	ids      []uint64
	dispatch func(c *Client, data jsontext.Value)
	internal int // leading handlers installed by Starlings itself
}

// On registers a handler for a gateway event. The event type determines which
// Discord event it listens to, so there is no name to get wrong:
//
//	starlings.On(bot, func(m *starlings.MessageCreate) {
//		m.Reply("hello")
//	})
//
// Handlers for the same event run in registration order. An event is skipped
// only when neither application handlers nor an enabled internal consumer,
// such as the state cache or voice bookkeeping, needs it.
//
// Client.On is the same thing with lighter syntax; use this form when you want
// the compiler to check the event type.
func On[E Event](c *Client, h func(*E)) {
	registerEvent(c, h, false)
}

// Listen registers a handler and returns an idempotent function that removes
// it. Use On when the handler lives for the entire client lifetime.
func Listen[E Event](c *Client, h func(*E)) func() {
	return registerEvent(c, h, false)
}

// Once registers a handler that automatically removes itself before its first
// invocation, so even concurrent dispatch can call it at most once.
func Once[E Event](c *Client, h func(*E)) func() {
	var fired atomic.Bool
	var off func()
	ready := make(chan struct{})
	off = Listen(c, func(e *E) {
		<-ready
		if !fired.CompareAndSwap(false, true) {
			return
		}
		off()
		h(e)
	})
	close(ready)
	return off
}

func internalOn[E Event](c *Client, h func(*E)) {
	registerEvent(c, h, true)
}

func registerEvent[E Event](c *Client, h func(*E), internal bool) func() {
	if c.shardRoot != nil {
		c = c.shardRoot
	}
	var zero E
	name := zero.eventName()

	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()

	// Copy-on-write: the gateway reads the map without locking, so it is
	// replaced wholesale rather than mutated.
	old := c.slots.Load()
	next := make(map[string]*eventSlot, len(*old)+1)
	for k, v := range *old {
		next[k] = v
	}

	var typed []func(*E)
	var ids []uint64
	internalCount := 0
	if prev := next[name]; prev != nil {
		typed = make([]func(*E), 0, len(prev.handlers)+1)
		for _, x := range prev.handlers {
			typed = append(typed, x.(func(*E)))
		}
		internalCount = prev.internal
		ids = append(ids, prev.ids...)
	}
	id := uint64(0)
	if !internal {
		id = c.handlerSeq.Add(1)
	}
	if internal {
		// Internal handlers must remain a leading block even when an Option
		// registered an application handler while New was still being built.
		typed = append(typed, nil)
		copy(typed[internalCount+1:], typed[internalCount:len(typed)-1])
		typed[internalCount] = h
		ids = append(ids, 0)
		copy(ids[internalCount+1:], ids[internalCount:len(ids)-1])
		ids[internalCount] = 0
		internalCount++
	} else {
		typed = append(typed, h)
		ids = append(ids, id)
	}

	boxed := make([]any, len(typed))
	for i, f := range typed {
		boxed[i] = f
	}

	next[name] = &eventSlot{
		handlers: boxed,
		ids:      ids,
		internal: internalCount,
		dispatch: func(c *Client, data jsontext.Value) {
			e := new(E)
			if err := json.Unmarshal(data, e); err != nil {
				c.log.Error("starlings: decoding event", "event", name, "err", err)
				return
			}
			if b, ok := any(e).(binder); ok {
				b.bind(c)
			}
			for i, f := range typed {
				if i < internalCount {
					f(e)
				} else if !c.asyncEvents {
					invokeApplicationHandler(c, name, f, e)
				} else {
					go invokeApplicationHandler(c, name, f, e)
				}
			}
		},
	}

	c.slots.Store(&next)
	c.shardsMu.RLock()
	for _, shard := range c.shards {
		if shard != c {
			shard.slots.Store(&next)
		}
	}
	c.shardsMu.RUnlock()

	if internal {
		return func() {}
	}
	var removed atomic.Bool
	return func() {
		if removed.CompareAndSwap(false, true) {
			removeEvent[E](c, name, id)
		}
	}
}

func invokeApplicationHandler[E Event](c *Client, name string, handler func(*E), event *E) {
	if c.guard == nil {
		handler(event)
		return
	}
	started := time.Now()
	handler(event)
	c.guard.observe("handler."+name, GuardApplication, time.Since(started), nil)
}

func removeEvent[E Event](c *Client, name string, id uint64) {
	if c.shardRoot != nil {
		c = c.shardRoot
	}
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	old := c.slots.Load()
	prev := (*old)[name]
	if prev == nil {
		return
	}
	idx := -1
	for i, candidate := range prev.ids {
		if candidate == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	typed := make([]func(*E), 0, len(prev.handlers)-1)
	ids := make([]uint64, 0, len(prev.ids)-1)
	for i, boxed := range prev.handlers {
		if i != idx {
			typed = append(typed, boxed.(func(*E)))
			ids = append(ids, prev.ids[i])
		}
	}
	next := make(map[string]*eventSlot, len(*old))
	for k, v := range *old {
		next[k] = v
	}
	if len(typed) == 0 {
		delete(next, name)
	} else {
		boxed := make([]any, len(typed))
		for i, f := range typed {
			boxed[i] = f
		}
		internalCount := prev.internal
		next[name] = &eventSlot{handlers: boxed, ids: ids, internal: internalCount, dispatch: func(c *Client, data jsontext.Value) {
			e := new(E)
			if err := json.Unmarshal(data, e); err != nil {
				c.log.Error("starlings: decoding event", "event", name, "err", err)
				return
			}
			if b, ok := any(e).(binder); ok {
				b.bind(c)
			}
			for i, f := range typed {
				if i < internalCount {
					f(e)
				} else if !c.asyncEvents {
					invokeApplicationHandler(c, name, f, e)
				} else {
					go invokeApplicationHandler(c, name, f, e)
				}
			}
		}}
	}
	c.slots.Store(&next)
	c.shardsMu.RLock()
	for _, shard := range c.shards {
		if shard != c {
			shard.slots.Store(&next)
		}
	}
	c.shardsMu.RUnlock()
}

// On registers a handler for the client's lifetime. Listen is the same
// convenient inferred form and returns a function that removes the handler.
func (c *Client) On(handler any) { _ = c.Listen(handler) }

// Listen registers a handler for a gateway event, inferring which event from
// the handler's argument type:
//
//	bot.On(func(m *starlings.MessageCreate) { m.Reply("hi") })
//	bot.On(func(r *starlings.Ready)         { log.Println("online as", r.User.Tag()) })
//
// It panics if handler is not a func taking exactly one pointer-to-event
// argument, because that is a mistake in the program rather than a runtime
// condition worth returning.
//
// It resolves the type once, at registration, and dispatches through the same
// reflection-free path as the generic starlings.Listen.
func (c *Client) Listen(handler any) func() {
	switch h := handler.(type) {
	case func(*ApplicationCommandPermissionsUpdate):
		return Listen(c, h)
	case func(*AutoModerationActionExecution):
		return Listen(c, h)
	case func(*AutoModerationRuleCreate):
		return Listen(c, h)
	case func(*AutoModerationRuleDelete):
		return Listen(c, h)
	case func(*AutoModerationRuleUpdate):
		return Listen(c, h)
	case func(*ChannelCreate):
		return Listen(c, h)
	case func(*Connect):
		return Listen(c, h)
	case func(*Disconnect):
		return Listen(c, h)
	case func(*ChannelDelete):
		return Listen(c, h)
	case func(*ChannelInfo):
		return Listen(c, h)
	case func(*VoiceChannelStatusUpdate):
		return Listen(c, h)
	case func(*VoiceChannelStartTimeUpdate):
		return Listen(c, h)
	case func(*ChannelPinsUpdate):
		return Listen(c, h)
	case func(*ChannelUpdate):
		return Listen(c, h)
	case func(*EntitlementCreate):
		return Listen(c, h)
	case func(*EntitlementDelete):
		return Listen(c, h)
	case func(*EntitlementUpdate):
		return Listen(c, h)
	case func(*GuildAuditLogEntryCreate):
		return Listen(c, h)
	case func(*GuildBanAdd):
		return Listen(c, h)
	case func(*GuildBanRemove):
		return Listen(c, h)
	case func(*GuildCreate):
		return Listen(c, h)
	case func(*GuildDelete):
		return Listen(c, h)
	case func(*GuildEmojisUpdate):
		return Listen(c, h)
	case func(*GuildIntegrationsUpdate):
		return Listen(c, h)
	case func(*GuildMemberAdd):
		return Listen(c, h)
	case func(*GuildMemberRemove):
		return Listen(c, h)
	case func(*GuildMemberUpdate):
		return Listen(c, h)
	case func(*GuildMembersChunk):
		return Listen(c, h)
	case func(*GuildRoleCreate):
		return Listen(c, h)
	case func(*GuildRoleDelete):
		return Listen(c, h)
	case func(*GuildRoleUpdate):
		return Listen(c, h)
	case func(*GuildScheduledEventCreate):
		return Listen(c, h)
	case func(*GuildScheduledEventDelete):
		return Listen(c, h)
	case func(*GuildScheduledEventUpdate):
		return Listen(c, h)
	case func(*GuildScheduledEventUserAdd):
		return Listen(c, h)
	case func(*GuildScheduledEventUserRemove):
		return Listen(c, h)
	case func(*GuildSoundboardSoundCreate):
		return Listen(c, h)
	case func(*GuildSoundboardSoundDelete):
		return Listen(c, h)
	case func(*GuildSoundboardSoundUpdate):
		return Listen(c, h)
	case func(*GuildSoundboardSoundsUpdate):
		return Listen(c, h)
	case func(*GuildStickersUpdate):
		return Listen(c, h)
	case func(*GuildUpdate):
		return Listen(c, h)
	case func(*IntegrationCreate):
		return Listen(c, h)
	case func(*IntegrationDelete):
		return Listen(c, h)
	case func(*IntegrationUpdate):
		return Listen(c, h)
	case func(*InteractionCreate):
		return Listen(c, h)
	case func(*InviteCreate):
		return Listen(c, h)
	case func(*InviteDelete):
		return Listen(c, h)
	case func(*MessageCreate):
		return Listen(c, h)
	case func(*MessageDelete):
		return Listen(c, h)
	case func(*MessageDeleteBulk):
		return Listen(c, h)
	case func(*MessagePollVoteAdd):
		return Listen(c, h)
	case func(*MessagePollVoteRemove):
		return Listen(c, h)
	case func(*MessageReactionAdd):
		return Listen(c, h)
	case func(*MessageReactionRemove):
		return Listen(c, h)
	case func(*MessageReactionRemoveAll):
		return Listen(c, h)
	case func(*MessageReactionRemoveEmoji):
		return Listen(c, h)
	case func(*MessageUpdate):
		return Listen(c, h)
	case func(*PresenceUpdate):
		return Listen(c, h)
	case func(*PresencesReplace):
		return Listen(c, h)
	case func(*RateLimited):
		return Listen(c, h)
	case func(*RateLimit):
		return Listen(c, h)
	case func(*Ready):
		return Listen(c, h)
	case func(*Resumed):
		return Listen(c, h)
	case func(*SoundboardSounds):
		return Listen(c, h)
	case func(*SubscriptionCreate):
		return Listen(c, h)
	case func(*SubscriptionDelete):
		return Listen(c, h)
	case func(*SubscriptionUpdate):
		return Listen(c, h)
	case func(*StageInstanceCreate):
		return Listen(c, h)
	case func(*StageInstanceDelete):
		return Listen(c, h)
	case func(*StageInstanceUpdate):
		return Listen(c, h)
	case func(*ThreadCreate):
		return Listen(c, h)
	case func(*ThreadDelete):
		return Listen(c, h)
	case func(*ThreadListSync):
		return Listen(c, h)
	case func(*ThreadMemberUpdate):
		return Listen(c, h)
	case func(*ThreadMembersUpdate):
		return Listen(c, h)
	case func(*ThreadUpdate):
		return Listen(c, h)
	case func(*TypingStart):
		return Listen(c, h)
	case func(*UserUpdate):
		return Listen(c, h)
	case func(*VoiceChannelEffectSend):
		return Listen(c, h)
	case func(*VoiceServerUpdate):
		return Listen(c, h)
	case func(*VoiceStateUpdate):
		return Listen(c, h)
	case func(*WebhooksUpdate):
		return Listen(c, h)
	default:
		panic("starlings: On expects a func taking one *starlings event pointer, got " + typeName(handler))
	}
}

// wants reports whether any handler is registered for a Discord event name.
// The gateway calls this before decoding, so unhandled events cost one map
// lookup and nothing else.
func (c *Client) slotFor(name string) *eventSlot {
	return (*c.slots.Load())[name]
}
