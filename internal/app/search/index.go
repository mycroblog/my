// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"context"
	"errors"
	"time"
)

// Visibility is a visibility a document of the index can have. unlisted, private, and direct
// statuses are never indexed, whatever the settings say, so they have no value here (ADR-0149 §5).
type Visibility string

const (
	// VisibilityPublic is the visibility public.
	VisibilityPublic Visibility = "public"
	// VisibilityLocal is the visibility local.
	VisibilityLocal Visibility = "local"
)

// ErrInvalidCursor reports a cursor that the backend did not issue. A client sends cursors back
// as it received them, so this is the client's error, unlike any other error of Index.Search.
var ErrInvalidCursor = errors.New("search: invalid cursor")

// Document is what the index holds of one status (ADR-0149 §4). The index is a copy of the
// database and nothing reads a document back.
type Document struct {
	// StatusID is the ID of the status, a UUIDv7, so its order is the order of time.
	StatusID string
	// AuthorID is the account ID of the author.
	AuthorID string
	// Visibility is the effective visibility of the status.
	Visibility Visibility
	// Language is status_revisions.language, empty when the status has none. A backend may
	// analyze the text by language (ADR-0149 §13).
	Language string
	// PublishedAt is when the status was published.
	PublishedAt time.Time
	// Text is what the search looks in: the body, the content warning, and the alternative texts
	// of the current revision.
	Text string
}

// Query is one search (ADR-0149 §4).
type Query struct {
	// Text is plain text. A backend splits it into words and finds the documents with every word.
	// It never reads the text as the query language of its engine: an operator, a wildcard, or a
	// field name in the text is just text. Text with no word finds nothing.
	Text string
	// Cursor is the Page.Next of the previous page, empty for the first page.
	Cursor string
	// Limit is the most status IDs of the page, at least 1.
	Limit int
}

// Page is one page of a search. It holds no text, HTML, highlight, or score: the results are
// loaded from the database by ID and pass the visibility policy there (ADR-0149 §8).
type Page struct {
	// StatusIDs are the IDs of the page, newest first by ID.
	StatusIDs []string
	// Next is the cursor of the next page, opaque and specific to the backend, and empty when
	// nothing follows. A page that is not the last may hold fewer than Query.Limit IDs, and the
	// page after a non-empty Next may be empty.
	Next string
}

// Index is the port SearchIndex of a search engine (ADR-0149 §4). Calls run outside transactions
// (ADR-0003). testkit.SearchIndexContract is its contract.
//
// Every write can be repeated with the same result, since the jobs behind them are delivered at
// least once: removing what is not there is not an error.
type Index interface {
	// Replace adds doc, or replaces the document of the same StatusID.
	Replace(ctx context.Context, doc Document) error
	// Remove removes the document of a status.
	Remove(ctx context.Context, statusID string) error
	// RemoveByAuthor removes every document of an author.
	RemoveByAuthor(ctx context.Context, authorID string) error
	// RemoveByVisibility removes every document of a visibility.
	RemoveByVisibility(ctx context.Context, visibility Visibility) error
	// Clear removes every document.
	Clear(ctx context.Context) error
	// Search returns one page of the status IDs that q finds, newest first by ID. It returns
	// ErrInvalidCursor for a cursor the backend did not issue.
	Search(ctx context.Context, q Query) (Page, error)
}
