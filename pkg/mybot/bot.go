// SPDX-License-Identifier: MIT

package mybot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mycroblog/my/pkg/mykit"
)

// Mode is how a bot receives its notifications.
type Mode int

const (
	// Streaming receives them from the event stream (GET /streaming) and catches up with the notifications after
	// every connection. It is the default.
	Streaming Mode = iota
	// Polling reads the notifications every Config.PollInterval, for networks whose proxies do not pass an event
	// stream.
	Polling
)

// The defaults of Config.
const (
	DefaultPollInterval       = time.Minute
	DefaultCatchUpLimit       = 24 * time.Hour
	DefaultProcessedRetention = 7 * 24 * time.Hour
	DefaultConcurrency        = 4
	DefaultShutdownGrace      = 10 * time.Second
)

// defaultUserAgent is the product token of the library in User-Agent, before mykit's own.
const defaultUserAgent = "mybot-go"

// Config configures a bot. The zero value of every field but Server and the token is its default.
type Config struct {
	// Server is the origin of the server, such as "https://example.com".
	Server string
	// HTTPClient sends the requests. Nil means a new http.Client. It must not have a Timeout, which would end
	// every event stream.
	HTTPClient *http.Client

	// OAuth and RefreshToken make the bot renew its access token with the refresh token (mykit's
	// RefreshingTokenSource; ADR-0094 §7): the token that the server issued to the bot's own account for an app
	// registered on its settings page, or that an authorization code was exchanged for. Every renewal rotates the
	// refresh token, which the bot saves in the Store; when the Store holds one, the bot starts from it rather
	// than from RefreshToken, which the server has seen used.
	OAuth        *mykit.OAuthConfig
	RefreshToken string
	// TokenSource, when OAuth is nil, gives the access token: mykit.StaticToken for tests and short runs, or a
	// source of one's own. The bot does not save its tokens.
	TokenSource mykit.TokenSource

	// Clock tells the time and waits, for the scheduled jobs, the send budget, and the reply limits. Nil means
	// the clock of the system; a test replaces it to move the time on.
	Clock Clock

	// Store keeps the state of the bot across restarts. Nil means a FileStore at DefaultStatePath.
	Store Store
	// Logger receives the log of the bot. Nil means slog.Default().
	Logger *slog.Logger
	// UserAgent is the product of the bot, such as "echobot/1.0", sent before the library's own in User-Agent.
	UserAgent string
	// AcceptLanguage is sent as Accept-Language, for the language of the server's problems.
	AcceptLanguage string

	// Mode is Streaming (the default) or Polling.
	Mode Mode
	// PollInterval is the interval of Polling. Zero means DefaultPollInterval.
	PollInterval time.Duration
	// CatchUpLimit is how old a notification the catch-up still handles: after a long stop, older ones are
	// skipped. Nil means DefaultCatchUpLimit, and zero turns the catch-up off (ADR-0094 §3), such as
	// CatchUpLimit: new(time.Duration(0)).
	CatchUpLimit *time.Duration
	// ProcessedRetention is how long the processed notifications are remembered, longer than CatchUpLimit. Zero
	// means DefaultProcessedRetention.
	ProcessedRetention time.Duration
	// Concurrency is how many handlers run at once; the events of one sender run one after the other. Zero
	// means DefaultConcurrency.
	Concurrency int
	// ShutdownGrace is how long Run waits for the running handlers when it stops; then it cancels their
	// contexts, and their events are handled again by the next start. Zero means DefaultShutdownGrace.
	ShutdownGrace time.Duration

	// SetBotFlag turns the bot flag of the account on when it is off (with the scope write:accounts), instead of
	// failing to start.
	SetBotFlag bool
	// AllowNonBot runs the bot on an account without the bot flag. By default the bot refuses to start there,
	// since the flag is what tells people and other servers that a program posts.
	AllowNonBot bool
	// RespondToBots hands the events of accounts with the bot flag to the handlers, which by default skip them.
	RespondToBots bool
	// ReplyLimits are the most replies to one sender, each a number in a window. Nil means DefaultReplyLimits.
	ReplyLimits []Rate
	// DisableReplyLimits lifts the reply limits.
	DisableReplyLimits bool
	// PostRate and WriteRate are the send budget of the posts and of the other writes. Zero means
	// DefaultPostRate and DefaultWriteRate.
	PostRate, WriteRate Rate
	// MaxSendWait is the longest a write waits for the send budget; a longer wait drops it. Zero means
	// DefaultMaxSendWait.
	MaxSendWait time.Duration

	// DisableHelp turns off the answer of the command help.
	DisableHelp bool
	// Help formats the answer of help. Nil means DefaultHelp.
	Help func([]Command) string
	// NoDefaultMiddleware leaves DefaultMiddleware out of the chain, for a bot that builds its own with Use.
	NoDefaultMiddleware bool
}

// withDefaults fills in the defaults of the zero fields.
func (c Config) withDefaults() Config {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.PollInterval <= 0 {
		c.PollInterval = DefaultPollInterval
	}
	if c.CatchUpLimit == nil {
		c.CatchUpLimit = new(DefaultCatchUpLimit)
	}
	if c.ProcessedRetention <= 0 {
		c.ProcessedRetention = DefaultProcessedRetention
	}
	if c.Concurrency <= 0 {
		c.Concurrency = DefaultConcurrency
	}
	if c.ShutdownGrace <= 0 {
		c.ShutdownGrace = DefaultShutdownGrace
	}
	switch {
	case c.DisableReplyLimits:
		c.ReplyLimits = nil
	case c.ReplyLimits == nil:
		c.ReplyLimits = DefaultReplyLimits
	}
	if c.PostRate == (Rate{}) {
		c.PostRate = DefaultPostRate
	}
	if c.WriteRate == (Rate{}) {
		c.WriteRate = DefaultWriteRate
	}
	if c.MaxSendWait <= 0 {
		c.MaxSendWait = DefaultMaxSendWait
	}
	if c.Help == nil {
		c.Help = DefaultHelp
	}
	return c
}

// Errors of starting a bot.
var (
	// ErrNotBot is the error of Run on an account without the bot flag, unless Config.SetBotFlag or
	// Config.AllowNonBot.
	ErrNotBot = errors.New("mybot: the account has no bot flag: set it, or set Config.SetBotFlag or Config.AllowNonBot")
	// ErrRunning is the error of a second Run of a bot.
	ErrRunning = errors.New("mybot: the bot runs already")
)

// ScopeError is the error of Run when the access token lacks scopes that the bot needs.
type ScopeError struct {
	// Missing are the scopes the token lacks.
	Missing []string
}

func (e *ScopeError) Error() string {
	return "mybot: the access token lacks the scopes " + strings.Join(e.Missing, " ")
}

// Bot is a bot: the handlers of its events, and Run, which receives the events and hands them to the handlers
// until it stops. Register the handlers before Run.
type Bot struct {
	cfg        Config
	h          handlers
	every      []*everyJob
	middleware []Middleware
	logger     *slog.Logger

	store   Store
	budget  *budget
	replies *replyLimiter
	tracker *tracker
	now     func() time.Time
	sleep   func(ctx context.Context, d time.Duration) error

	running sync.Mutex
	started bool

	// Set by Run.
	api        *mykit.API
	me         *mykit.CredentialAccount
	dispatcher *dispatcher
	// abandoned is set when the grace period of the shutdown is over: the handlers still running record nothing.
	abandonMu sync.RWMutex
	abandoned bool
}

// New returns the bot that cfg configures.
func New(cfg Config) (*Bot, error) {
	if cfg.OAuth == nil && cfg.TokenSource == nil {
		return nil, errors.New("mybot: Config needs OAuth and RefreshToken, or a TokenSource")
	}
	if cfg.Server == "" {
		return nil, errors.New("mybot: Config.Server is empty")
	}
	cfg = cfg.withDefaults()
	store := cfg.Store
	if store == nil {
		fs, err := NewFileStore(DefaultStatePath)
		if err != nil {
			return nil, err
		}
		store = fs
	}
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	return &Bot{
		cfg:     cfg,
		logger:  cfg.Logger,
		store:   store,
		budget:  newBudget(cfg.PostRate, cfg.WriteRate),
		replies: newReplyLimiter(cfg.ReplyLimits),
		tracker: newTracker(),
		now:     clock.Now,
		sleep:   clock.Sleep,
	}, nil
}

// Store is the Store of the bot.
func (b *Bot) Store() Store { return b.store }

// requiredScopes are the scopes the bot needs: its own account (verifyCredentials), the notifications and the
// event stream, and posting (ADR-0094 §7); and write:accounts to set the bot flag.
func (b *Bot) requiredScopes() []string {
	s := []string{"read:accounts", "read:notifications", "read:statuses", "write:statuses"}
	if b.cfg.SetBotFlag {
		s = append(s, "write:accounts")
	}
	return s
}

// tokenSource is the TokenSource of the Config: a RefreshingTokenSource that saves every rotated refresh token in
// the Store, starting from the one the Store holds.
func (b *Bot) tokenSource(ctx context.Context) (mykit.TokenSource, error) {
	if b.cfg.OAuth == nil {
		return b.cfg.TokenSource, nil
	}
	rt, err := b.store.RefreshToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("mybot: read the refresh token from the store: %w", err)
	}
	if rt == "" {
		rt = b.cfg.RefreshToken
	}
	if rt == "" {
		return nil, errors.New("mybot: no refresh token: set Config.RefreshToken")
	}
	oauth := *b.cfg.OAuth
	if oauth.Server == "" {
		oauth.Server = b.cfg.Server
	}
	if oauth.HTTPClient == nil {
		oauth.HTTPClient = b.cfg.HTTPClient
	}
	return mykit.NewRefreshingTokenSource(oauth, &mykit.Token{RefreshToken: rt}, func(ctx context.Context, t *mykit.Token) error {
		if err := b.store.SetRefreshToken(ctx, t.RefreshToken); err != nil {
			return fmt.Errorf("mybot: save the rotated refresh token: %w", err)
		}
		return nil
	}), nil
}

// start connects to the server: the client, the scopes of the token, the own account and its bot flag.
func (b *Bot) start(ctx context.Context) error {
	ts, err := b.tokenSource(ctx)
	if err != nil {
		return err
	}
	tok, err := ts.Token(ctx)
	if err != nil {
		return fmt.Errorf("mybot: get the access token: %w", err)
	}
	if tok.Scope != "" {
		if missing := missingScopes(tok.Scope, b.requiredScopes()); len(missing) > 0 {
			return &ScopeError{Missing: missing}
		}
	}

	hc := &http.Client{}
	if b.cfg.HTTPClient != nil {
		c := *b.cfg.HTTPClient
		hc = &c
	}
	next := hc.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	hc.Transport = &observingTransport{next: next, budget: b.budget, now: b.now}
	ua := defaultUserAgent
	if b.cfg.UserAgent != "" {
		ua = b.cfg.UserAgent + " " + defaultUserAgent
	}
	api, err := mykit.New(mykit.Config{
		Server:         b.cfg.Server,
		HTTPClient:     hc,
		TokenSource:    ts,
		AcceptLanguage: b.cfg.AcceptLanguage,
		UserAgent:      ua,
	})
	if err != nil {
		return err
	}
	b.api = api

	resp, err := api.VerifyAccountCredentialsWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("mybot: verifyCredentials: %w", err)
	}
	if resp.JSON200 == nil {
		return fmt.Errorf("mybot: verifyCredentials answered %s", resp.Status())
	}
	me := resp.JSON200
	if !me.Bot {
		switch {
		case b.cfg.SetBotFlag:
			on := true
			up, err := api.UpdateAccountCredentialsWithResponse(ctx, mykit.UpdateCredentialsRequest{Bot: &on})
			if err != nil {
				return fmt.Errorf("mybot: set the bot flag: %w", err)
			}
			if up.JSON200 == nil {
				return fmt.Errorf("mybot: updateCredentials answered %s", up.Status())
			}
			me = up.JSON200
			b.logger.Info("mybot: set the bot flag of the account", "account", me.Acct)
		case b.cfg.AllowNonBot:
			b.logger.Warn("mybot: the account has no bot flag", "account", me.Acct)
		default:
			return ErrNotBot
		}
	}
	b.me = me
	return nil
}

// missingScopes are the scopes of need that granted (space-separated) does not give. A broad scope gives those
// under it: read gives read:accounts, follow gives write:follows (the oauth2 security scheme of api/openapi.yaml).
func missingScopes(granted string, need []string) []string {
	have := strings.Fields(granted)
	var missing []string
	for _, n := range need {
		if !slices.ContainsFunc(have, func(h string) bool { return grants(h, n) }) {
			missing = append(missing, n)
		}
	}
	return missing
}

// followScopes are the scopes under follow.
var followScopes = []string{"read:follows", "read:blocks", "read:mutes", "write:follows", "write:blocks", "write:mutes"}

func grants(have, need string) bool {
	if have == need {
		return true
	}
	if have == "follow" {
		return slices.Contains(followScopes, need)
	}
	return (have == "read" || have == "write") && strings.HasPrefix(need, have+":")
}

// Run starts the bot and handles its events until ctx ends, then shuts down gracefully: it stops receiving,
// waits for the running handlers for Config.ShutdownGrace, cancels the contexts of those still running, whose
// events stay unprocessed for the next start, and returns nil. It returns an error when the bot cannot start
// (ErrNotBot, a *ScopeError, a *mykit.Problem of verifyCredentials) or when the server refuses the event stream
// for good, such as for a revoked token.
//
// It verifies the account, then receives the notifications: in Mode Streaming from the event stream, catching up
// with listNotifications back to the Store's position after every connection; in Mode Polling by reading them
// every PollInterval. Each notification is handled once: it counts as processed, in the Store, after its handler
// returns, and one that the Store knows is skipped.
func (b *Bot) Run(ctx context.Context) error {
	b.running.Lock()
	if b.started {
		b.running.Unlock()
		return ErrRunning
	}
	b.started = true
	b.running.Unlock()

	if err := b.start(ctx); err != nil {
		return err
	}
	b.prune(ctx)

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	handlerCtx, cancelHandlers := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelHandlers()
	b.dispatcher = newDispatcher(b.cfg.Concurrency, func(j job) { b.process(handlerCtx, j) })

	var wg sync.WaitGroup
	wg.Go(func() { b.pruneLoop(runCtx) })
	b.submitDue(runCtx, false)
	for _, j := range b.every {
		wg.Go(func() { b.schedule(runCtx, j) })
	}

	var err error
	switch b.cfg.Mode {
	case Polling:
		err = b.poll(runCtx)
	default:
		err = b.stream(runCtx, &wg)
	}
	cancelRun()
	wg.Wait()
	b.shutdown(cancelHandlers)

	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return nil
	}
	return err
}

// RunOnce starts the bot, handles once what is due, and returns, for a bot that a cron or a job runner such as
// Cloud Run jobs starts again and again (ADR-0094 §9): the notifications since the Store's position, back to
// Config.CatchUpLimit, and for each scheduled job the newest of its past times after its last run, whatever its
// CatchUp. It returns when their handlers have returned. When ctx ends first, it shuts down as Run does, and
// what did not finish is due again on the next run. It returns the errors of starting that Run returns, and that
// of reading the notifications.
func (b *Bot) RunOnce(ctx context.Context) error {
	b.running.Lock()
	if b.started {
		b.running.Unlock()
		return ErrRunning
	}
	b.started = true
	b.running.Unlock()

	if err := b.start(ctx); err != nil {
		return err
	}
	b.prune(ctx)

	handlerCtx, cancelHandlers := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelHandlers()
	b.dispatcher = newDispatcher(b.cfg.Concurrency, func(j job) { b.process(handlerCtx, j) })

	err := b.catchUp(ctx)
	b.submitDue(ctx, true)
	select {
	case <-b.dispatcher.done():
	case <-ctx.Done():
	}
	b.shutdown(cancelHandlers)
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return nil
	}
	return err
}

// shutdown stops the dispatcher, waits for the running handlers for ShutdownGrace, and cancels those still
// running, which record nothing.
func (b *Bot) shutdown(cancelHandlers context.CancelFunc) {
	b.dispatcher.stop()
	timer := time.NewTimer(b.cfg.ShutdownGrace)
	defer timer.Stop()
	select {
	case <-b.dispatcher.done():
	case <-timer.C:
		b.abandonMu.Lock()
		b.abandoned = true
		b.abandonMu.Unlock()
		b.logger.Warn("mybot: cancelled the handlers still running after the grace period", "grace", b.cfg.ShutdownGrace)
	}
	cancelHandlers()
}

// stream receives the event stream until ctx ends or the server refuses it, catching up after every connection.
func (b *Bot) stream(ctx context.Context, wg *sync.WaitGroup) error {
	catchUp := make(chan struct{}, 1)
	wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-catchUp:
				if err := b.catchUp(ctx); err != nil && ctx.Err() == nil {
					b.logger.Warn("mybot: the catch-up failed; it runs again on the next connection", "error", err)
				}
				b.releaseHold()
			}
		}
	})
	return b.api.Stream(ctx, mykit.StreamOptions{
		OnOpen: func(reconnected bool) {
			if reconnected {
				b.logger.Info("mybot: the event stream reconnected")
			}
			// Hold the position from the connection on: a notification of the new stream that is processed
			// before the catch-up reads the position must not move it past those the catch-up has yet to list.
			b.tracker.hold()
			select {
			case catchUp <- struct{}{}:
			default:
				// A catch-up is pending already, with its own hold.
				b.releaseHold()
			}
		},
		OnEvent: func(e mykit.StreamEvent) {
			switch e := e.(type) {
			case *mykit.NotificationEvent:
				n := e.Notification
				b.submitNotification(&n)
			case *mykit.UpdateEvent:
				if b.h.status != nil {
					st := e.Status
					b.dispatcher.submit(job{status: &st, sender: senderOf(&st.Account)})
				}
			}
		},
		OnError: func(err error) {
			b.logger.Warn("mybot: the event stream failed; connecting again", "error", err)
		},
	})
}

// poll catches up every PollInterval until ctx ends.
func (b *Bot) poll(ctx context.Context) error {
	for {
		if err := b.catchUp(ctx); err != nil && ctx.Err() == nil {
			var p *mykit.Problem
			if errors.As(err, &p) && p.Status >= 400 && p.Status < 500 && p.Status != http.StatusTooManyRequests {
				return err
			}
			b.logger.Warn("mybot: reading the notifications failed; trying again", "error", err)
		}
		if err := b.sleep(ctx, b.cfg.PollInterval); err != nil {
			return err
		}
	}
}

// catchUp reads the notifications newest first back to the Store's position, or to CatchUpLimit, and hands them
// to the handlers oldest first (ADR-0094 §3).
func (b *Bot) catchUp(ctx context.Context) error {
	if *b.cfg.CatchUpLimit <= 0 {
		return nil
	}
	pos, err := b.store.Position(ctx)
	if err != nil {
		return fmt.Errorf("mybot: read the position from the store: %w", err)
	}
	b.tracker.hold()
	defer b.releaseHold()
	cutoff := b.now().Add(-*b.cfg.CatchUpLimit)
	limit := 40
	var found []mykit.Notification
	for n, err := range mykit.All(ctx, func(ctx context.Context, c *mykit.Cursor) ([]mykit.Notification, *mykit.Cursor, error) {
		r, err := b.api.ListNotificationsWithResponse(ctx, &mykit.ListNotificationsParams{Cursor: c, Limit: &limit})
		if err != nil {
			return nil, nil, err
		}
		if r.JSON200 == nil {
			return nil, nil, fmt.Errorf("mybot: listNotifications answered %s", r.Status())
		}
		return r.JSON200.Items, r.JSON200.NextCursor, nil
	}) {
		if err != nil {
			return err
		}
		if (pos != "" && n.Id <= pos) || n.CreatedAt.Before(cutoff) {
			break
		}
		found = append(found, n)
	}
	for i := len(found) - 1; i >= 0; i-- {
		b.submitNotification(&found[i])
	}
	return nil
}

// releaseHold ends a hold of the tracker and saves the position it moves to.
func (b *Bot) releaseHold() {
	if p := b.tracker.release(); p != "" {
		b.setPosition(p)
	}
}

// submitNotification hands a notification to the dispatcher.
func (b *Bot) submitNotification(n *mykit.Notification) {
	b.tracker.add(n.Id)
	if !b.dispatcher.submit(job{notification: n, sender: senderOf(n.Account)}) {
		b.tracker.forget(n.Id)
	}
}

// process handles one job and records its notification processed.
func (b *Bot) process(ctx context.Context, j job) {
	if j.every != nil {
		b.runScheduled(ctx, j)
		return
	}
	n := j.notification
	if n != nil {
		done, err := b.store.IsProcessed(ctx, n.Id)
		if err != nil {
			b.logger.Error("mybot: read the store", "notification", n.Id, "error", err)
			return
		}
		if done {
			b.completed(n.Id)
			return
		}
	}
	c := b.newContext(ctx, j)
	if !b.isMe(c.Account) {
		if h := b.route(c); h != nil {
			if err := b.chain(h)(c); err != nil {
				c.Logger().Error("mybot: the handler failed", "error", err)
			}
		}
	}
	if n == nil {
		return
	}
	b.abandonMu.RLock()
	defer b.abandonMu.RUnlock()
	if b.abandoned || ctx.Err() != nil {
		// Cancelled by the shutdown: handled again by the next start.
		return
	}
	if err := b.store.MarkProcessed(ctx, n.Id, b.now()); err != nil {
		b.logger.Error("mybot: record the notification processed", "notification", n.Id, "error", err)
		return
	}
	b.completed(n.Id)
}

// completed moves the position after a processed notification.
func (b *Bot) completed(id string) {
	if p := b.tracker.complete(id); p != "" {
		b.setPosition(p)
	}
}

// setPosition saves the position when it is newer than the saved one.
func (b *Bot) setPosition(p string) {
	ctx := context.Background()
	cur, err := b.store.Position(ctx)
	if err == nil && cur >= p {
		return
	}
	if err := b.store.SetPosition(ctx, p); err != nil {
		b.logger.Error("mybot: save the position", "error", err)
	}
}

// isMe reports whether a is the bot's own account, whose events never reach a handler (ADR-0094 §8).
func (b *Bot) isMe(a *mykit.Account) bool { return a != nil && a.Id == b.me.Id }

// chain wraps h in the middleware: the default chain outermost, then those of Use.
func (b *Bot) chain(h Handler) Handler {
	var mws []Middleware
	if !b.cfg.NoDefaultMiddleware {
		mws = DefaultMiddleware()
	}
	mws = append(mws, b.middleware...)
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// newContext is the Context of a job.
func (b *Bot) newContext(ctx context.Context, j job) *Context {
	c := &Context{Context: ctx, bot: b, sender: j.sender}
	if n := j.notification; n != nil {
		c.Notification, c.Status, c.Account, c.source = n, n.Status, n.Account, n.Id
	} else {
		c.Status, c.source = j.status, j.status.Id
		c.Account = &j.status.Account
	}
	if st := c.Status; st != nil {
		c.Text = PlainText(st.Content)
		c.IsReply = st.InReplyToId != nil
		c.IsDirect = st.Visibility == mykit.VisibilityDirect
	}
	return c
}

// prune forgets the processed notifications older than ProcessedRetention.
func (b *Bot) prune(ctx context.Context) {
	if err := b.store.ForgetProcessed(ctx, b.now().Add(-b.cfg.ProcessedRetention)); err != nil {
		b.logger.Warn("mybot: forget the old processed notifications", "error", err)
	}
}

// pruneLoop prunes every hour until ctx ends.
func (b *Bot) pruneLoop(ctx context.Context) {
	for b.sleep(ctx, time.Hour) == nil {
		b.prune(ctx)
	}
}

// Clock is the time of a bot: Config.Clock.
type Clock interface {
	// Now is the current time.
	Now() time.Time
	// Sleep waits for d, or returns the error of ctx when it ends first. It returns at once for d <= 0.
	Sleep(ctx context.Context, d time.Duration) error
}

// systemClock is the Clock of the system.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) Sleep(ctx context.Context, d time.Duration) error { return sleepContext(ctx, d) }

// sleepContext waits for d, or returns the error of ctx when it ends first.
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
