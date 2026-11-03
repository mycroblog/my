// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// The domain block CSV of fakeModeration: the export of the actions in effect, a preview that
// imports every valid row, and an import that issues every kind of each block.

func (f *fakeModeration) ExportDomainBlocks(_ context.Context, w io.Writer) error {
	var actions []moderation.DomainAction
	for _, r := range f.actions {
		if r.State(f.now) == moderation.ActionActive {
			actions = append(actions, r.Action)
		}
	}
	return moderation.WriteDomainBlockCSV(w, moderation.DomainBlocksOf(actions))
}

func (f *fakeModeration) PreviewDomainBlocks(_ context.Context, text string) ([]appmoderation.DomainBlockPreviewRow, error) {
	rows, err := moderation.ParseDomainBlockCSV(text)
	if err != nil {
		return nil, err
	}
	out := make([]appmoderation.DomainBlockPreviewRow, len(rows))
	for i, r := range rows {
		out[i] = appmoderation.DomainBlockPreviewRow{Line: r.Line, Written: r.Domain, Block: r.DomainBlock, Status: appmoderation.DomainBlockImportable, Issue: r.Kinds()}
		if r.Invalid {
			out[i].Status, out[i].Issue = appmoderation.DomainBlockInvalid, nil
		}
	}
	return out, nil
}

func (f *fakeModeration) ImportDomainBlocks(ctx context.Context, in appmoderation.DomainBlockImportInput) (appmoderation.DomainBlockImportResult, error) {
	var res appmoderation.DomainBlockImportResult
	if len(in.Blocks) > moderation.MaxDomainBlockImport {
		return res, moderation.ErrTooManyDomainBlocks
	}
	if err := moderation.CheckDomainActionTexts("", in.PrivateNote); err != nil {
		return res, err
	}
	for _, b := range in.Blocks {
		kinds := b.Kinds()
		if len(kinds) == 0 {
			res.Skipped = append(res.Skipped, appmoderation.DomainBlockImportSkipped{Domain: b.Domain, Reason: appmoderation.DomainBlockNoAction})
		}
		for _, k := range kinds {
			a, err := f.IssueDomainAction(ctx, appmoderation.DomainActionInput{
				Domain: b.Domain, Kind: k, PublicComment: b.PublicComment, PrivateNote: in.PrivateNote,
				Obfuscate: b.Obfuscate, IssuedByUserID: in.IssuedByUserID,
			})
			if err != nil {
				return res, err
			}
			res.Issued = append(res.Issued, a)
		}
	}
	return res, nil
}

func TestDomainBlockCSVNeedsManageDomains(t *testing.T) {
	e, f := newModerationEnv(t)
	for _, op := range []struct{ method, path, body string }{
		{http.MethodGet, "/admin/domain_actions/export", ""},
		{http.MethodPost, "/admin/domain_actions/import/preview", `{"csv":"bad.example"}`},
		{http.MethodPost, "/admin/domain_actions/import", `{"blocks":[{"domain":"bad.example","severity":"suspend","reject_media":false,"reject_reports":false,"obfuscate":false}]}`},
	} {
		res := e.send(t, op.method, op.path, "bob-admin", op.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission {
			t.Errorf("%s %s by bob = %d %+v, want 403", op.method, op.path, res.StatusCode, p)
		}
	}
	if len(f.issued) != 0 {
		t.Error("a refused import issued an action")
	}
	res := e.send(t, http.MethodPost, "/admin/domain_actions/import/preview", "admin-read", `{"csv":"bad.example"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
		t.Errorf("preview with admin:read = %d %+v, want 403 insufficient-scope", res.StatusCode, p)
	}
}

func TestExportDomainBlocks(t *testing.T) {
	e, f := newModerationEnv(t)
	ctx := t.Context()
	for _, in := range []appmoderation.DomainActionInput{
		{Domain: "bad.example", Kind: moderation.DomainSuspend, PublicComment: "spam, and more"},
		{Domain: "bad.example", Kind: moderation.DomainRejectMedia},
		{Domain: "noisy.example", Kind: moderation.DomainLimit, Obfuscate: true},
	} {
		if _, err := f.IssueDomainAction(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	res := e.send(t, http.MethodGet, "/admin/domain_actions/export", "admin-read", "")
	body, _ := io.ReadAll(res.Body)
	want := "#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate\n" +
		"bad.example,suspend,true,false,\"spam, and more\",false\n" +
		"noisy.example,silence,false,false,,true\n"
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/csv" || res.Header.Get("Cache-Control") != "no-store" ||
		!strings.Contains(res.Header.Get("Content-Disposition"), "domain_blocks.csv") || string(body) != want {
		t.Errorf("export = %d %v\n%s", res.StatusCode, res.Header, body)
	}
}

func TestPreviewAndImportDomainBlocks(t *testing.T) {
	e, f := newModerationEnv(t)
	csv := "#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate\n" +
		"bad.example,suspend,false,true,spam,true\n" +
		"odd.example,ban,false,false,,false\n"
	res := e.sendJSON(t, http.MethodPost, "/admin/domain_actions/import/preview", "admin", `{"csv":`+jsonString(csv)+`}`)
	preview := decodeJSON[clientapi.DomainBlockImportPreview](t, res)
	if res.StatusCode != http.StatusOK || len(preview.Rows) != 2 {
		t.Fatalf("preview = %d %+v", res.StatusCode, preview)
	}
	if r := preview.Rows[0]; r.Line != 2 || r.Status != clientapi.DomainBlockImportStatusImportable || len(r.Issue) != 2 ||
		r.Block.PublicComment == nil || !r.Block.Obfuscate || !r.Block.RejectReports {
		t.Errorf("row 1 = %+v", r)
	}
	if r := preview.Rows[1]; r.Line != 3 || r.Status != clientapi.DomainBlockImportStatusInvalid {
		t.Errorf("row 2 = %+v", r)
	}
	if len(f.issued) != 0 {
		t.Error("the preview issued an action")
	}

	res = e.sendJSON(t, http.MethodPost, "/admin/domain_actions/import", "admin",
		`{"private_note":"from mastodon.example","blocks":[{"domain":"bad.example","severity":"suspend","reject_media":false,"reject_reports":true,"public_comment":"spam","obfuscate":true},`+
			`{"domain":"calm.example","severity":"noop","reject_media":false,"reject_reports":false,"obfuscate":false}]}`)
	got := decodeJSON[clientapi.DomainBlockImportResult](t, res)
	if res.StatusCode != http.StatusOK || len(got.Issued) != 2 || got.Issued[0].Kind != clientapi.AdminDomainActionKindSuspend ||
		got.Issued[1].Kind != clientapi.AdminDomainActionKindRejectReports || got.Issued[0].State != clientapi.ModerationActionStateActive ||
		len(got.Skipped) != 1 || got.Skipped[0].Reason != clientapi.DomainBlockImportStatusNoAction {
		t.Fatalf("import = %d %+v", res.StatusCode, got)
	}
	if in := f.issued[0]; in.IssuedByUserID != userOf(aliceAccountID) || in.PrivateNote != "from mastodon.example" || !in.Obfuscate || in.PublicComment != "spam" {
		t.Errorf("issued = %+v", in)
	}
}

func TestDomainBlockCSVRejectsInvalidInput(t *testing.T) {
	e, f := newModerationEnv(t)
	for path, body := range map[string]string{
		"/admin/domain_actions/import/preview": `{"csv":"a.example,\"b\"c\n\"unterminated"}`,
		"/admin/domain_actions/import":         `{"private_note":"\u0000","blocks":[]}`,
	} {
		res := e.sendJSON(t, http.MethodPost, path, "admin", body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s %s = %d %+v, want 422", path, body, res.StatusCode, p)
		}
	}
	rows := strings.Repeat("x.example\n", moderation.MaxDomainBlockImport+1)
	res := e.sendJSON(t, http.MethodPost, "/admin/domain_actions/import/preview", "admin", `{"csv":`+jsonString(rows)+`}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, "csv") {
		t.Errorf("too many rows = %d %+v", res.StatusCode, p)
	}
	if len(f.issued) != 0 {
		t.Error("a refused import issued an action")
	}
}

// jsonString returns s as a JSON string.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
