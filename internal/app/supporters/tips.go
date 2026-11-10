// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// The tips (ADR-0162, FR-918): a local user's one-time payment to the server's operation, with
// nothing in return, on a provider that takes one-time payments (OneTimePayments). The operator
// enables them and chooses their amounts; a user picks one, pays it on the provider's hosted
// checkout, and the payment, its refunds, and its chargebacks go to the income records with the
// source one_time. A user has one open tip at most: starting again expires the open one first,
// so that two tabs never pay twice.

// The jobs and the bounds of the tips.
const (
	// JobSyncTip re-reads the checkout of a tip from the provider and applies it; with expire, it
	// makes an open checkout unpayable first (an account deleted). Its idempotency key names the
	// tip and what queued it.
	JobSyncTip = "supporters.sync_tip"

	// TipCheckoutLifetime is how long an open tip waits for its payment: the checkout's own
	// expiry, which the open tip takes once it is recorded.
	TipCheckoutLifetime = time.Hour
	// MaxTipAmounts is the most amounts that the tips offer.
	MaxTipAmounts = 6
)

// tipBounds are the smallest and largest amount that the tips may offer by currency, in minor
// units (ADR-0162 §6): ¥100 to ¥100,000 and US$1 to US$1,000. A currency without bounds takes
// no tips.
var tipBounds = map[string][2]int64{"JPY": {100, 100_000}, "USD": {100, 100_000}}

// The rate limits of the start of a tip (ADR-0162 §17, ADR-0019 §5): per user by the hour and
// the day, and for the whole server by the hour.
var (
	TipUserHourLimit = kernel.Limit{Name: "supporters.tips.user_hour", Max: 5, Window: time.Hour}
	TipUserDayLimit  = kernel.Limit{Name: "supporters.tips.user_day", Max: 20, Window: 24 * time.Hour}
	TipServerLimit   = kernel.Limit{Name: "supporters.tips.server", Max: 100, Window: time.Hour}
)

// tipServerKey is the key of TipServerLimit, one for the whole server.
const tipServerKey = "server"

// The errors of the tips.
var (
	// ErrTipsDisabled: the server takes no tips now (they are disabled, or the program is).
	ErrTipsDisabled = errors.New("supporters: the server takes no tips")
	// ErrTipsUnavailable: the tips cannot be enabled: the supporter program is disabled, its
	// provider is not the configured one, or the provider takes no one-time payments.
	ErrTipsUnavailable = errors.New("supporters: the tips are not available")
	// ErrTipAmount: the amount is not one of the amounts that the tips offer.
	ErrTipAmount = errors.New("supporters: not an amount of the tips")
)

// TipAmountsRule is the rule that the amounts of the tips break.
type TipAmountsRule string

// The rules of the amounts.
const (
	// TipAmountsCount: 1 to MaxTipAmounts amounts, each once.
	TipAmountsCount TipAmountsRule = "count"
	// TipAmountsCurrency: a currency that has bounds and that the provider takes.
	TipAmountsCurrency TipAmountsRule = "currency"
	// TipAmountsRange: each amount within the bounds of its currency and at least the provider's
	// minimum.
	TipAmountsRange TipAmountsRule = "range"
)

// TipAmountsError reports amounts of the tips that break a rule (422).
type TipAmountsError struct{ Rule TipAmountsRule }

func (e *TipAmountsError) Error() string {
	return "supporters: the amounts of the tips break the rule " + string(e.Rule)
}

// TipLimits are the bounds of the amounts of the tips in a currency, in minor units.
type TipLimits struct {
	Currency         string
	Minimum, Maximum int64
}

// TipSettings is the state of the tips for the administrators (ADR-0162 §17).
type TipSettings struct {
	// Enabled reports that the operator enabled them, with Currency and Amounts; ChangedAt is
	// zero for never.
	Enabled   bool
	Currency  string
	Amounts   []int64
	ChangedAt time.Time
	// OneTimePayments reports that the configured provider takes one-time payments.
	OneTimePayments bool
	// Accepting reports that users may tip now: enabled, on the program's provider, which takes
	// one-time payments.
	Accepting bool
	// Limits are the bounds of the currencies that the tips may offer on this provider.
	Limits []TipLimits
	// ServerLimitReached reports that the starts of the last hour reached TipServerLimit.
	ServerLimitReached bool
}

// TipSettingsInput enables the tips with amounts in a currency, or disables them.
type TipSettingsInput struct {
	Enabled  bool
	Currency string
	// Amounts are in minor units of Currency.
	Amounts []int64
}

// oneTime returns the configured provider as OneTimePayments, false when it takes none.
func (s *Service) oneTime() (OneTimePayments, bool) {
	if s.d.Provider == nil {
		return nil, false
	}
	ot, ok := s.d.Provider.(OneTimePayments)
	return ot, ok
}

// lastTipChange returns the newest change of the tips, the zero value (disabled) without one.
func (s *Service) lastTipChange(ctx context.Context, tx uow.Tx) (TipChange, error) {
	c, err := s.d.Repository.LastTipChange(ctx, tx)
	if errors.Is(err, ErrNotFound) {
		return TipChange{}, nil
	}
	return c, err
}

// tipLimits returns the bounds of currency on the provider, false for a currency that takes no
// tips.
func tipLimits(ot OneTimePayments, currency string) (TipLimits, bool) {
	b, ok := tipBounds[currency]
	if !ok || ot == nil {
		return TipLimits{}, false
	}
	minimum, maximum, ok := ot.OneTimeLimits(currency)
	if !ok {
		return TipLimits{}, false
	}
	out := TipLimits{Currency: currency, Minimum: max(b[0], minimum), Maximum: min(b[1], maximum)}
	return out, out.Minimum <= out.Maximum
}

// TipSettings returns the state of the tips.
func (s *Service) TipSettings(ctx context.Context) (TipSettings, error) {
	c, err := s.lastTipChange(ctx, nil)
	if err != nil {
		return TipSettings{}, err
	}
	ot, ok := s.oneTime()
	out := TipSettings{
		Enabled: c.Enabled, Currency: c.Currency, Amounts: slices.Clone(c.Amounts), ChangedAt: c.ChangedAt,
		OneTimePayments: ok, Limits: []TipLimits{},
	}
	if out.Amounts == nil {
		out.Amounts = []int64{}
	}
	for _, cur := range []string{"JPY", "USD"} {
		if l, ok := tipLimits(ot, cur); ok {
			out.Limits = append(out.Limits, l)
		}
	}
	if out.Accepting, err = s.tipsAccepting(ctx, c); err != nil {
		return TipSettings{}, err
	}
	if s.d.RateLimiter != nil {
		d, err := s.d.RateLimiter.Check(ctx, TipServerLimit, tipServerKey)
		if err != nil {
			return TipSettings{}, err
		}
		out.ServerLimitReached = !d.Allowed
	}
	return out, nil
}

// tipsAccepting reports whether users may tip under c: enabled, with the program accepting on
// a provider that takes one-time payments.
func (s *Service) tipsAccepting(ctx context.Context, c TipChange) (bool, error) {
	if !c.Enabled {
		return false, nil
	}
	if _, ok := s.oneTime(); !ok {
		return false, nil
	}
	switch err := s.accepting(ctx); {
	case errors.Is(err, ErrDisabled), errors.Is(err, ErrProviderMismatch):
		return false, nil
	case err != nil:
		return false, err
	}
	return true, nil
}

// TipsEnabled reports whether users may tip now (ADR-0162 §16, §17).
func (s *Service) TipsEnabled(ctx context.Context) (bool, error) {
	c, err := s.lastTipChange(ctx, nil)
	if err != nil {
		return false, err
	}
	return s.tipsAccepting(ctx, c)
}

// TipAmounts returns the currency and the amounts, in minor units, that the tips offer; it fails
// with ErrTipsDisabled while users may not tip.
func (s *Service) TipAmounts(ctx context.Context) (string, []int64, error) {
	c, err := s.lastTipChange(ctx, nil)
	if err != nil {
		return "", nil, err
	}
	on, err := s.tipsAccepting(ctx, c)
	if err != nil {
		return "", nil, err
	}
	if !on {
		return "", nil, ErrTipsDisabled
	}
	return c.Currency, slices.Clone(c.Amounts), nil
}

// checkTipAmounts returns the amounts in ascending order, or a *TipAmountsError.
func checkTipAmounts(ot OneTimePayments, currency string, amounts []int64) ([]int64, error) {
	out := slices.Clone(amounts)
	slices.Sort(out)
	if len(out) == 0 || len(out) > MaxTipAmounts || len(slices.Compact(slices.Clone(out))) != len(out) {
		return nil, &TipAmountsError{Rule: TipAmountsCount}
	}
	l, ok := tipLimits(ot, currency)
	if !ok {
		return nil, &TipAmountsError{Rule: TipAmountsCurrency}
	}
	if out[0] < l.Minimum || out[len(out)-1] > l.Maximum {
		return nil, &TipAmountsError{Rule: TipAmountsRange}
	}
	return out, nil
}

// SetTips enables the tips with amounts, revises their amounts, or disables them as byUserID (""
// for the CLI), audited, and returns their state. Enabling needs the program enabled on the
// configured provider, which takes one-time payments (ErrTipsUnavailable), and amounts that keep
// the rules (*TipAmountsError). Asking for the state the tips are in records nothing.
func (s *Service) SetTips(ctx context.Context, in TipSettingsInput, byUserID string) (TipSettings, error) {
	var amounts []int64
	if in.Enabled {
		ot, ok := s.oneTime()
		if !ok {
			return TipSettings{}, ErrTipsUnavailable
		}
		switch err := s.accepting(ctx); {
		case errors.Is(err, ErrDisabled), errors.Is(err, ErrProviderMismatch):
			return TipSettings{}, ErrTipsUnavailable
		case err != nil:
			return TipSettings{}, err
		}
		var err error
		if amounts, err = checkTipAmounts(ot, in.Currency, in.Amounts); err != nil {
			return TipSettings{}, err
		}
	}
	now := s.now()
	id, auditID := s.d.IDs.NewID(), s.d.IDs.NewID()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		cur, err := s.lastTipChange(ctx, tx)
		if err != nil {
			return err
		}
		c := TipChange{ID: id, Enabled: in.Enabled, Currency: in.Currency, Amounts: amounts, ChangedByUserID: byUserID, ChangedAt: now}
		var action appmoderation.AuditAction
		switch {
		case !in.Enabled && !cur.Enabled:
			return nil
		case !in.Enabled:
			c.Currency, action = cur.Currency, appmoderation.AuditSupporterTipsDisabled
		case cur.Enabled && cur.Currency == in.Currency && slices.Equal(cur.Amounts, amounts):
			return nil
		case cur.Enabled:
			action = appmoderation.AuditSupporterTipsRevised
		default:
			action = appmoderation.AuditSupporterTipsEnabled
		}
		if err := s.d.Repository.InsertTipChange(ctx, tx, c); err != nil {
			return err
		}
		return tx.Emit(ctx, audit(auditID, byUserID, action, appmoderation.AuditTargetSupporterProgram, c.ID, tipLabel(c), c.ID, now))
	})
	if err != nil {
		return TipSettings{}, err
	}
	return s.TipSettings(ctx)
}

// tipLabel is the label of a change of the tips in the audit log: the currency and the amounts
// in minor units, such as "JPY 300 500 1000".
func tipLabel(c TipChange) string {
	parts := []string{c.Currency}
	for _, a := range c.Amounts {
		parts = append(parts, strconv.FormatInt(a, 10))
	}
	return strings.Join(parts, " ")
}

// TipInput starts a tip.
type TipInput struct {
	UserID      string
	AmountMinor int64
	Currency    string
	// Description is the line item of the checkout, in the user's language: "Tip for <server>".
	Description string
	// ReturnURL is the screen that the checkout returns to; the tip's ID and what returned are
	// added to its query (tip, checkout=success or canceled).
	ReturnURL string
}

// StartedTip is a tip whose checkout the browser goes to.
type StartedTip struct {
	TipID string
	// RedirectURL is the checkout, or, when the open tip was paid already, the return to
	// ReturnURL of that tip.
	RedirectURL string
}

// tipReturn returns the success and cancel URLs of a tip.
func tipReturn(base, tipID string) (string, string) {
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	q := sep + "tip=" + url.QueryEscape(tipID) + "&checkout="
	return base + q + "success", base + q + "canceled"
}

// StartTip starts a tip of one of the amounts that the tips offer (ADR-0162 §13): it checks the
// user (*IneligibleError), the amount (ErrTipAmount), and the rate limits
// (*kernel.RateLimitedError); expires the user's open tip, or sends the browser to its return
// when it was paid; records the tip; and creates its checkout at the provider outside any
// transaction. It fails with ErrTipsDisabled while users may not tip, with ErrServiceClosing from
// the announcement of the closure of the service on, and with ErrOpenTip when another start of
// the user's won the race.
func (s *Service) StartTip(ctx context.Context, in TipInput) (StartedTip, error) {
	c, err := s.lastTipChange(ctx, nil)
	if err != nil {
		return StartedTip{}, err
	}
	on, err := s.tipsAccepting(ctx, c)
	if err != nil {
		return StartedTip{}, err
	}
	if !on {
		return StartedTip{}, ErrTipsDisabled
	}
	if err := s.subscribingAllowed(ctx); err != nil {
		return StartedTip{}, err
	}
	u, err := s.eligible(ctx, in.UserID)
	if err != nil {
		return StartedTip{}, err
	}
	if in.Currency != c.Currency || !slices.Contains(c.Amounts, in.AmountMinor) {
		return StartedTip{}, ErrTipAmount
	}
	if err := s.takeTipLimits(ctx, in.UserID); err != nil {
		return StartedTip{}, err
	}
	ot, _ := s.oneTime()
	if paid, err := s.closeOpenTip(ctx, ot, in.UserID); err != nil || paid != nil {
		if err != nil {
			return StartedTip{}, err
		}
		success, _ := tipReturn(in.ReturnURL, paid.ID)
		return StartedTip{TipID: paid.ID, RedirectURL: success}, nil
	}
	now := s.now()
	tip := Tip{ID: s.d.IDs.NewID(), UserID: in.UserID, AmountMinor: in.AmountMinor, Currency: in.Currency, Provider: s.d.Provider.Name(), CreatedAt: now}
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.InsertTip(ctx, tx, tip, now.Add(TipCheckoutLifetime))
	}); err != nil {
		return StartedTip{}, err
	}
	success, cancel := tipReturn(in.ReturnURL, tip.ID)
	co, err := ot.StartOneTimeCheckout(ctx, OneTimeRequest{
		LocalID: tip.ID, AmountMinor: tip.AmountMinor, Currency: tip.Currency, Description: in.Description, Email: u.Email,
		Locale: u.Locale, SuccessURL: success, CancelURL: cancel, Key: "tip:" + tip.ID,
	})
	if err != nil {
		// No page reached the user: the tip ends here, and the user may start again.
		if _, cerr := s.recordTipOutcome(ctx, tip.ID, TipExpired, now); cerr != nil {
			s.d.Logger.WarnContext(ctx, "a tip whose checkout failed could not be closed", slog.String("tip", tip.ID), slog.Any("error", cerr))
		}
		return StartedTip{}, fmt.Errorf("supporters: start the checkout of tip %s: %w", tip.ID, err)
	}
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.InsertTipCheckout(ctx, tx, TipCheckout{
			TipID: tip.ID, Provider: tip.Provider, Ref: co.Ref, ExpiresAt: kernel.Normalize(co.ExpiresAt), CreatedAt: now,
		})
	}); err != nil {
		return StartedTip{}, err
	}
	s.countTip(ctx, TipOutcomeStarted)
	return StartedTip{TipID: tip.ID, RedirectURL: co.RedirectURL}, nil
}

// takeTipLimits takes a start of a tip of userID from the rate limits of the user and of the
// server, and fails with a *kernel.RateLimitedError when one refuses it.
func (s *Service) takeTipLimits(ctx context.Context, userID string) error {
	if s.d.RateLimiter == nil {
		return nil
	}
	for _, l := range []struct {
		lim kernel.Limit
		key string
	}{{TipUserHourLimit, userID}, {TipUserDayLimit, userID}, {TipServerLimit, tipServerKey}} {
		d, err := s.d.RateLimiter.Take(ctx, l.lim, l.key)
		if err != nil {
			return err
		}
		if !d.Allowed {
			s.countTip(ctx, TipOutcomeRateLimited)
			if l.lim.Name == TipServerLimit.Name {
				s.d.Logger.WarnContext(ctx, "the starts of the tips reached the limit of the server", slog.Int64("max", l.lim.Max))
			}
			return &kernel.RateLimitedError{RetryAfter: d.RetryAfter}
		}
	}
	return nil
}

// closeOpenTip ends the open tip of a user before another starts: it expires its checkout at the
// provider and applies what the provider reports. It returns the tip when the provider reports
// it paid, and fails when the checkout could not be expired. A tip without a checkout gave the
// user no page and expires here.
func (s *Service) closeOpenTip(ctx context.Context, ot OneTimePayments, userID string) (*TipRecord, error) {
	id, err := s.d.Repository.OpenTipID(ctx, nil, userID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rec, err := s.d.Repository.FindTip(ctx, nil, id)
	if err != nil {
		return nil, err
	}
	if rec.Checkout == nil || rec.Provider != s.d.Provider.Name() {
		_, err := s.recordTipOutcome(ctx, rec.ID, TipExpired, s.now())
		return nil, err
	}
	res, err := ot.ExpireOneTimeCheckout(ctx, rec.Checkout.Ref)
	if ErrorKindOf(err) == ErrorNotFound {
		res, err = OneTimeResult{Ref: rec.Checkout.Ref, Status: OneTimeExpired, LocalID: rec.ID}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("supporters: expire the checkout of tip %s: %w", rec.ID, err)
	}
	if err := s.applyTip(ctx, rec, res); err != nil {
		return nil, err
	}
	switch res.Status {
	case OneTimePaid:
		return &rec, nil
	case OneTimeOpen:
		return nil, fmt.Errorf("supporters: the checkout of tip %s stays payable", rec.ID)
	}
	return nil, nil
}

// recordTipOutcome records an outcome of a tip in its own transaction and reports whether it did.
func (s *Service) recordTipOutcome(ctx context.Context, tipID string, kind TipOutcomeKind, at time.Time) (bool, error) {
	var inserted bool
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		inserted, err = s.d.Repository.InsertTipOutcome(ctx, tx, TipOutcome{ID: s.d.IDs.NewID(), TipID: tipID, Kind: kind, OccurredAt: at})
		return err
	})
	return inserted, err
}

// applyTip applies what the provider reports of the checkout of rec (ADR-0162 §15): a payment,
// recorded once, or the expiry of an unpaid tip.
func (s *Service) applyTip(ctx context.Context, rec TipRecord, res OneTimeResult) error {
	now := s.now()
	switch res.Status {
	case OneTimePaid:
		if res.AmountMinor != rec.AmountMinor || res.Currency != rec.Currency {
			s.d.Logger.WarnContext(ctx, "a tip was paid in another amount than it was started with", slog.String("tip", rec.ID),
				slog.Int64("started", rec.AmountMinor), slog.Int64("paid", res.AmountMinor), slog.String("currency", res.Currency))
		}
		if res.Payment == "" || res.AmountMinor <= 0 || res.Currency == "" {
			return fmt.Errorf("%w: a paid tip without its payment", ErrInvalidPaymentResponse)
		}
		var inserted bool
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			inserted, err = s.d.Repository.InsertTipPayment(ctx, tx, TipPayment{
				TipID: rec.ID, Provider: rec.Provider, Ref: res.Payment, AmountMinor: res.AmountMinor, Currency: res.Currency, PaidAt: now,
			})
			return err
		})
		if err == nil && inserted {
			s.countTip(ctx, TipOutcomePaid)
		}
		return err
	case OneTimeExpired:
		if rec.Payment != nil {
			return nil
		}
		inserted, err := s.recordTipOutcome(ctx, rec.ID, TipExpired, now)
		if err == nil && inserted {
			s.countTip(ctx, TipOutcomeExpired)
		}
		return err
	}
	return nil
}

// SyncTip implements Syncer: it applies what GetOneTimeCheckout reported of a checkout of
// provider to its tip (LocalID, or the tip of the checkout), and reports whether the tip's state
// changed. A checkout of no tip of ours is logged and left.
func (s *Service) SyncTip(ctx context.Context, provider string, res OneTimeResult) (bool, error) {
	id := res.LocalID
	if id == "" {
		var err error
		id, err = s.d.Repository.FindTipIDByCheckout(ctx, nil, provider, res.Ref)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return false, err
		}
	}
	rec, err := s.d.Repository.FindTip(ctx, nil, id)
	if errors.Is(err, ErrNotFound) || (err == nil && rec.Provider != provider) {
		s.d.Logger.WarnContext(ctx, "a one-time checkout of no tip of ours", slog.String("provider", provider), slog.String("checkout", string(res.Ref)))
		return false, nil
	}
	if err != nil {
		return false, err
	}
	before := rec.State()
	if err := s.applyTip(ctx, rec, res); err != nil {
		return false, err
	}
	after, err := s.d.Repository.FindTip(ctx, nil, id)
	if err != nil {
		return false, err
	}
	return after.State() != before, nil
}

// OpenTipCheckouts implements Syncer.
func (s *Service) OpenTipCheckouts(ctx context.Context, provider string, now time.Time) ([]CheckoutRef, error) {
	return s.d.Repository.OpenTipCheckouts(ctx, provider, now)
}

// TipView is a tip as its user sees it.
type TipView struct {
	TipRecord
	State TipState
}

// OwnTip returns a tip of the user, ErrNotFound for another's. While it is open, it queues the
// re-read of its checkout from the provider (JobSyncTip), which the return from the checkout
// needs before the provider's notice comes.
func (s *Service) OwnTip(ctx context.Context, userID, tipID string) (TipView, error) {
	rec, err := s.d.Repository.FindTip(ctx, nil, tipID)
	if err != nil {
		return TipView{}, err
	}
	if rec.UserID != userID {
		return TipView{}, fmt.Errorf("%w: tip %s", ErrNotFound, tipID)
	}
	v := TipView{TipRecord: rec, State: rec.State()}
	if v.State == TipStateOpen && rec.Checkout != nil {
		cause := "read:" + strconv.FormatInt(s.now().Unix()/60, 10)
		if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Outbox.EnqueueTx(ctx, tx, SyncTipJob(tipID, cause, false)) }); err != nil {
			return TipView{}, err
		}
	}
	return v, nil
}

// syncTipPayload is the payload of JobSyncTip.
type syncTipPayload struct {
	TipID  string `json:"tip_id"`
	Expire bool   `json:"expire,omitempty"`
}

// SyncTipJob returns the JobSyncTip of a tip; cause makes its idempotency key, and expire makes
// its open checkout unpayable first.
func SyncTipJob(tipID, cause string, expire bool) queue.NewJob {
	b, err := json.Marshal(syncTipPayload{TipID: tipID, Expire: expire})
	if err != nil {
		panic(err) // a string always marshals
	}
	return queue.NewJob{
		Type: JobSyncTip, Payload: b, IdempotencyKey: "sync_tip:" + tipID + ":" + cause, OrderingKey: "supporter_tip:" + tipID,
	}
}

// SyncTipByID is the work of JobSyncTip: it re-reads the checkout of an open tip, or expires it
// first when expire is set, and applies what the provider reports. A tip without a checkout
// expires once TipCheckoutLifetime passed: its start never gave the user a page.
func (s *Service) SyncTipByID(ctx context.Context, tipID string, expire bool) queue.Result {
	rec, err := s.d.Repository.FindTip(ctx, nil, tipID)
	switch {
	case errors.Is(err, ErrNotFound):
		return queue.Done()
	case err != nil:
		return queue.Retry(0, err)
	case rec.State() != TipStateOpen:
		return queue.Done()
	}
	ot, ok := s.oneTime()
	if !ok || rec.Provider != s.d.Provider.Name() {
		return queue.Done()
	}
	if rec.Checkout == nil {
		if expire || !s.now().Before(rec.CreatedAt.Add(TipCheckoutLifetime)) {
			if _, err := s.recordTipOutcome(ctx, rec.ID, TipExpired, s.now()); err != nil {
				return queue.Retry(0, err)
			}
		}
		return queue.Done()
	}
	read := ot.GetOneTimeCheckout
	if expire {
		read = ot.ExpireOneTimeCheckout
	}
	res, err := read(ctx, rec.Checkout.Ref)
	switch kind := ErrorKindOf(err); {
	case kind == ErrorNone:
	case kind == ErrorNotFound:
		res = OneTimeResult{Ref: rec.Checkout.Ref, Status: OneTimeExpired, LocalID: rec.ID}
	case kind.Retried():
		return queue.Retry(0, fmt.Errorf("supporters: read the checkout of tip %s: %w", rec.ID, err))
	default:
		s.d.Logger.WarnContext(ctx, "payment provider failed", slog.String("call", "get one-time checkout"),
			slog.String("error_kind", string(kind)), slog.Any("error", err))
		return queue.Done()
	}
	if err := s.applyTip(ctx, rec, res); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// queueDueTips queues JobSyncTip for the open tips whose end passed, each once per end.
func (s *Service) queueDueTips(ctx context.Context) error {
	now := s.now()
	due, err := s.d.Repository.DueOpenTips(ctx, now, DeadlineBatch)
	if err != nil || len(due) == 0 {
		return err
	}
	jobs := make([]queue.NewJob, len(due))
	for i, id := range due {
		jobs[i] = SyncTipJob(id, "due:"+strconv.FormatInt(now.Unix()/int64(DeadlineInterval/time.Second), 10), false)
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Outbox.EnqueueTx(ctx, tx, jobs...) })
}

// registerTipJobs registers the handler of JobSyncTip.
func (s *Service) registerTipJobs(h *queue.JobHandlerRegistry) {
	h.Register(JobSyncTip, func(ctx context.Context, j queue.Job) queue.Result {
		var p syncTipPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.TipID == "" {
			return queue.Fail(fmt.Errorf("supporters: the payload of %s: %w", j.ID, err))
		}
		return s.SyncTipByID(ctx, p.TipID, p.Expire)
	})
}

// expireOpenTipOf queues inside tx the expiry of the open tip of a user whose account closed
// (ADR-0162 §18).
func (s *Service) expireOpenTipOf(ctx context.Context, tx uow.Tx, userID, cause string) error {
	id, err := s.d.Repository.OpenTipID(ctx, tx, userID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, SyncTipJob(id, cause, true))
}

// settleOpenTips re-reads the checkouts of the open tips of provider and applies them, so that a
// payment that the provider booked before its checkout's notice came finds its tip.
func (s *Service) settleOpenTips(ctx context.Context, provider string) error {
	ot, ok := s.oneTime()
	if !ok || s.d.Provider.Name() != provider {
		return nil
	}
	refs, err := s.d.Repository.OpenTipCheckouts(ctx, provider, time.Time{})
	if err != nil {
		return err
	}
	for _, ref := range refs {
		res, err := ot.GetOneTimeCheckout(ctx, ref)
		if err != nil {
			return fmt.Errorf("supporters: read the one-time checkout %s: %w", ref, err)
		}
		if _, err := s.SyncTip(ctx, provider, res); err != nil {
			return err
		}
	}
	return nil
}

// tipsOfMoney returns the tips of the movements' payments, by payment: the payments of tips of
// ours. A payment of a tip that is not recorded yet first settles the open tips.
func (s *Service) tipsOfMoney(ctx context.Context, provider string, ms []MoneyMovement) (map[PaymentRef]string, error) {
	out := map[PaymentRef]string{}
	settled := false
	for _, m := range ms {
		if m.Payment == "" || m.Subscription != "" {
			continue
		}
		if _, seen := out[m.Payment]; seen {
			continue
		}
		id, err := s.d.Repository.FindTipIDByPayment(ctx, nil, provider, m.Payment)
		if errors.Is(err, ErrNotFound) && m.Kind == MoneyPayment && !settled {
			settled = true
			if err := s.settleOpenTips(ctx, provider); err != nil {
				return nil, err
			}
			id, err = s.d.Repository.FindTipIDByPayment(ctx, nil, provider, m.Payment)
		}
		switch {
		case errors.Is(err, ErrNotFound):
			id = ""
		case err != nil:
			return nil, err
		}
		out[m.Payment] = id
	}
	return out, nil
}

// tipOutcomeOf returns the outcome of a tip that a refund, a chargeback, or its reversal makes:
// a refund in full, a chargeback, a won dispute; a partial refund makes none.
func tipOutcomeOf(m MoneyMovement) (TipOutcomeKind, bool) {
	switch m.Kind {
	case MoneyRefund:
		return TipRefunded, reversalKind(m) == ReversalFullRefund
	case MoneyChargeback:
		return TipChargedBack, true
	case MoneyChargebackReversal:
		return TipDisputeWon, true
	}
	return "", false
}

// TipUsers returns the users of the tips among ids, by tip ID; an unknown or erased tip is left
// out.
func (s *Service) TipUsers(ctx context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	users, err := s.d.Repository.TipUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		out[u.TipID] = u.UserID
	}
	return out, nil
}

// The outcomes of TipCheckouts (my.outcome).
const (
	TipOutcomeStarted     = "started"
	TipOutcomeRateLimited = "rate_limited"
	TipOutcomePaid        = "paid"
	TipOutcomeExpired     = "expired"
)

// LabelOutcome is the label key of the outcome of TipCheckouts.
const LabelOutcome = "my.outcome"

// TipCheckouts counts the starts of the tips and what became of them (ADR-0162 §17): a surge of
// started or rate_limited, or many started that end expired instead of paid, is card testing.
var TipCheckouts = kernel.Instrument{
	Name: "my.supporter.tip_checkouts", Unit: "{checkout}",
	Description: "Checkouts of the tips: started, refused by the rate limits, paid, and expired unpaid.",
	LabelKeys:   []string{LabelOutcome},
}

func (s *Service) countTip(ctx context.Context, outcome string) {
	s.tipCheckouts.Add(ctx, 1, kernel.NewLabels(LabelOutcome, outcome))
}
