// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"errors"
	"time"
)

// Subscription is a local account's subscription to the new posts of another account
// (post_subscriptions, a current value, ADR-0123 §1): each new post of the target that the
// subscriber may see notifies the subscriber with a status notification. The target is never
// told of it.
type Subscription struct {
	AccountID       string
	TargetAccountID string
	CreatedAt       time.Time
}

// SubscriptionRequest is a subscription asked for with a follow of a remote account that awaits
// its server's Accept (post_subscription_requests, a current value): the acceptance turns it into
// a Subscription, and the end or the rejection of the follow drops it.
type SubscriptionRequest struct {
	AccountID       string
	TargetAccountID string
	CreatedAt       time.Time
}

// MaxSubscriptions is how many subscriptions an account may have (ADR-0123 §1).
const MaxSubscriptions = 1000

// SubscriberBatch is how many subscribers one step of the fan-out of a new post notifies
// (ADR-0123 §2).
const SubscriberBatch = 100

// Errors of the subscriptions.
var (
	// ErrSelfSubscription reports a subscription of an account to itself.
	ErrSelfSubscription = errors.New("notification: an account cannot subscribe to itself")
	// ErrSubscriptionLimit reports a subscription beyond MaxSubscriptions.
	ErrSubscriptionLimit = errors.New("notification: too many subscriptions")
)

// Validate checks the invariants of a new subscription.
func (s Subscription) Validate() error {
	if s.AccountID == "" || s.TargetAccountID == "" {
		return errors.New("notification: a subscription needs a subscriber and a target")
	}
	if s.AccountID == s.TargetAccountID {
		return ErrSelfSubscription
	}
	return nil
}
