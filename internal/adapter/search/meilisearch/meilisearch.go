// SPDX-License-Identifier: AGPL-3.0-only

// Package meilisearch is the search.Index of search = meilisearch (ADR-0149 §3): a Meilisearch
// server, self-hosted or Meilisearch Cloud, holds the index. The adapter calls its REST API with
// net/http, without Meilisearch's SDK.
//
// Meilisearch applies every write as an asynchronous task. Each write of the adapter enqueues its
// task and waits, within the call's context, until the task has succeeded; a task that fails, is
// canceled, or does not finish in time is an error, so that the job behind the write retries.
// The tasks of an index run in the order they were enqueued, so a write that succeeded also means
// that every write before it is applied.
//
// The index settings are the adapter's defaults, overridden by the operator's settings file, and
// the attributes that the port needs (ownedSettings), which the file may not set. The first call
// in a process creates the index when it does not exist and sends the settings unless the index
// already has them: a marker document (settingsDocID) holds the hash of the settings applied
// last. Meilisearch reindexes the documents it holds by itself, so no change of the settings needs
// a rebuild; the writes wait for the settings task in the order of the tasks.
//
// A document keeps its text in the field of its language: text_ja for Japanese, text_zh for
// Chinese, text_ko for Korean, and text for every other language and for none. The index tells
// Meilisearch the language of each of the first three, so that it segments, for example,
// Japanese written only in kanji as Japanese rather than as Chinese (ADR-0149 §13).
//
// The API key is sent in the Authorization header only, and no error quotes it, a response
// body, or a query.
package meilisearch

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/mycroblog/my/internal/app/search"
)

// DefaultIndex is the uid of the index when the operator sets none.
const DefaultIndex = "my_statuses"

// The fields of a document.
const (
	fieldStatusID   = "status_id"
	fieldAuthorID   = "author_id"
	fieldVisibility = "visibility"
	fieldPublished  = "published_at"
	fieldText       = "text"
)

// localized maps the primary language subtag of Document.Language to the field of its text and
// the locale (ISO 639-3) that Meilisearch segments the field with.
var localized = []struct{ lang, field, locale string }{
	{"ja", "text_ja", "jpn"},
	{"zh", "text_zh", "cmn"},
	{"ko", "text_ko", "kor"},
}

// settingsDocID is the ID of the marker document that holds the hash of the settings applied
// last. It has no text, so no search finds it, and its visibility is neither of the port's.
const (
	settingsDocID      = "my_settings"
	settingsVisibility = "settings"
	fieldSettingsHash  = "settings_hash"
)

// cursorPrefix marks a cursor of this adapter, so that it can tell one it did not issue.
const cursorPrefix = "m1:"

// maxLimit bounds the hits of one search: Meilisearch returns at most pagination.maxTotalHits
// (1000 by default) hits of a query.
const maxLimit = 999

// The polling of a task: the first wait, and the longest one.
const (
	pollFirst = 10 * time.Millisecond
	pollMax   = 250 * time.Millisecond
)

// maxErrorBody bounds what is read of an error response.
const maxErrorBody = 64 << 10

// Options configure the Index.
type Options struct {
	// URL is the base URL of the Meilisearch server (search_meilisearch_url).
	URL string
	// APIKey is the master key or an API key of the server (search_meilisearch_api_key), empty
	// for a server without a master key.
	APIKey string
	// Index is the uid of the index (search_meilisearch_index), DefaultIndex when empty.
	Index string
	// Settings is the content of the settings file (search_meilisearch_settings_file), a JSON
	// object of index settings, nil for the adapter's defaults.
	Settings []byte
	// HTTPClient calls the API: the operator configured the server, which may be on a private
	// address, so it is the adapter's own client rather than the SSRF-guarded kernel.HTTPClient.
	HTTPClient *http.Client
}

// Index is the Meilisearch index. Use New.
type Index struct {
	base   *url.URL
	key    string
	uid    string
	client *http.Client
	set    indexSettings

	mu sync.Mutex
	// settingsTask is the task of the settings that this process enqueued, -1 before it did.
	settingsTask int64
	// settingsApplied says that the settings task succeeded.
	settingsApplied bool
}

var _ search.Index = (*Index)(nil)

// New returns the Index of o. It connects to nothing: the first call creates the index.
func New(o Options) (*Index, error) {
	base, err := parseURL(o.URL)
	if err != nil {
		return nil, err
	}
	uid := o.Index
	if uid == "" {
		uid = DefaultIndex
	}
	if err := CheckIndexUID(uid); err != nil {
		return nil, err
	}
	if o.HTTPClient == nil {
		return nil, errors.New("meilisearch: no HTTP client")
	}
	set, err := parseSettings(o.Settings)
	if err != nil {
		return nil, err
	}
	return &Index{base: base, key: o.APIKey, uid: uid, client: o.HTTPClient, set: set, settingsTask: -1}, nil
}

// CheckURL checks search_meilisearch_url: an http: or https: URL with a host and nothing but a
// path after it. Credentials go in search_meilisearch_api_key, never in the URL.
func CheckURL(raw string) error {
	_, err := parseURL(raw)
	return err
}

func parseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		// A *url.Error quotes the URL.
		if ue, ok := errors.AsType[*url.Error](err); ok {
			err = ue.Err
		}
		return nil, fmt.Errorf("meilisearch: invalid URL: %w", err)
	}
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return nil, errors.New("meilisearch: the URL must use http or https")
	case u.Host == "":
		return nil, errors.New("meilisearch: the URL has no host")
	case u.User != nil:
		return nil, errors.New("meilisearch: the URL must not hold credentials; set search_meilisearch_api_key")
	case u.RawQuery != "" || u.Fragment != "":
		return nil, errors.New("meilisearch: the URL must not have a query or a fragment")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawPath = ""
	return u, nil
}

// CheckIndexUID checks search_meilisearch_index: what Meilisearch accepts as the uid of an index.
func CheckIndexUID(uid string) error {
	if uid == "" || len(uid) > 400 {
		return errors.New("meilisearch: the index uid must have 1 to 400 characters")
	}
	if !isIdentifier(uid) {
		return errors.New("meilisearch: the index uid may hold only a-z, A-Z, 0-9, hyphens, and underscores")
	}
	return nil
}

// isIdentifier says whether s holds only what Meilisearch accepts in an index uid and a document
// ID, which are also the only characters a filter value of the adapter holds.
func isIdentifier(s string) bool {
	for _, r := range s {
		if !isIdentifierRune(r) {
			return false
		}
	}
	return s != ""
}

func isIdentifierRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
}

// checkID refuses an ID that cannot be a document ID or a filter value of the adapter. IDs of my
// are UUIDs, so this only catches a mistake.
func checkID(what, id string) error {
	if len(id) > 511 || !isIdentifier(id) {
		return fmt.Errorf("meilisearch: the %s is not an ID", what)
	}
	return nil
}

// Error is an error response of Meilisearch, or a task that failed. It holds the error code of
// Meilisearch and never its message, which may quote a document or a query.
type Error struct {
	// Op is the call, such as "POST /indexes/{uid}/search" or "task 12".
	Op string
	// Status is the HTTP status, 0 for a task.
	Status int
	// Code is the error code of Meilisearch, such as "invalid_api_key"; empty when the response
	// had none.
	Code string
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("meilisearch: ")
	b.WriteString(e.Op)
	if e.Status != 0 {
		b.WriteString(": HTTP ")
		b.WriteString(strconv.Itoa(e.Status))
	}
	if e.Code != "" {
		b.WriteString(": ")
		b.WriteString(e.Code)
	}
	return b.String()
}

// The error codes of Meilisearch that the adapter reads.
const (
	codeIndexNotFound    = "index_not_found"
	codeDocumentNotFound = "document_not_found"
)

func isCode(err error, code string) bool {
	e, ok := errors.AsType[*Error](err)
	return ok && e.Code == code
}

// document is a document of the index.
type document map[string]any

func toDocument(d search.Document) document {
	doc := document{
		fieldStatusID:   d.StatusID,
		fieldAuthorID:   d.AuthorID,
		fieldVisibility: string(d.Visibility),
		fieldPublished:  d.PublishedAt.Unix(),
	}
	doc[textField(d.Language)] = normalize(d.Text)
	return doc
}

// textField returns the field of the text of a document in lang, a BCP 47 tag or empty.
func textField(lang string) string {
	primary, _, _ := strings.Cut(strings.ToLower(lang), "-")
	primary, _, _ = strings.Cut(primary, "_")
	for _, l := range localized {
		if l.lang == primary {
			return l.field
		}
	}
	return fieldText
}

// normalize folds the compatibility forms of Unicode, so that half-width katakana and full-width
// Latin letters and digits match their usual forms, which Meilisearch does not fold itself.
func normalize(s string) string {
	return norm.NFKC.String(s)
}

// Replace implements search.Index. Meilisearch adds the document, or replaces the whole document
// of the same ID, so a document whose language changed leaves no text in its former field.
func (x *Index) Replace(ctx context.Context, d search.Document) error {
	if err := checkID("status ID", d.StatusID); err != nil {
		return err
	}
	if err := checkID("author ID", d.AuthorID); err != nil {
		return err
	}
	if d.Visibility != search.VisibilityPublic && d.Visibility != search.VisibilityLocal {
		return fmt.Errorf("meilisearch: the visibility %q is never indexed", d.Visibility)
	}
	return x.write(ctx, "POST", x.indexPath("/documents")+"?primaryKey="+fieldStatusID, []document{toDocument(d)}, false)
}

// Remove implements search.Index.
func (x *Index) Remove(ctx context.Context, statusID string) error {
	if err := checkID("status ID", statusID); err != nil {
		return err
	}
	return x.write(ctx, "DELETE", x.indexPath("/documents/"+url.PathEscape(statusID)), nil, true)
}

// RemoveByAuthor implements search.Index.
func (x *Index) RemoveByAuthor(ctx context.Context, authorID string) error {
	if err := checkID("author ID", authorID); err != nil {
		return err
	}
	return x.write(ctx, "POST", x.indexPath("/documents/delete"), map[string]any{"filter": filterEquals(fieldAuthorID, authorID)}, true)
}

// RemoveByVisibility implements search.Index.
func (x *Index) RemoveByVisibility(ctx context.Context, visibility search.Visibility) error {
	if !isIdentifier(string(visibility)) {
		return errors.New("meilisearch: the visibility is not a word")
	}
	return x.write(ctx, "POST", x.indexPath("/documents/delete"), map[string]any{"filter": filterEquals(fieldVisibility, string(visibility))}, true)
}

// Clear implements search.Index. It deletes the index, which is quicker than deleting each of its
// documents, and creates it again with its settings.
func (x *Index) Clear(ctx context.Context) error {
	var t taskRef
	err := x.call(ctx, "DELETE", x.indexPath(""), nil, &t)
	if err == nil {
		err = x.await(ctx, t.TaskUID)
	}
	if err != nil && !isCode(err, codeIndexNotFound) {
		return err
	}
	x.forget()
	if err := x.ensure(ctx); err != nil {
		return err
	}
	return x.settled(ctx)
}

// filterEquals returns the filter of a field equal to a value that isIdentifier accepts, so that
// the value needs no escape.
func filterEquals(field, value string) string {
	return field + ` = "` + value + `"`
}

// write ensures the index, enqueues the task of one write, and waits until it has succeeded. For a
// removal, an index that does not exist has nothing to remove.
func (x *Index) write(ctx context.Context, method, path string, body any, removal bool) error {
	if err := x.ensure(ctx); err != nil {
		return err
	}
	var t taskRef
	if err := x.call(ctx, method, path, body, &t); err != nil {
		if removal && isCode(err, codeIndexNotFound) {
			return nil
		}
		return err
	}
	if err := x.await(ctx, t.TaskUID); err != nil {
		if removal && isCode(err, codeIndexNotFound) {
			return nil
		}
		return err
	}
	return x.settled(ctx)
}

// ensure creates the index when it does not exist and enqueues the settings unless the marker
// document says the index has them, once in a process while they succeed. It does not wait for
// the settings: the writes after them wait for them in the order of the tasks, and settled checks
// that they succeeded.
func (x *Index) ensure(ctx context.Context) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.settingsTask >= 0 || x.settingsApplied {
		return nil
	}
	err := x.call(ctx, "GET", x.indexPath(""), nil, nil)
	switch {
	case isCode(err, codeIndexNotFound):
		var t taskRef
		if err := x.call(ctx, "POST", "/indexes", map[string]any{"uid": x.uid, "primaryKey": fieldStatusID}, &t); err != nil {
			return err
		}
		// Another process may create it at the same time, and the settings below create it
		// anyway; the primary key also comes with every document.
	case err != nil:
		return err
	default:
		var marker map[string]any
		err := x.call(ctx, "GET", x.indexPath("/documents/"+settingsDocID+"?fields="+fieldSettingsHash), nil, &marker)
		switch {
		case err == nil && marker[fieldSettingsHash] == x.set.hash:
			x.settingsApplied = true
			return nil
		case err != nil && !isCode(err, codeDocumentNotFound):
			return err
		}
	}
	var t taskRef
	if err := x.call(ctx, "PATCH", x.indexPath("/settings"), x.set.body, &t); err != nil {
		return err
	}
	x.settingsTask = t.TaskUID
	return nil
}

// settled checks, once, that the settings task of ensure succeeded, and then records their hash
// in the marker document. A write that succeeded came after the settings task, so it has
// finished; Clear waits for it. A failed one is enqueued again on the next call.
func (x *Index) settled(ctx context.Context) error {
	x.mu.Lock()
	uid, applied := x.settingsTask, x.settingsApplied
	x.mu.Unlock()
	if applied || uid < 0 {
		return nil
	}
	if err := x.await(ctx, uid); err != nil {
		if _, ok := errors.AsType[*Error](err); ok {
			x.mu.Lock()
			if x.settingsTask == uid {
				x.settingsTask = -1
			}
			x.mu.Unlock()
		}
		return fmt.Errorf("meilisearch: the settings of the index: %w", err)
	}
	// The marker only saves sending the same settings again on the next start, so a failure to
	// write it fails nothing. Its write is a task too: wait for it, or a process that starts right
	// after this one reads no marker yet and sends the same settings again.
	marker := []map[string]any{{fieldStatusID: settingsDocID, fieldVisibility: settingsVisibility, fieldSettingsHash: x.set.hash}}
	var written taskRef
	if x.call(ctx, "POST", x.indexPath("/documents")+"?primaryKey="+fieldStatusID, marker, &written) == nil {
		_ = x.await(ctx, written.TaskUID)
	}
	x.mu.Lock()
	if x.settingsTask == uid {
		x.settingsTask, x.settingsApplied = -1, true
	}
	x.mu.Unlock()
	return nil
}

// taskRef is the response of a call that enqueues a task.
type taskRef struct {
	TaskUID int64 `json:"taskUid"`
}

// task is the response of GET /tasks/{uid}.
type task struct {
	Status string `json:"status"`
	Error  *struct {
		Code string `json:"code"`
	} `json:"error"`
}

// await waits until a task has finished, and fails unless it succeeded. It fails as well when ctx
// ends first: the task may still succeed later, and the job retries the write, which gives the
// same result.
func (x *Index) await(ctx context.Context, uid int64) error {
	op := "task " + strconv.FormatInt(uid, 10)
	wait := pollFirst
	for {
		var t task
		if err := x.call(ctx, "GET", "/tasks/"+strconv.FormatInt(uid, 10), nil, &t); err != nil {
			return err
		}
		switch t.Status {
		case "succeeded":
			return nil
		case "failed", "canceled":
			e := &Error{Op: op + " " + t.Status}
			if t.Error != nil {
				e.Code = t.Error.Code
			}
			return e
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("meilisearch: %s did not finish: %w", op, ctx.Err())
		case <-timer.C:
		}
		wait = min(wait*3/2, pollMax)
	}
}

// searchRequest is the body of POST /indexes/{uid}/search.
type searchRequest struct {
	Q                    string   `json:"q"`
	Filter               string   `json:"filter,omitempty"`
	Sort                 []string `json:"sort"`
	Limit                int      `json:"limit"`
	AttributesToRetrieve []string `json:"attributesToRetrieve"`
	MatchingStrategy     string   `json:"matchingStrategy"`
	ShowMatchesPosition  bool     `json:"showMatchesPosition"`
}

type searchResponse struct {
	Hits []struct {
		StatusID        string                     `json:"status_id"`
		MatchesPosition map[string]json.RawMessage `json:"_matchesPosition"`
	} `json:"hits"`
}

// Search implements search.Index. The cursor is the ID of the last status of the page, and the
// next page holds the IDs before it, so a cursor stays valid while the index changes.
func (x *Index) Search(ctx context.Context, q search.Query) (search.Page, error) {
	before, err := decodeCursor(q.Cursor)
	if err != nil {
		return search.Page{}, err
	}
	text, ok := queryText(q.Text)
	if !ok || q.Limit < 1 {
		return search.Page{}, nil
	}
	limit := min(q.Limit, maxLimit)
	if err := x.ensure(ctx); err != nil {
		return search.Page{}, err
	}
	req := searchRequest{
		Q: text, Sort: []string{fieldStatusID + ":desc"}, Limit: limit + 1,
		AttributesToRetrieve: []string{fieldStatusID}, MatchingStrategy: "all", ShowMatchesPosition: true,
	}
	if before != "" {
		req.Filter = fieldStatusID + ` < "` + before + `"`
	}
	var res searchResponse
	if err := x.call(ctx, "POST", x.indexPath("/search"), req, &res); err != nil {
		if isCode(err, codeIndexNotFound) {
			// Nothing was indexed yet, or the index was deleted: the next call creates it.
			x.forget()
			return search.Page{}, nil
		}
		return search.Page{}, err
	}
	ids := make([]string, 0, len(res.Hits))
	for _, h := range res.Hits {
		// A query whose words Meilisearch reads as separators only is a search for every
		// document, whose hits match nothing: such a query has no word.
		if len(h.MatchesPosition) == 0 {
			return search.Page{}, nil
		}
		ids = append(ids, h.StatusID)
	}
	if len(ids) <= limit {
		return search.Page{StatusIDs: ids}, nil
	}
	ids = ids[:limit]
	return search.Page{StatusIDs: ids, Next: encodeCursor(ids[len(ids)-1])}, nil
}

// forget makes the next call create the index and set its settings again.
func (x *Index) forget() {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.settingsTask, x.settingsApplied = -1, false
}

// queryText returns the text of a query as Meilisearch takes it, and false when it has no word.
// Meilisearch reads two things in a query: a phrase between double quotes and a word after a
// minus sign, which it excludes. Neither is part of the port, so the double quotes become spaces
// and the minus signs before a word go.
func queryText(raw string) (string, bool) {
	words := strings.Fields(strings.ReplaceAll(normalize(raw), `"`, " "))
	kept := words[:0]
	hasWord := false
	for _, w := range words {
		w = strings.TrimLeft(w, "-")
		if w == "" {
			continue
		}
		kept = append(kept, w)
		for _, r := range w {
			if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.Is(unicode.So, r) {
				hasWord = true
				break
			}
		}
	}
	return strings.Join(kept, " "), hasWord
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
	if !ok || checkID("cursor", id) != nil {
		return "", search.ErrInvalidCursor
	}
	return id, nil
}

// indexPath returns the path of the index followed by rest.
func (x *Index) indexPath(rest string) string {
	return "/indexes/" + url.PathEscape(x.uid) + rest
}

// call sends one request and decodes the JSON response into out, when out is not nil. An error
// response is an *Error with the code of Meilisearch; an error of the transport says the method,
// the path without its IDs, and what failed, such as a refused connection to the address.
func (x *Index) call(ctx context.Context, method, path string, body, out any) error {
	op := method + " " + opPath(path)
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("meilisearch: %s: %w", op, err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, x.base.String()+path, r)
	if err != nil {
		return fmt.Errorf("meilisearch: %s: the request: %w", op, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if x.key != "" {
		req.Header.Set("Authorization", "Bearer "+x.key)
	}
	res, err := x.client.Do(req)
	if err != nil {
		return fmt.Errorf("meilisearch: %s: %w", op, transportError(err))
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		var e struct {
			Code string `json:"code"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, maxErrorBody)).Decode(&e)
		return &Error{Op: op, Status: res.StatusCode, Code: safeCode(e.Code)}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("meilisearch: %s: the response: %w", op, err)
	}
	return nil
}

// opPath returns the path of a call for its errors: the IDs of documents and tasks become
// placeholders, and the query goes.
func opPath(path string) string {
	path, _, _ = strings.Cut(path, "?")
	placeholders := map[string]string{"indexes": "{uid}", "documents": "{id}", "tasks": "{uid}"}
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts); i++ {
		if p, ok := placeholders[parts[i-1]]; ok && parts[i] != "delete" {
			parts[i] = p
		}
	}
	return strings.Join(parts, "/")
}

// safeCode keeps an error code that looks like one of Meilisearch's, so that a server that is not
// Meilisearch puts nothing of its own in the errors.
func safeCode(code string) string {
	if len(code) > 64 {
		return ""
	}
	for _, r := range code {
		if (r < 'a' || r > 'z') && r != '_' {
			return ""
		}
	}
	return code
}

// transportError drops the URL that a *url.Error quotes, with the IDs in its path, and keeps what
// failed.
func transportError(err error) error {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		return ue.Err
	}
	return err
}
