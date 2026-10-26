// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"net/http"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/federation"
)

// Label keys of the federation metrics (ADR-0055 §3). No metric is labeled with a remote
// domain: the state of each server is in instance_health and the admin screens (§6).
const (
	LabelOutcome      = "my.outcome"
	LabelActivityType = "activity.type"
	// LabelInboxRejection is why the inbox refused a request; only my.outcome = rejected has it.
	LabelInboxRejection = "my.inbox.rejection"
)

// The outcomes of a delivery attempt, the my.outcome label of the delivery metrics.
const (
	// DeliveryOutcomeSuccess is a 2xx answer.
	DeliveryOutcomeSuccess = "success"
	// DeliveryOutcomeRetry is a failure that the job retries, including an open circuit breaker.
	DeliveryOutcomeRetry = "retry"
	// DeliveryOutcomeGone is an inbox that is not there to take the activity: a 410 Gone, or a
	// server that is unavailable or suspended, to which nothing was sent.
	DeliveryOutcomeGone = "gone"
	// DeliveryOutcomeFailed is any other permanent failure.
	DeliveryOutcomeFailed = "failed"
)

// The outcomes of a POST to an inbox, the my.outcome label of my.inbox.activities.
const (
	// InboxOutcomeAccepted is a new activity, recorded and queued.
	InboxOutcomeAccepted = "accepted"
	// InboxOutcomeDuplicate is an activity that was already recorded.
	InboxOutcomeDuplicate = "duplicate"
	// InboxOutcomeRejected is a request that the inbox refused (the signature, a domain block,
	// a rate limit, or an invalid activity).
	InboxOutcomeRejected = "rejected"
	// InboxOutcomeIgnored is an unknown actor's Delete or Update of itself, dropped unverified.
	InboxOutcomeIgnored = "ignored"
	// InboxOutcomeError is an unexpected failure (a 500).
	InboxOutcomeError = "error"
)

// The reasons of a refusal of the inbox, the my.inbox.rejection label of my.inbox.activities.
const (
	// InboxRejectionSignature is a signature or an integrity proof that is missing, malformed,
	// below the policy, or does not verify, or whose key could not be fetched.
	InboxRejectionSignature = "signature"
	// InboxRejectionSuspended is a request signed by a suspended domain.
	InboxRejectionSuspended = "suspended"
	// InboxRejectionFederationOff is a request refused because the federation mode is off.
	InboxRejectionFederationOff = "federation_off"
	// InboxRejectionRateLimited is a request over the rate limit of its client IP or server.
	InboxRejectionRateLimited = "rate_limited"
	// InboxRejectionInvalid is an activity without a valid id, type, actor, or object.
	InboxRejectionInvalid = "invalid"
)

// The outcomes of an activity that a relay brought in (ADR-0061), the my.outcome label of
// my.relay.activities.
const (
	// RelayOutcomeAdmitted is an activity taken in: its post is fetched from its origin.
	RelayOutcomeAdmitted = "admitted"
	// RelayOutcomeOverLimit is an activity over the relay's hourly limit, dropped.
	RelayOutcomeOverLimit = "over_limit"
	// RelayOutcomeRestricted is an activity whose author or server is restricted, dropped.
	RelayOutcomeRestricted = "restricted"
	// RelayOutcomeNotReceiving is an activity of a relay that does not bring in posts (left,
	// rejected, removed, or registered to send only), dropped.
	RelayOutcomeNotReceiving = "not_receiving"
)

// ActivityTypeOther is the activity.type of a request whose type is unknown or was not read.
const ActivityTypeOther = "other"

// The federation metrics of ADR-0055 §3.
var (
	DeliveryAttempts = kernel.Instrument{
		Name: "my.delivery.attempts", Unit: "{attempt}", Description: "Delivery attempts of outbound activities, by outcome.",
		LabelKeys: []string{LabelOutcome},
	}
	DeliveryDuration = kernel.Instrument{
		Name: "my.delivery.duration", Unit: "s", Description: "Duration of the HTTP request of a delivery attempt, by outcome.",
		LabelKeys: []string{LabelOutcome}, Buckets: kernel.DurationBuckets,
	}
	InboxActivities = kernel.Instrument{
		Name: "my.inbox.activities", Unit: "{activity}", Description: "Activities POSTed to the inboxes, by type, outcome, and why they were refused.",
		LabelKeys: []string{LabelActivityType, LabelOutcome, LabelInboxRejection},
	}
	RelayActivities = kernel.Instrument{
		Name: "my.relay.activities", Unit: "{activity}", Description: "Activities that relays brought in, by whether they were taken in (ADR-0061).",
		LabelKeys: []string{LabelOutcome},
	}
)

// DeliverySpan is the span of the POST of a delivery attempt (ADR-0063 §1), around the client
// span of the request: the activity's type and the attempt's outcome, never the inbox.
var DeliverySpan = kernel.SpanSpec{
	Name: "my.delivery", Kind: kernel.SpanInternal, AttrKeys: []string{LabelActivityType, LabelOutcome},
}

// deliveryErrorType returns the error.type of the span of a delivery attempt: its my.outcome
// unless it succeeded, or why the job's context ended before an answer.
func deliveryErrorType(a federation.DeliveryAttempt, ctxErr error) string {
	if a.Outcome == "" {
		return kernel.ErrorType(ctxErr)
	}
	if o := deliveryOutcome(a); o != DeliveryOutcomeSuccess {
		return o
	}
	return ""
}

// deliveryOutcome returns the my.outcome of a delivery attempt.
func deliveryOutcome(a federation.DeliveryAttempt) string {
	switch a.Outcome {
	case federation.DeliverySucceeded:
		return DeliveryOutcomeSuccess
	case federation.DeliveryRetryableFailure:
		return DeliveryOutcomeRetry
	case federation.DeliverySkipped:
		return DeliveryOutcomeGone
	}
	if a.HTTPStatus == http.StatusGone {
		return DeliveryOutcomeGone
	}
	return DeliveryOutcomeFailed
}

// activityTypeLabel returns the activity.type of a received document's type: a known activity
// type, or ActivityTypeOther.
func activityTypeLabel(t string) string {
	if federation.KnownActivityType(t) {
		return t
	}
	return ActivityTypeOther
}

// MetricReason returns the my.inbox.rejection of the refusal: Reason grouped into the few
// values that an operator tells apart.
func (e *InboxRejection) MetricReason() string {
	switch e.Reason() {
	case ReasonDomainSuspended:
		return InboxRejectionSuspended
	case ReasonFederationOff:
		return InboxRejectionFederationOff
	case ReasonRateLimited:
		return InboxRejectionRateLimited
	case ReasonInvalidActivity:
		return InboxRejectionInvalid
	}
	return InboxRejectionSignature
}
