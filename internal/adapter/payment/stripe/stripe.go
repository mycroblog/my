// SPDX-License-Identifier: AGPL-3.0-only

// Package stripe is the supporters.PaymentProvider of Stripe (ADR-0074 §2): products and
// recurring prices, hosted Checkout in subscription mode, the customer portal, the commands on
// subscriptions (Subscription Schedules for the changes at the period's end), the webhook
// verifier, the lists of the reconciliation, and the balance transactions as money movements.
//
// stripe-go (github.com/stripe/stripe-go/v87) is used only inside this package; its types never
// leave it. The API version is the one the SDK pins. The webhook signatures are verified with the
// standard library and the events are read for a few fields only, so that a version of the SDK
// that differs from the endpoint's API version still accepts them (ADR-0074 W1).
//
// The SDK reaches only api.stripe.com, which the operator chose by configuring the key, so its
// HTTP client is not the SSRF-guarded kernel.HTTPClient (ADR-0074 §7, H1). The adapter keeps its
// requests within RequestsPerSecond and waits as Stripe's Retry-After asks on a 429.
package stripe

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	stripe "github.com/stripe/stripe-go/v87"

	"github.com/mycroblog/my/internal/app/supporters"
)

// Name is the provider's name: the value of payments and the {provider} of the webhook
// path.
const Name = "stripe"

// The hosts that the URLs of the hosted pages must be on (ADR-0074 §7).
const (
	CheckoutHost = "checkout.stripe.com"
	PortalHost   = "billing.stripe.com"
)

// SignatureHeader is the header of Stripe's webhook signatures.
const SignatureHeader = "Stripe-Signature"

// The requests per second that the adapter sends by default (ADR-0074 §4): below Stripe's read
// limits of about 100 per second in live mode and 25 in test mode.
const (
	LiveRequestsPerSecond = 20
	TestRequestsPerSecond = 10
)

// The metadata keys that carry our IDs on Stripe's objects.
const (
	metaPlanID         = "my_plan_id"
	metaSubscriptionID = "my_subscription_id"
	metaPortal         = "my"
)

// changesWindow is how far back Stripe's GET /v1/events reaches.
const changesWindow = 30 * 24 * time.Hour

// checkoutLifetime is how long a hosted checkout stays open (ADR-0074 §2).
const checkoutLifetime = time.Hour

// locationTTL is how long the account's time zone is used before it is read again.
const locationTTL = time.Hour

// The waits on a 429: at most maxThrottleRetries retries, each after the Retry-After that Stripe
// asks for (defaultRetryAfter without one), and none when it asks for more than maxRetryAfter.
const (
	maxThrottleRetries = 2
	defaultRetryAfter  = time.Second
	maxRetryAfter      = 30 * time.Second
)

// Options configure a Provider.
type Options struct {
	// SecretKey is the API key: a restricted key (rk_test_, rk_live_) or a secret key (sk_test_,
	// sk_live_). Its prefix sets the Mode.
	SecretKey string
	// WebhookSecret is the signing secret of the webhook endpoint (whsec_).
	WebhookSecret string
	// HTTPClient sends the requests; the default has a timeout of 30 seconds.
	HTTPClient *http.Client
	// URL replaces the base URL of Stripe's API; for tests only.
	URL string
	// RequestsPerSecond bounds the request rate; 0 is LiveRequestsPerSecond or
	// TestRequestsPerSecond by the mode.
	RequestsPerSecond float64
	// Logger defaults to slog.Default().
	Logger *slog.Logger
	// Now is the clock; the default is time.Now.
	Now func() time.Time
	// Sleep waits for d or until ctx ends; the default uses a timer. Tests replace it.
	Sleep func(ctx context.Context, d time.Duration) error
}

// ModeOfKey returns the mode of an API key by its prefix (ADR-0074 §5).
func ModeOfKey(key string) (supporters.Mode, error) {
	switch {
	case strings.HasPrefix(key, "sk_test_"), strings.HasPrefix(key, "rk_test_"):
		return supporters.ModeTest, nil
	case strings.HasPrefix(key, "sk_live_"), strings.HasPrefix(key, "rk_live_"):
		return supporters.ModeLive, nil
	}
	return "", errors.New("stripe: the secret key must start with sk_test_, rk_test_, sk_live_, or rk_live_")
}

// Provider is the supporters.PaymentProvider of Stripe. It is safe for concurrent use.
type Provider struct {
	o       Options
	mode    supporters.Mode
	sc      *stripe.Client
	limiter *limiter
	log     *slog.Logger

	locMu   sync.Mutex
	loc     *time.Location
	locRead time.Time

	portalMu     sync.Mutex
	portalConfig string

	links *linkCache
}

var _ supporters.PaymentProvider = (*Provider)(nil)

// New returns the Provider of o. It reads the account's time zone once; when Stripe does not
// answer, the time zone is UTC until a later read succeeds.
func New(ctx context.Context, o Options) (*Provider, error) {
	mode, err := ModeOfKey(o.SecretKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(o.WebhookSecret) == "" {
		return nil, errors.New("stripe: no webhook secret")
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Sleep == nil {
		o.Sleep = sleep
	}
	rate := o.RequestsPerSecond
	if rate <= 0 {
		rate = TestRequestsPerSecond
		if mode == supporters.ModeLive {
			rate = LiveRequestsPerSecond
		}
	}
	cfg := &stripe.BackendConfig{
		HTTPClient: o.HTTPClient,
		// The jobs retry with their backoff (ADR-0004); the adapter waits on a 429 itself.
		MaxNetworkRetries: stripe.Int64(0),
		// The responses are never logged (ADR-0074 §5).
		LeveledLogger:   &stripe.LeveledLogger{Level: stripe.LevelNull},
		EnableTelemetry: stripe.Bool(false),
	}
	if o.URL != "" {
		cfg.URL = stripe.String(o.URL)
	}
	p := &Provider{
		o: o, mode: mode, log: o.Logger.With(slog.String("provider", Name)),
		sc:      stripe.NewClient(o.SecretKey, stripe.WithBackends(stripe.NewBackendsWithConfig(cfg))),
		limiter: newLimiter(time.Duration(float64(time.Second)/rate), o.Now, o.Sleep),
		loc:     time.UTC,
		links:   newLinkCache(linkCacheSize),
	}
	p.refreshLocation(ctx)
	return p, nil
}

// Name implements supporters.PaymentProvider.
func (p *Provider) Name() string { return Name }

// Mode implements supporters.PaymentProvider.
func (p *Provider) Mode() supporters.Mode { return p.mode }

// Location implements supporters.PaymentProvider: the time zone of the account's dashboard
// (settings.dashboard.timezone), or UTC when it could not be read (ADR-0073 §3).
func (p *Provider) Location() *time.Location {
	p.locMu.Lock()
	defer p.locMu.Unlock()
	return p.loc
}

// refreshLocation reads the account's time zone when the last read is older than locationTTL.
func (p *Provider) refreshLocation(ctx context.Context) {
	p.locMu.Lock()
	if !p.locRead.IsZero() && p.o.Now().Sub(p.locRead) < locationTTL {
		p.locMu.Unlock()
		return
	}
	p.locRead = p.o.Now()
	p.locMu.Unlock()
	var acct *stripe.Account
	err := p.do(ctx, "read the account", func() (err error) {
		acct, err = p.sc.V1Accounts.Retrieve(ctx, &stripe.AccountRetrieveParams{})
		return err
	})
	loc := time.UTC
	switch {
	case err != nil:
		p.log.WarnContext(ctx, "the time zone of the Stripe account could not be read; the reports use UTC", slog.Any("error", err))
	case acct.Settings == nil || acct.Settings.Dashboard == nil || acct.Settings.Dashboard.Timezone == "":
		p.log.WarnContext(ctx, "the Stripe account has no time zone; the reports use UTC")
	default:
		l, lerr := time.LoadLocation(acct.Settings.Dashboard.Timezone)
		if lerr != nil {
			p.log.WarnContext(ctx, "unknown time zone of the Stripe account; the reports use UTC",
				slog.String("timezone", acct.Settings.Dashboard.Timezone))
		} else {
			loc = l
		}
	}
	p.locMu.Lock()
	p.loc = loc
	p.locMu.Unlock()
}

// do sends one request through f within the rate limit, retrying it on a 429 after the wait
// that Stripe asks for, and classifies its error (supporters.ErrorKindOf).
func (p *Provider) do(ctx context.Context, op string, f func() error) error {
	for attempt := 0; ; attempt++ {
		if err := p.limiter.wait(ctx); err != nil {
			return fmt.Errorf("stripe: %s: %w: %w", op, supporters.ErrPaymentUnavailable, err)
		}
		err := f()
		if err == nil {
			return nil
		}
		var se *stripe.Error
		if errors.As(err, &se) && se.HTTPStatusCode == http.StatusTooManyRequests && attempt < maxThrottleRetries {
			if d := retryAfter(se, p.o.Now()); d <= maxRetryAfter {
				p.limiter.pause(d)
				continue
			}
		}
		return fmt.Errorf("stripe: %s: %w", op, classify(ctx, err))
	}
}

// retryAfter returns the wait that a 429 asks for.
func retryAfter(se *stripe.Error, now time.Time) time.Duration {
	if se.LastResponse == nil {
		return defaultRetryAfter
	}
	s := strings.TrimSpace(se.LastResponse.Header.Get("Retry-After"))
	if s == "" {
		return defaultRetryAfter
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 0 {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(s); err == nil {
		return max(t.Sub(now), 0)
	}
	return defaultRetryAfter
}

// classify wraps an error of the SDK in the error of supporters that names its kind. The
// message keeps Stripe's type, code, and request ID, never the response's body.
func classify(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", supporters.ErrPaymentUnavailable, ctx.Err())
	}
	var se *stripe.Error
	if !errors.As(err, &se) {
		return fmt.Errorf("%w: %w", supporters.ErrPaymentUnavailable, err)
	}
	desc := fmt.Sprintf("HTTP %d %s %s (request %s)", se.HTTPStatusCode, se.Type, se.Code, se.RequestID)
	var kind error
	switch s := se.HTTPStatusCode; {
	case s == http.StatusUnauthorized || s == http.StatusForbidden:
		kind = supporters.ErrPaymentPermission
	case s == http.StatusNotFound:
		kind = supporters.ErrPaymentNotFound
	case s == http.StatusTooManyRequests:
		kind = supporters.ErrPaymentThrottled
	case s == http.StatusConflict || s >= http.StatusInternalServerError:
		// 409: a request with the same idempotency key is still running.
		kind = supporters.ErrPaymentUnavailable
	case s >= http.StatusBadRequest:
		kind = supporters.ErrPaymentInvalidRequest
	default:
		kind = supporters.ErrPaymentUnavailable
	}
	return fmt.Errorf("%w: %s", kind, desc)
}

// limiter spaces the requests by interval, and holds them all while Stripe asks to wait.
type limiter struct {
	interval time.Duration
	now      func() time.Time
	sleep    func(context.Context, time.Duration) error

	mu   sync.Mutex
	next time.Time
}

func newLimiter(interval time.Duration, now func() time.Time, sleep func(context.Context, time.Duration) error) *limiter {
	return &limiter{interval: interval, now: now, sleep: sleep}
}

// wait returns when the next request may be sent.
func (l *limiter) wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	now := l.now()
	at := now
	if l.next.After(now) {
		at = l.next
	}
	l.next = at.Add(l.interval)
	l.mu.Unlock()
	if d := at.Sub(now); d > 0 {
		return l.sleep(ctx, d)
	}
	return nil
}

// pause holds every request for d.
func (l *limiter) pause(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if until := l.now().Add(d); until.After(l.next) {
		l.next = until
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// idempotency returns the params of a write with the caller's key and the operation's name: one
// command may send several writes, and Stripe refuses a key reused with other parameters.
func idempotency(key, op string) stripe.Params {
	if key == "" {
		return stripe.Params{}
	}
	return stripe.Params{IdempotencyKey: stripe.String(key + ":" + op)}
}

// unixTime returns the time of a Unix timestamp of Stripe, zero for 0.
func unixTime(s int64) time.Time {
	if s == 0 {
		return time.Time{}
	}
	return time.Unix(s, 0).UTC()
}
