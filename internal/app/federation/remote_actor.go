// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"crypto"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// Jobs and schedule of the remote actors (federation.md §6.2).
const (
	// JobRefreshActor fetches one remote actor again.
	JobRefreshActor = "federation.refresh_actor"
	// JobRefreshActors walks the stale remote actors in batches and refreshes the ones that
	// local accounts follow.
	JobRefreshActors = "federation.refresh_stale_actors"
	// JobObserveInstance reads the NodeInfo of a server seen for the first time.
	JobObserveInstance = "federation.observe_instance"
	// RefreshActorsEvery is the period of JobRefreshActors.
	RefreshActorsEvery = time.Hour
	// RefreshActorsBatch is the number of stale actors one JobRefreshActors reads.
	RefreshActorsBatch = 200
	// KeyResolveTimeout bounds fetching the actor of an unknown key for an inbound signature
	// (federation.md §3.1); storing it is not cut.
	KeyResolveTimeout = 5 * time.Second
)

// RemoteAccounts is the part of the account module's public API that stores and reads remote
// actors. *appaccount.Service implements it.
type RemoteAccounts interface {
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
	FindByURI(ctx context.Context, uri string) (appaccount.Actor, error)
	// FindByHandle returns the actor with a confirmed handle (account_handles).
	FindByHandle(ctx context.Context, username, domain string) (appaccount.Actor, error)
	RemoteHead(ctx context.Context, accountID string) (appaccount.RemoteHead, error)
	CurrentKey(ctx context.Context, keyID string) (appaccount.Key, error)
	StaleRemoteActors(ctx context.Context, after appaccount.RemoteHead, limit int) ([]appaccount.RemoteHead, error)
	UpsertRemoteActor(ctx context.Context, tx uow.Tx, in appaccount.RemoteActorInput) (appaccount.Account, error)
	// ReviewRemoteProfile returns what the hook inbound.actor reviews of a fetched actor.
	ReviewRemoteProfile(ctx context.Context, in appaccount.RemoteActorInput) (appaccount.RemoteProfileReview, error)
	MarkRemoteDeleted(ctx context.Context, tx uow.Tx, accountID string) error
	MarkRemoteGone(ctx context.Context, tx uow.Tx, accountID string) error
	RestoreRemoteActor(ctx context.Context, tx uow.Tx, accountID string) error
	RemoteDeletionUnconfirmed(ctx context.Context, accountID string) (bool, error)
	RecordRemoteFetchFailure(ctx context.Context, accountID string) error
	// EnsureInstanceActor returns the instance actor, creating it with its keys on first use.
	EnsureInstanceActor(ctx context.Context) (appaccount.Account, error)
	PrivateKey(ctx context.Context, k appaccount.Key) (crypto.Signer, error)
}

// Followers says whether local accounts follow a remote actor: the remote actor's active
// followers, which are local. *appgraph.Service implements it.
type Followers interface {
	ActiveFollowerIDs(ctx context.Context, accountID, before string, limit int) (ids []string, next string, err error)
}

// RemoteActorsDeps are the ports and settings RemoteActors uses.
type RemoteActorsDeps struct {
	UnitOfWork uow.UnitOfWork
	Accounts   RemoteAccounts
	Instances  InstanceRepository
	Fetcher    *Fetcher
	// HTTP sends the unsigned WebFinger and NodeInfo requests.
	HTTP kernel.HTTPClient
	// Jobs enqueues refreshes outside transactions; Outbox enqueues inside them.
	Jobs      queue.JobQueue
	Outbox    queue.Outbox
	Followers Followers
	Clock     kernel.Clock
	IDs       kernel.IDGenerator
	Logger    *slog.Logger
	// Review asks the hook inbound.actor about a changed profile before it is stored (ADR-0147
	// §2); its zero value asks nothing.
	Review ActorReview
	// Batch is the number of stale actors one JobRefreshActors reads; 0 means
	// RefreshActorsBatch.
	Batch int
	// KeyResolveTimeout bounds the fetch of ResolvePublicKey; 0 means KeyResolveTimeout.
	KeyResolveTimeout time.Duration
}

// RemoteActors fetches remote actors and keeps them in the account module (federation.md
// §6.2): the document through the Fetcher, the handle through WebFinger, the keys and their
// rotation, 410 Gone as a remote deletion, the servers seen and their NodeInfo, and the refresh
// of actors fetched RemoteActorTTL ago. It implements PublicKeyResolver.
type RemoteActors struct {
	d RemoteActorsDeps
}

var _ PublicKeyResolver = (*RemoteActors)(nil)

// NewRemoteActors returns RemoteActors on d.
func NewRemoteActors(d RemoteActorsDeps) *RemoteActors {
	if d.Batch <= 0 {
		d.Batch = RefreshActorsBatch
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &RemoteActors{d: d}
}

// InstanceKey returns the RSA key of the instance actor, which signs every fetch (§2). The
// instance actor is created on the first fetch.
func InstanceKey(accounts RemoteAccounts) func(ctx context.Context) (SigningKey, error) {
	return func(ctx context.Context) (SigningKey, error) {
		ia, err := accounts.EnsureInstanceActor(ctx)
		if err != nil {
			return SigningKey{}, err
		}
		a, err := accounts.FindByID(ctx, ia.ID)
		if err != nil {
			return SigningKey{}, err
		}
		for _, k := range a.Keys {
			priv, err := accounts.PrivateKey(ctx, k)
			if err != nil {
				return SigningKey{}, err
			}
			if _, ok := priv.(*rsa.PrivateKey); ok {
				return SigningKey{KeyID: k.KeyID, PrivateKey: priv}, nil
			}
		}
		return SigningKey{}, errors.New("federation: the instance actor has no RSA key")
	}
}

// Resolve returns the remote actor with the id uri: the stored one, or the fetched one when it
// is unknown. A stored actor that is stale is returned as it is and refreshed asynchronously.
// It fails with the errors of Fetcher.Fetch, or ErrFetchNotFound when uri is not an actor.
func (r *RemoteActors) Resolve(ctx context.Context, uri string) (appaccount.Actor, error) {
	target, err := fetchTarget(uri)
	if err != nil {
		return appaccount.Actor{}, fmt.Errorf("%w: %w", ErrFetchNotFound, err)
	}
	a, err := r.d.Accounts.FindByURI(ctx, string(target))
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return r.Refresh(ctx, string(target))
	case err != nil:
		return appaccount.Actor{}, err
	}
	if !a.Account.IsLocal() {
		r.refreshIfStale(ctx, a.Account.ID)
	}
	return a, nil
}

// Refresh fetches the actor at uri (an actor's id, or a key id whose document leads to the
// actor) and stores it. When the stored actor's server answers 410 Gone, the actor is marked
// deleted; another failure is counted in the actor's head.
func (r *RemoteActors) Refresh(ctx context.Context, uri string) (appaccount.Actor, error) {
	return r.refreshWithin(ctx, ctx, uri)
}

// refreshWithin is Refresh with the fetch bound by fetchCtx and everything else, the storing
// included, by ctx: a deadline meant for the remote server never cuts a transaction or the reads
// after it.
func (r *RemoteActors) refreshWithin(ctx, fetchCtx context.Context, uri string) (appaccount.Actor, error) {
	actor, raw, err := r.fetchActor(fetchCtx, uri)
	if err != nil {
		r.recordFailure(ctx, uri, err)
		return appaccount.Actor{}, err
	}
	return r.store(ctx, actor, raw)
}

// RecheckDeletion implements appaccount.DeletionRechecker. An actor that only a 410 marked deleted
// is fetched once more: another 410 confirms the deletion, and its document brings the actor back.
// Any other outcome says nothing. The actor's own Delete, and an actor whose head the erasure has
// removed already, need no fetch.
func (r *RemoteActors) RecheckDeletion(ctx context.Context, accountID string) (appaccount.DeletionRecheck, error) {
	unconfirmed, err := r.d.Accounts.RemoteDeletionUnconfirmed(ctx, accountID)
	if err != nil {
		return appaccount.RecheckInconclusive, err
	}
	if !unconfirmed {
		return appaccount.RecheckConfirmed, nil
	}
	switch _, err := r.d.Accounts.RemoteHead(ctx, accountID); {
	case errors.Is(err, appaccount.ErrNotFound):
		return appaccount.RecheckConfirmed, nil
	case err != nil:
		return appaccount.RecheckInconclusive, err
	}
	a, err := r.d.Accounts.FindByID(ctx, accountID)
	if err != nil {
		return appaccount.RecheckInconclusive, err
	}
	actor, raw, err := r.fetchActor(ctx, a.Account.URI)
	switch {
	case errors.Is(err, ErrFetchGone):
		return appaccount.RecheckConfirmed, nil
	case ctx.Err() != nil:
		return appaccount.RecheckInconclusive, ctx.Err()
	case err != nil:
		return appaccount.RecheckInconclusive, nil
	case string(actor.ID) != a.Account.URI:
		return appaccount.RecheckInconclusive, nil
	}
	restore := func(tx uow.Tx) error { return r.d.Accounts.RestoreRemoteActor(ctx, tx, accountID) }
	if _, err := r.storeAfter(ctx, actor, raw, restore); err != nil {
		if errors.Is(err, appaccount.ErrActorDeleted) { // its Delete came during the fetch
			return appaccount.RecheckConfirmed, nil
		}
		return appaccount.RecheckInconclusive, err
	}
	r.d.Logger.InfoContext(ctx, "remote actor restored: its server serves it again after a 410", slog.String("uri", a.Account.URI))
	return appaccount.RecheckRestored, nil
}

// store saves a fetched actor in the account module and records its server.
func (r *RemoteActors) store(ctx context.Context, actor *federation.Actor, raw []byte) (appaccount.Actor, error) {
	stored, err := r.storeAfter(ctx, actor, raw, nil)
	if errors.Is(err, appaccount.ErrActorDeleted) {
		return r.d.Accounts.FindByURI(ctx, string(actor.ID)) // a deleted actor stays deleted, and stays erased
	}
	return stored, err
}

// storeAfter asks the hook inbound.actor about the actor's profile, then runs before, when set,
// and stores the fetched actor in one transaction.
func (r *RemoteActors) storeAfter(ctx context.Context, actor *federation.Actor, raw []byte, before func(uow.Tx) error) (appaccount.Actor, error) {
	in := r.input(ctx, actor, raw)
	v, err := r.reviewActor(ctx, &in)
	if err != nil {
		return appaccount.Actor{}, err
	}
	var stored appaccount.Account
	err = r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if before != nil {
			if err := before(tx); err != nil {
				return err
			}
		}
		var err error
		if stored, err = r.d.Accounts.UpsertRemoteActor(ctx, tx, in); err != nil {
			return err
		}
		if err := r.applyActorVerdict(ctx, tx, v, stored.ID); err != nil {
			return err
		}
		if err := enqueueFetchFeaturedAllTx(ctx, tx, r.d.Outbox, stored.ID, actor); err != nil {
			return err
		}
		return r.seeInstance(ctx, tx, in.Domain)
	})
	if err != nil {
		return appaccount.Actor{}, err
	}
	return r.d.Accounts.FindByID(ctx, stored.ID)
}

// fetchActor fetches uri and returns the actor it is or, for a key document, the actor that
// owns the key.
func (r *RemoteActors) fetchActor(ctx context.Context, uri string) (*federation.Actor, []byte, error) {
	got, err := r.d.Fetcher.FetchActor(ctx, uri)
	if err != nil {
		return nil, nil, err
	}
	if got.Document.Actor != nil {
		return usableActor(got)
	}
	owner := keyOwner(got.Raw)
	if owner == "" || !federation.SameOrigin(owner, got.Document.ID()) {
		return nil, nil, fmt.Errorf("%w: %s is not an actor", ErrFetchNotFound, uri)
	}
	got, err = r.d.Fetcher.FetchActor(ctx, string(owner))
	if err != nil {
		return nil, nil, err
	}
	if got.Document.Actor == nil {
		return nil, nil, fmt.Errorf("%w: the owner %s of %s is not an actor", ErrFetchNotFound, owner, uri)
	}
	return usableActor(got)
}

// usableActor returns a fetched actor that can be stored: one with an inbox. An actor with a
// shared inbox but no inbox of its own, as the service actors of Pleroma and Akkoma (the relay
// actor /relay, whose inbox is null) are, receives at its shared inbox.
func usableActor(got *Fetched) (*federation.Actor, []byte, error) {
	a := got.Document.Actor
	if a.Inbox == "" {
		a.Inbox = a.SharedInbox
	}
	if a.Inbox == "" || a.ID.Host() == "" {
		return nil, nil, fmt.Errorf("%w: the actor %s has no inbox", ErrFetchNotFound, a.ID)
	}
	return a, got.Raw, nil
}

// input turns a fetched actor into the account module's input, with its keys and handle.
func (r *RemoteActors) input(ctx context.Context, a *federation.Actor, raw []byte) appaccount.RemoteActorInput {
	in := appaccount.RemoteActorInput{
		URI: string(a.ID), Domain: a.ID.Host(), Type: string(a.Type),
		PreferredUsername: a.PreferredUsername, Name: a.Name, SummaryHTML: a.Summary,
		ProfileURL: string(a.URL), InboxURL: string(a.Inbox), SharedInboxURL: string(a.SharedInbox),
		OutboxURL: string(a.Outbox), FollowersURL: string(a.Followers), FollowingURL: string(a.Following),
		FeaturedURL: string(a.Featured), FeaturedTagsURL: string(a.FeaturedTags), AvatarURL: string(a.Icon), HeaderURL: string(a.Image),
		Locked: a.ManuallyApprovesFollowers, Bot: a.Bot(), Discoverable: a.Discoverable, Indexable: a.Indexable,
		Keys: r.actorKeys(ctx, a, raw), Document: raw, Handle: r.checkHandle(ctx, a),
	}
	for _, f := range a.Fields {
		in.Fields = append(in.Fields, appaccount.RemoteFieldInput{Name: f.Name, ValueHTML: f.Value})
	}
	for _, e := range a.Emojis {
		in.Emojis = append(in.Emojis, appaccount.RemoteEmojiInput{URI: string(e.ID), Shortcode: e.Shortcode, ImageURL: string(e.IconURL)})
	}
	return in
}

// seeInstance records a server the first time a document of it is stored, and enqueues the
// observation of its NodeInfo.
func (r *RemoteActors) seeInstance(ctx context.Context, tx uow.Tx, domain string) error {
	inserted, err := r.d.Instances.InsertRemoteInstance(ctx, tx, domain, r.d.Clock.Now())
	if err != nil || !inserted {
		return err
	}
	payload, err := json.Marshal(observePayload{Domain: domain})
	if err != nil {
		return err
	}
	return r.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{
		Type: JobObserveInstance, Payload: payload, IdempotencyKey: "observe_instance:" + domain,
	})
}

// recordFailure keeps the outcome of a failed fetch of a stored actor: 410 Gone marks it
// deleted on its server, other fetch failures count in its head.
func (r *RemoteActors) recordFailure(ctx context.Context, uri string, cause error) {
	if !isFetchError(cause) || errors.Is(cause, ErrFederationRestricted) {
		return // a fetch that the federation mode does not make says nothing of the actor
	}
	target, err := fetchTarget(uri)
	if err != nil {
		return
	}
	a, err := r.d.Accounts.FindByURI(ctx, string(target))
	if err != nil || a.Account.IsLocal() {
		return
	}
	if errors.Is(cause, ErrFetchGone) {
		err = r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.d.Accounts.MarkRemoteGone(ctx, tx, a.Account.ID) })
	} else {
		err = r.d.Accounts.RecordRemoteFetchFailure(ctx, a.Account.ID)
	}
	if err != nil {
		r.d.Logger.WarnContext(ctx, "record remote actor fetch failure", slog.String("uri", uri), slog.Any("error", err))
	}
}

func isFetchError(err error) bool {
	return errors.Is(err, ErrFetchGone) || errors.Is(err, ErrFetchNotFound) || errors.Is(err, ErrFetchFailed) ||
		errors.Is(err, ErrFetchUnavailable) || errors.Is(err, ErrFetchRefused)
}

// refreshIfStale enqueues a refresh of a remote actor fetched RemoteActorTTL ago.
func (r *RemoteActors) refreshIfStale(ctx context.Context, accountID string) {
	head, err := r.d.Accounts.RemoteHead(ctx, accountID)
	if err != nil || !head.Stale(r.d.Clock.Now()) {
		return
	}
	if err := r.d.Jobs.Enqueue(ctx, refreshJob(accountID)); err != nil {
		r.d.Logger.WarnContext(ctx, "enqueue remote actor refresh", slog.String("account_id", accountID), slog.Any("error", err))
	}
}

// ResolvePublicKey implements PublicKeyResolver: the current key of keyID from account_keys,
// or the key of the fetched owner when it is unknown or refresh is set (ADR-0006). A stored
// key of an actor fetched RemoteActorTTL ago is used and the actor refreshed asynchronously.
// Fetching gives up after KeyResolveTimeout, and the key is then unavailable (503, which the
// sender retries); storing the fetched actor runs with ctx alone.
func (r *RemoteActors) ResolvePublicKey(ctx context.Context, keyID string, refresh bool) (PublicKey, error) {
	kid, err := CanonicalIRI(keyID)
	if err != nil {
		return PublicKey{}, fmt.Errorf("%w: %w", ErrPublicKeyNotFound, err)
	}
	if !refresh {
		k, err := r.d.Accounts.CurrentKey(ctx, string(kid))
		switch {
		case err == nil:
			key, err := r.publicKey(ctx, k)
			if err == nil {
				r.refreshIfStale(ctx, k.AccountID)
			}
			return key, err
		case !errors.Is(err, appaccount.ErrNotFound):
			return PublicKey{}, err
		}
	}
	timeout := r.d.KeyResolveTimeout
	if timeout <= 0 {
		timeout = KeyResolveTimeout
	}
	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	owner, err := r.refreshWithin(ctx, fetchCtx, string(kid))
	switch {
	case err == nil:
	// A server that did not answer within the timeout, while the request itself goes on, is
	// unavailable now, whatever the fetch made of the cut.
	case errors.Is(err, ErrFetchUnavailable), ctx.Err() == nil && fetchCtx.Err() != nil:
		return PublicKey{}, fmt.Errorf("%w: %w", ErrPublicKeyUnavailable, err)
	case isFetchError(err):
		return PublicKey{}, fmt.Errorf("%w: %w", ErrPublicKeyNotFound, err)
	case err != nil:
		return PublicKey{}, err
	}
	k, err := r.d.Accounts.CurrentKey(ctx, string(kid))
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return PublicKey{}, fmt.Errorf("%w: %s does not declare %s", ErrPublicKeyNotFound, owner.Account.URI, kid)
	case err != nil:
		return PublicKey{}, err
	case k.AccountID != owner.Account.ID:
		return PublicKey{}, fmt.Errorf("%w: %s is not a key of %s", ErrPublicKeyNotFound, kid, owner.Account.URI)
	}
	return r.publicKey(ctx, k)
}

func (r *RemoteActors) publicKey(ctx context.Context, k appaccount.Key) (PublicKey, error) {
	pub, err := appaccount.ParsePublicKeyPEM(k.PublicKeyPEM)
	if err != nil {
		return PublicKey{}, fmt.Errorf("%w: stored key %s: %w", ErrPublicKeyNotFound, k.KeyID, err)
	}
	owner, err := r.d.Accounts.FindByID(ctx, k.AccountID)
	if err != nil {
		return PublicKey{}, fmt.Errorf("owner of key %s: %w", k.KeyID, err)
	}
	return PublicKey{KeyID: k.KeyID, Owner: owner.Account.URI, Key: pub}, nil
}

// RegisterJobs registers the handlers of JobRefreshActor, JobRefreshActors, and
// JobObserveInstance, and schedules JobRefreshActors every RefreshActorsEvery.
func (r *RemoteActors) RegisterJobs(h *queue.JobHandlerRegistry, s *queue.Scheduler) {
	h.Register(JobRefreshActor, r.handleRefresh)
	h.Register(JobRefreshActors, r.handleRefreshAll)
	h.Register(JobObserveInstance, r.handleObserve)
	s.Add(queue.Schedule{Name: JobRefreshActors, Every: RefreshActorsEvery, Job: queue.NewJob{Type: JobRefreshActors}})
}

type refreshPayload struct {
	AccountID string `json:"account_id"`
}

func refreshJob(accountID string) queue.NewJob {
	payload, _ := json.Marshal(refreshPayload{AccountID: accountID})
	return queue.NewJob{Type: JobRefreshActor, Payload: payload, IdempotencyKey: "refresh_actor:" + accountID}
}

func (r *RemoteActors) handleRefresh(ctx context.Context, j queue.Job) queue.Result {
	var p refreshPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.AccountID == "" {
		return queue.Fail(fmt.Errorf("federation: refresh payload %s: %w", j.Payload, err))
	}
	a, err := r.d.Accounts.FindByID(ctx, p.AccountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return queue.Done()
	case err != nil:
		return queue.Retry(0, err)
	case a.Account.IsLocal(), a.Deleted():
		return queue.Done()
	}
	_, err = r.Refresh(ctx, a.Account.URI)
	switch {
	case err == nil:
		return queue.Done()
	case isFetchError(err):
		// Recorded in the actor's head or lifecycle; the periodic refresh tries again later.
		r.d.Logger.InfoContext(ctx, "remote actor not refreshed", slog.String("uri", a.Account.URI), slog.Any("reason", err))
		return queue.Done()
	}
	return queue.Retry(0, err)
}

// refreshAllPayload is the cursor of a JobRefreshActors run: the last stale head read.
type refreshAllPayload struct {
	AfterFetchedAt int64  `json:"after_fetched_at,omitempty"` // Unix milliseconds
	AfterAccountID string `json:"after_account_id,omitempty"`
}

// followedAmong is a Followers that tells the actors of a batch with an active follower at once,
// as graph.Service does.
type followedAmong interface {
	FollowedAmong(ctx context.Context, accountIDs []string) (map[string]bool, error)
}

// followed returns the actors of heads that local accounts follow: with one read when Followers
// can, one by one otherwise.
func (r *RemoteActors) followed(ctx context.Context, heads []appaccount.RemoteHead) (map[string]bool, error) {
	ids := make([]string, len(heads))
	for i, h := range heads {
		ids[i] = h.AccountID
	}
	if b, ok := r.d.Followers.(followedAmong); ok {
		return b.FollowedAmong(ctx, ids)
	}
	out := map[string]bool{}
	for _, id := range ids {
		followers, _, err := r.d.Followers.ActiveFollowerIDs(ctx, id, "", 1)
		if err != nil {
			return nil, err
		}
		out[id] = len(followers) > 0
	}
	return out, nil
}

// handleRefreshAll reads one batch of stale actors, enqueues a refresh of each one that local
// accounts follow, and enqueues the next batch (federation.md §6.2, ADR-0006).
func (r *RemoteActors) handleRefreshAll(ctx context.Context, j queue.Job) queue.Result {
	var p refreshAllPayload
	if len(j.Payload) > 0 && string(j.Payload) != "null" {
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return queue.Fail(fmt.Errorf("federation: refresh payload %s: %w", j.Payload, err))
		}
	}
	var after appaccount.RemoteHead
	if p.AfterAccountID != "" {
		after = appaccount.RemoteHead{AccountID: p.AfterAccountID, LastFetchedAt: time.UnixMilli(p.AfterFetchedAt).UTC()}
	}
	heads, err := r.d.Accounts.StaleRemoteActors(ctx, after, r.d.Batch)
	if err != nil {
		return queue.Retry(0, err)
	}
	followed, err := r.followed(ctx, heads)
	if err != nil {
		return queue.Retry(0, err)
	}
	var jobs []queue.NewJob
	for _, h := range heads {
		if followed[h.AccountID] {
			jobs = append(jobs, refreshJob(h.AccountID))
		}
	}
	if len(heads) == r.d.Batch {
		last := heads[len(heads)-1]
		next := refreshAllPayload{AfterFetchedAt: last.LastFetchedAt.UnixMilli(), AfterAccountID: last.AccountID}
		payload, err := json.Marshal(next)
		if err != nil {
			return queue.Fail(err)
		}
		jobs = append(jobs, queue.NewJob{
			Type: JobRefreshActors, Payload: payload,
			IdempotencyKey: "refresh_actors:" + strconv.FormatInt(next.AfterFetchedAt, 10) + ":" + next.AfterAccountID,
		})
	}
	if len(jobs) == 0 {
		return queue.Done()
	}
	if err := r.d.Jobs.Enqueue(ctx, jobs...); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}
