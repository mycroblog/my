// SPDX-License-Identifier: AGPL-3.0-only

package webhook

import (
	"errors"
	"net/http"
	"strconv"
	"time"
)

// Message is one message to the endpoints that subscribe to its event (webhook_messages): its
// ID is the webhook-id, the same for every endpoint and attempt, and Body is what is sent.
type Message struct {
	ID        string
	EventType EventType
	Body      []byte
	CreatedAt time.Time
}

// Delivery is a message to one endpoint (webhook_deliveries).
type Delivery struct {
	ID         string
	MessageID  string
	EndpointID string
	CreatedAt  time.Time
}

// Outcome is the outcome of a delivery attempt (webhook_delivery_attempts.outcome).
type Outcome string

// Outcomes.
const (
	OutcomeSucceeded       Outcome = "succeeded"
	OutcomeHTTPError       Outcome = "http_error"
	OutcomeTimeout         Outcome = "timeout"
	OutcomeConnectionError Outcome = "connection_error"
	OutcomeForbidden       Outcome = "forbidden_destination"
)

// Valid reports whether o is one of the outcomes.
func (o Outcome) Valid() bool {
	switch o {
	case OutcomeSucceeded, OutcomeHTTPError, OutcomeTimeout, OutcomeConnectionError, OutcomeForbidden:
		return true
	}
	return false
}

// Attempt is one attempt to send a delivery (webhook_delivery_attempts).
type Attempt struct {
	ID         string
	DeliveryID string
	Outcome    Outcome
	// StatusCode is the response's status code for OutcomeSucceeded and OutcomeHTTPError, 0
	// otherwise.
	StatusCode  int
	Duration    time.Duration
	AttemptedAt time.Time
}

// ErrInvalidAttempt reports an attempt whose status code does not match its outcome.
var ErrInvalidAttempt = errors.New("webhook: invalid attempt")

// Validate checks that the status code is there exactly for the outcomes that have one.
func (a Attempt) Validate() error {
	hasCode := a.Outcome == OutcomeSucceeded || a.Outcome == OutcomeHTTPError
	if a.ID == "" || a.DeliveryID == "" || !a.Outcome.Valid() || hasCode != (a.StatusCode != 0) || a.Duration < 0 {
		return ErrInvalidAttempt
	}
	return nil
}

// Failed reports whether the attempt failed.
func (a Attempt) Failed() bool { return a.Outcome != OutcomeSucceeded }

// OutcomeOf returns the outcome of a response's status code: 2xx succeeds, and every other code,
// redirects included (Standard Webhooks), is an HTTP error.
func OutcomeOf(statusCode int) Outcome {
	if statusCode >= 200 && statusCode < 300 {
		return OutcomeSucceeded
	}
	return OutcomeHTTPError
}

// StandardStatus reports whether a response's status code is in the range HTTP defines. Go's client
// passes on any other three digits (000 to 999) that a server sends. Such an answer is not an HTTP
// one, so it counts as a connection error and has no status code to record.
func StandardStatus(statusCode int) bool { return statusCode >= 100 && statusCode <= 599 }

// Gone reports whether a status code disables its endpoint at once: 410 Gone.
func Gone(statusCode int) bool { return statusCode == http.StatusGone }

// RetryDelays are the waits after each failed attempt of a delivery (ADR-0060 "配送と再送",
// the schedule of Standard Webhooks): 8 attempts over about 27 hours.
var RetryDelays = []time.Duration{
	5 * time.Second, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 5 * time.Hour, 10 * time.Hour, 10 * time.Hour,
}

// MaxAttempts is the number of attempts of a delivery before it becomes a dead job.
const MaxAttempts = 8

// MaxRetryAfter caps the wait that a Retry-After header asks for.
const MaxRetryAfter = time.Hour

// RetryAfter returns the wait that a Retry-After header value asks for at now, capped at
// MaxRetryAfter, and false when there is none or it cannot be read.
func RetryAfter(value string, now time.Time) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	var d time.Duration
	if secs, err := strconv.Atoi(value); err == nil {
		if secs < 0 {
			return 0, false
		}
		d = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(value); err == nil {
		d = t.Sub(now)
	} else {
		return 0, false
	}
	if d <= 0 {
		return 0, false
	}
	return min(d, MaxRetryAfter), true
}

// Retention is how long the messages, deliveries, and attempts are kept after a message is
// created (ADR-0060, FR-803): the bodies hold personal data.
const Retention = 14 * 24 * time.Hour

// DeliveryStatus is the state of a delivery as the administrators see it.
type DeliveryStatus string

// Delivery statuses.
const (
	// DeliveryPending: no attempt yet.
	DeliveryPending DeliveryStatus = "pending"
	// DeliverySucceeded: an attempt succeeded.
	DeliverySucceeded DeliveryStatus = "succeeded"
	// DeliveryFailing: every attempt so far failed.
	DeliveryFailing DeliveryStatus = "failing"
)

// DeliveryRecord is a delivery with its message's event, its attempts so far, and its last
// attempt.
type DeliveryRecord struct {
	Delivery  Delivery
	EventType EventType
	Attempts  int
	Succeeded bool
	Last      *Attempt
}

// Status returns the state of the delivery.
func (r DeliveryRecord) Status() DeliveryStatus {
	switch {
	case r.Succeeded:
		return DeliverySucceeded
	case r.Attempts == 0:
		return DeliveryPending
	}
	return DeliveryFailing
}
