// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// This file holds the rules of one delivery (ADR-0006 §3): how a response is classified,
// the short-term circuit breaker of an inbox host, and the long-term health of a remote server.

// DeliveryOutcome is the result of one delivery attempt (delivery_attempts.outcome).
type DeliveryOutcome string

// Delivery outcomes.
const (
	// DeliverySucceeded is a 2xx response.
	DeliverySucceeded DeliveryOutcome = "succeeded"
	// DeliveryRetryableFailure is a failure that may pass later: 408, 429, a 5xx other than
	// 501, a connection, TLS, or timeout error, or an open circuit breaker.
	DeliveryRetryableFailure DeliveryOutcome = "retryable_failure"
	// DeliveryPermanentFailure is an answer that the server will not take the activity: any
	// other 4xx (after double-knocking), 501, or a destination the HTTP client refuses.
	DeliveryPermanentFailure DeliveryOutcome = "permanent_failure"
	// DeliverySkipped is an attempt that sent nothing because the server is not a delivery
	// target (unavailable, or suspended by a domain block).
	DeliverySkipped DeliveryOutcome = "skipped"
)

// ClassifyResponse returns the outcome of a delivery that got a response with status code
// status.
func ClassifyResponse(status int) DeliveryOutcome {
	switch {
	case status >= 200 && status < 300:
		return DeliverySucceeded
	case status == http.StatusRequestTimeout, status == http.StatusTooManyRequests:
		return DeliveryRetryableFailure
	case status == http.StatusNotImplemented:
		return DeliveryPermanentFailure
	case status >= 500:
		return DeliveryRetryableFailure
	}
	// 1xx and 3xx never reach here as final answers of a delivery; they count with the 4xx as
	// answers the server gave, so the delivery is not repeated.
	return DeliveryPermanentFailure
}

// DeliveryAttempt is one attempt of a delivery (delivery_attempts).
type DeliveryAttempt struct {
	ID         string
	DeliveryID string
	Outcome    DeliveryOutcome
	// Scheme is the signature scheme of the request that got the final response
	// ("rfc9421" or "cavage"), "" when no response came back.
	Scheme string
	// HTTPStatus is the final response's status code, 0 without a response.
	HTTPStatus int
	// Error describes a failure without a response, or why nothing was sent; "" otherwise.
	Error       string
	AttemptedAt time.Time
}

// RetryAfter parses a Retry-After value (RFC 9110 §10.2.3), delay-seconds or an HTTP date, and
// returns the wait from now. It returns 0 for an empty, malformed, or past value.
func RetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if s, err := strconv.ParseInt(value, 10, 64); err == nil {
		switch {
		case s <= 0:
			return 0
		case s > int64(MaxRetryAfter/time.Second):
			return MaxRetryAfter
		}
		return time.Duration(s) * time.Second
	}
	t, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	return min(max(t.Sub(now), 0), MaxRetryAfter)
}

// MaxRetryAfter bounds the wait a server can ask for with Retry-After, so that a far-off date
// does not park a job beyond the retry schedule.
const MaxRetryAfter = 24 * time.Hour

// The circuit breaker of an inbox host (ADR-0006 §3, Mastodon's Stoplight values).
const (
	// BreakerThreshold is the number of consecutive failures that opens the breaker.
	BreakerThreshold = 10
	// BreakerCooldown is how long an open breaker sends nothing.
	BreakerCooldown = 60 * time.Second
)

// BreakerState is the short-term state of one inbox host: its consecutive failures and, once
// they reach BreakerThreshold, when the breaker lets a request through again. The zero value is
// a closed breaker.
type BreakerState struct {
	Failures  int
	OpenUntil time.Time
}

// Allows reports whether a request may be sent at now.
func (s BreakerState) Allows(now time.Time) bool { return !now.Before(s.OpenUntil) }

// Failed returns the state after a failed request at now. The breaker opens for
// BreakerCooldown when the failures reach BreakerThreshold; once it lets a request through
// again, a single further failure opens it again.
func (s BreakerState) Failed(now time.Time) BreakerState {
	s.Failures++
	if s.Failures >= BreakerThreshold {
		s.OpenUntil = now.Add(BreakerCooldown)
	}
	return s
}

// Succeeded returns the state after a request that got an answer from the server (a success or
// a permanent failure): a closed breaker.
func (s BreakerState) Succeeded() BreakerState { return BreakerState{} }

// FailureDaysThreshold is the number of distinct UTC days with a retryable failure, without a
// success in between, that make a server unavailable (ADR-0006 §3, Mastodon's
// DeliveryFailureTracker).
const FailureDaysThreshold = 7

// AvailabilityKind is the kind of an instance_availability_events row.
type AvailabilityKind string

// Availability kinds.
const (
	MarkedUnavailable AvailabilityKind = "marked_unavailable"
	MarkedAvailable   AvailabilityKind = "marked_available"
)

// AvailabilityReason is why a server was marked available or unavailable.
type AvailabilityReason string

// Availability reasons.
const (
	// ReasonFailureDays: retryable failures on FailureDaysThreshold distinct days.
	ReasonFailureDays AvailabilityReason = "failure_days"
	// ReasonDeliverySucceeded: a delivery to the server succeeded.
	ReasonDeliverySucceeded AvailabilityReason = "delivery_succeeded"
	// ReasonInboundRequest: the server sent a signed request that verified.
	ReasonInboundRequest AvailabilityReason = "inbound_request"
	// ReasonProbeSucceeded: the daily probe fetched the server's NodeInfo.
	ReasonProbeSucceeded AvailabilityReason = "probe_succeeded"
	// ReasonOperator: an operator marked the server.
	ReasonOperator AvailabilityReason = "operator"
)

// Valid reports whether r is one of the availability reasons.
func (r AvailabilityReason) Valid() bool {
	switch r {
	case ReasonFailureDays, ReasonDeliverySucceeded, ReasonInboundRequest, ReasonProbeSucceeded, ReasonOperator:
		return true
	}
	return false
}

// AvailabilityEvent records that a server was marked unavailable or available as a delivery
// target, with the reason (instance_availability_events).
type AvailabilityEvent struct {
	ID         string
	Domain     string
	Kind       AvailabilityKind
	Reason     AvailabilityReason
	OccurredAt time.Time
}

// NewInstanceHealth returns the health of a server without a row: available since at, without
// failures.
func NewInstanceHealth(domain string, at time.Time) InstanceHealth {
	return InstanceHealth{Domain: domain, State: InstanceAvailable, Since: at}
}

// FailureDay returns the UTC day of at as YYYYMMDD, the unit of InstanceHealth.LastFailureDay.
func FailureDay(at time.Time) string { return at.UTC().Format("20060102") }

// Failed returns the health after a retryable failure at at, and whether the failure marked the
// server unavailable. A failure on a day other than LastFailureDay counts one more failure day;
// reaching FailureDaysThreshold makes an available server unavailable.
func (h InstanceHealth) Failed(at time.Time) (InstanceHealth, bool) {
	if day := FailureDay(at); day != h.LastFailureDay {
		h.FailureDays++
		h.LastFailureDay = day
	}
	if h.State != InstanceUnavailable && h.FailureDays >= FailureDaysThreshold {
		h.State, h.Since = InstanceUnavailable, at
		return h, true
	}
	return h, false
}

// Reached returns the health after the server answered at at (a successful delivery, a signed
// request from it, or a successful probe): no failure days, and available. changed is false when
// h already was so, and the projection needs no write; marked is true when the server was
// unavailable.
func (h InstanceHealth) Reached(at time.Time) (next InstanceHealth, changed, marked bool) {
	if h.State != InstanceUnavailable && h.FailureDays == 0 && h.LastFailureDay == "" {
		return h, false, false
	}
	marked = h.State == InstanceUnavailable
	h.FailureDays, h.LastFailureDay = 0, ""
	if marked {
		h.State, h.Since = InstanceAvailable, at
	}
	return h, true, marked
}
