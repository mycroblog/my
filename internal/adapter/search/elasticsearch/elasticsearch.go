// SPDX-License-Identifier: AGPL-3.0-only

// Package elasticsearch is the search.Index of search = elasticsearch (ADR-0149 §14): the full-text
// search of statuses in an Elasticsearch cluster (8 or later) that the operator runs. The adapter
// calls the REST API with net/http and encoding/json, without Elastic's client library.
//
// The documents live in one index behind an alias, the name the operator configures. my owns the
// fields of the index, which it writes, filters, sorts, and pages on; the operator may own its
// settings (Options.Settings): the analysis, the shards, the replicas. The adapter creates the
// index when the alias is missing, and when the mapping of this build is newer or the settings
// differ from those the index was made with (mapping.go): in place when the mapping only adds
// fields, and otherwise onto a new empty index that `my search rebuild` fills. Clear moves the
// alias to a new empty index too, which is faster than deleting every document and gives the disk
// back.
//
// The text field is analyzed by the analyzer AnalyzerText of the settings, and the words of a
// query by AnalyzerTextSearch when the settings define it. A query is one match_phrase of each of
// its words on the text, all of which must match. The default settings (DefaultSettings) need no
// plugin: their standard tokenizer makes a token of each kanji and each hiragana, and of each run
// of katakana, so a Japanese word is found wherever its characters stand together, and a Latin
// word as a whole word.
//
// The credentials (an API key, or a user name and password) go in the Authorization header only,
// and no error or log of the adapter holds them, the query, or what the engine said beyond the
// type of its error.
package elasticsearch

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/app/search"
)

// cursorPrefix marks a cursor of this adapter, so that it can tell one it did not issue.
const cursorPrefix = "es1:"

// DefaultIndex is the alias of the index when the operator names none.
const DefaultIndex = "my-statuses"

// maxResponseBytes bounds a response that the adapter reads.
const maxResponseBytes = 4 << 20

// Options configure the Index.
type Options struct {
	// URL is the base URL of the cluster's REST API (search_elasticsearch_url), such as
	// https://search.internal:9200; a path is kept, for a proxy that serves the API under one.
	URL string
	// APIKey is the encoded API key (search_elasticsearch_api_key), sent as "ApiKey <key>".
	APIKey string
	// Username and Password are the basic authentication of a native user, when APIKey is empty.
	Username string
	Password string
	// Index is the name of the alias that the documents live behind (search_elasticsearch_index).
	Index string
	// Settings is the JSON object of the settings of the indexes that the adapter creates, nil for
	// DefaultSettings. It must define the analyzer AnalyzerText, and may define
	// AnalyzerTextSearch.
	Settings []byte
	// HTTPClient calls the API: the operator configured the cluster, which may be on a private
	// address, so it is the adapter's own client rather than the SSRF-guarded kernel.HTTPClient
	// (ADR-0149 §3).
	HTTPClient *http.Client
	// Logger receives the warnings of an upgrade of the index; nil discards them.
	Logger *slog.Logger
}

// Index is the Elasticsearch index. Use Open.
type Index struct {
	o        Options
	base     *url.URL
	settings settings
}

var _ search.Index = (*Index)(nil)

// New returns the Index of o without calling the cluster; Open also creates or upgrades the index.
func New(o Options) (*Index, error) {
	switch {
	case o.URL == "":
		return nil, errors.New("elasticsearch: the URL is required")
	case o.HTTPClient == nil:
		return nil, errors.New("elasticsearch: an HTTP client is required")
	case o.APIKey != "" && (o.Username != "" || o.Password != ""):
		return nil, errors.New("elasticsearch: set an API key or a user name and password, not both")
	case (o.Username == "") != (o.Password == ""):
		return nil, errors.New("elasticsearch: a user name needs a password, and a password a user name")
	}
	if err := CheckIndexName(o.Index); err != nil {
		return nil, err
	}
	base, err := url.Parse(o.URL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil {
		// A *url.Error quotes the URL, so it is not wrapped.
		return nil, errors.New("elasticsearch: the URL must be an http or https URL with a host and no credentials")
	}
	base.Path = strings.TrimSuffix(base.Path, "/")
	base.RawPath, base.RawQuery, base.Fragment = "", "", ""
	if o.Settings == nil {
		o.Settings = []byte(DefaultSettings)
	}
	st, err := parseSettings(o.Settings)
	if err != nil {
		return nil, err
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	return &Index{o: o, base: base, settings: st}, nil
}

// Open returns the Index of o, with the index created or upgraded to the mapping of this build.
func Open(ctx context.Context, o Options) (*Index, error) {
	x, err := New(o)
	if err != nil {
		return nil, err
	}
	if err := x.ensure(ctx, currentMapping); err != nil {
		return nil, err
	}
	return x, nil
}

// CheckIndexName reports whether name can be the alias of the index: a name of Elasticsearch
// (lowercase, no character it refuses, not starting with -, _, or +) short enough for the
// suffixes of the indexes behind it, and not starting with a dot, which marks system indexes.
func CheckIndexName(name string) error {
	const maxBytes = 200
	if name == "" || len(name) > maxBytes || strings.ContainsAny(name[:1], "-_+.") || strings.ContainsFunc(name, isNotIndexNameRune) {
		return fmt.Errorf("elasticsearch: the index name must be up to %d lowercase letters, digits, '-', '_', or '.', starting with a letter or digit", maxBytes)
	}
	return nil
}

func isNotIndexNameRune(r rune) bool {
	return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' && r != '.'
}

// document is the source of a document of the index.
type document struct {
	StatusID   string `json:"status_id"`
	AuthorID   string `json:"author_id"`
	Visibility string `json:"visibility"`
	Text       string `json:"text"`
}

// Replace implements search.Index with the index API on the alias, which require_alias keeps from
// creating an index by itself when the alias is gone.
func (x *Index) Replace(ctx context.Context, doc search.Document) error {
	if doc.Visibility != search.VisibilityPublic && doc.Visibility != search.VisibilityLocal {
		return fmt.Errorf("elasticsearch: the visibility %q is never indexed", doc.Visibility)
	}
	body := document{StatusID: doc.StatusID, AuthorID: doc.AuthorID, Visibility: string(doc.Visibility), Text: doc.Text}
	return x.call(ctx, http.MethodPut, x.docPath(doc.StatusID), url.Values{"require_alias": {"true"}}, body, nil)
}

// Remove implements search.Index; a document that is not there is not an error, an index that is
// not there is.
func (x *Index) Remove(ctx context.Context, statusID string) error {
	err := x.call(ctx, http.MethodDelete, x.docPath(statusID), nil, nil, nil)
	var e *apiError
	if errors.As(err, &e) && e.Status == http.StatusNotFound && e.Type == "" {
		return nil
	}
	return err
}

// RemoveByAuthor implements search.Index.
func (x *Index) RemoveByAuthor(ctx context.Context, authorID string) error {
	return x.deleteByQuery(ctx, map[string]any{"term": map[string]any{"author_id": authorID}})
}

// RemoveByVisibility implements search.Index.
func (x *Index) RemoveByVisibility(ctx context.Context, visibility search.Visibility) error {
	return x.deleteByQuery(ctx, map[string]any{"term": map[string]any{"visibility": string(visibility)}})
}

// deleteByQuery removes the documents that query matches. The delete by query API sees only what
// a refresh made searchable, so the index is refreshed first: a document written a moment ago goes
// too.
func (x *Index) deleteByQuery(ctx context.Context, query map[string]any) error {
	if err := x.refresh(ctx); err != nil {
		return err
	}
	params := url.Values{"conflicts": {"proceed"}, "refresh": {"true"}, "slices": {"auto"}}
	return x.call(ctx, http.MethodPost, x.path(x.o.Index, "_delete_by_query"), params, map[string]any{"query": query}, nil)
}

// Clear implements search.Index: the alias moves to a new empty index and the old one is deleted
// in the same request (mapping.go).
func (x *Index) Clear(ctx context.Context) error {
	return x.replaceIndex(ctx, currentMapping, x.clearedIndexName(currentMapping))
}

// refresh makes every write so far searchable.
func (x *Index) refresh(ctx context.Context) error {
	return x.call(ctx, http.MethodPost, x.path(x.o.Index, "_refresh"), nil, nil, nil)
}

// Search implements search.Index: every word of the query is a phrase of the text, newest first by
// status ID, and a page goes on after the last ID of the previous one (search_after).
func (x *Index) Search(ctx context.Context, q search.Query) (search.Page, error) {
	after, err := decodeCursor(q.Cursor)
	if err != nil {
		return search.Page{}, err
	}
	words := strings.Fields(q.Text)
	if len(words) == 0 || q.Limit < 1 {
		return search.Page{}, nil
	}
	body := searchRequest(words, after, q.Limit+1)
	var res struct {
		Hits struct {
			Hits []struct {
				ID string `json:"_id"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := x.call(ctx, http.MethodPost, x.path(x.o.Index, "_search"), nil, body, &res); err != nil {
		return search.Page{}, err
	}
	ids := make([]string, 0, len(res.Hits.Hits))
	for _, h := range res.Hits.Hits {
		ids = append(ids, h.ID)
	}
	if len(ids) <= q.Limit {
		return search.Page{StatusIDs: ids}, nil
	}
	ids = ids[:q.Limit]
	return search.Page{StatusIDs: ids, Next: encodeCursor(ids[len(ids)-1])}, nil
}

// searchRequest is the body of a search for words, after the status ID after when it is not
// empty. Each word is the query of a match_phrase, which analyzes it as text: nothing in it is the
// query language of the engine.
func searchRequest(words []string, after string, size int) map[string]any {
	filters := make([]any, 0, len(words))
	for _, w := range words {
		filters = append(filters, map[string]any{"match_phrase": map[string]any{"text": map[string]any{"query": w}}})
	}
	body := map[string]any{
		"size":             size,
		"_source":          false,
		"track_total_hits": false,
		"sort":             []any{map[string]any{"status_id": "desc"}},
		"query":            map[string]any{"bool": map[string]any{"filter": filters}},
	}
	if after != "" {
		body["search_after"] = []any{after}
	}
	return body
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
	if !ok || id == "" || !utf8.ValidString(id) || strings.ContainsFunc(id, unicode.IsControl) {
		return "", search.ErrInvalidCursor
	}
	return id, nil
}

// docPath is the path of the document of a status.
func (x *Index) docPath(statusID string) string {
	return x.path(x.o.Index, "_doc", statusID)
}

// path joins the escaped segments onto the path of the base URL.
func (x *Index) path(segments ...string) string {
	var b strings.Builder
	b.WriteString(x.base.Path)
	for _, s := range segments {
		b.WriteByte('/')
		b.WriteString(url.PathEscape(s))
	}
	return b.String()
}

// apiError is an error response of the API. It keeps the type of the engine's error and never its
// reason, which may quote a document or the query.
type apiError struct {
	Method string
	Path   string
	Status int
	// Type is error.type of the response, such as index_not_found_exception, and empty when the
	// response has none, like the 404 of a document that is not there.
	Type string
	// Reason is error.reason of the response, which Error leaves out. Only the error of creating an
	// index shows it, since that reason quotes nothing but the settings and the mapping.
	Reason string
}

func (e *apiError) Error() string {
	if e.Type == "" {
		return fmt.Sprintf("elasticsearch: %s %s: status %d", e.Method, e.Path, e.Status)
	}
	return fmt.Sprintf("elasticsearch: %s %s: status %d: %s", e.Method, e.Path, e.Status, e.Type)
}

// errorType returns error.type of an error response when it is an identifier, and empty otherwise,
// with error.reason.
func errorType(body []byte) (typ, reason string) {
	var res struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &res) != nil || len(res.Error) == 0 {
		return "", ""
	}
	var e struct {
		Type   string `json:"type"`
		Reason string `json:"reason"`
	}
	if json.Unmarshal(res.Error, &e) != nil || len(e.Type) > 100 {
		return "", ""
	}
	if strings.ContainsFunc(e.Type, func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' }) {
		return "", ""
	}
	return e.Type, e.Reason
}

// call sends one request of the API with the JSON of in, when it is not nil, and decodes the
// response into out, when it is not nil. An error never holds the credentials, the URL, or the
// body of the request or the response; a failure to connect names the address it dialed.
func (x *Index) call(ctx context.Context, method, path string, params url.Values, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("elasticsearch: %s %s: %w", method, path, err)
		}
		body = bytes.NewReader(b)
	}
	u := *x.base
	u.Path = path
	u.RawQuery = params.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return fmt.Errorf("elasticsearch: %s %s: cannot build the request", method, path)
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	switch {
	case x.o.APIKey != "":
		req.Header.Set("Authorization", "ApiKey "+x.o.APIKey)
	case x.o.Username != "":
		req.SetBasicAuth(x.o.Username, x.o.Password)
	}
	resp, err := x.o.HTTPClient.Do(req)
	if err != nil {
		// A *url.Error quotes the URL; keep only what went wrong.
		if ue, ok := errors.AsType[*url.Error](err); ok {
			err = ue.Err
		}
		return fmt.Errorf("elasticsearch: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("elasticsearch: %s %s: read the response: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		typ, reason := errorType(b)
		return &apiError{Method: method, Path: path, Status: resp.StatusCode, Type: typ, Reason: reason}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("elasticsearch: %s %s: the response is not the expected JSON", method, path)
	}
	return nil
}
