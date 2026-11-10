// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import "context"

// OneTimePayments is a PaymentProvider that takes one-time payments: the tips (ADR-0162 §9). It
// is optional; whether the configured adapter implements it is whether the server can take tips.
// Its types are of the standard library and this package only, like the PaymentProvider's. The
// money of a tip comes through the PaymentProvider's ListMoney, GetMoney, and Resolve, its
// movements naming the payment in MoneyMovement.Payment, and the notices of its checkouts resolve
// to Change.OneTimeCheckout.
type OneTimePayments interface {
	// OneTimeLimits reports the smallest and largest one-time charge the provider takes in
	// currency (ISO 4217), in minor units; ok is false for a currency it does not take.
	OneTimeLimits(currency string) (minimum, maximum int64, ok bool)
	// StartOneTimeCheckout creates a hosted checkout of one payment and returns its page. The same
	// Key returns the same checkout. An amount outside OneTimeLimits fails with
	// ErrPaymentInvalidRequest. No customer of the provider is created or reused.
	StartOneTimeCheckout(ctx context.Context, req OneTimeRequest) (Checkout, error)
	// GetOneTimeCheckout reports whether a one-time checkout was paid and what was charged.
	GetOneTimeCheckout(ctx context.Context, ref CheckoutRef) (OneTimeResult, error)
	// ExpireOneTimeCheckout makes an open one-time checkout unpayable and reports what it is now:
	// expired, or paid when the customer got there first. Safe to repeat. It fails, and the
	// checkout stays payable, when the provider refuses to expire a checkout it still reports open.
	ExpireOneTimeCheckout(ctx context.Context, ref CheckoutRef) (OneTimeResult, error)
}

// OneTimeRequest asks for a hosted checkout of one payment.
type OneTimeRequest struct {
	// LocalID is our tip's ID; the provider keeps it on the checkout and the payment (my_tip_id)
	// and returns it in OneTimeResult.
	LocalID string
	// AmountMinor is in minor units of Currency: one of the operator's choices (ADR-0162 §6).
	AmountMinor int64
	// Currency is an ISO 4217 code.
	Currency string
	// Description is the line item of the checkout, localized: "Tip for <server>".
	Description string
	// Email prefills the checkout.
	Email string
	// Locale is the BCP 47 tag of the user's language.
	Locale string
	// SuccessURL and CancelURL are on our origin; the provider appends its checkout's ID to
	// SuccessURL.
	SuccessURL string
	CancelURL  string
	// Key is the idempotency key.
	Key string
}

// OneTimeStatus is the state of a one-time checkout.
type OneTimeStatus string

// The states of a one-time checkout.
const (
	OneTimeOpen OneTimeStatus = "open"
	// OneTimePaid: the payment settled and its amount is final. Only cards are taken, so a
	// checkout that completed without its payment does not happen.
	OneTimePaid    OneTimeStatus = "paid"
	OneTimeExpired OneTimeStatus = "expired"
)

// OneTimeResult is what GetOneTimeCheckout and ExpireOneTimeCheckout report.
type OneTimeResult struct {
	Ref     CheckoutRef
	Status  OneTimeStatus
	LocalID string
	// Payment, AmountMinor, and Currency are what a paid checkout charged; zero otherwise.
	Payment     PaymentRef
	AmountMinor int64
	Currency    string
}
