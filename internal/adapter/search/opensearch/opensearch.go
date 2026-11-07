// SPDX-License-Identifier: AGPL-3.0-only

// Package opensearch is the search.Index of search = opensearch (ADR-0149 §14): an OpenSearch
// cluster that the operator runs, or a domain of Amazon OpenSearch Service. It calls the REST API
// with net/http and encoding/json, without a client library, and signs the requests with AWS
// Signature Version 4 when the domain needs it.
//
// The index is an alias, Options.Index, on the index Options.Index-v<mappingVersion> that holds
// the documents. Ensure creates that index and points the alias at it; when a release changes the
// mapping, it creates the index of the new version, moves the alias, and the operator rebuilds.
// Every write names the alias with require_alias, so that a missing alias never lets the engine
// create an index of its own with a mapping it guesses.
//
// Nothing the engine says reaches an error: its error responses can quote the document or the
// query, so an error holds the operation, the HTTP status, and the type of the engine's error.
package opensearch

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/mycroblog/my/internal/app/search"
)

// DefaultIndex is the default of Options.Index (search_opensearch_index).
const DefaultIndex = "my-statuses"

// The AWS services whose requests Options.AWS signs.
const (
	// ServiceManaged is a domain of Amazon OpenSearch Service.
	ServiceManaged = "es"
)

// Options configure the Index.
type Options struct {
	// URL is the base URL of the REST API (search_opensearch_url), with no credentials, query, or
	// fragment; a path is kept, for a cluster behind a reverse proxy.
	URL string
	// Username and Password authenticate with HTTP Basic authentication, when Username is set.
	Username string
	Password string
	// AWS signs every request with Signature Version 4, when it is set; it excludes Username.
	AWS *AWSSigning
	// Index is the name of the alias (search_opensearch_index).
	Index string
	// HTTPClient calls the API: the operator configured the cluster, which may be on a private
	// address, so it is the adapter's own client rather than the SSRF-guarded kernel.HTTPClient.
	HTTPClient *http.Client
	Logger     *slog.Logger
}

// AWSSigning is the request signing of Amazon OpenSearch Service.
type AWSSigning struct {
	// Credentials come from the SDK's default chain: the execution role, the environment, the
	// shared files.
	Credentials aws.CredentialsProvider
	// Region is the region of the domain, such as us-east-1.
	Region string
	// Service is the signing name: ServiceManaged.
	Service string
	// Now is the clock of the signatures; nil is time.Now.
	Now func() time.Time
}

// Index is the OpenSearch index. Use New, then Ensure.
type Index struct {
	base   string
	o      Options
	signer *signer
	// version is the version of the mapping, mappingVersion but in the tests of an upgrade.
	version int
}

var _ search.Index = (*Index)(nil)

// New returns the Index of o. It sends nothing: Ensure creates the index.
func New(o Options) (*Index, error) {
	base, err := url.Parse(o.URL)
	switch {
	case err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https"):
		return nil, errors.New("opensearch: the URL must be an http or https URL with a host")
	case base.User != nil || base.RawQuery != "" || base.Fragment != "":
		return nil, errors.New("opensearch: the URL must have no credentials, query, or fragment")
	case o.Index == "":
		return nil, errors.New("opensearch: the index name is required")
	case o.HTTPClient == nil || o.Logger == nil:
		return nil, errors.New("opensearch: an HTTP client and a logger are required")
	case o.Username == "" && o.Password != "":
		return nil, errors.New("opensearch: a password needs a username")
	case o.AWS != nil && o.Username != "":
		return nil, errors.New("opensearch: AWS request signing and a username exclude each other")
	}
	x := &Index{base: strings.TrimSuffix(base.String(), "/"), o: o, version: mappingVersion}
	if o.AWS != nil {
		s, err := newSigner(*o.AWS)
		if err != nil {
			return nil, err
		}
		x.signer = s
	}
	return x, nil
}

// document is the source of a document of the index; mapping() declares its fields.
type document struct {
	StatusID    string    `json:"status_id"`
	AuthorID    string    `json:"author_id"`
	Visibility  string    `json:"visibility"`
	Language    string    `json:"language,omitempty"`
	PublishedAt time.Time `json:"published_at"`
	Text        string    `json:"text"`
}

// writeQuery is the query of a write: the name must be the alias, never an index the engine
// would create.
var writeQuery = url.Values{"require_alias": {"true"}}

// Replace implements search.Index with the index API, which replaces the document of the ID.
func (x *Index) Replace(ctx context.Context, doc search.Document) error {
	if doc.Visibility != search.VisibilityPublic && doc.Visibility != search.VisibilityLocal {
		return fmt.Errorf("opensearch: the visibility %q is never indexed", doc.Visibility)
	}
	if doc.StatusID == "" {
		return errors.New("opensearch: a document needs a status ID")
	}
	body := document{
		StatusID: doc.StatusID, AuthorID: doc.AuthorID, Visibility: string(doc.Visibility),
		Language: doc.Language, PublishedAt: doc.PublishedAt.UTC(), Text: doc.Text,
	}
	return x.call(ctx, "replace", http.MethodPut, x.o.Index+"/_doc/"+url.PathEscape(doc.StatusID), writeQuery, body, nil)
}

// Remove implements search.Index with the delete API. A document that is not there is not an
// error, but an alias that is not there is.
func (x *Index) Remove(ctx context.Context, statusID string) error {
	err := x.call(ctx, "remove", http.MethodDelete, x.o.Index+"/_doc/"+url.PathEscape(statusID), nil, nil, nil)
	var e *Error
	if errors.As(err, &e) && e.Status == http.StatusNotFound && e.Type == "" {
		return nil
	}
	return err
}

// RemoveByAuthor implements search.Index.
func (x *Index) RemoveByAuthor(ctx context.Context, authorID string) error {
	return x.deleteByQuery(ctx, "remove by author", term("author_id", authorID))
}

// RemoveByVisibility implements search.Index.
func (x *Index) RemoveByVisibility(ctx context.Context, visibility search.Visibility) error {
	return x.deleteByQuery(ctx, "remove by visibility", term("visibility", string(visibility)))
}

// Clear implements search.Index. It keeps the index and its mapping.
func (x *Index) Clear(ctx context.Context) error {
	return x.deleteByQuery(ctx, "clear", map[string]any{"match_all": map[string]any{}})
}

func term(field, value string) map[string]any {
	return map[string]any{"term": map[string]any{field: value}}
}

// deleteByQuery deletes the documents that query matches. conflicts=proceed lets it go on past
// a document that a write changed meanwhile: the job of that write sees the current state after
// it. A call that times out leaves the engine's task running, and the retry of the job deletes
// what is left.
func (x *Index) deleteByQuery(ctx context.Context, op string, query map[string]any) error {
	// A delete by query sees only the documents of the last refresh: refresh first, so that a
	// document written within the last second is deleted too.
	if err := x.refresh(ctx); err != nil {
		return err
	}
	return x.call(ctx, op, http.MethodPost, x.o.Index+"/_delete_by_query", url.Values{"conflicts": {"proceed"}},
		map[string]any{"query": query}, nil)
}

// cursorPrefix marks a cursor of this adapter, so that it can tell one it did not issue.
const cursorPrefix = "before:"

// searchResponse is the part of a response of the search API that the adapter reads.
type searchResponse struct {
	Hits struct {
		Hits []struct {
			ID string `json:"_id"`
		} `json:"hits"`
	} `json:"hits"`
}

// Search implements search.Index with a match query of every word, newest first by status ID.
// The query is never read as the query language: a match query analyzes the text as the field's
// search analyzer does, and the operator and joins the words. The cursor is the last ID of the
// page, and the next page holds the IDs below it, so the index can change between the pages.
func (x *Index) Search(ctx context.Context, q search.Query) (search.Page, error) {
	before, err := decodeCursor(q.Cursor)
	if err != nil {
		return search.Page{}, err
	}
	if strings.TrimSpace(q.Text) == "" || q.Limit < 1 {
		return search.Page{}, nil
	}
	query := map[string]any{"bool": map[string]any{
		"must": []any{map[string]any{"match": map[string]any{"text": map[string]any{
			"query": q.Text, "operator": "and", "zero_terms_query": "none",
		}}}},
	}}
	if before != "" {
		query["bool"].(map[string]any)["filter"] = []any{map[string]any{"range": map[string]any{"status_id": map[string]any{"lt": before}}}}
	}
	body := map[string]any{
		"size": q.Limit + 1, "_source": false, "track_total_hits": false,
		"query": query, "sort": []any{map[string]any{"status_id": "desc"}},
	}
	var res searchResponse
	if err := x.call(ctx, "search", http.MethodPost, x.o.Index+"/_search", nil, body, &res); err != nil {
		return search.Page{}, err
	}
	ids := make([]string, 0, len(res.Hits.Hits))
	for _, h := range res.Hits.Hits {
		ids = append(ids, h.ID)
	}
	if len(ids) <= q.Limit {
		return search.Page{StatusIDs: ids}, nil
	}
	page := ids[:q.Limit]
	return search.Page{StatusIDs: page, Next: encodeCursor(page[len(page)-1])}, nil
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
