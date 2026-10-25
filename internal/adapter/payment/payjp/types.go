// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// The objects of PAY.JP API v1 that the adapter reads, with the fields it uses only.

type list[T any] struct {
	Data    []T  `json:"data"`
	HasMore bool `json:"has_more"`
}

type plan struct {
	ID       string   `json:"id"`
	Amount   int64    `json:"amount"`
	Currency string   `json:"currency"`
	Interval string   `json:"interval"`
	Name     string   `json:"name"`
	Metadata metadata `json:"metadata"`
}

type subscription struct {
	ID                 string   `json:"id"`
	Customer           string   `json:"customer"`
	Plan               plan     `json:"plan"`
	NextCyclePlan      *plan    `json:"next_cycle_plan"`
	Status             string   `json:"status"`
	CurrentPeriodStart int64    `json:"current_period_start"`
	CurrentPeriodEnd   int64    `json:"current_period_end"`
	PausedAt           int64    `json:"paused_at"`
	Metadata           metadata `json:"metadata"`
}

// The statuses of a subscription.
const (
	subTrial    = "trial"
	subActive   = "active"
	subCanceled = "canceled"
	subPaused   = "paused"
)

type customer struct {
	ID          string     `json:"id"`
	DefaultCard string     `json:"default_card"`
	Cards       list[card] `json:"cards"`
}

type card struct {
	ID       string `json:"id"`
	Brand    string `json:"brand"`
	Last4    string `json:"last4"`
	ExpMonth int    `json:"exp_month"`
	ExpYear  int    `json:"exp_year"`
	// ThreeDSecureStatus is the result of the 3-D Secure of the token of the card.
	ThreeDSecureStatus string `json:"three_d_secure_status"`
}

type token struct {
	ID   string `json:"id"`
	Used bool   `json:"used"`
	Card card   `json:"card"`
}

type charge struct {
	ID             string   `json:"id"`
	Amount         int64    `json:"amount"`
	AmountRefunded int64    `json:"amount_refunded"`
	Captured       bool     `json:"captured"`
	CapturedAt     int64    `json:"captured_at"`
	Created        int64    `json:"created"`
	Currency       string   `json:"currency"`
	Customer       string   `json:"customer"`
	FeeRate        string   `json:"fee_rate"`
	Paid           bool     `json:"paid"`
	Refunded       bool     `json:"refunded"`
	Subscription   string   `json:"subscription"`
	TermID         string   `json:"term_id"`
	Metadata       metadata `json:"metadata"`
}

type event struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Livemode bool            `json:"livemode"`
	Created  int64           `json:"created"`
	Data     json.RawMessage `json:"data"`
}

// eventObject is the fields of an event's data that the adapter reads without knowing its kind.
type eventObject struct {
	ID           string   `json:"id"`
	Object       string   `json:"object"`
	Subscription string   `json:"subscription"`
	Charge       string   `json:"charge"`
	Amount       int64    `json:"amount"`
	Metadata     metadata `json:"metadata"`
}

type statement struct {
	ID      string          `json:"id"`
	Created int64           `json:"created"`
	Type    string          `json:"type"`
	Items   []statementItem `json:"items"`
	Term    *term           `json:"term"`
}

type statementItem struct {
	Subject string `json:"subject"`
	Amount  int64  `json:"amount"`
}

type term struct {
	ID      string `json:"id"`
	StartAt int64  `json:"start_at"`
	EndAt   int64  `json:"end_at"`
}

// metadata is the metadata of an object; PAY.JP writes its values as strings, and the adapter
// also reads numbers and booleans as their text.
type metadata map[string]string

func (m *metadata) UnmarshalJSON(b []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	out := make(metadata, len(raw))
	for k, v := range raw {
		switch v := v.(type) {
		case string:
			out[k] = v
		case float64:
			out[k] = strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			out[k] = strconv.FormatBool(v)
		case nil:
		default:
			return fmt.Errorf("payjp: metadata %q is not a string", k)
		}
	}
	*m = out
	return nil
}
