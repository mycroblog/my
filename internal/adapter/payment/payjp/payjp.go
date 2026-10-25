// SPDX-License-Identifier: AGPL-3.0-only

// Package payjp is the supporters.PaymentProvider of PAY.JP on its API v1 (ADR-0099): plans
// with IDs of our own, the subscriptions and their commands, the webhook verifier of the
// account's fixed token, the lists of the reconciliation over the Events API, and the charges,
// refunds, chargebacks, and statements as money movements. PAY.JP API v2 has no subscriptions.
//
// API v1 has no hosted checkout and no customer portal; the adapter's own pages stand in for
// them (supporters.PageServer, ADR-0099 §3), and StartCheckout and PortalURL return their URLs.
//
// The adapter calls https://api.pay.jp/v1 with net/http (ADR-0099 R1): Basic authentication with
// the secret key, form bodies, and JSON responses. It reaches only the endpoint of the key that
// the operator configured, so its HTTP client is not the SSRF-guarded kernel.HTTPClient
// (ADR-0074 §7, H1). It keeps its requests within RequestsPerSecond and backs off on a 429.
// API v1 documents no idempotency keys, so every command reads the subscription first and
// writes only what is not in the asked state yet.
package payjp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

// Name is the provider's name: the value of payments_provider and the {provider} of the webhook
// and pages paths.
const Name = "payjp"

// BaseURL is PAY.JP's API v1.
const BaseURL = "https://api.pay.jp/v1"

// TokenHeader is the header of the account's webhook token (ADR-0099 §4).
const TokenHeader = "X-Payjp-Webhook-Token" //nolint:gosec // G101: a header name, not a credential

// The requests per second that the adapter sends by default (ADR-0099 §2): below PAY.JP's
// limits for the secret key of 30 per second in live mode and 2 in test mode.
const (
	LiveRequestsPerSecond = 20
	TestRequestsPerSecond = 2
)

// MinimumAmount and MaximumAmount bound the amount of a plan and a charge in yen.
const (
	MinimumAmount = 50
	MaximumAmount = 9_999_999
)

// currency is the only currency of PAY.JP, as its API writes it.
const currency = "jpy"

// The metadata keys that carry our IDs on PAY.JP's objects.
const (
	metaPlanID         = "my_plan_id"
	metaArchived       = "my_archived"
	metaSubscriptionID = "my_subscription_id"
	// metaSubscription is PAY.JP's subscription of a charge of the difference of a change.
	metaSubscription = "my_subscription"
	metaChange       = "my_change"
	metaPeriodEnd    = "my_period_end"
)

// The waits on a 429: at most maxThrottleRetries retries, each twice as long as the one before
// from firstBackoff. PAY.JP sends no Retry-After.
const (
	maxThrottleRetries = 3
	firstBackoff       = time.Second
)

// tokyo is the time zone of PAY.JP's reports: Japan Standard Time, without daylight saving time.
var tokyo = time.FixedZone("Asia/Tokyo", 9*60*60)

// Options configure a Provider.
type Options struct {
	// SecretKey is the secret key (sk_test_, sk_live_); its prefix sets the Mode.
	SecretKey string
	// PublicKey is the public key (pk_test_, pk_live_) of the same mode, for payjp.js.
	PublicKey string
	// WebhookToken is the account's webhook token (whook_) of the mode.
	WebhookToken string
	// HTTPClient sends the requests; the default has a timeout of 30 seconds.
	HTTPClient *http.Client
	// URL replaces BaseURL; for tests only.
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

// ModeOfKey returns the mode of a secret key by its prefix (ADR-0099 §7).
func ModeOfKey(key string) (supporters.Mode, error) {
	switch {
	case strings.HasPrefix(key, "sk_test_"):
		return supporters.ModeTest, nil
	case strings.HasPrefix(key, "sk_live_"):
		return supporters.ModeLive, nil
	}
	return "", errors.New("payjp: the secret key must start with sk_test_ or sk_live_")
}

// modeOfPublicKey returns the mode of a public key by its prefix.
func modeOfPublicKey(key string) (supporters.Mode, error) {
	switch {
	case strings.HasPrefix(key, "pk_test_"):
		return supporters.ModeTest, nil
	case strings.HasPrefix(key, "pk_live_"):
		return supporters.ModeLive, nil
	}
	return "", errors.New("payjp: the public key must start with pk_test_ or pk_live_")
}

// CheckKeys reports whether the keys are well formed and of one mode (ADR-0099 §7).
func CheckKeys(secretKey, publicKey string) error {
	mode, err := ModeOfKey(secretKey)
	if err != nil {
		return err
	}
	pub, err := modeOfPublicKey(publicKey)
	if err != nil {
		return err
	}
	if pub != mode {
		return fmt.Errorf("payjp: the secret key is for %s payments and the public key for %s ones", mode, pub)
	}
	return nil
}

// Provider is the supporters.PaymentProvider of PAY.JP. It is safe for concurrent use.
type Provider struct {
	o       Options
	mode    supporters.Mode
	base    string
	limiter *limiter
	log     *slog.Logger
	// stateKey signs the state of the pages (ADR-0099 §3).
	stateKey []byte
}

var _ supporters.PaymentProvider = (*Provider)(nil)

// New returns the Provider of o.
func New(o Options) (*Provider, error) {
	if err := CheckKeys(o.SecretKey, o.PublicKey); err != nil {
		return nil, err
	}
	mode, _ := ModeOfKey(o.SecretKey)
	if strings.TrimSpace(o.WebhookToken) == "" {
		return nil, errors.New("payjp: no webhook token")
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
	base := strings.TrimRight(o.URL, "/")
	if base == "" {
		base = BaseURL
	}
	return &Provider{
		o: o, mode: mode, base: base, log: o.Logger.With(slog.String("provider", Name)),
		limiter:  newLimiter(time.Duration(float64(time.Second)/rate), o.Now, o.Sleep),
		stateKey: deriveKey(o.SecretKey, "my/payjp/pages"),
	}, nil
}

// Name implements supporters.PaymentProvider.
func (p *Provider) Name() string { return Name }

// Mode implements supporters.PaymentProvider.
func (p *Provider) Mode() supporters.Mode { return p.mode }

// Location implements supporters.PaymentProvider: Japan Standard Time, the time zone of PAY.JP's
// reports (ADR-0099 §2).
func (p *Provider) Location() *time.Location { return tokyo }

// MinimumAmount implements supporters.PaymentProvider: 50 yen; PAY.JP takes no other currency.
func (p *Provider) MinimumAmount(c string) (int64, bool) {
	if !strings.EqualFold(c, currency) {
		return 0, false
	}
	return MinimumAmount, true
}

// apiError is the error body of PAY.JP.
type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Param   string `json:"param"`
		Status  int    `json:"status"`
		Type    string `json:"type"`
	} `json:"error"`
}

// get reads path with query into out.
func (p *Provider) get(ctx context.Context, op, path string, query url.Values, out any) error {
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	return p.call(ctx, op, http.MethodGet, path, nil, out)
}

// post writes form to path and reads the response into out.
func (p *Provider) post(ctx context.Context, op, path string, form url.Values, out any) error {
	return p.call(ctx, op, http.MethodPost, path, form, out)
}

// del deletes path.
func (p *Provider) del(ctx context.Context, op, path string, form url.Values) error {
	return p.call(ctx, op, http.MethodDelete, path, form, nil)
}

// call sends one request within the rate limit, retrying it on a 429 with a backoff, and
// classifies its error (supporters.ErrorKindOf). The message keeps PAY.JP's status, type, and
// code, never the body.
func (p *Provider) call(ctx context.Context, op, method, path string, form url.Values, out any) error {
	backoff := firstBackoff
	for attempt := 0; ; attempt++ {
		if err := p.limiter.wait(ctx); err != nil {
			return fmt.Errorf("payjp: %s: %w: %w", op, supporters.ErrPaymentUnavailable, err)
		}
		status, err := p.send(ctx, method, path, form, out)
		if err == nil {
			return nil
		}
		if status == http.StatusTooManyRequests && attempt < maxThrottleRetries {
			p.limiter.pause(backoff)
			backoff *= 2
			continue
		}
		return fmt.Errorf("payjp: %s: %w", op, err)
	}
}

// send sends one request and returns its status with the classified error of a failure.
func (p *Provider) send(ctx context.Context, method, path string, form url.Values, out any) (int, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, body)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", supporters.ErrPaymentInvalidRequest, err)
	}
	req.SetBasicAuth(p.o.SecretKey, "")
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := p.o.HTTPClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return 0, fmt.Errorf("%w: %w", supporters.ErrPaymentUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("%w: %w", supporters.ErrPaymentUnavailable, err)
	}
	if resp.StatusCode >= 300 {
		var e apiError
		_ = json.Unmarshal(data, &e)
		return resp.StatusCode, fmt.Errorf("%w: HTTP %d %s %s", kindOfStatus(resp.StatusCode), resp.StatusCode, e.Error.Type, e.Error.Code)
	}
	if out == nil {
		return resp.StatusCode, nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return resp.StatusCode, fmt.Errorf("%w: %w", supporters.ErrInvalidPaymentResponse, err)
	}
	return resp.StatusCode, nil
}

// kindOfStatus returns the error of supporters that names the kind of a failed status
// (ADR-0099 §2).
func kindOfStatus(status int) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return supporters.ErrPaymentPermission
	case status == http.StatusNotFound:
		return supporters.ErrPaymentNotFound
	case status == http.StatusTooManyRequests:
		return supporters.ErrPaymentThrottled
	case status >= http.StatusInternalServerError:
		return supporters.ErrPaymentUnavailable
	case status >= http.StatusBadRequest:
		return supporters.ErrPaymentInvalidRequest
	}
	return supporters.ErrPaymentUnavailable
}

// limiter spaces the requests by interval, and holds them all during a backoff.
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

// unixTime returns the time of a Unix timestamp of PAY.JP, zero for 0.
func unixTime(s int64) time.Time {
	if s == 0 {
		return time.Time{}
	}
	return time.Unix(s, 0).UTC()
}
