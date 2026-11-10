// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// Fetching the pinned posts of a remote account, its featured collection, when the account is
// fetched or refreshed, or when it sends an Add or Remove whose target is the collection
// (federation.md §3.4, §6.2, ADR-0006).
const (
	// JobFetchFeatured fetches the featured collection of a remote account and makes its pins
	// those the collection lists.
	JobFetchFeatured = "federation.fetch_featured"
	// FeaturedMaxPosts bounds the pins that one fetch takes (content.MaxPins, the pins of a
	// local account), and FeaturedMaxDocuments the documents it fetches: the collection, its
	// first page, and the posts.
	FeaturedMaxPosts     = 5
	FeaturedMaxDocuments = FeaturedMaxPosts + 2
)

// RemotePins replaces the pins of a remote account. *appcontent.Interactions implements it.
type RemotePins interface {
	// ReplaceRemotePins makes the account's pins the statuses statusIDs, the newest pin first.
	ReplaceRemotePins(ctx context.Context, accountID string, statusIDs []string) error
}

// fetchFeaturedPayload is the payload of JobFetchFeatured.
type fetchFeaturedPayload struct {
	AccountID string `json:"account_id"`
}

// enqueueFetchFeaturedTx enqueues in tx the JobFetchFeatured of the remote account accountID,
// whose actor document was just stored or which changed its pins, when the actor declares a
// featured collection of its origin. One job of an account is pending at a time.
func enqueueFetchFeaturedTx(ctx context.Context, tx uow.Tx, outbox queue.Outbox, accountID string, actor, featured federation.IRI) error {
	if outbox == nil || !mirrorsCollection(actor, featured) {
		return nil
	}
	payload, err := json.Marshal(fetchFeaturedPayload{AccountID: accountID})
	if err != nil {
		return err
	}
	return outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: JobFetchFeatured, Payload: payload, IdempotencyKey: "fetch_featured:" + accountID})
}

// FeaturedChanged is the inbox handler of Add and Remove (federation.md §3.4): an Add or Remove
// whose target is the featured collection of its actor, as the stored actor document declares
// it, enqueues the JobFetchFeatured of the actor, which makes the pins those the collection lists
// then, and the JobFetchFeaturedTags of the actor when its document declares a featuredTags
// collection: Mastodon sends the change of its featured hashtags as the Add or Remove of a Hashtag
// to the featured collection (ADR-0006 §17). The activity's object is not read: fetching the
// collections again applies the same checks and bounds as the fetch of a refreshed actor, and
// catches up on the changes that did not arrive. The caller has checked that the signer is the
// activity's actor (federation.md §3.2), so the actor is the owner of the collection. Any other
// Add or Remove is dropped, as is one of an account whose featured collection is not mirrored.
// While a fetch of the account is pending, the activity enqueues none.
func (h *StatusHandlers) FeaturedChanged(ctx context.Context, a *InboxActivity) error {
	target := a.Activity.Target
	if (h.d.Pins == nil && h.d.FeaturedTags == nil) || h.d.Profiles == nil || h.d.Outbox == nil || target == "" ||
		!federation.SameOrigin(target, a.Activity.Actor) {
		return fmt.Errorf("%w: %s to %q by %s", ErrDropActivity, a.Activity.Type, target, a.Activity.Actor)
	}
	accountID := a.Actor.Account.ID
	featured, err := ignoreNotFound(h.d.Profiles.RemoteFeaturedURL(ctx, accountID))
	if err != nil {
		return err
	}
	if featured == "" || federation.IRI(featured) != target {
		return fmt.Errorf("%w: %s to %s, not the featured collection of %s", ErrDropActivity, a.Activity.Type, target, a.Activity.Actor)
	}
	var featuredTags string
	if h.d.FeaturedTags != nil {
		if featuredTags, err = ignoreNotFound(h.d.Profiles.RemoteFeaturedTagsURL(ctx, accountID)); err != nil {
			return err
		}
	}
	return h.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if h.d.Pins != nil {
			if err := enqueueFetchFeaturedTx(ctx, tx, h.d.Outbox, accountID, a.Activity.Actor, target); err != nil {
				return err
			}
		}
		return enqueueFetchFeaturedTagsTx(ctx, tx, h.d.Outbox, accountID, a.Activity.Actor, federation.IRI(featuredTags))
	})
}

// ignoreNotFound returns uri, or "" for an account that appaccount.ErrNotFound reports unknown.
func ignoreNotFound(uri string, err error) (string, error) {
	if errors.Is(err, appaccount.ErrNotFound) {
		return "", nil
	}
	return uri, err
}

// handleFetchFeatured is the queue.Handler of JobFetchFeatured. A collection that cannot be
// fetched ends the job and leaves the pins as they are; the failures of the stores retry it.
func (h *StatusHandlers) handleFetchFeatured(ctx context.Context, j queue.Job) queue.Result {
	var p fetchFeaturedPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return queue.Fail(fmt.Errorf("federation: fetch_featured payload %s: %w", j.Payload, err))
	}
	if p.AccountID == "" {
		return queue.Fail(fmt.Errorf("federation: fetch_featured payload %s has no account_id", j.Payload))
	}
	pinned, err := h.fetchFeatured(ctx, p.AccountID)
	switch {
	case err == nil:
		h.d.Logger.InfoContext(ctx, "remote featured collection fetched", slog.String("account_id", p.AccountID), slog.Int("pinned", pinned))
		return queue.Done()
	case isFetchError(err), errors.Is(err, ErrNotTaken):
		h.d.Logger.InfoContext(ctx, "remote featured collection not fetched", slog.String("account_id", p.AccountID), slog.Any("reason", err))
		return queue.Done()
	}
	return queue.Retry(0, err)
}

// fetchFeatured does the work of one JobFetchFeatured: it reads the first page of the account's
// featured collection and makes the account's pins the first FeaturedMaxPosts posts of the
// account that it lists, storing those that are not stored. A document that no longer declares a
// featured collection, or an empty one, removes the pins. It returns how many posts are pinned.
func (h *StatusHandlers) fetchFeatured(ctx context.Context, accountID string) (int, error) {
	if h.d.Pins == nil || h.d.Profiles == nil {
		return 0, nil
	}
	actor, featured, ok, err := h.remoteCollectionOf(ctx, accountID, h.d.Profiles.RemoteFeaturedURL)
	if err != nil || !ok {
		return 0, err
	}
	var ids []string
	if featured != "" {
		author := federation.IRI(actor.Account.URI)
		budget := newFetchBudget(FeaturedMaxDocuments)
		page, err := h.collectionPage(ctx, budget, author, federation.IRI(featured))
		if err != nil {
			return 0, err
		}
		if ids, err = h.featuredPosts(ctx, budget, author, page); err != nil {
			return 0, err
		}
	}
	if err := h.d.Pins.ReplaceRemotePins(ctx, accountID, ids); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// featuredPosts returns the IDs of the statuses of the first FeaturedMaxPosts posts of author
// that page lists, in its order, storing those that are not stored: the embedded posts
// attributed to author and addressed to Public, and the posts that page refers to by an IRI of
// author's origin. Activities and the posts that cannot be taken are left out.
func (h *StatusHandlers) featuredPosts(ctx context.Context, b *fetchBudget, author federation.IRI, page *federation.Collection) ([]string, error) {
	if page == nil {
		return nil, nil
	}
	var ids []string
	for _, item := range page.Items {
		if len(ids) == FeaturedMaxPosts {
			break
		}
		if item.ID == "" {
			continue
		}
		if doc := item.Trusted(); doc != nil {
			if doc.Object == nil || doc.Object.AttributedTo != author || !addressedToPublic(doc.Object.Audience) {
				continue
			}
		}
		st, _, err := h.collectionPost(ctx, b, author, item)
		switch {
		case err == nil:
			ids = append(ids, st.ID)
		case isFetchError(err), errors.Is(err, ErrNotTaken):
			h.d.Logger.InfoContext(ctx, "remote featured post not taken", slog.String("uri", string(item.ID)), slog.Any("reason", err))
		default:
			return ids, err
		}
	}
	return ids, nil
}
