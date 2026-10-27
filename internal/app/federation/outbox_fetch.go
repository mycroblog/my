// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/federation"
)

// Fetching the recent posts of a remote profile that a user opens (federation.md §6.3, ADR-0044).
const (
	// JobFetchOutbox fetches the first page of the outbox of a remote account and stores its
	// recent public posts.
	JobFetchOutbox = "federation.fetch_outbox"
	// OutboxMaxPosts bounds the posts that one fetch of an outbox takes, and OutboxMaxDocuments
	// the documents it fetches: the collection, its first page, and the posts.
	OutboxMaxPosts     = 20
	OutboxMaxDocuments = OutboxMaxPosts + 2
	// OutboxFetchTTL is how long the outbox of an account is not fetched again after a fetch
	// started; OutboxFetchNamespace keeps those starts, keyed by the SHA-256 of the actor's URI.
	OutboxFetchTTL       = 24 * time.Hour
	OutboxFetchNamespace = "federation.outbox_fetch"
	// OutboxUserLimit bounds the accounts whose outboxes one user starts fetching in
	// OutboxUserWindow; OutboxUserNamespace counts them, keyed by the user's account ID.
	OutboxUserLimit     = 30
	OutboxUserWindow    = 5 * time.Minute
	OutboxUserNamespace = "federation.outbox_fetch_user"
)

// OutboxAccounts is the part of the account module's public API that fetching outboxes and
// featured collections reads: the account and the collections of its newest document.
// *appaccount.Service implements it.
type OutboxAccounts interface {
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
	// RemoteOutboxURL and RemoteFeaturedURL fail with appaccount.ErrNotFound for a local account.
	RemoteOutboxURL(ctx context.Context, accountID string) (string, error)
	RemoteFeaturedURL(ctx context.Context, accountID string) (string, error)
}

// fetchOutboxPayload is the payload of JobFetchOutbox.
type fetchOutboxPayload struct {
	AccountID string `json:"account_id"`
}

// fetchOutboxJob is the JobFetchOutbox of accountID enqueued at now: one per account and day.
func fetchOutboxJob(accountID string, now time.Time) (queue.NewJob, error) {
	payload, err := json.Marshal(fetchOutboxPayload{AccountID: accountID})
	if err != nil {
		return queue.NewJob{}, err
	}
	day := now.UTC().Truncate(OutboxFetchTTL).Format(time.DateOnly)
	return queue.NewJob{Type: JobFetchOutbox, Payload: payload, IdempotencyKey: "fetch_outbox:" + accountID + ":" + day}, nil
}

// FetchOutbox starts fetching the recent posts of the account accountID, whose profile the
// local account viewerID opened (ADR-0044), and reports whether it did. It does only for an
// active remote account with an outbox, without the federation restriction, when neither of the
// two blocks the other, when the account's outbox was not fetched in the last OutboxFetchTTL,
// and when the viewer started fewer than OutboxUserLimit fetches in the last OutboxUserWindow.
// The fetch runs as a JobFetchOutbox job; FetchOutbox does not wait for it.
func (h *StatusHandlers) FetchOutbox(ctx context.Context, viewerID, accountID string) (bool, error) {
	if h.d.Jobs == nil || h.d.KV == nil || h.d.Profiles == nil || viewerID == "" || viewerID == accountID {
		return false, nil
	}
	actor, outbox, ok, err := h.outboxOf(ctx, accountID)
	if err != nil || !ok || outbox == "" {
		return false, err
	}
	if h.d.Relationships != nil {
		rel, err := h.d.Relationships.Relationship(ctx, viewerID, accountID)
		if err != nil {
			return false, err
		}
		if rel.Blocking || rel.BlockedBy {
			return false, nil
		}
	}
	key := uriKey(actor.Account.URI)
	started, err := h.d.KV.IncrBy(ctx, OutboxFetchNamespace, key, 1, OutboxFetchTTL)
	if err != nil || started > 1 {
		return false, err
	}
	n, err := h.d.KV.IncrBy(ctx, OutboxUserNamespace, viewerID, 1, OutboxUserWindow)
	if err == nil && n > OutboxUserLimit {
		// Over the viewer's limit: leave the account for a later viewer, and the count as it was.
		_, err = h.d.KV.IncrBy(ctx, OutboxUserNamespace, viewerID, -1, OutboxUserWindow)
		err = errors.Join(err, h.d.KV.Delete(ctx, OutboxFetchNamespace, key))
		return false, err
	}
	if err != nil {
		return false, errors.Join(err, h.d.KV.Delete(ctx, OutboxFetchNamespace, key))
	}
	job, err := fetchOutboxJob(accountID, h.d.Clock.Now())
	if err == nil {
		err = h.d.Jobs.Enqueue(ctx, job)
	}
	if err != nil {
		return false, errors.Join(err, h.d.KV.Delete(ctx, OutboxFetchNamespace, key))
	}
	return true, nil
}

// outboxOf returns the account accountID and its outbox when the account is an active remote
// account without the federation restriction (ok); the outbox is "" when the account's document
// declares none or one of another origin.
func (h *StatusHandlers) outboxOf(ctx context.Context, accountID string) (appaccount.Actor, string, bool, error) {
	return h.remoteCollectionOf(ctx, accountID, h.d.Profiles.RemoteOutboxURL)
}

// remoteCollectionOf returns the account accountID and the collection that collection reads of
// it when the account is an active remote account without the federation restriction (ok); the
// collection is "" when the account's document declares none or one of another origin.
func (h *StatusHandlers) remoteCollectionOf(ctx context.Context, accountID string, collection func(ctx context.Context, accountID string) (string, error)) (appaccount.Actor, string, bool, error) {
	actor, err := h.d.Profiles.FindByID(ctx, accountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return appaccount.Actor{}, "", false, nil
	case err != nil:
		return appaccount.Actor{}, "", false, err
	case actor.Account.IsLocal() || !actor.Lifecycle.Active():
		return actor, "", false, nil
	}
	if h.d.Restrictions != nil {
		suspended, err := h.d.Restrictions.AccountFederationSuspended(ctx, accountID)
		if err != nil || suspended {
			return actor, "", false, err
		}
	}
	uri, err := collection(ctx, accountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return actor, "", false, nil
	case err != nil:
		return actor, "", false, err
	case !federation.SameOrigin(federation.IRI(uri), federation.IRI(actor.Account.URI)):
		return actor, "", true, nil
	}
	return actor, uri, true, nil
}

// handleFetchOutbox is the queue.Handler of JobFetchOutbox. A document that cannot be fetched
// ends the job, as do posts that are not taken; the failures of the stores retry it.
func (h *StatusHandlers) handleFetchOutbox(ctx context.Context, j queue.Job) queue.Result {
	var p fetchOutboxPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return queue.Fail(fmt.Errorf("federation: fetch_outbox payload %s: %w", j.Payload, err))
	}
	if p.AccountID == "" {
		return queue.Fail(fmt.Errorf("federation: fetch_outbox payload %s has no account_id", j.Payload))
	}
	stored, err := h.fetchOutbox(ctx, p.AccountID)
	switch {
	case err == nil:
		h.d.Logger.InfoContext(ctx, "remote outbox fetched", slog.String("account_id", p.AccountID), slog.Int("stored", stored))
		return queue.Done()
	case isFetchError(err), errors.Is(err, ErrNotTaken):
		h.d.Logger.InfoContext(ctx, "remote outbox not fetched", slog.String("account_id", p.AccountID), slog.Any("reason", err))
		return queue.Done()
	}
	return queue.Retry(0, err)
}

// fetchBudget counts the documents that one fetch of a collection (an outbox, a featured
// collection) fetches: left of at most limit.
type fetchBudget struct{ left, limit int }

func newFetchBudget(limit int) *fetchBudget { return &fetchBudget{left: limit, limit: limit} }

// fetchWithin fetches uri when the budget allows another document; it fails with ErrFetchRefused
// otherwise.
func (h *StatusHandlers) fetchWithin(ctx context.Context, b *fetchBudget, uri federation.IRI) (*Fetched, error) {
	if b.left <= 0 {
		return nil, fmt.Errorf("%w: %s: the collection fetch reached %d documents", ErrFetchRefused, uri, b.limit)
	}
	b.left--
	return h.d.Fetcher.Fetch(ctx, string(uri))
}

// fetchOutbox does the work of one JobFetchOutbox: it reads the first page of the account's
// outbox and stores its newest OutboxMaxPosts public posts by the account, fetching those that
// the page does not embed and that are not stored. It returns how many it stored.
func (h *StatusHandlers) fetchOutbox(ctx context.Context, accountID string) (int, error) {
	actor, outbox, ok, err := h.outboxOf(ctx, accountID)
	if err != nil || !ok || outbox == "" {
		return 0, err
	}
	author := federation.IRI(actor.Account.URI)
	budget := newFetchBudget(OutboxMaxDocuments)
	page, err := h.collectionPage(ctx, budget, author, federation.IRI(outbox))
	if err != nil || page == nil {
		return 0, err
	}
	stored := 0
	for _, post := range outboxPosts(page, author) {
		got, err := h.outboxPost(ctx, budget, author, post)
		switch {
		case err == nil && got:
			stored++
		case err == nil, isFetchError(err), errors.Is(err, ErrNotTaken):
			if err != nil {
				h.d.Logger.InfoContext(ctx, "remote outbox post not taken", slog.String("uri", string(post.ID)), slog.Any("reason", err))
			}
		default:
			return stored, err
		}
	}
	return stored, nil
}

// collectionPage returns the page of a collection of author (its outbox, its featured
// collection) to read: the collection itself when it lists items, or else its first page,
// embedded or fetched. Only a page of the author's origin is read; it returns nil when there is
// none.
func (h *StatusHandlers) collectionPage(ctx context.Context, b *fetchBudget, author, collection federation.IRI) (*federation.Collection, error) {
	got, err := h.fetchWithin(ctx, b, collection)
	if err != nil {
		return nil, err
	}
	c := got.Document.Collection
	switch {
	case c == nil:
		return nil, fmt.Errorf("%w: %s is not a collection", ErrNotTaken, collection)
	case c.Page || len(c.Items) > 0:
		return c, nil
	}
	if doc := c.First.Trusted(); doc != nil {
		if doc.Collection == nil || (doc.Collection.ID != "" && !federation.SameOrigin(doc.Collection.ID, author)) {
			return nil, nil
		}
		return doc.Collection, nil
	}
	if c.First.ID == "" || !federation.SameOrigin(c.First.ID, author) {
		return nil, nil
	}
	if got, err = h.fetchWithin(ctx, b, c.First.ID); err != nil {
		return nil, err
	}
	return got.Document.Collection, nil
}

// outboxPosts returns the posts that page lists in Create activities of author addressed to
// Public, newest first, at most OutboxMaxPosts: the object of each Create, embedded and
// attributed to author, or by an IRI of author's origin. Announces and other activities, and
// activities the page refers to only by IRI, are left out.
func outboxPosts(page *federation.Collection, author federation.IRI) []federation.Ref {
	type post struct {
		ref       federation.Ref
		published time.Time
	}
	var posts []post
	dated := true
	for _, item := range page.Items {
		doc := item.Trusted()
		if doc == nil || doc.Activity == nil {
			continue
		}
		a := doc.Activity
		obj := a.Object()
		if a.Type != federation.ActivityCreate || a.Actor != author || obj.ID == "" {
			continue
		}
		published := a.Published
		if o := obj.Trusted(); o != nil {
			if o.Object == nil || o.Object.AttributedTo != author || !addressedToPublic(o.Object.Audience) {
				continue
			}
			if published.IsZero() {
				published = o.Object.Published
			}
		} else if !addressedToPublic(a.Audience) || !federation.SameOrigin(obj.ID, author) {
			continue
		}
		dated = dated && !published.IsZero()
		posts = append(posts, post{ref: obj, published: published})
	}
	if dated {
		// Outboxes list the newest first; the published times keep that order when one does not.
		slices.SortStableFunc(posts, func(x, y post) int { return y.published.Compare(x.published) })
	}
	out := make([]federation.Ref, 0, min(len(posts), OutboxMaxPosts))
	seen := map[federation.IRI]bool{}
	for _, p := range posts {
		if len(out) == OutboxMaxPosts {
			break
		}
		if !seen[p.ref.ID] {
			seen[p.ref.ID] = true
			out = append(out, p.ref)
		}
	}
	return out
}

func addressedToPublic(a federation.Audience) bool {
	return slices.Contains(a.To, federation.PublicIRI) || slices.Contains(a.Cc, federation.PublicIRI)
}

// outboxPost stores the post that ref refers to unless it is stored (reporting false): the
// embedded document as it is, or the document fetched from its IRI when it has the author's
// origin, is addressed to Public, and is attributed to the author. Its parent is not fetched.
func (h *StatusHandlers) outboxPost(ctx context.Context, b *fetchBudget, author federation.IRI, ref federation.Ref) (bool, error) {
	_, stored, err := h.collectionPost(ctx, b, author, ref)
	return stored, err
}

// collectionPost returns the status of the post that ref refers to: the stored one, or else the
// one it stores (reporting stored) from the embedded document as it is, or from the document
// fetched from its IRI when it has the author's origin, is addressed to Public, and is attributed
// to the author. Its parent is not fetched.
func (h *StatusHandlers) collectionPost(ctx context.Context, b *fetchBudget, author federation.IRI, ref federation.Ref) (appcontent.Status, bool, error) {
	switch st, err := h.d.Statuses.FindStatusByURI(ctx, string(ref.ID)); {
	case err == nil:
		return st, false, nil
	case !errors.Is(err, appcontent.ErrNotFound):
		return appcontent.Status{}, false, err
	}
	got := &Fetched{Document: ref.Trusted()}
	if got.Document == nil {
		if !federation.SameOrigin(ref.ID, author) {
			return appcontent.Status{}, false, fmt.Errorf("%w: %s has another origin than its author %s", ErrNotTaken, ref.ID, author)
		}
		if err := h.untombstoned(ctx, ref.ID, author); err != nil {
			return appcontent.Status{}, false, err
		}
		var err error
		if got, err = h.fetchWithin(ctx, b, ref.ID); err != nil {
			return appcontent.Status{}, false, err
		}
		obj := got.Document.Object
		if obj == nil || obj.AttributedTo != author || !addressedToPublic(obj.Audience) {
			return appcontent.Status{}, false, fmt.Errorf("%w: %s is not a public post of %s", ErrNotTaken, ref.ID, author)
		}
	}
	st, err := h.fetched(ctx, got, false)
	if err != nil {
		return appcontent.Status{}, false, err
	}
	return st, true, nil
}
