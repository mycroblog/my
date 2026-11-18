// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeImports reads files with the domain's parser; each account has one block, and an import
// stays in progress until done is set.
type fakeImports struct {
	now     time.Time
	started []appidentity.ImportInput
	done    bool
}

func (f *fakeImports) WriteExport(_ context.Context, accountID string, kind identity.ImportKind, w io.Writer) error {
	rows := []identity.ExportRow{{Target: "blocked-by-" + accountID + "@remote.example"}}
	return identity.WriteExportCSV(w, kind, rows)
}

func (f *fakeImports) PreviewImport(_ context.Context, in appidentity.ImportInput) (appidentity.ImportPreview, error) {
	entries, skipped, err := identity.ParseImportCSV(in.Kind, in.Text)
	if err != nil {
		return appidentity.ImportPreview{}, err
	}
	p := appidentity.ImportPreview{Entries: entries, Skipped: skipped}
	if in.Mode == identity.ImportOverwrite {
		p.Removals = []appidentity.ImportTarget{{ID: testkit.NamedID("removal")}}
	}
	return p, nil
}

func (f *fakeImports) StartImport(ctx context.Context, in appidentity.ImportInput) (identity.DataImportProgress, error) {
	if len(f.started) > 0 && !f.done {
		return identity.DataImportProgress{}, appidentity.ErrImportInProgress
	}
	p, err := f.PreviewImport(ctx, in)
	if err != nil {
		return identity.DataImportProgress{}, err
	}
	f.started = append(f.started, in)
	f.done = false
	return f.progress(len(p.Entries) + len(p.Removals)), nil
}

func (f *fakeImports) progress(total int) identity.DataImportProgress {
	in := f.started[len(f.started)-1]
	p := identity.DataImportProgress{
		DataImport: identity.DataImport{ID: "01890a5d-ac96-774b-bcce-b302099d0001", UserID: in.UserID, Kind: in.Kind, Mode: in.Mode, CreatedAt: f.now},
		Total:      total,
	}
	if f.done {
		p.Processed, p.Failed = total, 1
	}
	return p
}

func (f *fakeImports) ListImports(_ context.Context, userID string) ([]identity.DataImportProgress, error) {
	if len(f.started) == 0 || f.started[0].UserID != userID {
		return nil, nil
	}
	return []identity.DataImportProgress{f.progress(2)}, nil
}

func TestCSVExports(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Imports = &fakeImports{} })
	res := e.send(t, http.MethodGet, "/csv_exports/blocks", "read-accounts", "")
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/csv" || res.Header.Get("Cache-Control") != "no-store" ||
		res.Header.Get("Content-Disposition") != `attachment; filename="blocked_accounts.csv"` ||
		string(body) != "blocked-by-"+aliceAccountID+"@remote.example\n" {
		t.Errorf("GET /csv_exports/blocks = %d %q %v", res.StatusCode, body, res.Header)
	}
	res = e.send(t, http.MethodGet, "/csv_exports/following", "read-accounts", "")
	if body, _ := io.ReadAll(res.Body); !strings.HasPrefix(string(body), "Account address,Show boosts,Notify on new posts,Languages\n") {
		t.Errorf("following = %q, want Mastodon's header", body)
	}
	if res := e.send(t, http.MethodGet, "/csv_exports/lists", "read-accounts", ""); res.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown kind = %d, want 400", res.StatusCode)
	}
	if res := e.send(t, http.MethodGet, "/csv_exports/blocks", "follow-push", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("without read:accounts = %d, want 403", res.StatusCode)
	}
}

func TestDataImports(t *testing.T) {
	f := &fakeImports{now: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Imports = f })
	file := `{"kind":"blocks","mode":"overwrite","text":"a@remote.example\nnot an address\n@A@REMOTE.EXAMPLE\n"}`

	res := e.sendJSON(t, http.MethodPost, "/imports/preview", "write", file)
	preview := decodeJSON[clientapi.DataImportPreview](t, res)
	if res.StatusCode != http.StatusOK || preview.Entries != 1 || preview.Removals != 1 || len(preview.Skipped) != 2 ||
		preview.Skipped[0] != (clientapi.DataImportSkipped{Line: 2, Value: "not an address", Reason: clientapi.DataImportSkippedReasonInvalid}) ||
		preview.Skipped[1].Reason != clientapi.DataImportSkippedReasonDuplicate {
		t.Errorf("preview = %d %+v", res.StatusCode, preview)
	}
	if len(f.started) != 0 {
		t.Fatalf("a preview started %+v", f.started)
	}
	for body, want := range map[string]int{
		`{"kind":"blocks","mode":"merge","text":"\"a"}`: http.StatusUnprocessableEntity,
	} {
		if res := e.sendJSON(t, http.MethodPost, "/imports/preview", "write", body); res.StatusCode != want {
			t.Errorf("preview %s = %d, want %d", body, res.StatusCode, want)
		}
	}
	if res := e.sendJSON(t, http.MethodPost, "/imports", "read", file); res.StatusCode != http.StatusForbidden {
		t.Errorf("start without write:accounts = %d, want 403", res.StatusCode)
	}

	res = e.sendJSON(t, http.MethodPost, "/imports", "write", file)
	started := decodeJSON[clientapi.DataImport](t, res)
	if res.StatusCode != http.StatusAccepted || started.State != clientapi.DataImportStateInProgress || started.Total != 2 ||
		started.Kind != clientapi.DataImportKindBlocks || started.Mode != clientapi.DataImportModeOverwrite || !started.CreatedAt.Equal(f.now) {
		t.Errorf("start = %d %+v, want 202 in progress", res.StatusCode, started)
	}
	if len(f.started) != 1 || f.started[0].UserID != userOf(aliceAccountID) || f.started[0].AccountID != aliceAccountID {
		t.Errorf("started = %+v, want alice's import", f.started)
	}
	res = e.sendJSON(t, http.MethodPost, "/imports", "write", file)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeImportInProgress {
		t.Errorf("second start = %d %+v, want 409 import-in-progress", res.StatusCode, p)
	}

	f.done = true
	res = e.send(t, http.MethodGet, "/imports", "read-accounts", "")
	list := decodeJSON[clientapi.DataImportList](t, res)
	if len(list.Items) != 1 || list.Items[0].State != clientapi.DataImportStateFinished || list.Items[0].Processed != 2 || list.Items[0].Failed != 1 {
		t.Errorf("GET /imports = %+v, want the finished import", list)
	}
	if list := decodeJSON[clientapi.DataImportList](t, e.send(t, http.MethodGet, "/imports", "bob", "")); len(list.Items) != 0 {
		t.Errorf("bob's imports = %+v, want none", list)
	}
}

// importBodyLimit is the largest request body of the two import operations; the CSV text of the
// contract is up to 1,000,000 characters, so the body is far over the 1 MiB of other operations.
const importBodyLimit = 6 << 20

// importRequest builds a JSON POST to path as alice, with a body that declares length or, when
// length is -1, one that does not.
func importRequest(path string, body []byte, length int64, acceptLanguage string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, clientapi.BasePath+path, bytes.NewReader(body))
	if length < 0 {
		req.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body)))
	}
	req.ContentLength = length
	if length >= 0 {
		req.Header.Set("Content-Length", strconv.FormatInt(length, 10))
	}
	req.RemoteAddr = "192.0.2.1:1234"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer write")
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	return req
}

func importBody(text string) []byte {
	return []byte(`{"kind":"blocks","mode":"merge","text":"` + text + `"}`)
}

// TestDataImportTakesTheTextItsContractAllows checks that both import operations read a body
// over 1 MiB that is valid under the contract.
func TestDataImportTakesTheTextItsContractAllows(t *testing.T) {
	var crlf strings.Builder
	// 5,000 addresses of 17 characters, then blank lines up to 985,000 characters (a file takes at
	// most 10,000 entries). Each CRLF is 4 bytes in the JSON, so the body is over 1 MiB.
	for i := 0; i < 5_000; i++ {
		fmt.Fprintf(&crlf, "u%06d@r.example\\r\\n", i)
	}
	crlf.WriteString(strings.Repeat("\\r\\n", (985_000-5_000*17)/2))
	bodies := map[string][]byte{
		"400,000 Japanese characters": importBody(strings.Repeat("あ", 400_000)),
		"985,000 ASCII characters":    importBody(crlf.String()),
	}
	for name, body := range bodies {
		if len(body) <= 1<<20 || len(body) > importBodyLimit {
			t.Fatalf("%s: body is %d bytes, want over 1 MiB and within the limit", name, len(body))
		}
	}
	for _, path := range []string{"/imports/preview", "/imports"} {
		for name, body := range bodies {
			t.Run(path+" "+name, func(t *testing.T) {
				f := &fakeImports{now: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)}
				e := newAuthEnv(t, func(o *clientapi.Options) { o.Imports = f })
				res := e.spec.Serve(t, e.h, importRequest(path, body, int64(len(body)), ""))
				if res.StatusCode == http.StatusRequestEntityTooLarge {
					t.Fatalf("POST %s = 413 %+v, want the handler to run", path, decodeProblem(t, res))
				}
				if strings.Contains(name, "ASCII") {
					want := http.StatusOK
					if path == "/imports" {
						want = http.StatusAccepted
					}
					if res.StatusCode != want {
						p := decodeProblem(t, res)
						t.Errorf("POST %s = %d %q %q, want %d", path, res.StatusCode, deref(p.Detail), deref(p.Field), want)
					}
				}
			})
		}
	}
}

// TestDataImportBodyOverTheLimit checks that a body over 6 MiB is refused with a 413 that names
// the CSV limit, whether it declares its length or not, and in the language asked for.
func TestDataImportBodyOverTheLimit(t *testing.T) {
	messages, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeImports{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Imports = f
		// No saved locale, so that Accept-Language decides.
		o.Locales = &i18n.Resolver{Bundle: messages}
	})
	body := importBody(strings.Repeat("a", importBodyLimit))
	if len(body) <= importBodyLimit {
		t.Fatalf("body is %d bytes, want over %d", len(body), importBodyLimit)
	}
	other := []byte(`{"status":"` + strings.Repeat("a", 1<<20) + `"}`)

	tests := []struct{ lang, detail string }{
		{"en", "A CSV file can have at most 1,000,000 characters. Split the file and import each part."},
		{"ja", messages.Localizer("ja").ServerErrorsImportTooLargeDetail(1_000_000)},
	}
	for _, tt := range tests {
		title := messages.Localizer(tt.lang).ServerErrorsRequestTooLarge()
		if tt.detail == "" || title == "" {
			t.Fatalf("%s: empty catalog string", tt.lang)
		}
		for _, path := range []string{"/imports/preview", "/imports"} {
			for name, length := range map[string]int64{"declared length": int64(len(body)), "chunked": -1} {
				t.Run(tt.lang+" "+path+" "+name, func(t *testing.T) {
					req := importRequest(path, body, -1, tt.lang)
					if length >= 0 {
						// The body is not read when its declared length is over the limit.
						req = importRequest(path, nil, length, tt.lang)
						req.Body = io.NopCloser(unreadBody{t})
					}
					p := wantProblem(t, e.spec.Serve(t, e.h, req), http.StatusRequestEntityTooLarge, clientapi.ProblemTypeBlank)
					if p.Title != title {
						t.Errorf("title = %q, want %q", p.Title, title)
					}
					if p.Detail == nil || *p.Detail != tt.detail {
						t.Errorf("detail = %v, want %q", p.Detail, tt.detail)
					}
				})
			}
		}
		t.Run(tt.lang+" other operation", func(t *testing.T) {
			res := e.spec.Serve(t, e.h, importRequest("/statuses", other, int64(len(other)), tt.lang))
			p := wantProblem(t, res, http.StatusRequestEntityTooLarge, clientapi.ProblemTypeBlank)
			if p.Title != title {
				t.Errorf("title = %q, want %q", p.Title, title)
			}
			if p.Detail != nil && *p.Detail != "" {
				t.Errorf("detail = %q, want none: only an import names the CSV limit", *p.Detail)
			}
		})
	}
	if len(f.started) != 0 {
		t.Errorf("a refused body started %+v", f.started)
	}
}

// TestOtherOperationsKeepTheJSONBodyLimit checks that a JSON operation other than the imports
// still refuses a body over 1 MiB, with no detail. Media upload's own limit is pinned by
// TestUploadMediaBodyLimit.
func TestOtherOperationsKeepTheJSONBodyLimit(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Imports = &fakeImports{} })
	body := []byte(`{"status":"` + strings.Repeat("a", 1<<20) + `"}`)
	for name, length := range map[string]int64{"declared length": int64(len(body)), "chunked": -1} {
		t.Run(name, func(t *testing.T) {
			res := e.spec.Serve(t, e.h, importRequest("/statuses", body, length, ""))
			p := wantProblem(t, res, http.StatusRequestEntityTooLarge, clientapi.ProblemTypeBlank)
			if p.Detail != nil && *p.Detail != "" {
				t.Errorf("detail = %q, want none", *p.Detail)
			}
		})
	}
}
