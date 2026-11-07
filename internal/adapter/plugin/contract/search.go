// SPDX-License-Identifier: AGPL-3.0-only

package contract

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"time"

	searchplugin "github.com/mycroblog/my/internal/adapter/search/plugin"
	"github.com/mycroblog/my/internal/adapter/system"
	"github.com/mycroblog/my/internal/app/search"
)

// searchCases check a search plugin through the server's adapter, which also checks each page against the port. The
// plugin may hold a real index, so the default cases search, and write and remove only documents of their own: IDs that
// no post has, of an author that no account is, holding a word made for the run. The cases that remove every document of
// a visibility or the whole index run only with --clear-index (ADR-0146 §16).
var searchCases = []Case{
	{Name: "search_no_word", Run: searchNoWord},
	{Name: "search_plain_text", Run: searchPlainText},
	{Name: "search_japanese", Run: searchJapanese},
	{Name: "search_invalid_cursor", Run: searchInvalidCursor},
	{Name: "replace_and_search", Run: searchReplaceAndSearch},
	{Name: "replace_replaces", Run: searchReplaceReplaces},
	{Name: "remove", Run: searchRemove},
	{Name: "remove_by_author", Run: searchRemoveByAuthor},
	{Name: "remove_by_visibility", Skip: withoutClearIndex, Run: searchRemoveByVisibility},
	{Name: "clear", Skip: withoutClearIndex, Run: searchClear},
}

func withoutClearIndex(env Env) string {
	if !env.ClearIndex {
		return "it removes every document of the index; --clear-index runs it, against an index for testing"
	}
	return ""
}

// searchFixture is the documents of one case: their own author and two words that no post holds.
type searchFixture struct {
	index       *searchplugin.Index
	author      string
	word, other string
	ids         []string
}

// randomWord returns a word that no post holds, made for the run.
func randomWord() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "myplugintest" + hex.EncodeToString(b[:]), nil
}

// newSearchFixture returns the documents of a case, n of them, oldest first.
func newSearchFixture(env Env, n int) (*searchFixture, error) {
	word, err := randomWord()
	if err != nil {
		return nil, err
	}
	other, err := randomWord()
	if err != nil {
		return nil, err
	}
	var ids system.IDGenerator
	f := &searchFixture{index: searchplugin.New(env.Conn), author: ids.NewID(), word: word, other: other}
	for range n {
		f.ids = append(f.ids, ids.NewID())
	}
	return f, nil
}

func (f *searchFixture) doc(i int, visibility search.Visibility, text string) search.Document {
	return search.Document{
		StatusID: f.ids[i], AuthorID: f.author, Visibility: visibility, Language: "en",
		PublishedAt: time.Now().UTC().Truncate(time.Second), Text: text,
	}
}

// replace adds the documents of the fixture, each holding the fixture's word.
func (f *searchFixture) replace(ctx context.Context, env Env, visibilities ...search.Visibility) error {
	for i := range f.ids {
		v := search.VisibilityPublic
		if i < len(visibilities) {
			v = visibilities[i]
		}
		if err := f.index.Replace(ctx, f.doc(i, v, "my plugin test "+f.word)); err != nil {
			return fmt.Errorf("Replace: %w", describeCallError(env, err))
		}
	}
	return nil
}

// cleanUp removes what the case left, whatever it found.
func (f *searchFixture) cleanUp(ctx context.Context) {
	_ = f.index.RemoveByAuthor(ctx, f.author)
	for _, id := range f.ids {
		_ = f.index.Remove(ctx, id)
	}
}

// find returns every page of a search of text, a page of limit IDs at most.
func (f *searchFixture) find(ctx context.Context, env Env, text string, limit int) ([]string, error) {
	var found []string
	cursor := ""
	for range 100 {
		page, err := f.index.Search(ctx, search.Query{Text: text, Cursor: cursor, Limit: limit})
		if err != nil {
			return nil, fmt.Errorf("Search: %w", describeCallError(env, err))
		}
		found = append(found, page.StatusIDs...)
		if page.Next == "" {
			return found, nil
		}
		cursor = page.Next
	}
	return nil, errors.New("Search: the pages do not end")
}

// want checks that a search of the fixture's word finds exactly these documents of it, newest first.
func (f *searchFixture) want(ctx context.Context, env Env, limit int, indexes ...int) error {
	got, err := f.find(ctx, env, f.word, limit)
	if err != nil {
		return err
	}
	want := []string{}
	for _, i := range slices.Backward(indexes) {
		want = append(want, f.ids[i])
	}
	if !slices.Equal(got, want) && (len(got) != 0 || len(want) != 0) {
		return fmt.Errorf("a search of the word of the case, %d IDs a page, found %v, want %v", limit, got, want)
	}
	return nil
}

// searchOnly runs one search and checks that it fails neither the call nor the page.
func searchOnly(ctx context.Context, env Env, text string) (search.Page, error) {
	page, err := searchplugin.New(env.Conn).Search(ctx, search.Query{Text: text, Limit: 10})
	if err != nil {
		return search.Page{}, fmt.Errorf("Search: %w", describeCallError(env, err))
	}
	return page, nil
}

// searchNoWord sends text with no word, which finds nothing.
func searchNoWord(ctx context.Context, env Env) error {
	for _, text := range []string{"", "  \t "} {
		page, err := searchOnly(ctx, env, text)
		if err != nil {
			return err
		}
		if len(page.StatusIDs) != 0 || page.Next != "" {
			return fmt.Errorf("a query with no word found %d IDs and a next cursor %t, want nothing", len(page.StatusIDs), page.Next != "")
		}
	}
	return nil
}

// searchPlainText sends what the query language of an engine would read, which must be just text. It checks that the
// search does not fail: what it finds depends on the posts of a real index.
func searchPlainText(ctx context.Context, env Env) error {
	for _, text := range []string{`"my plugin test`, `*:* OR status_id:*`, `-a +b ~c ^d (e) [f] {g} \ / && || !`, `author_id = '' ; DROP`} {
		if _, err := searchOnly(ctx, env, text); err != nil {
			return fmt.Errorf("the query %q: %w", text, err)
		}
	}
	return nil
}

// searchJapanese sends a query in Japanese, which must not fail.
func searchJapanese(ctx context.Context, env Env) error {
	_, err := searchOnly(ctx, env, "検索のテスト")
	return err
}

// searchInvalidCursor sends a cursor that no plugin issued, which the plugin must refuse as OUTCOME_INVALID_CURSOR.
func searchInvalidCursor(ctx context.Context, env Env) error {
	_, err := searchplugin.New(env.Conn).Search(ctx, search.Query{Text: "my plugin test", Cursor: "my-plugin-test:not-a-cursor", Limit: 10})
	switch {
	case errors.Is(err, search.ErrInvalidCursor):
		return nil
	case err != nil:
		return fmt.Errorf("Search: %w", describeCallError(env, err))
	}
	return errors.New("a cursor that the plugin did not issue: the outcome is OUTCOME_OK, not OUTCOME_INVALID_CURSOR")
}

// searchReplaceAndSearch adds three documents of either visibility and finds them, newest first, on one page and on pages
// of one ID.
func searchReplaceAndSearch(ctx context.Context, env Env) error {
	f, err := newSearchFixture(env, 3)
	if err != nil {
		return err
	}
	defer f.cleanUp(context.WithoutCancel(ctx))
	if err := f.replace(ctx, env, search.VisibilityPublic, search.VisibilityLocal, search.VisibilityPublic); err != nil {
		return err
	}
	if err := f.want(ctx, env, 10, 0, 1, 2); err != nil {
		return err
	}
	return f.want(ctx, env, 1, 0, 1, 2)
}

// searchReplaceReplaces replaces a document with another text, twice, and finds one document by the new text only.
func searchReplaceReplaces(ctx context.Context, env Env) error {
	f, err := newSearchFixture(env, 1)
	if err != nil {
		return err
	}
	defer f.cleanUp(context.WithoutCancel(ctx))
	if err := f.replace(ctx, env); err != nil {
		return err
	}
	for range 2 {
		if err := f.index.Replace(ctx, f.doc(0, search.VisibilityPublic, "my plugin test "+f.other)); err != nil {
			return fmt.Errorf("Replace: %w", describeCallError(env, err))
		}
	}
	if err := f.want(ctx, env, 10); err != nil {
		return fmt.Errorf("after the text changed: %w", err)
	}
	got, err := f.find(ctx, env, f.other, 10)
	if err != nil {
		return err
	}
	if !slices.Equal(got, f.ids) {
		return fmt.Errorf("a search of the new text found %v, want %v", got, f.ids)
	}
	return nil
}

// searchRemove removes one of two documents, twice, and a document that was never there.
func searchRemove(ctx context.Context, env Env) error {
	f, err := newSearchFixture(env, 3)
	if err != nil {
		return err
	}
	defer f.cleanUp(context.WithoutCancel(ctx))
	if err := f.replace(ctx, env); err != nil {
		return err
	}
	for _, id := range []string{f.ids[1], f.ids[1], new(system.IDGenerator).NewID()} {
		if err := f.index.Remove(ctx, id); err != nil {
			return fmt.Errorf("Remove: %w", describeCallError(env, err))
		}
	}
	return f.want(ctx, env, 10, 0, 2)
}

// searchRemoveByAuthor removes the documents of the fixture's author, twice.
func searchRemoveByAuthor(ctx context.Context, env Env) error {
	f, err := newSearchFixture(env, 2)
	if err != nil {
		return err
	}
	defer f.cleanUp(context.WithoutCancel(ctx))
	if err := f.replace(ctx, env); err != nil {
		return err
	}
	for range 2 {
		if err := f.index.RemoveByAuthor(ctx, f.author); err != nil {
			return fmt.Errorf("RemoveByAuthor: %w", describeCallError(env, err))
		}
	}
	return f.want(ctx, env, 10)
}

// searchRemoveByVisibility removes every local document, twice, and keeps the public ones.
func searchRemoveByVisibility(ctx context.Context, env Env) error {
	f, err := newSearchFixture(env, 2)
	if err != nil {
		return err
	}
	defer f.cleanUp(context.WithoutCancel(ctx))
	if err := f.replace(ctx, env, search.VisibilityLocal, search.VisibilityPublic); err != nil {
		return err
	}
	for range 2 {
		if err := f.index.RemoveByVisibility(ctx, search.VisibilityLocal); err != nil {
			return fmt.Errorf("RemoveByVisibility: %w", describeCallError(env, err))
		}
	}
	return f.want(ctx, env, 10, 1)
}

// searchClear clears the index, twice, and checks that it takes documents again.
func searchClear(ctx context.Context, env Env) error {
	f, err := newSearchFixture(env, 1)
	if err != nil {
		return err
	}
	defer f.cleanUp(context.WithoutCancel(ctx))
	if err := f.replace(ctx, env); err != nil {
		return err
	}
	for range 2 {
		if err := f.index.Clear(ctx); err != nil {
			return fmt.Errorf("Clear: %w", describeCallError(env, err))
		}
	}
	if err := f.want(ctx, env, 10); err != nil {
		return fmt.Errorf("after Clear: %w", err)
	}
	if err := f.replace(ctx, env); err != nil {
		return fmt.Errorf("after Clear: %w", err)
	}
	return f.want(ctx, env, 10, 0)
}
