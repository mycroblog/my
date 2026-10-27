// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// DeliveryRetryPolicy is the retry schedule of JobDeliver (ADR-0004, federation.md §7.3): waits
// of 1 minute, 5 minutes, 30 minutes, 2 hours, 12 hours, and 24 hours three times, with jitter,
// for 9 attempts over about 4 days. A job that uses them up becomes a dead job.
var DeliveryRetryPolicy = queue.RetryPolicy{
	MaxAttempts: 9,
	Delays: []time.Duration{
		time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour,
		24 * time.Hour, 24 * time.Hour, 24 * time.Hour,
	},
	Jitter: 0.2,
}

// maxAttemptError is the longest error text kept in delivery_attempts.error.
const maxAttemptError = 1000

// SigningKeys returns the key a local actor signs requests with. *appaccount.Service implements
// it through AccountSigningKeys.
type SigningKeys interface {
	// SigningKey fails with an error wrapping appaccount.ErrNotFound when the actor has no key.
	SigningKey(ctx context.Context, accountID string) (SigningKey, error)
}

// AccountSigningKeys adapts the account module's signing keys to SigningKeys.
type AccountSigningKeys struct {
	Accounts interface {
		SigningKey(ctx context.Context, accountID string) (string, crypto.Signer, error)
	}
}

// SigningKey implements SigningKeys.
func (k AccountSigningKeys) SigningKey(ctx context.Context, accountID string) (SigningKey, error) {
	id, key, err := k.Accounts.SigningKey(ctx, accountID)
	if err != nil {
		return SigningKey{}, err
	}
	return SigningKey{KeyID: id, PrivateKey: key}, nil
}

// ProofKey is the key that signs the integrity proofs of a local actor's activities (FEP-8b32):
// its Ed25519 key and the IRI that its document publishes it under in assertionMethod.
type ProofKey struct {
	VerificationMethod string
	PrivateKey         ed25519.PrivateKey
}

// ProofKeys returns the key a local actor signs integrity proofs with. *appaccount.Service
// implements it through AccountProofKeys.
type ProofKeys interface {
	// ProofKey fails with an error wrapping appaccount.ErrNotFound when the actor has no key.
	ProofKey(ctx context.Context, accountID string) (ProofKey, error)
}

// AccountProofKeys adapts the account module's proof keys to ProofKeys.
type AccountProofKeys struct {
	Accounts interface {
		ProofKey(ctx context.Context, accountID string) (string, ed25519.PrivateKey, error)
	}
}

// ProofKey implements ProofKeys.
func (k AccountProofKeys) ProofKey(ctx context.Context, accountID string) (ProofKey, error) {
	id, key, err := k.Accounts.ProofKey(ctx, accountID)
	if err != nil {
		return ProofKey{}, err
	}
	return ProofKey{VerificationMethod: id, PrivateKey: key}, nil
}

// DeliverDeps are the ports of the delivery.
type DeliverDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	// Client signs and sends the POST (double-knocking, federation.md §2).
	Client *SignedHTTPClient
	Keys   SigningKeys
	// Proofs signs the integrity proof of every delivered document (FEP-8b32); nil sends the
	// documents without one.
	Proofs ProofKeys
	// LinkedData canonicalizes the documents of public posts for their linked data signature
	// (RsaSignature2017, ADR-0061); nil sends them without one.
	LinkedData Canonicalizer
	// Domains are the domain blocks; nil means NoDomainRestrictions.
	Domains DomainPolicy
	// Modes reads the federation mode when each delivery runs (ADR-0076); nil federates both
	// ways.
	Modes ModeSettings
	// Closure reads the service closure when each delivery runs (ADR-0088 §2); nil never
	// closes.
	Closure ClosureSettings
	// Breaker is the process's circuit breaker; nil means a new one.
	Breaker *CircuitBreaker
	Clock   kernel.Clock
	IDs     kernel.IDGenerator
	// Logger logs deliveries that cannot run; nil means slog.Default().
	Logger *slog.Logger
	// Meter records my.delivery.attempts and my.delivery.duration (ADR-0055 §3); nil records
	// nothing.
	Meter kernel.Meter
	// Tracer records the span of each POST to an inbox (DeliverySpan, ADR-0063 §1); nil records
	// nothing.
	Tracer kernel.Tracer
}

// Deliverer runs JobDeliver: one POST of an outbound activity's document to one inbox
// (federation.md §7.3).
//
//  1. When the inbox's server does not get the activity (unavailable, or suspended by a domain
//     block; federation.Deliverable), or the federation mode at the time does not send it
//     (federation.Sends, ADR-0076), it records a skipped attempt and is done: a skipped
//     delivery is never sent again.
//  2. When the circuit breaker of the host is open, it records a retryable failure without
//     sending and retries on the schedule.
//  3. It adds the linked data signature of the actor's key to the stored document of the
//     Create, Update, or Delete of a public post (ADR-0061), or else the integrity proof of the
//     actor's Ed25519 key (withProof), signs the request with the actor's key, and POSTs it.
//  4. It classifies the result (federation.ClassifyResponse) and records the attempt in
//     delivery_attempts, whose projector updates instance_health in the same transaction.
//  5. A success or a permanent failure is Done; a retryable failure is Retry, after the
//     server's Retry-After when it gives one, otherwise on DeliveryRetryPolicy.
type Deliverer struct {
	d        DeliverDeps
	attempts kernel.Counter
	duration kernel.Histogram
}

// NewDeliverer returns the delivery on d.
func NewDeliverer(d DeliverDeps) *Deliverer {
	if d.Domains == nil {
		d.Domains = NoDomainRestrictions{}
	}
	if d.Breaker == nil {
		d.Breaker = NewCircuitBreaker()
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	d.Tracer = kernel.TracerOrNop(d.Tracer)
	m := kernel.MeterOrNop(d.Meter)
	return &Deliverer{d: d, attempts: m.Counter(DeliveryAttempts), duration: m.Histogram(DeliveryDuration)}
}

// RegisterJobs registers the JobDeliver handler with DeliveryRetryPolicy.
func (d *Deliverer) RegisterJobs(h *queue.JobHandlerRegistry) {
	h.RegisterWithPolicy(JobDeliver, d.handle, DeliveryRetryPolicy)
}

func (d *Deliverer) handle(ctx context.Context, j queue.Job) queue.Result {
	var p deliveryPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.DeliveryID == "" {
		return queue.Fail(fmt.Errorf("federation: delivery payload %s: %w", j.Payload, err))
	}
	return d.Deliver(ctx, p.DeliveryID)
}

// Deliver makes one attempt of the delivery deliveryID and returns the job's result. It is
// safe to repeat: every run records its own attempt, and a delivery that already succeeded
// is sent again only if its job runs again (at-least-once), which inboxes deduplicate by the
// activity's id.
func (d *Deliverer) Deliver(ctx context.Context, deliveryID string) queue.Result {
	del, err := d.d.Repository.FindDelivery(ctx, deliveryID)
	if errors.Is(err, ErrNotFound) {
		return queue.Fail(err)
	}
	if err != nil {
		return queue.Retry(0, err)
	}
	act, err := d.d.Repository.FindOutboundActivity(ctx, del.OutboundActivityID)
	if errors.Is(err, ErrNotFound) {
		return queue.Fail(err)
	}
	if err != nil {
		return queue.Retry(0, err)
	}

	var st federation.DomainStatus
	if st.Suspended, err = d.d.Domains.FederationSuspended(ctx, del.Domain); err != nil {
		return queue.Retry(0, err)
	}
	if st.State, err = d.d.Repository.InstanceState(ctx, del.Domain); err != nil {
		return queue.Retry(0, err)
	}
	if !federation.Deliverable(act.Type, st) {
		why := "server unavailable"
		if st.Suspended {
			why = "domain suspended"
		}
		return d.finish(ctx, del, federation.DeliveryAttempt{Outcome: federation.DeliverySkipped, Error: why}, 0)
	}
	switch sends, err := d.sends(ctx, act); {
	case errors.Is(err, ErrNotFound):
		return queue.Fail(err)
	case err != nil:
		return queue.Retry(0, err)
	case !sends:
		return d.finish(ctx, del, federation.DeliveryAttempt{Outcome: federation.DeliverySkipped, Error: "federation mode"}, 0)
	}

	now := d.d.Clock.Now()
	if !d.d.Breaker.Allows(del.Domain, now) {
		return d.finish(ctx, del, federation.DeliveryAttempt{
			Outcome: federation.DeliveryRetryableFailure, Error: "circuit breaker open",
		}, 0)
	}

	document, err := d.d.Repository.FindOutboundDocument(ctx, act.ID)
	if errors.Is(err, ErrNotFound) {
		return queue.Fail(err)
	}
	if err != nil {
		return queue.Retry(0, err)
	}
	key, err := d.d.Keys.SigningKey(ctx, act.ActorAccountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		// The actor is gone, or is not local: nothing can sign the activity.
		return queue.Fail(err)
	}
	if err != nil {
		return queue.Retry(0, err)
	}

	if document, err = d.sign(ctx, act, document, key); err != nil {
		return queue.Retry(0, err)
	}

	started := time.Now()
	sctx, span := d.d.Tracer.Start(ctx, DeliverySpan, kernel.NewAttrs(kernel.StringAttr(LabelActivityType, string(act.Type))))
	attempt, retryAfter := d.send(sctx, del, document, key)
	if attempt.Outcome != "" {
		outcome := deliveryOutcome(attempt)
		d.duration.Record(ctx, time.Since(started).Seconds(), kernel.NewLabels(LabelOutcome, outcome))
		span.Set(kernel.StringAttr(LabelOutcome, outcome))
	}
	span.End(deliveryErrorType(attempt, ctx.Err()))
	if attempt.Outcome == "" {
		// The job's context ended (lease or shutdown) before an answer: try again without
		// counting an attempt against the server.
		return queue.Retry(0, ctx.Err())
	}
	d.d.Breaker.Record(del.Domain, attempt.Outcome, d.d.Clock.Now())
	return d.finish(ctx, del, attempt, retryAfter)
}

// sign returns document with the linked data signature of key when it is the Create, Update, or
// Delete of a public post (federation.SignsLinkedData), which relays forward as it is (ADR-0061),
// and with the integrity proof of withProof otherwise. A proof added after the linked data
// signature would break it, and a proof made before it would need the data integrity context
// that the receivers' linked data processing does not bundle, so a signed document carries no
// proof. The signature is created at the activity's time and is deterministic: every delivery
// and retry of the activity sends the same bytes. A document that cannot be signed goes with its
// proof instead, and a warning is logged.
func (d *Deliverer) sign(ctx context.Context, act federation.OutboundActivity, document []byte, key SigningKey) ([]byte, error) {
	if d.d.LinkedData != nil && federation.SignsLinkedData(act, addressedTo(document)) {
		signed, err := SignLinkedData(document, d.d.LinkedData, key, act.CreatedAt)
		if err == nil {
			return signed, nil
		}
		d.d.Logger.WarnContext(ctx, "delivery without a linked data signature", slog.String("outbound_activity_id", act.ID), slog.Any("error", err))
	}
	return d.withProof(ctx, act, document)
}

// withProof returns document with the eddsa-jcs-2022 proof of the actor's Ed25519 key
// (FEP-8b32, federation.md §7.4), created at the activity's time, so that every delivery and
// retry of the activity sends the same bytes. The document goes as it is when there is no
// ProofKeys, when the actor has no Ed25519 key, and when the document carries a linked data
// signature: a proof added after that signature would break it, so a document that has both
// gets its proof where the signature is made, before it (FEP-8b32, Backward compatibility).
func (d *Deliverer) withProof(ctx context.Context, act federation.OutboundActivity, document []byte) ([]byte, error) {
	if d.d.Proofs == nil {
		return document, nil
	}
	var head struct {
		Signature json.RawMessage `json:"signature"`
	}
	if json.Unmarshal(document, &head) != nil || len(head.Signature) > 0 {
		return document, nil
	}
	key, err := d.d.Proofs.ProofKey(ctx, act.ActorAccountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return document, nil
	}
	if err != nil {
		return nil, err
	}
	signed, err := federation.SignEdDSAJCS2022(document, key.PrivateKey, key.VerificationMethod, act.CreatedAt)
	if err != nil {
		d.d.Logger.WarnContext(ctx, "delivery without an integrity proof", slog.String("outbound_activity_id", act.ID), slog.Any("error", err))
		return document, nil
	}
	return signed, nil
}

// send POSTs document to the delivery's inbox and returns the classified attempt and the wait
// that a retryable response asks for. The attempt has no outcome when ctx ended first.
func (d *Deliverer) send(ctx context.Context, del federation.Delivery, document []byte, key SigningKey) (federation.DeliveryAttempt, time.Duration) {
	h := http.Header{}
	h.Set("Content-Type", "application/activity+json")
	h.Set("Accept", "application/activity+json, application/ld+json")
	resp, err := d.d.Client.Do(ctx, &kernel.HTTPRequest{
		Method: http.MethodPost, URL: string(del.InboxURL), Header: h, Body: document, Purpose: kernel.HTTPPurposeJSON,
	}, key)
	if err != nil {
		if ctx.Err() != nil {
			return federation.DeliveryAttempt{}, 0
		}
		outcome := federation.DeliveryRetryableFailure
		if errors.Is(err, kernel.ErrForbiddenDestination) || errors.Is(err, kernel.ErrTooManyRedirects) ||
			errors.Is(err, kernel.ErrInvalidHTTPRequest) {
			outcome = federation.DeliveryPermanentFailure
		}
		return federation.DeliveryAttempt{Outcome: outcome, Error: err.Error()}, 0
	}
	discard(resp.HTTPResponse)
	a := federation.DeliveryAttempt{
		Outcome: federation.ClassifyResponse(resp.StatusCode), Scheme: string(resp.Scheme), HTTPStatus: resp.StatusCode,
	}
	var after time.Duration
	if a.Outcome == federation.DeliveryRetryableFailure {
		after = federation.RetryAfter(resp.Header.Get("Retry-After"), d.d.Clock.Now())
	}
	return a, after
}

// finish records a in one transaction with its event, so that the instance_health projector
// sees it, and turns the outcome into the job's result.
func (d *Deliverer) finish(ctx context.Context, del federation.Delivery, a federation.DeliveryAttempt, retryAfter time.Duration) queue.Result {
	d.attempts.Add(ctx, 1, kernel.NewLabels(LabelOutcome, deliveryOutcome(a)))
	a.ID = d.d.IDs.NewID()
	a.DeliveryID = del.ID
	a.AttemptedAt = kernel.Normalize(d.d.Clock.Now())
	if len(a.Error) > maxAttemptError {
		a.Error = strings.ToValidUTF8(a.Error[:maxAttemptError], "")
	}
	err := d.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := d.d.Repository.InsertDeliveryAttempt(ctx, tx, a); err != nil {
			return err
		}
		return tx.Emit(ctx, DeliveryAttempted{Attempt: a, Domain: del.Domain})
	})
	if err != nil {
		d.d.Logger.WarnContext(ctx, "delivery attempt not recorded", slog.String("delivery_id", del.ID), slog.Any("error", err))
		if a.Outcome != federation.DeliveryRetryableFailure {
			// The server has its answer; sending again would change nothing.
			return queue.Done()
		}
		return queue.Retry(retryAfter, err)
	}
	switch a.Outcome {
	case federation.DeliveryRetryableFailure:
		cause := a.Error
		if cause == "" {
			cause = fmt.Sprintf("inbox answered %d", a.HTTPStatus)
		}
		// The server failed, not this one: a delivery that gives up is no error to report.
		return queue.RetryRemote(retryAfter, fmt.Errorf("deliver to %s: %s", del.InboxURL, cause))
	case federation.DeliveryPermanentFailure:
		d.d.Logger.InfoContext(ctx, "delivery refused", slog.String("delivery_id", del.ID),
			slog.String("inbox", string(del.InboxURL)), slog.Int("status", a.HTTPStatus), slog.String("error", a.Error))
	}
	return queue.Done()
}

// sends reports whether the federation mode at the time lets act go out (federation.Sends). The
// Update of an actor tells its suspension by its stored document, which is marked suspended.
// While the service closure delivers its Deletes, it alone decides (federation.ClosureSends).
func (d *Deliverer) sends(ctx context.Context, act federation.OutboundActivity) (bool, error) {
	switch closing, err := closureRetracts(ctx, d.d.Closure); {
	case err != nil:
		return false, err
	case closing:
		return federation.ClosureSends(act), nil
	}
	mode, err := currentMode(ctx, d.d.Modes)
	if err != nil {
		return false, err
	}
	suspension := false
	if act.ObjectKind == federation.OutboundAccount && act.Type == federation.ActivityUpdate && !federation.Sends(mode, act, false) {
		document, err := d.d.Repository.FindOutboundDocument(ctx, act.ID)
		if err != nil {
			return false, err
		}
		suspension = suspendedActorUpdate(document)
	}
	return federation.Sends(mode, act, suspension), nil
}

// suspendedActorUpdate reports whether document is the Update of an actor marked suspended
// (ADR-0036).
func suspendedActorUpdate(document []byte) bool {
	var doc struct {
		Object struct {
			Suspended bool `json:"suspended"`
		} `json:"object"`
	}
	return json.Unmarshal(document, &doc) == nil && doc.Object.Suspended
}
