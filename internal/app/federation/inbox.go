// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// JobInbox processes one received activity (federation.md §3.2). Its payload is inboxPayload.
const JobInbox = "federation.inbox"

// InboxMaxBodyBytes is the largest request body an inbox reads (ADR-0019 §5).
const InboxMaxBodyBytes = 1 << 20

// Rate limits of the inbox (ADR-0019 §5): per client IP before the signature is parsed, and per
// signing server (the host of the keyId) once the signature has verified, so that only the
// server itself uses up its limit.
var (
	InboxIPLimit     = kernel.Limit{Name: "inbox.ip", Max: 1000, Window: 5 * time.Minute}
	InboxDomainLimit = kernel.Limit{Name: "inbox.domain", Max: 2000, Window: 5 * time.Minute}
)

// inboxPayload is the payload of JobInbox: the received activity to read from
// received_activity_documents, and whether its object must be fetched again from the origin.
type inboxPayload struct {
	ActivityURI string           `json:"activity_uri"`
	Trust       federation.Trust `json:"trust"`
	// RelayID is the accepted relay whose actor signed the activity (ADR-0061), "" otherwise.
	RelayID string `json:"relay_id,omitempty"`
}

// InboxJob returns the JobInbox of a received activity. Its idempotency key is the hash of the
// activity URI, which may be long; its ordering key serializes the activities of one actor.
func InboxJob(a federation.ReceivedActivity) (queue.NewJob, error) {
	return inboxJob(a, "")
}

// inboxJob is InboxJob of an activity that the accepted relay relayID signed ("" for none).
func inboxJob(a federation.ReceivedActivity, relayID string) (queue.NewJob, error) {
	b, err := json.Marshal(inboxPayload{ActivityURI: string(a.ActivityURI), Trust: a.Trust, RelayID: relayID})
	if err != nil {
		return queue.NewJob{}, err
	}
	sum := sha256.Sum256([]byte(a.ActivityURI))
	return queue.NewJob{
		Type: JobInbox, Payload: b,
		IdempotencyKey: "inbox:" + hex.EncodeToString(sum[:]),
		OrderingKey:    "inbox:" + string(a.ActorURI),
	}, nil
}

// InboxRepository is the persistence port of received activities (data-model §4.8): the
// deduplication row, the document as it arrived, and the tombstones of unknown objects. The
// database adapters implement it and pass testkit.InboxContract and
// testkit.InboxProcessingContract. Lookups that find nothing return an error wrapping
// ErrNotFound.
type InboxRepository interface {
	// InsertReceivedActivity records a received activity unless one with its activity URI is
	// recorded, and reports whether it did.
	InsertReceivedActivity(ctx context.Context, tx uow.Tx, a federation.ReceivedActivity) (bool, error)
	// InsertReceivedDocument stores the document of a received activity.
	InsertReceivedDocument(ctx context.Context, tx uow.Tx, activityURI federation.IRI, document []byte) error

	FindReceivedActivity(ctx context.Context, activityURI federation.IRI) (federation.ReceivedActivity, error)
	FindReceivedDocument(ctx context.Context, activityURI federation.IRI) ([]byte, error)
	// ReleaseUnconfirmedActivity removes a received activity of trust refetch with its document,
	// so that the activity is recorded again when it next arrives, and reports whether there was
	// one. An activity of trust signed stays.
	ReleaseUnconfirmedActivity(ctx context.Context, tx uow.Tx, activityURI federation.IRI) (bool, error)

	// InsertRemoteObjectTombstone records a tombstone unless its actor has one for its URI, and
	// reports whether it did: the first tombstone of the pair stays, and another actor's
	// tombstone of the URI does not stop it.
	InsertRemoteObjectTombstone(ctx context.Context, tx uow.Tx, t federation.RemoteObjectTombstone) (bool, error)
	// RemoteObjectTombstoned reports whether uri has a tombstone that actorURI left: a tombstone
	// holds back only the objects and activities of the actor whose Delete or Undo left it.
	RemoteObjectTombstoned(ctx context.Context, uri, actorURI federation.IRI) (bool, error)
	// InsertRemoteFollowTombstone records the tombstone of a pair unless its Undo has one, and
	// reports whether it did.
	InsertRemoteFollowTombstone(ctx context.Context, tx uow.Tx, t federation.RemoteFollowTombstone) (bool, error)
	// LiveRemoteFollowTombstone returns the oldest tombstone of the pair from actorURI to
	// objectURI that has not dropped a Follow yet: no remote object tombstone has its
	// ActivityURI. It fails with ErrNotFound when there is none.
	LiveRemoteFollowTombstone(ctx context.Context, tx uow.Tx, actorURI, objectURI federation.IRI) (federation.RemoteFollowTombstone, error)
}

// InboxAccounts is the part of the account module's public API that the inbox reads.
// *appaccount.Service implements it.
type InboxAccounts interface {
	FindByURI(ctx context.Context, uri string) (appaccount.Actor, error)
}

// RelaySigners tells the accepted relay whose actor signed a request; *Relays implements it.
type RelaySigners interface {
	// AcceptedRelayOf returns the ID of the accepted relay whose actor is signer, "" for none.
	AcceptedRelayOf(ctx context.Context, signer federation.IRI) (string, error)
}

// InstanceReacher records that a remote server is up; *Health implements it.
type InstanceReacher interface {
	Reached(ctx context.Context, domain string, reason federation.AvailabilityReason) error
}

// InboxDeps are the ports and settings of Inbox.
type InboxDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository InboxRepository
	// Outbox enqueues JobInbox in the transaction that records the activity.
	Outbox     queue.Outbox
	Signatures *SignatureVerifier
	Accounts   InboxAccounts
	// Domains are the domain blocks; nil means NoDomainRestrictions.
	Domains DomainPolicy
	// Modes reads the federation mode (ADR-0076); nil federates both ways.
	Modes  ModeSettings
	Health InstanceReacher
	// Relays tells the activities that an accepted relay signed (ADR-0061); nil knows none.
	Relays RelaySigners
	// RateLimiter applies IPLimit and DomainLimit; nil applies none.
	RateLimiter *kernel.RateLimiter
	// IPLimit and DomainLimit default to InboxIPLimit and InboxDomainLimit.
	IPLimit     kernel.Limit
	DomainLimit kernel.Limit
	Clock       kernel.Clock
	Logger      *slog.Logger
	// Meter records my.inbox.activities (ADR-0055 §3); nil records nothing.
	Meter kernel.Meter
}

// Inbox is the synchronous part of receiving an activity (federation.md §3.1, ADR-0033): it
// verifies the request, records the activity once, and enqueues JobInbox, whose handlers do the
// rest asynchronously.
type Inbox struct {
	d          InboxDeps
	activities kernel.Counter
}

// NewInbox returns the Inbox on d.
func NewInbox(d InboxDeps) *Inbox {
	if d.Domains == nil {
		d.Domains = NoDomainRestrictions{}
	}
	if d.IPLimit.Max == 0 {
		d.IPLimit = InboxIPLimit
	}
	if d.DomainLimit.Max == 0 {
		d.DomainLimit = InboxDomainLimit
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Inbox{d: d, activities: kernel.MeterOrNop(d.Meter).Counter(InboxActivities)}
}

// InboxRequest is one POST to an inbox whose content type and body size the HTTP handler has
// checked (federation.md §3.1 steps 1 and 2).
type InboxRequest struct {
	Request *SignedRequest
	// ClientIP is the address the request came from, for the IP rate limit; "" skips it.
	ClientIP string
}

// InboxRejection is the answer to a request that the inbox refuses before it touches the
// database or the queue: the HTTP status of the step that refused it (federation.md §3.1).
type InboxRejection struct {
	Status int
	// RetryAfter is the Retry-After of a 429; 0 otherwise.
	RetryAfter time.Duration
	// AcceptSignature is the Accept-Signature of a 401 about the signature; "" otherwise.
	AcceptSignature string
	Err             error
}

func (e *InboxRejection) Error() string {
	return fmt.Sprintf("inbox rejected the request (%d): %v", e.Status, e.Err)
}

// The reasons of an InboxRejection that is not about the signature (InboxRejection.Reason).
const (
	ReasonRateLimited     = "rate limited"
	ReasonDomainSuspended = "domain suspended"
	ReasonInvalidActivity = "invalid activity"
	// ReasonFederationOff is a request that the federation mode off refuses (ADR-0076).
	ReasonFederationOff = "federation off"
)

// Reason names why the request was refused, from a fixed vocabulary: the SignatureError
// reasons and the ones above. Logs record it instead of the error itself, whose message quotes
// values of the request's headers (ADR-0019).
func (e *InboxRejection) Reason() string {
	var se *SignatureError
	if errors.As(e.Err, &se) {
		return se.Reason()
	}
	if errors.Is(e.Err, ErrFederationRestricted) {
		return ReasonFederationOff
	}
	switch e.Status {
	case http.StatusTooManyRequests:
		return ReasonRateLimited
	case http.StatusForbidden:
		return ReasonDomainSuspended
	case http.StatusBadRequest:
		return ReasonInvalidActivity
	}
	return ReasonSignaturePolicy
}

// Unwrap returns the cause.
func (e *InboxRejection) Unwrap() error { return e.Err }

// Received is the outcome of an accepted request, answered with 202 Accepted.
type Received struct {
	// ActivityURI is the activity's deduplication key; "" when the request was dropped.
	ActivityURI federation.IRI
	Trust       federation.Trust
	// Recorded reports that the activity was new: it was recorded and JobInbox enqueued.
	Recorded bool
	// Dropped reports an unknown actor's Delete or Update of itself, dropped unverified.
	Dropped bool
	// Rejection is set, and nothing else, when the inbox refuses the request.
	Rejection *InboxRejection
}

// Receive runs steps 3 to 14 of federation.md §3.1 on in. A refused request comes back with
// Received.Rejection set before anything is written; the error is only for unexpected
// failures (500). They are separate results so that the caller can log an unexpected failure
// without the header values that a refusal quotes.
func (i *Inbox) Receive(ctx context.Context, in InboxRequest) (Received, error) {
	typ := ActivityTypeOther
	rec, err := i.receive(ctx, in, &typ)
	outcome := InboxOutcomeAccepted
	switch {
	case err != nil:
		outcome = InboxOutcomeError
	case rec.Rejection != nil:
		outcome = InboxOutcomeRejected
	case rec.Dropped:
		outcome = InboxOutcomeIgnored
	case !rec.Recorded:
		outcome = InboxOutcomeDuplicate
	}
	l := kernel.NewLabels(LabelActivityType, typ, LabelOutcome, outcome)
	if outcome == InboxOutcomeRejected {
		l = l.With(LabelInboxRejection, rec.Rejection.MetricReason())
	}
	i.activities.Add(ctx, 1, l)
	return rec, err
}

// receive is Receive; it sets *typ to the activity.type label once the envelope is read.
func (i *Inbox) receive(ctx context.Context, in InboxRequest, typ *string) (Received, error) {
	r := in.Request
	// 3. The client IP's rate limit.
	if in.ClientIP != "" {
		if rej := i.take(ctx, i.d.IPLimit, kernel.ClientKey(in.ClientIP)); rej != nil {
			return Received{Rejection: rej}, nil
		}
	}
	// 4. The signature header, and the host of its keyId. Parse and Check fail only with a
	// *SignatureError: a refusal, never an unexpected error.
	sig, perr := i.d.Signatures.Parse(r)
	if perr != nil {
		return Received{Rejection: rejection(perr)}, nil
	}
	keyHost := sig.KeyHost()
	if keyHost == "" {
		return Received{Rejection: rejection(i.d.Signatures.unauthorized(r, fmt.Errorf("keyId %q has no valid host", sig.KeyID)))}, nil
	}
	// 5. The federation mode, which refuses everything when it is off (ADR-0076), and the domain
	// blocks of the signing server. Every activity that comes in belongs to a flow that off stops,
	// and the other modes take every request in and sort the activities asynchronously.
	mode, err := currentMode(ctx, i.d.Modes)
	if err != nil {
		return Received{}, err
	}
	if !allowsAny(mode, federation.Flows()...) {
		return Received{Rejection: &InboxRejection{Status: http.StatusForbidden, Err: fmt.Errorf("%w: mode %s", ErrFederationRestricted, mode)}}, nil
	}
	suspended, err := i.d.Domains.FederationSuspended(ctx, keyHost)
	if err != nil {
		return Received{}, err
	}
	if suspended {
		return Received{Rejection: &InboxRejection{Status: http.StatusForbidden, Err: fmt.Errorf("%s is suspended", keyHost)}}, nil
	}
	// 6. The body digest, with the other checks that need no key (components, time window).
	if cerr := i.d.Signatures.Check(r, sig); cerr != nil {
		return Received{Rejection: rejection(cerr)}, nil
	}
	// 7. id, type, actor, and object.
	env, err := federation.ReadEnvelope(r.Body, CanonicalHost)
	if err != nil {
		return Received{Rejection: &InboxRejection{Status: http.StatusBadRequest, Err: err}}, nil
	}
	*typ = activityTypeLabel(env.Type)
	// 8. An unknown actor's Delete or Update of itself.
	if env.SelfRemoval() {
		switch _, err := i.d.Accounts.FindByURI(ctx, string(env.Actor)); {
		case errors.Is(err, appaccount.ErrNotFound):
			return Received{Dropped: true}, nil
		case err != nil:
			return Received{}, err
		}
	}
	// 9, 10. The key, fetched when unknown or stale, and the signature.
	v, rejected, err := i.d.Signatures.Verify(ctx, r, sig)
	if err != nil {
		return Received{}, err
	}
	if rejected != nil {
		return Received{Rejection: rejection(rejected)}, nil
	}
	// 11. The signing server's rate limit, charged only to a server whose signature verified.
	if rej := i.take(ctx, i.d.DomainLimit, keyHost); rej != nil {
		return Received{Rejection: rej}, nil
	}
	// 12. The signer against the actor.
	owner, ok := signer(v)
	if !ok {
		owner = "" // a key whose owner is not on the key's host speaks for no one
	}
	a := federation.ReceivedActivity{
		ActivityURI: env.ActivityURI(r.Body), ActorURI: env.Actor, Type: env.Type,
		Scheme: string(v.Scheme), Trust: env.TrustFor(owner), ReceivedAt: kernel.Normalize(i.d.Clock.Now()),
	}
	// The integrity proof of an activity that its actor signed (FEP-8b32). A relayed or
	// forwarded activity is fetched again from its origin anyway (ADR-0033), so its proof is not
	// checked.
	if a.Trust == federation.TrustSigned {
		_, rejected, err := i.d.Signatures.VerifyProof(ctx, r, env.Actor)
		if err != nil {
			return Received{}, err
		}
		if rejected != nil {
			return Received{Rejection: rejection(rejected)}, nil
		}
	}
	// An activity that an accepted relay relayed (signed by the relay for another actor) or
	// announced carries the relay to its processing (ADR-0061).
	var relayID string
	if i.d.Relays != nil && owner != "" && (a.Trust == federation.TrustRefetch || env.Type == string(federation.ActivityAnnounce)) {
		if relayID, err = i.d.Relays.AcceptedRelayOf(ctx, owner); err != nil {
			return Received{}, err
		}
	}
	// 13. The record, the document, and the job, once per activity.
	recorded, err := i.record(ctx, a, relayID, r.Body)
	if err != nil {
		return Received{}, err
	}
	// 14. A signed request shows that its server is up.
	if err := i.d.Health.Reached(ctx, keyHost, federation.ReasonInboundRequest); err != nil && !errors.Is(err, context.Canceled) {
		i.d.Logger.WarnContext(ctx, "inbox could not record the server as reached", slog.String("domain", keyHost), slog.Any("error", err))
	}
	return Received{ActivityURI: a.ActivityURI, Trust: a.Trust, Recorded: recorded}, nil
}

func (i *Inbox) record(ctx context.Context, a federation.ReceivedActivity, relayID string, body []byte) (bool, error) {
	job, err := inboxJob(a, relayID)
	if err != nil {
		return false, err
	}
	var recorded bool
	err = i.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ok, err := i.d.Repository.InsertReceivedActivity(ctx, tx, a)
		if err != nil || !ok {
			recorded = false
			return err
		}
		if err := i.d.Repository.InsertReceivedDocument(ctx, tx, a.ActivityURI, body); err != nil {
			return err
		}
		recorded = true
		return i.d.Outbox.EnqueueTx(ctx, tx, job)
	})
	if err != nil {
		return false, fmt.Errorf("record received activity %s: %w", a.ActivityURI, err)
	}
	return recorded, nil
}

// take takes one event of key from lim. Over the limit it returns the 429 rejection. A store
// failure lets the request through: rate limits protect against abuse, and the inbox must not
// refuse every server when the store is down.
func (i *Inbox) take(ctx context.Context, lim kernel.Limit, key string) *InboxRejection {
	if i.d.RateLimiter == nil {
		return nil
	}
	d, err := i.d.RateLimiter.Take(ctx, lim, key)
	if err != nil {
		i.d.Logger.WarnContext(ctx, "inbox rate limit unavailable", slog.String("limit", lim.Name), slog.Any("error", err))
		return nil
	}
	if d.Allowed {
		return nil
	}
	return &InboxRejection{
		Status: http.StatusTooManyRequests, RetryAfter: d.RetryAfter,
		Err: &kernel.RateLimitedError{RetryAfter: d.RetryAfter},
	}
}

// rejection turns the refusal of a signature into the InboxRejection with its status. An
// error that is not a *SignatureError, which Parse and Check never return, is a 401.
func rejection(err error) *InboxRejection {
	var se *SignatureError
	if !errors.As(err, &se) {
		return &InboxRejection{Status: http.StatusUnauthorized, Err: err}
	}
	return &InboxRejection{Status: se.Status, AcceptSignature: se.AcceptSignature, Err: err}
}
