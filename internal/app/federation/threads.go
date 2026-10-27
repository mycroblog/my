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
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// Fetching remote posts and their threads (federation.md §6.3, ADR-0034).
const (
	// JobFetchObject fetches one remote post and, within the bounds of its payload, enqueues
	// the fetches of its ancestors and replies.
	JobFetchObject = "federation.fetch_object"
	// ThreadDepth bounds the ancestors, and the levels of replies, that opening a thread fetches.
	ThreadDepth = 20
	// ThreadMaxItems bounds the documents (posts and pages of replies collections) that the
	// fetches of one thread get together. The ancestors take at most ThreadDepth of them, which
	// the replies leave: the replies get ThreadMaxItems - ThreadDepth.
	ThreadMaxItems = 100
	// ThreadFetchTTL is how long a thread is not fetched again after it was opened, and how
	// long the count of its documents lasts.
	ThreadFetchTTL = time.Hour
	// ThreadFetchNamespace is the KeyValueStore namespace of the threads opened in the last
	// ThreadFetchTTL, and ThreadBudgetNamespace that of the documents fetched for each; both
	// are keyed by the SHA-256 of the URI of the opened post.
	ThreadFetchNamespace  = "federation.thread_fetch"
	ThreadBudgetNamespace = "federation.thread_budget"
)

// ErrNotTaken reports a fetched document that is not stored as a remote post: it is not a post,
// its attributedTo is missing or of another origin than its id, its author cannot be resolved or
// is not an active remote account, or a deletion or tombstone keeps its URI.
var ErrNotTaken = errors.New("federation: the remote object is not taken")

// RemoteObjectTombstones tells which URIs have a tombstone of a Delete or Undo whose object was
// unknown (federation.md §3.3), left by the actor that owns the object. InboxRepository
// implements it.
type RemoteObjectTombstones interface {
	RemoteObjectTombstoned(ctx context.Context, uri, actorURI federation.IRI) (bool, error)
}

// fetchObjectPayload is the payload of JobFetchObject.
type fetchObjectPayload struct {
	URI string `json:"uri"`
	// Thread is the URI of the post whose opening the fetch is part of: every document fetched
	// counts against its ThreadMaxItems. It is empty for the fetch of a parent on receipt.
	Thread string `json:"thread,omitempty"`
	// Ancestors is how many levels of inReplyTo above the post to fetch.
	Ancestors int `json:"ancestors,omitempty"`
	// Replies is how many levels of replies below the post to fetch.
	Replies int `json:"replies,omitempty"`
	// Up marks the fetch of an ancestor, which spends the documents that the replies leave.
	Up bool `json:"up,omitempty"`
}

func fetchObjectJob(p fetchObjectPayload) (queue.NewJob, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return queue.NewJob{}, err
	}
	sum := sha256.Sum256(payload)
	return queue.NewJob{Type: JobFetchObject, Payload: payload, IdempotencyKey: "fetch_object:" + hex.EncodeToString(sum[:])}, nil
}

// RegisterJobs registers the handlers of JobFetchObject, JobFetchOutbox, JobFetchFeatured, and
// JobVerifyQuote.
func (h *StatusHandlers) RegisterJobs(r *queue.JobHandlerRegistry) {
	r.Register(JobFetchObject, h.handleFetchObject)
	r.Register(JobFetchOutbox, h.handleFetchOutbox)
	r.Register(JobFetchFeatured, h.handleFetchFeatured)
	r.Register(JobVerifyQuote, h.handleVerifyQuote)
}

// fetchParentTx enqueues in tx the fetch of the parent of a reply that was just ingested, when
// the parent is not stored (its conversation is unknown) and is remote: one level, with no
// further ancestors (federation.md §6.3).
func (h *StatusHandlers) fetchParentTx(ctx context.Context, tx uow.Tx, st appcontent.Status) error {
	parent := federation.IRI(st.InReplyToURI)
	if h.d.Outbox == nil || parent == "" || st.ConversationURI != "" || h.isLocal(parent) {
		return nil
	}
	job, err := fetchObjectJob(fetchObjectPayload{URI: string(parent)})
	if err != nil {
		return err
	}
	return h.d.Outbox.EnqueueTx(ctx, tx, job)
}

// FetchThread starts fetching the thread of the status statusID that a user opened
// (federation.md §6.3, ADR-0034): its missing ancestors up to ThreadDepth levels, and for a
// remote status the replies its server lists, ThreadDepth levels deep, ThreadMaxItems documents
// in all. The fetches run as JobFetchObject jobs. A thread opened in the last ThreadFetchTTL is
// not fetched again, and neither is a local status that replies to nothing (its replies are
// delivered to this server). It does not wait for the fetches.
func (h *StatusHandlers) FetchThread(ctx context.Context, statusID string) error {
	if h.d.Jobs == nil || h.d.KV == nil {
		return nil
	}
	st, err := h.d.Statuses.FindStatus(ctx, statusID)
	if err != nil {
		return err
	}
	if st.IsLocal && st.InReplyToURI == "" {
		return nil
	}
	opened, err := h.d.KV.IncrBy(ctx, ThreadFetchNamespace, uriKey(st.URI), 1, ThreadFetchTTL)
	if err != nil || opened > 1 {
		return err
	}
	job, err := fetchObjectJob(fetchObjectPayload{URI: st.URI, Thread: st.URI, Ancestors: ThreadDepth, Replies: ThreadDepth})
	if err != nil {
		return err
	}
	return h.d.Jobs.Enqueue(ctx, job)
}

func uriKey(uri string) string {
	sum := sha256.Sum256([]byte(uri))
	return hex.EncodeToString(sum[:])
}

// handleFetchObject is the queue.Handler of JobFetchObject: it stores the post at the payload's
// URI unless it is stored, then enqueues the fetch of its first missing ancestor and of its
// replies, as deep as the payload allows. A document that cannot be fetched or is not taken ends
// the job; a fetch that failed is remembered by the Fetcher, so the same URI is not fetched again
// for FetchFailureTTL.
func (h *StatusHandlers) handleFetchObject(ctx context.Context, j queue.Job) queue.Result {
	var p fetchObjectPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return queue.Fail(fmt.Errorf("federation: fetch_object payload %s: %w", j.Payload, err))
	}
	if p.URI == "" {
		return queue.Fail(fmt.Errorf("federation: fetch_object payload %s has no uri", j.Payload))
	}
	jobs, err := h.fetchObject(ctx, p)
	if err == nil && len(jobs) > 0 && h.d.Jobs != nil {
		err = h.d.Jobs.Enqueue(ctx, jobs...)
	}
	switch {
	case err == nil:
		return queue.Done()
	case isFetchError(err), errors.Is(err, ErrNotTaken):
		h.d.Logger.InfoContext(ctx, "remote object not fetched", slog.String("uri", p.URI), slog.Any("reason", err))
		return queue.Done()
	}
	return queue.Retry(0, err)
}

// fetchObject does the work of one JobFetchObject and returns the jobs it leads to.
func (h *StatusHandlers) fetchObject(ctx context.Context, p fetchObjectPayload) ([]queue.NewJob, error) {
	target, err := fetchTarget(p.URI)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFetchNotFound, err)
	}
	var obj *federation.Object
	st, err := h.d.Statuses.FindStatusByURI(ctx, string(target))
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		if ok, err := h.spend(ctx, p.Thread, p.Up); err != nil || !ok {
			return nil, err
		}
		if st, obj, err = h.fetchStatus(ctx, target); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	case p.Replies > 0 && !st.IsLocal:
		// The stored post does not keep its replies collection: its document does.
		if ok, err := h.spend(ctx, p.Thread, false); err != nil || !ok {
			return nil, err
		}
		got, err := h.d.Fetcher.Fetch(ctx, st.URI)
		switch {
		case err == nil && got.Document.Object != nil && got.Document.Object.ID == federation.IRI(st.URI):
			obj = got.Document.Object
		case err != nil && !isFetchError(err):
			return nil, err
		}
	}
	var jobs []queue.NewJob
	if p.Ancestors > 0 {
		job, ok, err := h.ancestorJob(ctx, st, p)
		if err != nil {
			return nil, err
		}
		if ok {
			jobs = append(jobs, job)
		}
	}
	if p.Replies > 0 && obj != nil {
		replies, err := h.replyJobs(ctx, obj, p)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, replies...)
	}
	return jobs, nil
}

// spend counts one more document fetched for thread and reports whether it is within the
// thread's bound: ThreadMaxItems for an ancestor (up), ThreadMaxItems - ThreadDepth for the
// replies, so that the ancestors always have their share. A fetch that is not part of a thread is
// always within bounds.
func (h *StatusHandlers) spend(ctx context.Context, thread string, up bool) (bool, error) {
	if thread == "" {
		return true, nil
	}
	if h.d.KV == nil {
		return false, nil
	}
	n, err := h.d.KV.IncrBy(ctx, ThreadBudgetNamespace, uriKey(thread), 1, ThreadFetchTTL)
	if err != nil {
		return false, err
	}
	limit := int64(ThreadMaxItems - ThreadDepth)
	if up {
		limit = ThreadMaxItems
	}
	if n > limit {
		// Take the count back, so that refused fetches of the replies do not use up the share
		// of the ancestors.
		if _, err := h.d.KV.IncrBy(ctx, ThreadBudgetNamespace, uriKey(thread), -1, ThreadFetchTTL); err != nil {
			return false, err
		}
		h.d.Logger.InfoContext(ctx, "thread fetch reached its bound", slog.String("thread", thread), slog.Int64("documents", limit))
		return false, nil
	}
	return true, nil
}

// ancestorJob walks up the stored ancestors of st, at most p.Ancestors levels, and returns the
// fetch of the first one that is missing, with the levels left above it.
func (h *StatusHandlers) ancestorJob(ctx context.Context, st appcontent.Status, p fetchObjectPayload) (queue.NewJob, bool, error) {
	cur := st
	for level := range p.Ancestors {
		parent := federation.IRI(cur.InReplyToURI)
		if parent == "" {
			return queue.NewJob{}, false, nil
		}
		next, err := h.d.Statuses.FindStatusByURI(ctx, string(parent))
		switch {
		case errors.Is(err, appcontent.ErrNotFound):
			if h.isLocal(parent) {
				return queue.NewJob{}, false, nil
			}
			job, err := fetchObjectJob(fetchObjectPayload{URI: string(parent), Thread: p.Thread, Ancestors: p.Ancestors - level - 1, Up: true})
			return job, err == nil, err
		case err != nil:
			return queue.NewJob{}, false, err
		}
		cur = next
	}
	return queue.NewJob{}, false, nil
}

// replyJobs reads the replies collection of obj, page by page while the thread's bound allows,
// and returns the fetch of each remote reply with one level less below it. A reply that is
// stored is fetched only to go further down.
func (h *StatusHandlers) replyJobs(ctx context.Context, obj *federation.Object, p fetchObjectPayload) ([]queue.NewJob, error) {
	items, err := h.replyItems(ctx, obj, p.Thread)
	if err != nil {
		return nil, err
	}
	var jobs []queue.NewJob
	for _, iri := range items {
		if p.Replies == 1 {
			switch _, err := h.d.Statuses.FindStatusByURI(ctx, string(iri)); {
			case err == nil:
				continue
			case !errors.Is(err, appcontent.ErrNotFound):
				return nil, err
			}
		}
		job, err := fetchObjectJob(fetchObjectPayload{URI: string(iri), Thread: p.Thread, Replies: p.Replies - 1})
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// replyItems returns the remote replies that the replies collection of obj lists, at most
// ThreadMaxItems: the collection and its pages are read where they are embedded and fetched
// otherwise, each fetch counting against the thread. Only a collection of the post's origin
// is read.
func (h *StatusHandlers) replyItems(ctx context.Context, obj *federation.Object, thread string) ([]federation.IRI, error) {
	var (
		out  []federation.IRI
		seen = map[federation.IRI]bool{obj.ID: true}
		next = obj.Replies
	)
	for pages := 0; (next.ID != "" || next.Trusted() != nil) && len(out) < ThreadMaxItems && pages <= ThreadMaxItems; pages++ {
		c, err := h.collection(ctx, next, obj.ID, thread)
		if err != nil || c == nil {
			return out, err
		}
		for _, item := range c.Items {
			if len(out) == ThreadMaxItems {
				break
			}
			if iri := item.ID; !seen[iri] && !h.isLocal(iri) {
				seen[iri] = true
				out = append(out, iri)
			}
		}
		switch {
		case c.Page:
			next = c.Next
		default:
			next = c.First
		}
		if next.ID != "" {
			if seen[next.ID] {
				break
			}
			seen[next.ID] = true
		}
	}
	return out, nil
}

// collection returns the collection or page that r refers to: the embedded one when it is
// trusted, or the one fetched from its id when it has the origin of the post and the thread
// allows another document. It returns nil when there is none to read.
func (h *StatusHandlers) collection(ctx context.Context, r federation.Ref, post federation.IRI, thread string) (*federation.Collection, error) {
	if doc := r.Trusted(); doc != nil {
		if doc.Collection == nil || (doc.Collection.ID != "" && !federation.SameOrigin(doc.Collection.ID, post)) {
			return nil, nil
		}
		return doc.Collection, nil
	}
	if !federation.SameOrigin(r.ID, post) {
		return nil, nil
	}
	if ok, err := h.spend(ctx, thread, false); err != nil || !ok {
		return nil, err
	}
	got, err := h.d.Fetcher.Fetch(ctx, string(r.ID))
	switch {
	case isFetchError(err):
		return nil, nil
	case err != nil:
		return nil, err
	}
	return got.Document.Collection, nil
}

// fetchStatus fetches the remote post at target and stores it (fetched), unless a deletion or
// tombstone keeps target. It fails with ErrNotTaken or the errors of Fetcher.Fetch; other
// errors come from the stores.
func (h *StatusHandlers) fetchStatus(ctx context.Context, target federation.IRI) (appcontent.Status, *federation.Object, error) {
	if err := h.untombstoned(ctx, target, ""); err != nil {
		return appcontent.Status{}, nil, err
	}
	got, err := h.d.Fetcher.Fetch(ctx, string(target))
	if err != nil {
		return appcontent.Status{}, nil, err
	}
	st, err := h.fetched(ctx, got, false)
	return st, got.Document.Object, err
}

// fetched stores a post fetched from its origin (federation.md §6.1, §6.3): a post whose
// attributedTo has the origin of its id, by an active remote author, whose URI no deletion or
// tombstone keeps. A post stored already is returned as it is. With parent, the fetch of its
// parent is enqueued as on receipt. It fails with ErrNotTaken or the errors of resolving the
// author; other errors come from the stores.
func (h *StatusHandlers) fetched(ctx context.Context, got *Fetched, parent bool) (appcontent.Status, error) {
	return h.fetchedWith(ctx, got, storeOptions{parent: parent})
}

// storeOptions say how a post is stored: with the fetch of its parent (parent), or as one that a
// relay brought in (relay), which is stored only when it is public, without fetching its parent
// or the unknown actors it mentions, and is recorded in relayed_statuses (ADR-0061).
type storeOptions struct {
	parent bool
	relay  *RelayMembership
}

// fetchedWith is fetched with opts.
func (h *StatusHandlers) fetchedWith(ctx context.Context, got *Fetched, opts storeOptions) (appcontent.Status, error) {
	obj := got.Document.Object
	switch {
	case obj == nil || !isPost(obj.Type):
		return appcontent.Status{}, fmt.Errorf("%w: %s is not a post", ErrNotTaken, got.Document.ID())
	case obj.AttributedTo == "" || !federation.SameOrigin(obj.AttributedTo, obj.ID):
		return appcontent.Status{}, fmt.Errorf("%w: %s is attributed to %q of another origin", ErrNotTaken, obj.ID, obj.AttributedTo)
	}
	switch st, err := h.d.Statuses.FindStatusByURI(ctx, string(obj.ID)); {
	case err == nil:
		return st, nil
	case !errors.Is(err, appcontent.ErrNotFound):
		return appcontent.Status{}, err
	}
	if err := h.untombstoned(ctx, obj.ID, obj.AttributedTo); err != nil {
		return appcontent.Status{}, err
	}
	author, err := h.d.Actors.Resolve(ctx, string(obj.AttributedTo))
	switch {
	case isFetchError(err):
		return appcontent.Status{}, fmt.Errorf("%w: the author of %s: %w", ErrNotTaken, obj.ID, err)
	case err != nil:
		return appcontent.Status{}, err
	case author.Account.IsLocal() || !author.Lifecycle.Active():
		return appcontent.Status{}, fmt.Errorf("%w: the author %s of %s is not an active remote account", ErrNotTaken, obj.AttributedTo, obj.ID)
	}
	if opts.relay != nil && h.d.RelayPolicy != nil {
		restricted, err := h.d.RelayPolicy.DiscoveryRestricted(ctx, author.Account.Domain, author.Account.ID)
		if err != nil {
			return appcontent.Status{}, err
		}
		if restricted {
			return appcontent.Status{}, fmt.Errorf("%w: the author %s of the relayed %s is restricted", ErrNotTaken, obj.AttributedTo, obj.ID)
		}
	}
	return h.store(ctx, author, obj, opts)
}

// untombstoned fails with ErrNotTaken when a deletion keeps uri, or a tombstone that owner, the
// actor the post is attributed to, left; "" checks the deletions only, before the post's owner
// is known.
func (h *StatusHandlers) untombstoned(ctx context.Context, uri, owner federation.IRI) error {
	deleted, err := h.d.Statuses.Tombstoned(ctx, string(uri))
	if err != nil {
		return err
	}
	if !deleted && owner != "" && h.d.Tombstones != nil {
		if deleted, err = h.d.Tombstones.RemoteObjectTombstoned(ctx, uri, owner); err != nil {
			return err
		}
	}
	if deleted {
		return fmt.Errorf("%w: %s is deleted", ErrNotTaken, uri)
	}
	return nil
}

// store ingests a fetched post of author, with its mentions, and returns the stored status.
// With opts.parent, the fetch of its parent is enqueued in the same transaction; with opts.relay,
// it is stored only when it is public, and recorded as relayed.
func (h *StatusHandlers) store(ctx context.Context, author appaccount.Actor, obj *federation.Object, opts storeOptions) (appcontent.Status, error) {
	followers, err := h.followers(ctx, author)
	if err != nil {
		return appcontent.Status{}, err
	}
	mentions, err := h.mentions(ctx, obj, followers)
	if err != nil {
		return appcontent.Status{}, err
	}
	if opts.relay == nil {
		if err := h.resolveMentions(ctx, mentions); err != nil {
			return appcontent.Status{}, err
		}
	}
	in := h.inputOf(author, federation.Audience{}, "", obj, followers, mentions)
	if opts.relay != nil && in.Visibility != appcontent.VisibilityPublic {
		return appcontent.Status{}, fmt.Errorf("%w: the relayed %s is not public", ErrNotTaken, obj.ID)
	}
	var st appcontent.Status
	err = h.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		p, err := h.d.Statuses.IngestRemoteStatus(ctx, tx, in)
		if err != nil {
			return err
		}
		st = p.Status
		if err := h.verifyQuoteTx(ctx, tx, obj.ID, obj.Quote, false); err != nil {
			return err
		}
		switch {
		case opts.relay != nil:
			return h.recordRelayed(ctx, tx, st, opts.relay)
		case opts.parent:
			return h.fetchParentTx(ctx, tx, st)
		}
		return nil
	})
	if errors.Is(err, appcontent.ErrStatusExists) {
		return h.d.Statuses.FindStatusByURI(ctx, string(obj.ID))
	}
	return st, err
}
