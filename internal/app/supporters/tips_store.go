// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
)

// The records of the tips (ADR-0162 §12), which TipRepository keeps. A tip's
// state is the rows it has: paid with a payment, expired with that outcome, and open with neither.

// ErrOpenTip reports a tip of a user who has an open one already.
var ErrOpenTip = errors.New("supporters: the user has an open tip")

// TipChange is the tips enabled, their amounts revised, or the tips disabled
// (supporter_tip_changes and supporter_tip_amounts).
type TipChange struct {
	ID      string
	Enabled bool
	// Currency and Amounts are what an enabled change offers, the amounts in minor units in
	// ascending order; a disabled change keeps the currency of the change before it.
	Currency string
	Amounts  []int64
	// ChangedByUserID is "" for the CLI.
	ChangedByUserID string
	ChangedAt       time.Time
}

// Tip is a tip that a local user started (supporter_tips).
type Tip struct {
	ID          string
	UserID      string
	AmountMinor int64
	Currency    string
	Provider    string
	CreatedAt   time.Time
}

// TipCheckout is the hosted checkout of a tip (supporter_tip_checkouts).
type TipCheckout struct {
	TipID     string
	Provider  string
	Ref       CheckoutRef
	ExpiresAt time.Time
	CreatedAt time.Time
}

// TipPayment is the payment of a tip as the provider charged it (supporter_tip_payments).
type TipPayment struct {
	TipID       string
	Provider    string
	Ref         PaymentRef
	AmountMinor int64
	Currency    string
	PaidAt      time.Time
}

// TipOutcomeKind is what happened to a tip (supporter_tip_outcomes.kind).
type TipOutcomeKind string

// The outcomes of a tip (ADR-0162 §15).
const (
	// TipExpired: its checkout expired, or was expired, unpaid.
	TipExpired TipOutcomeKind = "expired"
	// TipRefunded: its payment was refunded in full.
	TipRefunded TipOutcomeKind = "refunded"
	// TipChargedBack: its payment was charged back.
	TipChargedBack TipOutcomeKind = "charged_back"
	// TipDisputeWon: the chargeback was reversed.
	TipDisputeWon TipOutcomeKind = "dispute_won"
)

// TipOutcome is an outcome of a tip.
type TipOutcome struct {
	ID         string
	TipID      string
	Kind       TipOutcomeKind
	OccurredAt time.Time
}

// TipState is the state of a tip.
type TipState string

// The states of a tip.
const (
	TipStateOpen    TipState = "open"
	TipStatePaid    TipState = "paid"
	TipStateExpired TipState = "expired"
)

// TipRecord is a tip with its checkout, its payment, and its outcomes.
type TipRecord struct {
	Tip
	// Checkout and Payment are nil without one.
	Checkout *TipCheckout
	Payment  *TipPayment
	Outcomes []TipOutcomeKind
}

// State returns the state of the tip: paid with a payment, expired with that outcome and no
// payment, and open otherwise.
func (r TipRecord) State() TipState {
	switch {
	case r.Payment != nil:
		return TipStatePaid
	case r.Has(TipExpired):
		return TipStateExpired
	}
	return TipStateOpen
}

// Has reports whether the tip has an outcome of kind.
func (r TipRecord) Has(kind TipOutcomeKind) bool { return slices.Contains(r.Outcomes, kind) }

// TipUser is the user of a tip (ADR-0162 §16).
type TipUser struct {
	TipID  string
	UserID string
}

// TipRepository is the persistence port of the tips (ADR-0162 §12), part of Repository. Methods
// that take a uow.Tx run inside it; a nil tx, where a method allows it, reads outside any
// transaction. Lookups that find nothing return an error wrapping ErrNotFound.
type TipRepository interface {
	// InsertTipChange records the tips enabled, revised, or disabled, with the amounts of an
	// enabled change.
	InsertTipChange(ctx context.Context, tx uow.Tx, c TipChange) error
	// LastTipChange returns the newest change of the tips; ErrNotFound without one. tx may be nil.
	LastTipChange(ctx context.Context, tx uow.Tx) (TipChange, error)

	// InsertTip records a tip and makes it the open tip of its user until expiresAt; ErrOpenTip
	// when the user has an open tip.
	InsertTip(ctx context.Context, tx uow.Tx, t Tip, expiresAt time.Time) error
	// InsertTipCheckout records the checkout of a tip, and moves the end of the open tip to the
	// checkout's; a tip with a checkout recorded already writes nothing.
	InsertTipCheckout(ctx context.Context, tx uow.Tx, c TipCheckout) error
	// InsertTipPayment records the payment of a tip and closes the open tip, and reports whether
	// it did: a tip or a payment recorded already writes nothing.
	InsertTipPayment(ctx context.Context, tx uow.Tx, p TipPayment) (bool, error)
	// InsertTipOutcome records an outcome of a tip, which closes the open tip when it is
	// TipExpired, and reports whether it did: an outcome recorded already writes nothing.
	InsertTipOutcome(ctx context.Context, tx uow.Tx, o TipOutcome) (bool, error)

	// FindTip returns a tip with its records; tx may be nil.
	FindTip(ctx context.Context, tx uow.Tx, id string) (TipRecord, error)
	// FindTipIDByCheckout returns the tip of a provider's checkout; tx may be nil.
	FindTipIDByCheckout(ctx context.Context, tx uow.Tx, provider string, ref CheckoutRef) (string, error)
	// FindTipIDByPayment returns the tip of a provider's payment; tx may be nil.
	FindTipIDByPayment(ctx context.Context, tx uow.Tx, provider string, ref PaymentRef) (string, error)
	// OpenTipID returns the open tip of a user; tx may be nil.
	OpenTipID(ctx context.Context, tx uow.Tx, userID string) (string, error)
	// DueOpenTips returns up to limit open tips whose end is at or before now, oldest first.
	DueOpenTips(ctx context.Context, now time.Time, limit int) ([]string, error)
	// OpenTipCheckouts returns the checkouts of provider of the open tips that end after now.
	OpenTipCheckouts(ctx context.Context, provider string, now time.Time) ([]CheckoutRef, error)
	// TipUsers returns the users of the tips among ids; an unknown tip is left out.
	TipUsers(ctx context.Context, ids []string) ([]TipUser, error)
}
