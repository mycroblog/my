// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"errors"
	"fmt"
)

// ErrorKind classifies a failure of the PaymentProvider (ADR-0074 §1), as the reconciliation
// records it (payment_reconcile_runs.error_kind).
type ErrorKind string

// The kinds of failure, in the order of the schema's CHECK.
const (
	ErrorNone ErrorKind = "none"
	// ErrorRequestCap: the reconciliation used its payments_reconcile_max_requests.
	ErrorRequestCap       ErrorKind = "request_cap"
	ErrorPermission       ErrorKind = "permission"
	ErrorNotFound         ErrorKind = "not_found"
	ErrorInvalidRequest   ErrorKind = "invalid_request"
	ErrorThrottled        ErrorKind = "throttled"
	ErrorUnavailable      ErrorKind = "unavailable"
	ErrorInvalidSignature ErrorKind = "invalid_signature"
	ErrorStaleSignature   ErrorKind = "stale_signature"
	ErrorModeMismatch     ErrorKind = "mode_mismatch"
	ErrorInvalidResponse  ErrorKind = "invalid_response"
)

// ErrorKinds returns every kind that payment_reconcile_runs records, ErrorNone first.
func ErrorKinds() []ErrorKind {
	return []ErrorKind{
		ErrorNone, ErrorRequestCap, ErrorPermission, ErrorNotFound, ErrorInvalidRequest, ErrorThrottled,
		ErrorUnavailable, ErrorInvalidSignature, ErrorStaleSignature, ErrorModeMismatch, ErrorInvalidResponse,
	}
}

// Retried reports whether a job retries a call that failed so (ADR-0004 backoff): throttling and
// unavailability pass; the other failures wait for a change of the configuration or the next
// reconciliation.
func (k ErrorKind) Retried() bool { return k == ErrorThrottled || k == ErrorUnavailable }

// The errors of the PaymentProvider. Adapters wrap the provider's error in one of them.
var (
	// ErrPaymentPermission: the credentials are wrong or may not do this (401, 403).
	ErrPaymentPermission = errors.New("supporters: the payment provider refused the credentials")
	// ErrPaymentNotFound: the object does not exist, or is not final yet (404).
	ErrPaymentNotFound = errors.New("supporters: the payment provider has no such object")
	// ErrPaymentInvalidRequest: the provider refused the request itself, such as a checkout in
	// another currency than the customer's (400).
	ErrPaymentInvalidRequest = errors.New("supporters: the payment provider refused the request")
	// ErrPaymentCanceling: the subscription waits to end, so its plan cannot change until the
	// cancellation is withdrawn.
	ErrPaymentCanceling = errors.New("supporters: the subscription at the payment provider is canceling")
	// ErrPaymentDeclined: the provider could not take the payment of a change at once, whether it
	// was declined or needs the customer's authentication, and the plan stays. It is a refused
	// request of its kind.
	ErrPaymentDeclined = fmt.Errorf("%w: the payment failed", ErrPaymentInvalidRequest)
	// ErrPaymentProcessing: the payment of a change at once has not settled. The plan stays, and
	// moves when the payment succeeds. It is a refused request of its kind.
	ErrPaymentProcessing = fmt.Errorf("%w: the payment is processing", ErrPaymentInvalidRequest)
	// ErrPaymentThrottled: the provider throttles the requests (429).
	ErrPaymentThrottled = errors.New("supporters: the payment provider throttles the requests")
	// ErrPaymentUnavailable: the provider did not answer (5xx, network).
	ErrPaymentUnavailable = errors.New("supporters: the payment provider is unavailable")
	// ErrInvalidPaymentResponse: the provider's answer is not usable.
	ErrInvalidPaymentResponse = errors.New("supporters: invalid response of the payment provider")
	// ErrInvalidSignature: the webhook's signature does not match its body.
	ErrInvalidSignature = errors.New("supporters: invalid webhook signature")
	// ErrStaleSignature: the webhook's signature is from outside WebhookTolerance.
	ErrStaleSignature = errors.New("supporters: stale webhook signature")
	// ErrModeMismatch: a test event reached live credentials, or a live one test credentials.
	ErrModeMismatch = errors.New("supporters: the webhook's mode is not the credentials' mode")
	// ErrChangesWindow: ListChanges was asked for changes older than the provider keeps.
	ErrChangesWindow = errors.New("supporters: the changes are older than the provider keeps")
)

var errorKinds = []struct {
	err  error
	kind ErrorKind
}{
	{ErrPaymentPermission, ErrorPermission},
	{ErrPaymentNotFound, ErrorNotFound},
	{ErrPaymentInvalidRequest, ErrorInvalidRequest},
	{ErrPaymentCanceling, ErrorInvalidRequest},
	{ErrPaymentThrottled, ErrorThrottled},
	{ErrPaymentUnavailable, ErrorUnavailable},
	{ErrInvalidPaymentResponse, ErrorInvalidResponse},
	{ErrInvalidSignature, ErrorInvalidSignature},
	{ErrStaleSignature, ErrorStaleSignature},
	{ErrModeMismatch, ErrorModeMismatch},
	// The reconciliation skips the changes on ErrChangesWindow; elsewhere it is a request that
	// the provider cannot serve.
	{ErrChangesWindow, ErrorInvalidRequest},
}

// ErrorKindOf classifies an error of the PaymentProvider: ErrorNone for nil, the kind of the
// error it wraps, and ErrorUnavailable for any other (a canceled context included), which the
// jobs retry.
func ErrorKindOf(err error) ErrorKind {
	if err == nil {
		return ErrorNone
	}
	for _, e := range errorKinds {
		if errors.Is(err, e.err) {
			return e.kind
		}
	}
	return ErrorUnavailable
}
