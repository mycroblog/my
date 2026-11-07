// SPDX-License-Identifier: AGPL-3.0-only

// Package fake is a search.Index that keeps its documents in memory, for tests (ADR-0149 §3). It
// is separate from search = none, which has no backend. Go tests inject it with
// platform.AppOptions.SearchIndex; its tests run searchtest.SearchIndexContract on it.
//
// A word matches anywhere in a document, ignoring case, whatever the language.
//
// It is never part of a released binary: only a test build with the build tag e2e_fakes links it
// into the server, as search = fake, which such a build refuses outside profile = development.
package fake

import (
	"cmp"
	"context"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/mycroblog/my/internal/app/search"
)

// cursorPrefix marks a cursor of this fake, so that it can tell one it did not issue.
const cursorPrefix = "before:"

// Index is the fake. Use New.
type Index struct {
	mu   sync.Mutex
	docs map[string]search.Document
}

// New returns an empty index.
func New() *Index {
	return &Index{docs: map[string]search.Document{}}
}

// Replace implements search.Index. It refuses a visibility that is never indexed, so that a test
// sees the mistake of whoever sent it.
func (x *Index) Replace(_ context.Context, doc search.Document) error {
	if doc.Visibility != search.VisibilityPublic && doc.Visibility != search.VisibilityLocal {
		return fmt.Errorf("search/fake: the visibility %q is never indexed", doc.Visibility)
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	x.docs[doc.StatusID] = doc
	return nil
}

// Remove implements search.Index.
func (x *Index) Remove(_ context.Context, statusID string) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	delete(x.docs, statusID)
	return nil
}

// RemoveByAuthor implements search.Index.
func (x *Index) RemoveByAuthor(_ context.Context, authorID string) error {
	x.removeWhere(func(d search.Document) bool { return d.AuthorID == authorID })
	return nil
}

// RemoveByVisibility implements search.Index.
func (x *Index) RemoveByVisibility(_ context.Context, visibility search.Visibility) error {
	x.removeWhere(func(d search.Document) bool { return d.Visibility == visibility })
	return nil
}

// Clear implements search.Index.
func (x *Index) Clear(context.Context) error {
	x.removeWhere(func(search.Document) bool { return true })
	return nil
}

func (x *Index) removeWhere(match func(search.Document) bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	for id, d := range x.docs {
		if match(d) {
			delete(x.docs, id)
		}
	}
}

// Search implements search.Index.
func (x *Index) Search(_ context.Context, q search.Query) (search.Page, error) {
	before, err := decodeCursor(q.Cursor)
	if err != nil {
		return search.Page{}, err
	}
	words := strings.Fields(strings.ToLower(q.Text))
	if len(words) == 0 || q.Limit < 1 {
		return search.Page{}, nil
	}

	found := x.matching(words, before)
	slices.SortFunc(found, func(a, b string) int { return cmp.Compare(b, a) })
	if len(found) <= q.Limit {
		return search.Page{StatusIDs: found}, nil
	}
	page := found[:q.Limit]
	return search.Page{StatusIDs: page, Next: encodeCursor(page[len(page)-1])}, nil
}

// matching returns the IDs before the given one, any when it is empty, of the documents with every
// word.
func (x *Index) matching(words []string, before string) []string {
	x.mu.Lock()
	defer x.mu.Unlock()
	var found []string
	for id, d := range x.docs {
		if (before == "" || id < before) && hasEvery(d.Text, words) {
			found = append(found, id)
		}
	}
	return found
}

func hasEvery(text string, words []string) bool {
	text = strings.ToLower(text)
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}

func encodeCursor(id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursorPrefix + id))
}

func decodeCursor(cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", search.ErrInvalidCursor
	}
	id, ok := strings.CutPrefix(string(b), cursorPrefix)
	if !ok || id == "" {
		return "", search.ErrInvalidCursor
	}
	return id, nil
}
