// SPDX-License-Identifier: AGPL-3.0-only

// Package plugin is the search.Index that delegates to an operator's plugin (ADR-0146 §11, ADR-0149 §3): one unary RPC
// per operation of the port. The plugin returns IDs only, and the search service reads each one again from the database
// and applies the view policy, so the plugin decides nothing that a person sees.
//
// A document holds the text of a post and a query what a person typed, so nothing here logs, and an error holds neither
// nor anything the plugin said: the client drops the text of a failed call, and no string of a response reaches an error
// (ADR-0146 §10, ADR-0149 §9). Every failure is search.ErrBackendFailed with the RPC, except a cursor that the plugin
// says it did not issue, which is search.ErrInvalidCursor.
package plugin

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	searchv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/search/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/search/v1/searchv1connect"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/search"
)

// MaxCursor is the longest cursor in bytes that the plugin may return: the longest that the client API takes back.
const MaxCursor = 256

// Index is the search.Index of a plugin. It is safe for concurrent use.
type Index struct {
	client searchv1connect.SearchServiceClient
}

var _ search.Index = (*Index)(nil)

// New returns the Index that reaches its plugin through conn. It makes no call.
func New(conn *client.Conn) *Index {
	return &Index{client: searchv1connect.NewSearchServiceClient(conn.RPC)}
}

var visibilities = map[search.Visibility]searchv1.Visibility{
	search.VisibilityPublic: searchv1.Visibility_VISIBILITY_PUBLIC,
	search.VisibilityLocal:  searchv1.Visibility_VISIBILITY_LOCAL,
}

// failed returns the error of a call of rpc that failed, which says how and nothing that the plugin said.
func failed(rpc string, format string, args ...any) error {
	return fmt.Errorf("%w: search plugin: %s: "+format, append([]any{search.ErrBackendFailed, rpc}, args...)...)
}

// outcome returns the error of a write's result: anything but OUTCOME_OK fails, which the job retries (ADR-0146 §10).
func outcome(rpc string, o searchv1.Outcome, err error) error {
	switch {
	case err != nil:
		return failed(rpc, "%w", err)
	case o == searchv1.Outcome_OUTCOME_OK:
		return nil
	case o == searchv1.Outcome_OUTCOME_FAILED:
		return failed(rpc, "the plugin reports a failure")
	default:
		return failed(rpc, "the outcome is %d, which is not a result of the call", int32(o))
	}
}

// Replace implements search.Index. A visibility that is never indexed fails before the call.
func (x *Index) Replace(ctx context.Context, doc search.Document) error {
	v, ok := visibilities[doc.Visibility]
	if !ok {
		return failed("Replace", "the visibility %q is never indexed", doc.Visibility)
	}
	res, err := x.client.Replace(ctx, &searchv1.ReplaceRequest{Document: &searchv1.Document{
		StatusId:    doc.StatusID,
		AuthorId:    doc.AuthorID,
		Visibility:  v,
		Language:    doc.Language,
		PublishedAt: doc.PublishedAt.UTC().Format(time.RFC3339Nano),
		Text:        doc.Text,
	}})
	return outcome("Replace", res.GetOutcome(), err)
}

// Remove implements search.Index.
func (x *Index) Remove(ctx context.Context, statusID string) error {
	res, err := x.client.Remove(ctx, &searchv1.RemoveRequest{StatusId: statusID})
	return outcome("Remove", res.GetOutcome(), err)
}

// RemoveByAuthor implements search.Index.
func (x *Index) RemoveByAuthor(ctx context.Context, authorID string) error {
	res, err := x.client.RemoveByAuthor(ctx, &searchv1.RemoveByAuthorRequest{AuthorId: authorID})
	return outcome("RemoveByAuthor", res.GetOutcome(), err)
}

// RemoveByVisibility implements search.Index. A visibility that is never indexed has no document to remove, so it
// succeeds with no call.
func (x *Index) RemoveByVisibility(ctx context.Context, visibility search.Visibility) error {
	v, ok := visibilities[visibility]
	if !ok {
		return nil
	}
	res, err := x.client.RemoveByVisibility(ctx, &searchv1.RemoveByVisibilityRequest{Visibility: v})
	return outcome("RemoveByVisibility", res.GetOutcome(), err)
}

// Clear implements search.Index.
func (x *Index) Clear(ctx context.Context) error {
	res, err := x.client.Clear(ctx, &searchv1.ClearRequest{})
	return outcome("Clear", res.GetOutcome(), err)
}

// Search implements search.Index. It checks the page against the port before anything of it goes on: IDs that are no
// status IDs, out of order, more than the limit, or a cursor that the client API could not take back fail the search,
// and nothing of the page is returned.
func (x *Index) Search(ctx context.Context, q search.Query) (search.Page, error) {
	limit := min(max(q.Limit, 1), 1<<31-1)
	res, err := x.client.Search(ctx, &searchv1.SearchRequest{Text: q.Text, Cursor: q.Cursor, Limit: int32(limit)}) //nolint:gosec // bounded above
	if err != nil {
		return search.Page{}, failed("Search", "%w", err)
	}
	switch res.GetOutcome() {
	case searchv1.Outcome_OUTCOME_OK:
	case searchv1.Outcome_OUTCOME_INVALID_CURSOR:
		if q.Cursor == "" {
			return search.Page{}, failed("Search", "the plugin refuses the cursor of the first page, which has none")
		}
		return search.Page{}, fmt.Errorf("search plugin: Search: %w", search.ErrInvalidCursor)
	case searchv1.Outcome_OUTCOME_FAILED:
		return search.Page{}, failed("Search", "the plugin reports a failure")
	default:
		return search.Page{}, failed("Search", "the outcome is %d, which is not a result of the call", int32(res.GetOutcome()))
	}
	ids := res.GetStatusIds()
	if len(ids) > limit {
		return search.Page{}, failed("Search", "%d IDs, over the limit %d", len(ids), limit)
	}
	for i, id := range ids {
		if !kernel.ValidID(id) {
			return search.Page{}, failed("Search", "ID %d is not a status ID", i+1)
		}
		if i > 0 && id >= ids[i-1] {
			return search.Page{}, failed("Search", "ID %d is not older than the one before it", i+1)
		}
	}
	next := res.GetNextCursor()
	if !validCursor(next) {
		return search.Page{}, failed("Search", "the cursor of the next page is over %d bytes or not printable ASCII", MaxCursor)
	}
	return search.Page{StatusIDs: ids, Next: next}, nil
}

// validCursor reports whether a cursor fits what the client API takes back: at most MaxCursor bytes of printable ASCII
// with no space, so that it passes in a query string and a header as it is.
func validCursor(c string) bool {
	if len(c) > MaxCursor {
		return false
	}
	for i := range len(c) {
		if c[i] < '!' || c[i] > '~' {
			return false
		}
	}
	return true
}
