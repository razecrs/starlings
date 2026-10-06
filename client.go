package starlings

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Version is the Discord API version starlings speaks.
const Version = "10"

// Client is a bot: a REST client and a gateway connection sharing one token.
//
// The zero Client is not usable - build one with New.
type Client struct {
	token     string // Authorization header value
	userToken bool   // token is a user (selfbot) token, sent without "Bot "
	id        ClientIdentity
	intents   Intent
	log       *slog.Logger
	rest      *rest

	// slots maps a Discord event name to its handlers. It is replaced
	// wholesale on registration so the gateway can read it without a lock.
	slots       atomic.Pointer[map[string]*eventSlot]
	rawHandlers atomic.Pointer[[]rawHandler]
	handlerMu   sync.Mutex
	handlerSeq  atomic.Uint64
	asyncEvents bool

	// Gateway session state.
	gw           gateway
	seq          atomic.Int64
	sessionID    atomic.Pointer[string]
	resumeURL    atomic.Pointer[string]
	self         atomic.Pointer[User]
	appID        atomic.Uint64
	shard        [2]int
	compress     bool
	autoShards   bool
	gatewayBase  string
	initialState *presence
	voiceMu      sync.Mutex
	voice        *voiceManager
	shardsMu     sync.RWMutex
	shards       []*Client
	shardRoot    *Client

	// State is the gateway-maintained cache. It is ready immediately and safe
	// to read from handlers and other goroutines.
	State   *State
	guard   *Guard
	starlog *Starlog

	// Prefix commands, registered with Command.
	prefix    string
	cmdMu     sync.RWMutex
	cmds      map[string]CommandFunc
	cmdHooked bool

	// Slash commands, registered with Slash.
	slashMu     sync.RWMutex
	slashes     map[string]slashEntry
	slashHooked bool

	ready     chan struct{}
	readyOnce sync.Once
	closeOnce sync.Once
	cancel    context.CancelFunc
}

// Option configures a Client. Pass them to New.
type Option func(*Client)

// WithIntents sets which families of events the gateway should send. Without
// it the bot receives only events that need no intent, such as interactions.
func WithIntents(i Intent) Option { return func(c *Client) { c.intents = i } }

// WithLogger replaces the default logger. Pass one writing to io.Discard to
// silence starlings entirely.
func WithLogger(l *slog.Logger) Option { return func(c *Client) { c.log = l } }

// WithHTTPClient supplies the http.Client used for REST calls. The default is
// tuned for Discord's API; replace it if you need a proxy or custom transport.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.rest.http = h
		}
	}
}

// WithShard tells Discord this connection handles one slice of the bot's
// guilds. Required once a bot is in more than around 2,500 guilds.
func WithShard(id, total int) Option {
	return func(c *Client) {
		c.shard = [2]int{id, total}
		c.autoShards = false
	}
}

// WithAutoSharding controls whether Run uses Discord's recommended shard
// count. It is enabled by default. WithShard selects one manual shard and
// disables automatic management.
func WithAutoSharding(enabled bool) Option {
	return func(c *Client) { c.autoShards = enabled }
}

// WithGatewayCompression controls Discord's transport compression. It is on
// by default because large READY and GUILD_CREATE events shrink dramatically.
// Disable it when inspecting raw gateway traffic with a proxy.
func WithGatewayCompression(enabled bool) Option {
	return func(c *Client) { c.compress = enabled }
}

// WithAsyncEvents runs application event handlers in their own goroutines.
// Internal cache maintenance remains ordered. Synchronous dispatch is the
// default because it is deterministic and naturally applies backpressure.
func WithAsyncEvents(enabled bool) Option {
	return func(c *Client) { c.asyncEvents = enabled }
}

// WithPrefix sets the character (or word) that triggers commands registered
// with Command. The default is DefaultPrefix, "!".
func WithPrefix(prefix string) Option { return func(c *Client) { c.prefix = prefix } }

// WithUserToken marks the token as a user (selfbot) token. Discord expects
// user tokens bare in the Authorization header, so the "Bot " prefix is not
// added - and an accidentally passed one is stripped.
func WithUserToken() Option { return func(c *Client) { c.userToken = true } }

// ClientIdentity is what the client tells Discord it is: the browser and device
// strings in the gateway identify payload, and the User-Agent on REST calls.
// An empty OS falls back to the runtime OS at identify time.
type ClientIdentity struct {
	OS        string
	Browser   string
	Device    string
	UserAgent string
}

// selfbotUserAgent is a desktop-Discord User-Agent. A user token behind a
// bot-looking User-Agent still works, but a real client's is less likely to
// trip a rate-limit or fraud check.
const selfbotUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) discord/1.0.9197 Chrome/128.0.6613.186 Electron/32.2.1 Safari/537.36"

var (
	botIdentity     = ClientIdentity{Browser: "starlings", Device: "starlings", UserAgent: userAgent}
	selfbotIdentity = ClientIdentity{Browser: "discord", Device: "discord", UserAgent: selfbotUserAgent}
)

// WithIdentity overrides what the client tells Discord it is: the browser and
// device strings in the identify payload, and the REST User-Agent.
func WithIdentity(id ClientIdentity) Option { return func(c *Client) { c.id = id } }

// SelfbotPace is the default minimum interval between two REST requests a
// Selfbot makes: spaced out enough to read as a person taking their time,
// tight enough that nothing feels sluggish.
const SelfbotPace = 2 * time.Second

// WithPacing sets the minimum interval between any two REST requests the
// client makes - a global gate in front of the rate limiter, applied one
// request at a time. New leaves it at zero (as fast as Discord's rate limits
// allow), and Selfbot starts at SelfbotPace, which is what keeps a user token
// from getting limited for spam. Pass WithPacing(0) to opt out of it.
func WithPacing(gap time.Duration) Option {
	return func(c *Client) { c.rest.pacer.set(gap) }
}

// Pacing reports the current minimum interval between REST requests.
func (c *Client) Pacing() time.Duration { return c.rest.pacer.Gap() }

// WithStatus sets the status and activity shown for the bot as soon as it
// connects.
func WithStatus(status Status, activity Activity) Option {
	return func(c *Client) {
		c.initialState = &presence{Status: string(status), Activities: []Activity{activity}}
	}
}

// New builds a client from a bot token. The "Bot " prefix Discord expects is
// added if it is not already there - a missing space there is a classic cause
// of 401s, so starlings normalises it for you.
//
// New does no I/O. Register handlers, then call Run.
func New(token string, opts ...Option) *Client {
	c := &Client{
		id:         botIdentity,
		log:        slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
		ready:      make(chan struct{}),
		State:      newState(),
		compress:   true,
		autoShards: true,
	}
	c.rest = newREST(c)

	empty := map[string]*eventSlot{}
	c.slots.Store(&empty)
	emptyRaw := []rawHandler{}
	c.rawHandlers.Store(&emptyRaw)

	for _, opt := range opts {
		opt(c)
	}
	if c.State != nil {
		c.State.guard = c.guard
		if c.guard != nil && c.State.Mode() == StateManual {
			c.log.Info("starlings guard: manual implementation active", "feature", "state")
		}
	}

	// Normalised after the options so WithUserToken can pick the header form.
	if c.userToken {
		c.token = normalizeUserToken(token)
	} else {
		c.token = normalizeToken(token)
	}
	c.installStateHandlers()
	return c
}

// Selfbot builds a client that acts as a user account, the same one-liner
// route New gives bot applications:
//
//	bot := starlings.Selfbot(os.Getenv("DC_TOKEN"))
//	bot.Command("ping", func(m *starlings.MessageCreate, _ []string) { m.Reply("pong") })
//	log.Fatal(bot.Run())
//
// The token is a user token - a logged-in account's token, not a bot
// application's. Selfbot is New plus everything the bot route
// would need you to click: the token goes out without the "Bot " prefix,
// every user intent is on, the connection identifies as desktop Discord,
// REST requests are paced one at a time (SelfbotPace), and your existing
// presence is left alone. Options passed here override those defaults, and
// everything else - Command, On, Reply, SendDM, voice - is the same API.
func Selfbot(token string, opts ...Option) *Client {
	c := New(token, append([]Option{
		WithUserToken(),
		WithIntents(IntentsSelfbot),
		WithIdentity(selfbotIdentity),
		WithPacing(SelfbotPace),
	}, opts...)...)
	if c.initialState == nil {
		// A real client always carries a presence in its identify. Without
		// one, Discord accepts later presence updates from the session but
		// never applies them, so a selfbot comes up online the way the
		// desktop client does. WithStatus still overrides this.
		c.initialState = &presence{Status: string(StatusOnline), Activities: []Activity{}}
	}
	return c
}

// normalizeToken accepts a raw token, "Bot <token>", or the "Bot<token>" form
// that results from forgetting the space, and returns the header value Discord
// wants.
func normalizeToken(token string) string {
	t := strings.TrimSpace(token)
	if rest, ok := strings.CutPrefix(t, "Bot "); ok {
		return "Bot " + strings.TrimSpace(rest)
	}
	if rest, ok := strings.CutPrefix(t, "Bot"); ok && rest != "" && !strings.ContainsAny(rest, " \t") {
		// "Bot<token>" - a missing space, which Discord rejects with a 401.
		return "Bot " + rest
	}
	return "Bot " + t
}

// normalizeUserToken returns a user (selfbot) token the way Discord wants it:
// bare, with no "Bot " prefix. An accidentally prefixed one is stripped.
func normalizeUserToken(token string) string {
	t := strings.TrimSpace(token)
	if rest, ok := strings.CutPrefix(t, "Bot "); ok {
		return strings.TrimSpace(rest)
	}
	if rest, ok := strings.CutPrefix(t, "Bot"); ok && rest != "" && !strings.ContainsAny(rest, " \t") {
		return rest
	}
	return t
}

// Run connects to the gateway and blocks until the process is interrupted with
// Ctrl-C or SIGTERM, then disconnects cleanly.
//
// This is the whole lifecycle for most bots:
//
//	bot := starlings.New(token, starlings.WithIntents(starlings.IntentGuildMessages))
//	bot.On(func(m *starlings.MessageCreate) { m.Reply("hi") })
//	log.Fatal(bot.Run())
func (c *Client) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return c.RunContext(ctx)
}

// RunContext connects to the gateway and blocks until ctx is cancelled or the
// connection fails in a way that cannot be retried.
//
// It reconnects on its own through network drops, Discord asking for a
// reconnect, and expired sessions, resuming where it can and starting a fresh
// session where it cannot. It gives up only on a configuration error - a bad
// token, or intents the bot has not been granted - because retrying those
// would fail identically forever.
func (c *Client) RunContext(ctx context.Context) error {
	if c.starlog != nil {
		c.starlog.Attach(c)
		c.starlog.Start(ctx)
		defer c.starlog.Close()
	}
	for _, w := range c.intentWarnings() {
		c.log.Warn("starlings: " + w)
	}

	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	defer cancel()
	if !c.autoShards || c.userToken {
		return c.gw.run(ctx, c)
	}
	return c.runAutoSharded(ctx)
}

// WaitReady blocks until the gateway has sent READY, or ctx is cancelled.
// Useful when something has to happen right after connecting but you are not
// driving it from a Ready handler.
func (c *Client) WaitReady(ctx context.Context) error {
	select {
	case <-c.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close disconnects from the gateway. RunContext returns after it.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.voiceMu.Lock()
		voice := c.voice
		c.voiceMu.Unlock()
		if voice != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			voice.close(ctx)
			cancel()
		}
		if c.cancel != nil {
			c.cancel()
		}
	})
	return nil
}

// Self returns the bot's own user, or nil before READY has arrived.
func (c *Client) Self() *User { return c.self.Load() }

// ApplicationID returns the bot's application ID, which the slash command
// endpoints need. It is zero before READY.
func (c *Client) ApplicationID() Snowflake { return Snowflake(c.appID.Load()) }

// Logger returns the client's logger, so bots can log through the same handler.
func (c *Client) Logger() *slog.Logger { return c.log }

// gatewayLogger adds the shard once at the rare lifecycle log sites. It is
// deliberately not kept as another Client field so non-Starlog handlers see
// the same useful structured attribute without changing their setup.
func (c *Client) gatewayLogger() *slog.Logger {
	return c.log.With("shard", c.shard[0])
}

// Status is the presence shown next to the bot's name.
type Status string

const (
	StatusOnline    Status = "online"
	StatusIdle      Status = "idle"
	StatusDND       Status = "dnd"
	StatusInvisible Status = "invisible"
	StatusOffline   Status = "offline"
)

// ActivityType selects the verb Discord shows before an activity's name.
type ActivityType int

const (
	ActivityPlaying ActivityType = iota
	ActivityStreaming
	ActivityListening
	ActivityWatching
	ActivityCustom
	ActivityCompeting
)

// Activity is the line under a bot's name in the member list.
type Activity struct {
	Name  string       `json:"name"`
	Type  ActivityType `json:"type"`
	URL   string       `json:"url,omitzero"` // required for ActivityStreaming
	State string       `json:"state,omitzero"`
}

// Playing builds the most common activity.
func Playing(name string) Activity { return Activity{Name: name, Type: ActivityPlaying} }

// Watching builds a "Watching ..." activity.
func Watching(name string) Activity { return Activity{Name: name, Type: ActivityWatching} }

// Listening builds a "Listening to ..." activity.
func Listening(name string) Activity { return Activity{Name: name, Type: ActivityListening} }

// presence is the gateway payload for a presence update.
type presence struct {
	Since      int64      `json:"since"`
	Activities []Activity `json:"activities"`
	Status     string     `json:"status"`
	AFK        bool       `json:"afk"`
}

// defaultHTTPClient is tuned for a long-lived connection to one host: keep
// connections warm, and cap how long a stuck request can hang.
func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          32,
			MaxIdleConnsPerHost:   32,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
	}
}

// discardLogger throws every log line away, for tests and for bots that do
// their own logging.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
