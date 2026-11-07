// SPDX-License-Identifier: AGPL-3.0-only

package plugin_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	searchv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/search/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/search/v1/searchv1connect"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/adapter/search/fake"
	searchplugin "github.com/mycroblog/my/internal/adapter/search/plugin"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/pkg/testkit/searchtest"
)

// secret stands for the text of a post and a query, which no error may hold.
const secret = "the words of a local post"

const (
	idNewer = "0190b000-0000-7000-8000-000000000002"
	idOlder = "0190b000-0000-7000-8000-000000000001"
	author  = "0190a000-0000-7000-8000-00000000000a"
)

func newIndex(t *testing.T, d kind.Destination) *searchplugin.Index {
	t.Helper()
	k, ok := kind.ByName(kind.Search)
	if !ok {
		t.Fatal("no search kind")
	}
	conn, err := client.New(client.Options{Kind: k, Destination: d})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	return searchplugin.New(conn)
}

// The adapter passes the contract of the port against a plugin that holds the fake index, over each protocol.
func TestContract(t *testing.T) {
	t.Parallel()
	for _, protocol := range kind.Protocols() {
		t.Run(protocol, func(t *testing.T) {
			t.Parallel()
			searchtest.SearchIndexContract(t, func(t *testing.T) search.Index {
				t.Helper()
				h, _ := fakeplugin.Connect(fakeplugin.Options{Search: fake.New()})
				return newIndex(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h), Protocol: protocol})
			})
		})
	}
}

// answers is a plugin that answers every call as its fields say, and counts the calls.
type answers struct {
	searchv1connect.UnimplementedSearchServiceHandler

	write searchv1.Outcome
	page  *searchv1.SearchResponse
	err   error
	calls atomic.Int64
	got   atomic.Value
}

func (a *answers) answer() (searchv1.Outcome, error) {
	a.calls.Add(1)
	return a.write, a.err
}

func (a *answers) Replace(_ context.Context, req *searchv1.ReplaceRequest) (*searchv1.ReplaceResponse, error) {
	a.got.Store(req.GetDocument())
	o, err := a.answer()
	if err != nil {
		return nil, err
	}
	return &searchv1.ReplaceResponse{Outcome: o}, nil
}

func (a *answers) Remove(context.Context, *searchv1.RemoveRequest) (*searchv1.RemoveResponse, error) {
	o, err := a.answer()
	if err != nil {
		return nil, err
	}
	return &searchv1.RemoveResponse{Outcome: o}, nil
}

func (a *answers) RemoveByAuthor(context.Context, *searchv1.RemoveByAuthorRequest) (*searchv1.RemoveByAuthorResponse, error) {
	o, err := a.answer()
	if err != nil {
		return nil, err
	}
	return &searchv1.RemoveByAuthorResponse{Outcome: o}, nil
}

func (a *answers) RemoveByVisibility(context.Context, *searchv1.RemoveByVisibilityRequest) (*searchv1.RemoveByVisibilityResponse, error) {
	o, err := a.answer()
	if err != nil {
		return nil, err
	}
	return &searchv1.RemoveByVisibilityResponse{Outcome: o}, nil
}

func (a *answers) Clear(context.Context, *searchv1.ClearRequest) (*searchv1.ClearResponse, error) {
	o, err := a.answer()
	if err != nil {
		return nil, err
	}
	return &searchv1.ClearResponse{Outcome: o}, nil
}

func (a *answers) Search(context.Context, *searchv1.SearchRequest) (*searchv1.SearchResponse, error) {
	if _, err := a.answer(); err != nil {
		return nil, err
	}
	return a.page, nil
}

// serving returns an Index on a plugin that answers as a does.
func serving(t *testing.T, a *answers) *searchplugin.Index {
	t.Helper()
	server := connect.NewServer()
	searchv1connect.RegisterSearchServiceHandler(server, a)
	mux := http.NewServeMux()
	connecthttp.Mount(mux, server)
	return newIndex(t, kind.Destination{Address: fakeplugin.ServeUnix(t, mux)})
}

func doc() search.Document {
	return search.Document{
		StatusID: idNewer, AuthorID: author, Visibility: search.VisibilityLocal, Language: "ja",
		PublishedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("JST", 9*60*60)), Text: secret,
	}
}

// writes calls every write of the port and returns their errors by RPC.
func writes(t *testing.T, x *searchplugin.Index) map[string]error {
	t.Helper()
	ctx := t.Context()
	return map[string]error{
		"Replace":            x.Replace(ctx, doc()),
		"Remove":             x.Remove(ctx, idNewer),
		"RemoveByAuthor":     x.RemoveByAuthor(ctx, author),
		"RemoveByVisibility": x.RemoveByVisibility(ctx, search.VisibilityPublic),
		"Clear":              x.Clear(ctx),
	}
}

// The document reaches the plugin whole, its time in UTC.
func TestReplaceSendsTheDocument(t *testing.T) {
	t.Parallel()
	a := &answers{write: searchv1.Outcome_OUTCOME_OK}
	if err := serving(t, a).Replace(t.Context(), doc()); err != nil {
		t.Fatal(err)
	}
	got, _ := a.got.Load().(*searchv1.Document)
	if got.GetStatusId() != idNewer || got.GetAuthorId() != author || got.GetVisibility() != searchv1.Visibility_VISIBILITY_LOCAL ||
		got.GetLanguage() != "ja" || got.GetPublishedAt() != "2026-01-01T18:04:05Z" || got.GetText() != secret {
		t.Errorf("document = %v", got)
	}
}

// Anything but OUTCOME_OK fails a write as ErrBackendFailed, which names the RPC and holds no text.
func TestWriteFailures(t *testing.T) {
	t.Parallel()
	for name, a := range map[string]*answers{
		"failed":        {write: searchv1.Outcome_OUTCOME_FAILED},
		"empty":         {},
		"unknown":       {write: searchv1.Outcome(99)},
		"cursor":        {write: searchv1.Outcome_OUTCOME_INVALID_CURSOR},
		"connect_error": {err: connect.NewError(connect.CodeInternal, secret)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for rpc, err := range writes(t, serving(t, a)) {
				if !errors.Is(err, search.ErrBackendFailed) {
					t.Errorf("%s = %v, want ErrBackendFailed", rpc, err)
					continue
				}
				if !strings.Contains(err.Error(), rpc) || strings.Contains(err.Error(), secret) {
					t.Errorf("%s: the error %q must name the RPC and hold no text", rpc, err)
				}
			}
		})
	}
}

// A visibility that is never indexed reaches no plugin: Replace fails and RemoveByVisibility has nothing to remove.
func TestVisibilitiesNeverIndexed(t *testing.T) {
	t.Parallel()
	a := &answers{write: searchv1.Outcome_OUTCOME_OK}
	x := serving(t, a)
	d := doc()
	d.Visibility = "private"
	if err := x.Replace(t.Context(), d); !errors.Is(err, search.ErrBackendFailed) || strings.Contains(err.Error(), secret) {
		t.Errorf("Replace(private) = %v", err)
	}
	if err := x.RemoveByVisibility(t.Context(), "direct"); err != nil {
		t.Errorf("RemoveByVisibility(direct) = %v", err)
	}
	if n := a.calls.Load(); n != 0 {
		t.Errorf("%d calls", n)
	}
}

func ok(ids []string, next string) *searchv1.SearchResponse {
	return &searchv1.SearchResponse{Outcome: searchv1.Outcome_OUTCOME_OK, StatusIds: ids, NextCursor: next}
}

// A page passes only when it keeps to the port: anything else fails as ErrBackendFailed, and nothing of it is returned.
func TestSearchChecksThePage(t *testing.T) {
	t.Parallel()
	q := search.Query{Text: secret, Limit: 2}
	if page, err := serving(t, &answers{page: ok([]string{idNewer, idOlder}, "before:"+idOlder)}).Search(t.Context(), q); err != nil ||
		len(page.StatusIDs) != 2 || page.Next != "before:"+idOlder {
		t.Errorf("a good page: %v, %v", page, err)
	}
	if page, err := serving(t, &answers{page: ok(nil, "")}).Search(t.Context(), q); err != nil || len(page.StatusIDs) != 0 || page.Next != "" {
		t.Errorf("an empty page: %v, %v", page, err)
	}
	for name, res := range map[string]*searchv1.SearchResponse{
		"empty response":      {},
		"failed":              {Outcome: searchv1.Outcome_OUTCOME_FAILED},
		"unknown outcome":     {Outcome: searchv1.Outcome(99), StatusIds: []string{idNewer}},
		"over the limit":      ok([]string{"0190b000-0000-7000-8000-000000000003", idNewer, idOlder}, ""),
		"not an ID":           ok([]string{secret}, ""),
		"uppercase ID":        ok([]string{strings.ToUpper(idNewer)}, ""),
		"oldest first":        ok([]string{idOlder, idNewer}, ""),
		"twice":               ok([]string{idNewer, idNewer}, ""),
		"long cursor":         ok([]string{idNewer}, strings.Repeat("c", searchplugin.MaxCursor+1)),
		"cursor with a space": ok([]string{idNewer}, "page 2"),
		"cursor not ASCII":    ok([]string{idNewer}, "ページ"),
		"invalid first page":  {Outcome: searchv1.Outcome_OUTCOME_INVALID_CURSOR},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			page, err := serving(t, &answers{page: res}).Search(t.Context(), q)
			if !errors.Is(err, search.ErrBackendFailed) || len(page.StatusIDs) != 0 || page.Next != "" {
				t.Fatalf("Search = %v, %v; want ErrBackendFailed and nothing", page, err)
			}
			if !strings.Contains(err.Error(), "Search") || strings.Contains(err.Error(), secret) {
				t.Errorf("the error %q must name the RPC and hold no text", err)
			}
		})
	}
	if _, err := serving(t, &answers{page: ok([]string{idNewer}, strings.Repeat("c", searchplugin.MaxCursor))}).Search(t.Context(), q); err != nil {
		t.Errorf("a cursor of %d bytes: %v", searchplugin.MaxCursor, err)
	}
}

// OUTCOME_INVALID_CURSOR is the client's mistake when the server sent a cursor.
func TestSearchInvalidCursor(t *testing.T) {
	t.Parallel()
	x := serving(t, &answers{page: &searchv1.SearchResponse{Outcome: searchv1.Outcome_OUTCOME_INVALID_CURSOR}})
	_, err := x.Search(t.Context(), search.Query{Text: secret, Cursor: "made up", Limit: 1})
	if !errors.Is(err, search.ErrInvalidCursor) || errors.Is(err, search.ErrBackendFailed) {
		t.Errorf("Search = %v, want ErrInvalidCursor", err)
	}
}

// A plugin that fails in transport, is down, or does not answer in time fails the search as ErrBackendFailed, and the
// error holds neither the query nor what the plugin said.
func TestSearchTransportFailures(t *testing.T) {
	t.Parallel()
	q := search.Query{Text: secret, Limit: 1}
	_, err := serving(t, &answers{err: connect.NewError(connect.CodeUnavailable, secret)}).Search(t.Context(), q)
	if !errors.Is(err, search.ErrBackendFailed) || strings.Contains(err.Error(), secret) {
		t.Errorf("a Connect error: %v", err)
	}
	_, err = newIndex(t, kind.Destination{Address: "unix:///nowhere/search.sock"}).Search(t.Context(), q)
	if !errors.Is(err, search.ErrBackendFailed) || strings.Contains(err.Error(), secret) {
		t.Errorf("a plugin that is down: %v", err)
	}
	h, _ := fakeplugin.Connect(fakeplugin.Options{Search: fake.New(), Block: true})
	x := newIndex(t, kind.Destination{Address: fakeplugin.ServeUnix(t, h), Timeout: 50 * time.Millisecond})
	_, err = x.Search(t.Context(), q)
	if !errors.Is(err, search.ErrBackendFailed) || strings.Contains(err.Error(), secret) {
		t.Errorf("a plugin that does not answer: %v", err)
	}
}
