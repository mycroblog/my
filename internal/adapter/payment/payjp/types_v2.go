// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"encoding/json"
	"time"
)

// The objects of PAY.JP API v2 that the adapter reads, with the fields it uses only. API v2 writes
// its times in ISO 8601 (RFC 3339) and lists newest first with the cursors starting_after and
// ending_before.

type listV2[T any] struct {
	Data    []T  `json:"data"`
	HasMore bool `json:"has_more"`
}

// checkoutSession is a Checkout Session.
type checkoutSession struct {
	ID            string    `json:"id"`
	Livemode      bool      `json:"livemode"`
	Mode          string    `json:"mode"`
	Status        string    `json:"status"`
	URL           string    `json:"url"`
	ExpiresAt     time.Time `json:"expires_at"`
	PaymentFlowID string    `json:"payment_flow_id"`
	// ClientReferenceID is our tip's ID, as metadata.my_tip_id is.
	ClientReferenceID string   `json:"client_reference_id"`
	Metadata          metadata `json:"metadata"`
}

// The modes and the states of a Checkout Session.
const (
	sessionModePayment = "payment"
	sessionOpen        = "open"
	sessionComplete    = "complete"
	sessionExpired     = "expired"
)

// paymentFlow is a PaymentFlow: the payment of a checkout.
type paymentFlow struct {
	ID             string   `json:"id"`
	Amount         int64    `json:"amount"`
	AmountReceived int64    `json:"amount_received"`
	Currency       string   `json:"currency"`
	Status         string   `json:"status"`
	Metadata       metadata `json:"metadata"`
}

// flowSucceeded is the state of a PaymentFlow whose payment is done.
const flowSucceeded = "succeeded"

// paymentRefund is a refund of a PaymentFlow.
type paymentRefund struct {
	ID            string `json:"id"`
	PaymentFlowID string `json:"payment_flow_id"`
	Amount        int64  `json:"amount"`
	Status        string `json:"status"`
}

// paymentDispute is a dispute of a PaymentFlow.
type paymentDispute struct {
	ID            string    `json:"id"`
	PaymentFlowID string    `json:"payment_flow_id"`
	Amount        int64     `json:"amount"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// disputeCanceled is the state of a dispute that ended in the merchant's favour, which returns
// a chargeback's amount.
const disputeCanceled = "cancel"

// paymentTransaction is an entry of the account's ledger of payments: a payment, a refund, a
// chargeback, or the cancellation of a chargeback.
type paymentTransaction struct {
	ID string `json:"id"`
	// ResourceID is the object that made the entry: the PaymentFlow of a payment, the refund of a
	// refund, and the dispute of a chargeback and of its cancellation.
	ResourceID string    `json:"resource_id"`
	Amount     int64     `json:"amount"`
	Currency   string    `json:"currency"`
	Fee        int64     `json:"fee"`
	Type       string    `json:"type"`
	TermID     string    `json:"term_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// The types of a payment transaction.
const (
	txnPayment          = "payment"
	txnRefund           = "refund"
	txnChargeback       = "chargeback"
	txnChargebackCancel = "chargeback_cancel"
)

// eventV2 is an event of API v2: its resource is data itself, and its time is created_at.
type eventV2 struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Livemode  bool            `json:"livemode"`
	CreatedAt time.Time       `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}

// v2Objects are the objects of API v2 that the events of the adapter carry; API v1 has none of
// these names.
var v2Objects = map[string]bool{
	"checkout.session": true, "payment_flow": true, "payment_refund": true, "payment_dispute": true,
}

// eventShape is what tells an event of API v1 from one of v2: v1 has created (a Unix time), v2
// created_at (ISO 8601) and the objects of v2Objects.
type eventShape struct {
	Created   json.RawMessage `json:"created"`
	CreatedAt json.RawMessage `json:"created_at"`
	Data      struct {
		Object string `json:"object"`
	} `json:"data"`
}

// isV2Event reports whether body is an event of API v2.
func isV2Event(body []byte) bool {
	var s eventShape
	if json.Unmarshal(body, &s) != nil {
		return false
	}
	if v2Objects[s.Data.Object] {
		return true
	}
	return len(s.Created) == 0 && len(s.CreatedAt) > 0
}
