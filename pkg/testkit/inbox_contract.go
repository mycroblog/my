// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// InboxHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, the federation module's repositories on it, and the account repository that the
// handlers of the own actor's Update and Delete write through.
type InboxHarness struct {
	UnitOfWork uow.UnitOfWork
	Inbox      appfederation.InboxRepository
	Federation appfederation.Repository
	// Graph and Moderation are the repositories of the modules whose inbox handlers
	// InboxProcessingContract runs for real (Block, Undo Block, Flag).
	Graph      appgraph.Repository
	Moderation appmoderation.Repository
	Instances  appfederation.InstanceRepository
	Accounts   appaccount.Repository
	// Statuses is the content module's repository, which the handlers of remote posts write in
	// their cases of the order and duplicate property test (inboxOrderCases).
	Statuses appcontent.Repository
}

// InboxFactory returns a harness on a new, empty database whose UnitOfWork uses opts. The
// factory registers the cleanup of everything it creates with t.Cleanup.
type InboxFactory func(t *testing.T, opts uow.Options) InboxHarness

// InboxContract verifies the inbox repository of a database adapter through the synchronous part
// of the inbox (federation.md §3.1): an activity recorded once with its document and one
// federation.inbox job, the URI synthesized for an activity without an id, trust = refetch for an
// activity whose signer is not its actor, every refusal answered with its status before the
// database or the queue is touched, an unknown actor's own Delete dropped without fetching its
// key, and the signer's server marked available again. It also checks that a tombstone of a
// remote object is recorded once. Signatures, keys, and the other modules are in-memory
// stand-ins.
func InboxContract(t *testing.T, newHarness InboxFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *inboxEnv)
	}{
		{"RecordsAnActivityOnce", inboxRecordsOnce},
		{"KeepsTheDocumentByteExact", inboxKeepsDocumentByteExact},
		{"SynthesizesTheURIOfAnActivityWithoutID", inboxSynthesizesURI},
		{"MarksActivitiesOfOtherSignersForRefetch", inboxMarksRefetch},
		{"RefusesWithoutTouchingTheDatabaseOrTheQueue", inboxRefuses},
		{"ChargesTheDomainLimitOnlyToVerifiedSignatures", inboxChargesDomainLimitAfterVerification},
		{"DropsAnUnknownActorsOwnDelete", inboxDropsUnknownSelfDelete},
		{"MarksTheSignersServerAvailable", inboxMarksServerAvailable},
		{"RecordsTombstonesOnce", inboxRecordsTombstonesOnce},
		{"VerifiesIntegrityProofs", inboxVerifiesProofs},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.run(t, newInboxEnv(t, newHarness))
		})
	}
}

const (
	inboxAlice    = "https://remote.example/users/alice"
	inboxAliceKey = inboxAlice + "#main-key"
	inboxRelay    = "https://relay.example/actor"
	inboxRelayKey = inboxRelay + "#main-key"
)

// inboxRSAKey is the public key of every signer; the stand-in Verifier never checks it.
var inboxRSAKey = sync.OnceValue(func() *rsa.PublicKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return &k.PublicKey
})

type inboxEnv struct {
	h        InboxHarness
	clock    *Clock
	kv       *KeyValueStore
	jobs     *inboxJobs
	keys     *inboxKeys
	accounts *inboxAccounts
	domains  inboxDomains
	health   *appfederation.Health
	outbox   queue.Outbox
}

func newInboxEnv(t *testing.T, newHarness InboxFactory) *inboxEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	clock := NewClock(time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC))
	appfederation.RegisterHealthProjectors(projectors, appfederation.HealthProjectorDeps{Repository: h.Federation, IDs: NewIDGenerator(clock)})
	e := &inboxEnv{
		h: h, clock: clock, kv: NewKeyValueStore(clock), jobs: &inboxJobs{seen: map[string]bool{}},
		keys: &inboxKeys{owners: map[string]string{
			inboxAliceKey: inboxAlice, inboxRelayKey: inboxRelay,
			// A key whose owner is on another host than the key speaks for no one.
			"https://evil.example/key": inboxAlice,
		}, ed: map[string]ed25519.PublicKey{}, unavailable: map[string]bool{}, calls: map[string]int{}},
		accounts: &inboxAccounts{known: map[string]bool{inboxAlice: true}},
		domains:  inboxDomains{"blocked.example": true},
		health:   appfederation.NewHealth(appfederation.HealthDeps{UnitOfWork: h.UnitOfWork, Repository: h.Federation, Clock: clock}),
	}
	e.outbox = queue.AfterCommit(projectors, e.jobs, nil)
	return e
}

// inbox returns the Inbox on the harness with the given rate limits (zero: the defaults).
func (e *inboxEnv) inbox(ipLimit, domainLimit kernel.Limit) *appfederation.Inbox {
	return appfederation.NewInbox(appfederation.InboxDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Inbox, Outbox: e.outbox,
		Signatures: appfederation.NewSignatureVerifier(inboxVerifier{}, e.keys, e.kv, e.clock),
		Accounts:   e.accounts, Domains: e.domains, Health: e.health,
		RateLimiter: kernel.NewRateLimiter(e.kv, e.clock), IPLimit: ipLimit, DomainLimit: domainLimit,
		Clock: e.clock,
	})
}

// inboxPost describes one POST to the shared inbox.
type inboxPost struct {
	body []byte
	// signature is the stand-in Signature header: "<keyId> <yes|no>"; "" sends none.
	signature string
	// digest overrides the SHA-256 of the body.
	digest []byte
	ip     string
}

func (e *inboxEnv) request(p inboxPost) appfederation.InboxRequest {
	origin := &url.URL{Scheme: "https", Host: contractHost}
	r, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://internal:8080/ap/inbox", bytes.NewReader(p.body))
	r.Header.Set("Content-Type", "application/activity+json")
	r.Header.Set("Date", e.clock.Now().Format(http.TimeFormat))
	digest := p.digest
	if digest == nil {
		sum := sha256.Sum256(p.body)
		digest = sum[:]
	}
	r.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(digest))
	if p.signature != "" {
		r.Header.Set("Signature", p.signature)
	}
	ip := p.ip
	if ip == "" {
		ip = "192.0.2.1"
	}
	return appfederation.InboxRequest{Request: appfederation.NewSignedRequest(origin, r, p.body), ClientIP: ip}
}

func (e *inboxEnv) receive(t *testing.T, in *appfederation.Inbox, p inboxPost) (appfederation.Received, error) {
	t.Helper()
	got, err := in.Receive(t.Context(), e.request(p))
	if err != nil {
		return got, err
	}
	if got.Rejection != nil {
		if got != (appfederation.Received{Rejection: got.Rejection}) {
			t.Errorf("Receive = %+v, want only the rejection", got)
		}
		return appfederation.Received{}, got.Rejection
	}
	return got, nil
}

func inboxActivity(t *testing.T, fields map[string]any) []byte {
	t.Helper()
	doc := map[string]any{"@context": "https://www.w3.org/ns/activitystreams"}
	for k, v := range fields {
		if v != nil {
			doc[k] = v
		}
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func inboxCreate(t *testing.T, id string) []byte {
	return inboxActivity(t, map[string]any{
		"id": id, "type": "Create", "actor": inboxAlice,
		"object": map[string]any{"id": inboxAlice + "/notes/1", "type": "Note", "content": "hi"},
	})
}

// inboxVerifier is a Verifier whose Signature header is "<keyId> <yes|no>": it covers the
// components of a draft-cavage POST and declares the Digest header; it verifies when it says yes.
type inboxVerifier struct{}

func (inboxVerifier) Parse(r *appfederation.SignedRequest) (*appfederation.Signature, error) {
	v := r.Header.Get("Signature")
	if v == "" {
		return nil, appfederation.ErrNoSignature
	}
	keyID, _, ok := strings.Cut(v, " ")
	if !ok {
		return nil, appfederation.ErrMalformedSignature
	}
	sig := &appfederation.Signature{
		Scheme: appfederation.SchemeCavage, KeyID: keyID, Algorithm: "rsa-sha256",
		Components: []string{"(request-target)", "host", "date", "digest"},
	}
	if alg, value, ok := strings.Cut(r.Header.Get("Digest"), "="); ok {
		b, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, appfederation.ErrMalformedSignature
		}
		sig.Digests = append(sig.Digests, appfederation.Digest{Algorithm: strings.ToLower(alg), Value: b})
	}
	return sig, nil
}

func (inboxVerifier) Verify(r *appfederation.SignedRequest, _ *appfederation.Signature, _ appfederation.VerifyOptions) error {
	if strings.HasSuffix(r.Header.Get("Signature"), " yes") {
		return nil
	}
	return appfederation.ErrSignatureMismatch
}

// inboxKeys resolves the keys of owners and counts the calls per key ID.
type inboxKeys struct {
	mu     sync.Mutex
	owners map[string]string
	// ed holds the Ed25519 keys among owners; the others are inboxRSAKey.
	ed          map[string]ed25519.PublicKey
	unavailable map[string]bool
	calls       map[string]int
}

func (k *inboxKeys) ResolvePublicKey(_ context.Context, keyID string, _ bool) (appfederation.PublicKey, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.calls[keyID]++
	if k.unavailable[keyID] {
		return appfederation.PublicKey{}, fmt.Errorf("%w: %s", appfederation.ErrPublicKeyUnavailable, keyID)
	}
	owner, ok := k.owners[keyID]
	if !ok {
		return appfederation.PublicKey{}, fmt.Errorf("%w: %s", appfederation.ErrPublicKeyNotFound, keyID)
	}
	if pub, ok := k.ed[keyID]; ok {
		return appfederation.PublicKey{KeyID: keyID, Owner: owner, Key: pub}, nil
	}
	return appfederation.PublicKey{KeyID: keyID, Owner: owner, Key: inboxRSAKey()}, nil
}

func (k *inboxKeys) called(keyID string) int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.calls[keyID]
}

// inboxAccounts knows the remote actors in known.
type inboxAccounts struct{ known map[string]bool }

func (a *inboxAccounts) FindByURI(_ context.Context, uri string) (appaccount.Actor, error) {
	if !a.known[uri] {
		return appaccount.Actor{}, appaccount.ErrNotFound
	}
	var actor appaccount.Actor
	actor.Account.URI = uri
	return actor, nil
}

type inboxDomains map[string]bool

func (d inboxDomains) FederationSuspended(_ context.Context, domain string) (bool, error) {
	return d[domain], nil
}

// inboxJobs is a queue.JobQueue that keeps the jobs given to it, skipping a job whose idempotency
// key it has seen.
type inboxJobs struct {
	mu   sync.Mutex
	jobs []queue.NewJob
	seen map[string]bool
}

func (q *inboxJobs) Enqueue(_ context.Context, jobs ...queue.NewJob) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range jobs {
		if j.IdempotencyKey != "" && q.seen[j.IdempotencyKey] {
			continue
		}
		q.seen[j.IdempotencyKey] = true
		q.jobs = append(q.jobs, j)
	}
	return nil
}

func (q *inboxJobs) all() []queue.NewJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]queue.NewJob(nil), q.jobs...)
}

func inboxRecordsOnce(t *testing.T, e *inboxEnv) {
	ctx := t.Context()
	in := e.inbox(kernel.Limit{}, kernel.Limit{})
	id := inboxAlice + "/activities/1"
	body := inboxCreate(t, id)
	got, err := e.receive(t, in, inboxPost{body: body, signature: inboxAliceKey + " yes"})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if !got.Recorded || got.Dropped || got.ActivityURI != federation.IRI(id) || got.Trust != federation.TrustSigned {
		t.Errorf("Receive = %+v, want recorded %s, signed", got, id)
	}
	a, err := e.h.Inbox.FindReceivedActivity(ctx, federation.IRI(id))
	if err != nil {
		t.Fatalf("FindReceivedActivity: %v", err)
	}
	want := federation.ReceivedActivity{
		ActivityURI: federation.IRI(id), ActorURI: inboxAlice, Type: "Create", Scheme: "cavage",
		Trust: federation.TrustSigned, ReceivedAt: e.clock.Now(),
	}
	if !a.ReceivedAt.Equal(want.ReceivedAt) {
		t.Errorf("ReceivedAt = %v, want %v", a.ReceivedAt, want.ReceivedAt)
	}
	a.ReceivedAt = want.ReceivedAt
	if a != want {
		t.Errorf("received activity = %+v, want %+v", a, want)
	}
	doc, err := e.h.Inbox.FindReceivedDocument(ctx, federation.IRI(id))
	if err != nil {
		t.Fatalf("FindReceivedDocument: %v", err)
	}
	if !bytes.Equal(doc, body) {
		t.Errorf("document = %s, want %s", doc, body)
	}
	jobs := e.jobs.all()
	if len(jobs) != 1 {
		t.Fatalf("jobs = %+v, want one federation.inbox", jobs)
	}
	sum := sha256.Sum256([]byte(id))
	j := jobs[0]
	if j.Type != appfederation.JobInbox || j.IdempotencyKey != "inbox:"+hex.EncodeToString(sum[:]) || j.OrderingKey != "inbox:"+inboxAlice {
		t.Errorf("job = %s %q %q, want %s, inbox:<sha256 of the URI>, inbox:<actor>", j.Type, j.IdempotencyKey, j.OrderingKey, appfederation.JobInbox)
	}
	var payload map[string]any
	if err := json.Unmarshal(j.Payload, &payload); err != nil || payload["activity_uri"] != id || payload["trust"] != "signed" {
		t.Errorf("payload = %s, want activity_uri %s and trust signed", j.Payload, id)
	}

	// The same activity again, as a sender retries it, records nothing and enqueues nothing.
	e.clock.Advance(time.Minute)
	again, err := e.receive(t, in, inboxPost{body: body, signature: inboxAliceKey + " yes"})
	if err != nil {
		t.Fatalf("Receive again: %v", err)
	}
	if again.Recorded || again.ActivityURI != federation.IRI(id) {
		t.Errorf("Receive again = %+v, want not recorded", again)
	}
	if n := len(e.jobs.all()); n != 1 {
		t.Errorf("jobs after the retry = %d, want 1", n)
	}
	if a, err := e.h.Inbox.FindReceivedActivity(ctx, federation.IRI(id)); err != nil || !a.ReceivedAt.Equal(want.ReceivedAt) {
		t.Errorf("received activity after the retry = %+v, %v; want the first", a, err)
	}
}

// inboxKeepsDocumentByteExact checks that the document reads back with the bytes it arrived
// with, whatever a JSON column would change: whitespace, key order, a duplicate key (the
// asynchronous processing reads the first), escapes, and an escaped NUL or lone surrogate, which
// a PostgreSQL jsonb column refuses.
func inboxKeepsDocumentByteExact(t *testing.T, e *inboxEnv) {
	in := e.inbox(kernel.Limit{}, kernel.Limit{})
	id := inboxAlice + "/activities/raw"
	body := []byte(strings.NewReplacer("ID", id, "ACTOR", inboxAlice).Replace(`{ "type":"Create",
	"id": "ID", "actor":"ACTOR", "@context": "https://www.w3.org/ns/activitystreams",
  "object": {"type": "Note", "id": "ACTOR/notes/raw", "content": "first", "content": "second",
    "summary": "nul \u0000, lone \ud800, \u00e9 vs é, \/"},
  "zz": 1.50, "aa": 1e2 }
`))
	got, err := e.receive(t, in, inboxPost{body: body, signature: inboxAliceKey + " yes"})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if !got.Recorded || got.ActivityURI != federation.IRI(id) {
		t.Fatalf("Receive = %+v, want recorded %s", got, id)
	}
	doc, err := e.h.Inbox.FindReceivedDocument(t.Context(), federation.IRI(id))
	if err != nil {
		t.Fatalf("FindReceivedDocument: %v", err)
	}
	if !bytes.Equal(doc, body) {
		t.Errorf("document = %q, want %q", doc, body)
	}
}

func inboxSynthesizesURI(t *testing.T, e *inboxEnv) {
	in := e.inbox(kernel.Limit{}, kernel.Limit{})
	body := inboxActivity(t, map[string]any{"type": "Like", "actor": inboxAlice, "object": "https://" + contractHost + "/ap/objects/x"})
	got, err := e.receive(t, in, inboxPost{body: body, signature: inboxAliceKey + " yes"})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	h := sha256.New()
	h.Write([]byte(inboxAlice + "\n"))
	h.Write(body)
	want := federation.IRI("urn:my:digest:" + hex.EncodeToString(h.Sum(nil)))
	if !got.Recorded || got.ActivityURI != want {
		t.Errorf("Receive = %+v, want recorded %s", got, want)
	}
	if _, err := e.h.Inbox.FindReceivedActivity(t.Context(), want); err != nil {
		t.Errorf("FindReceivedActivity(%s): %v", want, err)
	}
	if again, err := e.receive(t, in, inboxPost{body: body, signature: inboxAliceKey + " yes"}); err != nil || again.Recorded {
		t.Errorf("Receive again = %+v, %v; want not recorded", again, err)
	}
	if n := len(e.jobs.all()); n != 1 {
		t.Errorf("jobs = %d, want 1", n)
	}
}

func inboxMarksRefetch(t *testing.T, e *inboxEnv) {
	in := e.inbox(kernel.Limit{}, kernel.Limit{})
	tests := []struct {
		name, id, signature string
	}{
		{"relayed by another actor", inboxAlice + "/activities/relayed", inboxRelayKey + " yes"},
		{"id on another host than the actor", "https://elsewhere.example/activities/1", inboxAliceKey + " yes"},
		{"key owner on another host than the key", inboxAlice + "/activities/evil", "https://evil.example/key yes"},
	}
	for _, tt := range tests {
		got, err := e.receive(t, in, inboxPost{body: inboxCreate(t, tt.id), signature: tt.signature})
		if err != nil {
			t.Fatalf("%s: Receive: %v", tt.name, err)
		}
		if !got.Recorded || got.Trust != federation.TrustRefetch {
			t.Errorf("%s: Receive = %+v, want recorded with trust refetch", tt.name, got)
		}
		a, err := e.h.Inbox.FindReceivedActivity(t.Context(), federation.IRI(tt.id))
		if err != nil || a.Trust != federation.TrustRefetch {
			t.Errorf("%s: received activity = %+v, %v; want trust refetch", tt.name, a, err)
		}
	}
	for _, j := range e.jobs.all() {
		var payload map[string]any
		if err := json.Unmarshal(j.Payload, &payload); err != nil || payload["trust"] != "refetch" {
			t.Errorf("payload = %s, want trust refetch", j.Payload)
		}
	}
}

func inboxRefuses(t *testing.T, e *inboxEnv) {
	e.keys.unavailable["https://down.example/actor#main-key"] = true
	one := kernel.Limit{Name: "test", Max: 1, Window: time.Minute}
	id := inboxAlice + "/activities/refused"
	good := inboxCreate(t, id)
	tests := []struct {
		name        string
		ipLimit     kernel.Limit
		domainLimit kernel.Limit
		// before is sent first, from another activity, to use up a limit.
		before          *inboxPost
		post            inboxPost
		status          int
		acceptSignature bool
	}{
		{name: "no signature", post: inboxPost{body: good}, status: http.StatusUnauthorized, acceptSignature: true},
		{name: "malformed signature", post: inboxPost{body: good, signature: "garbage"}, status: http.StatusBadRequest},
		{name: "keyId without a host", post: inboxPost{body: good, signature: "main-key yes"}, status: http.StatusUnauthorized, acceptSignature: true},
		{name: "suspended domain", post: inboxPost{body: good, signature: "https://blocked.example/actor#main-key yes"}, status: http.StatusForbidden},
		{
			name: "IP rate limit", ipLimit: one,
			before: &inboxPost{body: inboxCreate(t, inboxAlice+"/activities/first"), signature: inboxAliceKey + " yes", ip: "198.51.100.7"},
			post:   inboxPost{body: good, signature: inboxAliceKey + " yes", ip: "198.51.100.7"}, status: http.StatusTooManyRequests,
		},
		{
			name: "domain rate limit", domainLimit: one,
			before: &inboxPost{body: inboxCreate(t, inboxAlice+"/activities/second"), signature: inboxAliceKey + " yes"},
			post:   inboxPost{body: good, signature: inboxAliceKey + " yes"}, status: http.StatusTooManyRequests,
		},
		{name: "digest mismatch", post: inboxPost{body: good, signature: inboxAliceKey + " yes", digest: make([]byte, 32)}, status: http.StatusUnauthorized, acceptSignature: true},
		{name: "not JSON", post: inboxPost{body: []byte("{"), signature: inboxAliceKey + " yes"}, status: http.StatusBadRequest},
		{name: "no actor", post: inboxPost{body: inboxActivity(t, map[string]any{"id": id, "type": "Create"}), signature: inboxAliceKey + " yes"}, status: http.StatusBadRequest},
		{name: "unknown key", post: inboxPost{body: good, signature: "https://remote.example/users/nobody#main-key yes"}, status: http.StatusUnauthorized, acceptSignature: true},
		{name: "key origin unavailable", post: inboxPost{body: good, signature: "https://down.example/actor#main-key yes"}, status: http.StatusServiceUnavailable},
		{name: "signature does not verify", post: inboxPost{body: good, signature: inboxAliceKey + " no"}, status: http.StatusUnauthorized, acceptSignature: true},
	}
	for _, tt := range tests {
		in := e.inbox(tt.ipLimit, tt.domainLimit)
		if tt.before != nil {
			if _, err := e.receive(t, in, *tt.before); err != nil {
				t.Fatalf("%s: first request: %v", tt.name, err)
			}
		}
		jobs := len(e.jobs.all())
		_, err := e.receive(t, in, tt.post)
		var rej *appfederation.InboxRejection
		if !errors.As(err, &rej) {
			t.Errorf("%s: Receive = %v, want an InboxRejection", tt.name, err)
			continue
		}
		if rej.Status != tt.status || (rej.AcceptSignature != "") != tt.acceptSignature {
			t.Errorf("%s: rejection = %d %q, want %d (Accept-Signature %t)", tt.name, rej.Status, rej.AcceptSignature, tt.status, tt.acceptSignature)
		}
		if tt.status == http.StatusTooManyRequests && rej.RetryAfter <= 0 {
			t.Errorf("%s: RetryAfter = %v, want > 0", tt.name, rej.RetryAfter)
		}
		if _, err := e.h.Inbox.FindReceivedActivity(t.Context(), federation.IRI(id)); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("%s: FindReceivedActivity = %v, want ErrNotFound", tt.name, err)
		}
		if n := len(e.jobs.all()); n != jobs {
			t.Errorf("%s: jobs = %d, want %d", tt.name, n, jobs)
		}
		e.clock.Advance(time.Hour) // a fresh window for the next case's limits
	}
}

// inboxChargesDomainLimitAfterVerification checks that requests whose signature does not verify
// leave the signing server's rate limit to the server itself.
func inboxChargesDomainLimitAfterVerification(t *testing.T, e *inboxEnv) {
	in := e.inbox(kernel.Limit{}, kernel.Limit{Name: "test", Max: 1, Window: time.Hour})
	for i := range 3 {
		_, err := e.receive(t, in, inboxPost{body: inboxCreate(t, fmt.Sprintf("%s/activities/unverified/%d", inboxAlice, i)), signature: inboxAliceKey + " no"})
		var rej *appfederation.InboxRejection
		if !errors.As(err, &rej) || rej.Status != http.StatusUnauthorized {
			t.Fatalf("unverified request %d: Receive = %v, want 401", i, err)
		}
	}
	got, err := e.receive(t, in, inboxPost{body: inboxCreate(t, inboxAlice+"/activities/verified"), signature: inboxAliceKey + " yes"})
	if err != nil || !got.Recorded {
		t.Fatalf("verified request: Receive = %+v, %v; want recorded within the domain limit", got, err)
	}
	_, err = e.receive(t, in, inboxPost{body: inboxCreate(t, inboxAlice+"/activities/verified/2"), signature: inboxAliceKey + " yes"})
	var rej *appfederation.InboxRejection
	if !errors.As(err, &rej) || rej.Status != http.StatusTooManyRequests {
		t.Errorf("second verified request: Receive = %v, want 429", err)
	}
}

func inboxDropsUnknownSelfDelete(t *testing.T, e *inboxEnv) {
	in := e.inbox(kernel.Limit{}, kernel.Limit{})
	stranger := "https://remote.example/users/gone"
	e.keys.owners[stranger+"#main-key"] = stranger
	del := inboxActivity(t, map[string]any{"id": stranger + "#delete", "type": "Delete", "actor": stranger, "object": stranger})
	got, err := e.receive(t, in, inboxPost{body: del, signature: stranger + "#main-key yes"})
	if err != nil || !got.Dropped || got.Recorded {
		t.Errorf("Receive(unknown Delete) = %+v, %v; want dropped", got, err)
	}
	if n := e.keys.called(stranger + "#main-key"); n != 0 {
		t.Errorf("key resolved %d times for a dropped Delete, want 0", n)
	}
	if _, err := e.h.Inbox.FindReceivedActivity(t.Context(), federation.IRI(stranger+"#delete")); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindReceivedActivity = %v, want ErrNotFound", err)
	}
	// A known actor's Delete of itself is recorded.
	known := inboxActivity(t, map[string]any{"id": inboxAlice + "#delete", "type": "Delete", "actor": inboxAlice, "object": inboxAlice})
	got, err = e.receive(t, in, inboxPost{body: known, signature: inboxAliceKey + " yes"})
	if err != nil || !got.Recorded {
		t.Errorf("Receive(known Delete) = %+v, %v; want recorded", got, err)
	}
	if n := len(e.jobs.all()); n != 1 {
		t.Errorf("jobs = %d, want 1", n)
	}
}

func inboxMarksServerAvailable(t *testing.T, e *inboxEnv) {
	ctx := t.Context()
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.h.Federation.UpsertInstanceHealth(ctx, tx, federation.InstanceHealth{
			Domain: "remote.example", State: federation.InstanceUnavailable, Since: e.clock.Now(), FailureDays: 7, LastFailureDay: "20261017",
		})
	})
	if err != nil {
		t.Fatalf("UpsertInstanceHealth: %v", err)
	}
	in := e.inbox(kernel.Limit{}, kernel.Limit{})
	// A refused request does not show that the server is up.
	if _, err := e.receive(t, in, inboxPost{body: inboxCreate(t, inboxAlice+"/activities/bad"), signature: inboxAliceKey + " no"}); err == nil {
		t.Fatal("Receive with a bad signature succeeded")
	}
	if s, err := e.h.Federation.InstanceState(ctx, "remote.example"); err != nil || s != federation.InstanceUnavailable {
		t.Errorf("InstanceState after a refused request = %q, %v; want unavailable", s, err)
	}
	if _, err := e.receive(t, in, inboxPost{body: inboxCreate(t, inboxAlice+"/activities/up"), signature: inboxAliceKey + " yes"}); err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if s, err := e.h.Federation.InstanceState(ctx, "remote.example"); err != nil || s != federation.InstanceAvailable {
		t.Errorf("InstanceState after a signed request = %q, %v; want available", s, err)
	}
	events, err := e.h.Federation.ListAvailabilityEvents(ctx, "remote.example", "", 10)
	if err != nil || len(events) != 1 || events[0].Kind != federation.MarkedAvailable || events[0].Reason != federation.ReasonInboundRequest {
		t.Errorf("availability events = %+v, %v; want one marked_available for inbound_request", events, err)
	}
}

func inboxRecordsTombstonesOnce(t *testing.T, e *inboxEnv) {
	ctx := t.Context()
	note := federation.IRI(inboxAlice + "/notes/gone")
	first := federation.RemoteObjectTombstone{
		URI: note, ActorURI: inboxAlice, ActivityURI: inboxAlice + "/activities/delete-1", ReceivedAt: e.clock.Now(),
	}
	if ok, err := e.h.Inbox.RemoteObjectTombstoned(ctx, note, inboxAlice); err != nil || ok {
		t.Fatalf("RemoteObjectTombstoned before = %t, %v; want false", ok, err)
	}
	insert := func(ts federation.RemoteObjectTombstone) bool {
		t.Helper()
		var inserted bool
		err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			inserted, err = e.h.Inbox.InsertRemoteObjectTombstone(ctx, tx, ts)
			return err
		})
		if err != nil {
			t.Fatalf("InsertRemoteObjectTombstone: %v", err)
		}
		return inserted
	}
	// Another actor of the same origin records a tombstone of the URI first: it holds back only
	// that actor's objects, and the owner's own tombstone of the URI is still recorded.
	carol := federation.IRI("https://remote.example/users/carol")
	if !insert(federation.RemoteObjectTombstone{
		URI: note, ActorURI: carol, ActivityURI: carol + "/activities/delete-1", ReceivedAt: e.clock.Now(),
	}) {
		t.Error("InsertRemoteObjectTombstone(another actor) = false, want it recorded")
	}
	if ok, err := e.h.Inbox.RemoteObjectTombstoned(ctx, note, inboxAlice); err != nil || ok {
		t.Errorf("RemoteObjectTombstoned after another actor's tombstone = %t, %v; want false", ok, err)
	}
	if !insert(first) {
		t.Error("InsertRemoteObjectTombstone = false, want the first tombstone of the owner recorded")
	}
	again := first
	again.ActivityURI = inboxAlice + "/activities/delete-2"
	again.ReceivedAt = e.clock.Now().Add(time.Minute)
	if insert(again) {
		t.Error("InsertRemoteObjectTombstone again = true, want the first tombstone to stay")
	}
	if ok, err := e.h.Inbox.RemoteObjectTombstoned(ctx, note, inboxAlice); err != nil || !ok {
		t.Errorf("RemoteObjectTombstoned = %t, %v; want true", ok, err)
	}
	if ok, err := e.h.Inbox.RemoteObjectTombstoned(ctx, inboxAlice+"/notes/other", inboxAlice); err != nil || ok {
		t.Errorf("RemoteObjectTombstoned(other) = %t, %v; want false", ok, err)
	}
	// The tombstone holds back only what its actor owns.
	if ok, err := e.h.Inbox.RemoteObjectTombstoned(ctx, note, "https://remote.example/users/dave"); err != nil || ok {
		t.Errorf("RemoteObjectTombstoned(another actor) = %t, %v; want false", ok, err)
	}
}

// inboxVerifiesProofs checks the integrity proofs of received activities (FEP-8b32): a proof
// that verifies with the actor's key passes, one that does not or that names another actor's
// key is refused, and one that cannot be checked is ignored, as is the proof of a relayed
// activity, which is fetched again anyway.
func inboxVerifiesProofs(t *testing.T, e *inboxEnv) {
	in := e.inbox(kernel.Limit{}, kernel.Limit{})
	aliceEd, bobEd := inboxAlice+"#ed25519-key", "https://remote.example/users/bob#ed25519-key"
	alicePub, alicePriv, _ := ed25519.GenerateKey(rand.Reader)
	bobPub, bobPriv, _ := ed25519.GenerateKey(rand.Reader)
	e.keys.mu.Lock()
	e.keys.owners[aliceEd], e.keys.ed[aliceEd] = inboxAlice, alicePub
	e.keys.owners[bobEd], e.keys.ed[bobEd] = "https://remote.example/users/bob", bobPub
	e.keys.mu.Unlock()
	sign := func(body []byte, key ed25519.PrivateKey, keyID string) []byte {
		t.Helper()
		signed, err := federation.SignEdDSAJCS2022(body, key, keyID, e.clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}
	n := 0
	create := func() (string, []byte) {
		n++
		id := fmt.Sprintf("%s/activities/proof-%d", inboxAlice, n)
		return id, inboxCreate(t, id)
	}

	accepted := []struct {
		name string
		body func(id string, doc []byte) []byte
		key  string
	}{
		{"a proof that verifies", func(_ string, doc []byte) []byte { return sign(doc, alicePriv, aliceEd) }, inboxAliceKey},
		{"a proof of an unknown key", func(_ string, doc []byte) []byte {
			return sign(doc, alicePriv, inboxAlice+"#unknown-key")
		}, inboxAliceKey},
		{"a proof of another cryptosuite", func(_ string, doc []byte) []byte {
			return bytes.Replace(sign(doc, alicePriv, aliceEd), []byte(`"eddsa-jcs-2022"`), []byte(`"eddsa-rdfc-2022"`), 1)
		}, inboxAliceKey},
		{"a proof that names an RSA key", func(_ string, doc []byte) []byte { return sign(doc, alicePriv, inboxAliceKey) }, inboxAliceKey},
		{"a relayed activity with a broken proof", func(_ string, doc []byte) []byte {
			return bytes.Replace(sign(doc, alicePriv, aliceEd), []byte(`"hi"`), []byte(`"bye"`), 1)
		}, inboxRelayKey},
	}
	for _, tt := range accepted {
		id, doc := create()
		got, err := e.receive(t, in, inboxPost{body: tt.body(id, doc), signature: tt.key + " yes"})
		if err != nil || !got.Recorded {
			t.Errorf("%s: Receive = %+v, %v; want recorded", tt.name, got, err)
		}
	}

	refused := []struct {
		name string
		body func(doc []byte) []byte
	}{
		{"a proof of a changed document", func(doc []byte) []byte {
			return bytes.Replace(sign(doc, alicePriv, aliceEd), []byte(`"hi"`), []byte(`"bye"`), 1)
		}},
		{"a proof of another actor's key", func(doc []byte) []byte { return sign(doc, bobPriv, bobEd) }},
		{"a proof made with another key", func(doc []byte) []byte { return sign(doc, bobPriv, aliceEd) }},
	}
	for _, tt := range refused {
		id, doc := create()
		_, err := e.receive(t, in, inboxPost{body: tt.body(doc), signature: inboxAliceKey + " yes"})
		var rej *appfederation.InboxRejection
		if !errors.As(err, &rej) || rej.Status != http.StatusUnauthorized || rej.Reason() != appfederation.ReasonProofMismatch {
			t.Errorf("%s: Receive = %v, want a 401 for the proof", tt.name, err)
			continue
		}
		if _, err := e.h.Inbox.FindReceivedActivity(t.Context(), federation.IRI(id)); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("%s: FindReceivedActivity = %v, want ErrNotFound", tt.name, err)
		}
	}
}
