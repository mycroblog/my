// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// Fetching the featured hashtags of a remote account, its featuredTags collection, when the
// account is fetched or refreshed, or when it sends an Add or Remove whose target is its featured
// collection, which Mastodon sends when the featured hashtags change (federation.md §3.4, §6.2,
// ADR-0006 §17).
const (
	// JobFetchFeaturedTags fetches the featuredTags collection of a remote account and makes its
	// featured hashtags those the collection lists.
	JobFetchFeaturedTags = "federation.fetch_featured_tags"
	// FeaturedTagsMaxTags bounds the hashtags that one fetch keeps (timeline.MaxFeaturedTags, the
	// featured hashtags of a local account), and FeaturedTagsMaxDocuments the documents it
	// fetches: the collection and its first page. The posts are not fetched.
	FeaturedTagsMaxTags      = 10
	FeaturedTagsMaxDocuments = 2
)

// RemoteFeaturedTags replaces the featured hashtags of a remote account.
// *apptimeline.Service implements it.
type RemoteFeaturedTags interface {
	// ReplaceRemoteFeaturedTags makes the account's featured hashtags the valid names among
	// names, in their order, at most FeaturedTagsMaxTags.
	ReplaceRemoteFeaturedTags(ctx context.Context, accountID string, names []string) error
}

// fetchFeaturedTagsPayload is the payload of JobFetchFeaturedTags.
type fetchFeaturedTagsPayload struct {
	AccountID string `json:"account_id"`
}

// enqueueFetchFeaturedTagsTx enqueues in tx the JobFetchFeaturedTags of the remote account
// accountID, whose actor document was just stored or which changed what it features, when the
// actor declares a featuredTags collection of its origin. One job of an account is pending at a
// time.
func enqueueFetchFeaturedTagsTx(ctx context.Context, tx uow.Tx, outbox queue.Outbox, accountID string, actor, featuredTags federation.IRI) error {
	if outbox == nil || featuredTags == "" || !federation.SameOrigin(featuredTags, actor) {
		return nil
	}
	payload, err := json.Marshal(fetchFeaturedTagsPayload{AccountID: accountID})
	if err != nil {
		return err
	}
	return outbox.EnqueueTx(ctx, tx, queue.NewJob{
		Type: JobFetchFeaturedTags, Payload: payload, IdempotencyKey: "fetch_featured_tags:" + accountID,
	})
}

// enqueueFetchFeaturedAllTx enqueues in tx the fetches of what a remote actor features that its
// document declares: its pinned posts (JobFetchFeatured) and its featured hashtags
// (JobFetchFeaturedTags).
func enqueueFetchFeaturedAllTx(ctx context.Context, tx uow.Tx, outbox queue.Outbox, accountID string, actor *federation.Actor) error {
	if err := enqueueFetchFeaturedTx(ctx, tx, outbox, accountID, actor.ID, actor.Featured); err != nil {
		return err
	}
	return enqueueFetchFeaturedTagsTx(ctx, tx, outbox, accountID, actor.ID, actor.FeaturedTags)
}

// handleFetchFeaturedTags is the queue.Handler of JobFetchFeaturedTags. A collection that cannot
// be fetched ends the job and leaves the featured hashtags as they are; the failures of the
// stores retry it.
func (h *StatusHandlers) handleFetchFeaturedTags(ctx context.Context, j queue.Job) queue.Result {
	var p fetchFeaturedTagsPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return queue.Fail(fmt.Errorf("federation: fetch_featured_tags payload %s: %w", j.Payload, err))
	}
	if p.AccountID == "" {
		return queue.Fail(fmt.Errorf("federation: fetch_featured_tags payload %s has no account_id", j.Payload))
	}
	n, err := h.fetchFeaturedTags(ctx, p.AccountID)
	switch {
	case err == nil:
		h.d.Logger.InfoContext(ctx, "remote featured hashtags fetched", slog.String("account_id", p.AccountID), slog.Int("featured", n))
		return queue.Done()
	case isFetchError(err), errors.Is(err, ErrNotTaken):
		h.d.Logger.InfoContext(ctx, "remote featured hashtags not fetched", slog.String("account_id", p.AccountID), slog.Any("reason", err))
		return queue.Done()
	}
	return queue.Retry(0, err)
}

// fetchFeaturedTags does the work of one JobFetchFeaturedTags: it reads the Hashtag items of the
// account's featuredTags collection, or of its first page of the account's origin, and makes the
// first FeaturedTagsMaxTags of them the account's featured hashtags. A document that no longer
// declares a featuredTags collection, or an empty one, removes them. It returns how many
// hashtags the collection listed.
func (h *StatusHandlers) fetchFeaturedTags(ctx context.Context, accountID string) (int, error) {
	if h.d.FeaturedTags == nil || h.d.Profiles == nil {
		return 0, nil
	}
	actor, collection, ok, err := h.remoteCollectionOf(ctx, accountID, h.d.Profiles.RemoteFeaturedTagsURL)
	if err != nil || !ok {
		return 0, err
	}
	var names []string
	if collection != "" {
		page, err := h.collectionPage(ctx, newFetchBudget(FeaturedTagsMaxDocuments), federation.IRI(actor.Account.URI), federation.IRI(collection))
		if err != nil {
			return 0, err
		}
		if page != nil {
			names = page.Hashtags
		}
	}
	if err := h.d.FeaturedTags.ReplaceRemoteFeaturedTags(ctx, accountID, names); err != nil {
		return 0, err
	}
	return min(len(names), FeaturedTagsMaxTags), nil
}
