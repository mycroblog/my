// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeSignupBlocks keeps sign-up blocks in memory, newest last, with the validation of the
// moderation use cases reduced to what the handlers map to responses.
type fakeSignupBlocks struct {
	blocks []appmoderation.SignupBlockView
	added  []appmoderation.SignupBlockInput
	lifted []string
	// imported are the imports that got past the checks.
	imported []appmoderation.SignupBlockImportInput
}

func (f *fakeSignupBlocks) SignupBlocks(_ context.Context, sf appmoderation.SignupBlockFilter, limit int) ([]appmoderation.SignupBlockView, error) {
	if sf.Kind != "" && !sf.Kind.Valid() {
		return nil, appmoderation.ErrInvalidQuery
	}
	var out []appmoderation.SignupBlockView
	for _, v := range slices.Backward(f.blocks) {
		if (sf.Kind == "" || v.Block.Kind == sf.Kind) && (sf.Before == "" || v.Block.ID < sf.Before) && len(out) < limit {
			out = append(out, v)
		}
	}
	return out, nil
}

func (f *fakeSignupBlocks) SignupBlock(_ context.Context, id string) (appmoderation.SignupBlockView, error) {
	for _, v := range f.blocks {
		if v.Block.ID == id {
			return v, nil
		}
	}
	return appmoderation.SignupBlockView{}, appmoderation.ErrNotFound
}

func (f *fakeSignupBlocks) AddSignupBlock(_ context.Context, in appmoderation.SignupBlockInput) (moderation.SignupBlock, error) {
	if len([]rune(in.Comment)) > moderation.MaxSignupBlockComment {
		return moderation.SignupBlock{}, fmt.Errorf("%w: %w", moderation.ErrInvalidSignupBlock, &moderation.TextError{Field: "comment", Max: moderation.MaxSignupBlockComment})
	}
	if in.Value == "bad" {
		return moderation.SignupBlock{}, fmt.Errorf("%w: bad value", moderation.ErrInvalidSignupBlock)
	}
	f.added = append(f.added, in)
	b := moderation.SignupBlock{
		ID: fmt.Sprintf("01890a5d-ac96-774b-bcce-b302099a80%02d", len(f.blocks)), Kind: in.Kind, Value: in.Value, Severity: in.Severity,
		Comment: in.Comment, CreatedByUserID: in.CreatedByUserID, CreatedAt: registrationsNow,
	}
	if in.Duration > 0 {
		at := registrationsNow.Add(in.Duration)
		b.ExpiresAt = &at
	}
	f.blocks = append(f.blocks, appmoderation.SignupBlockView{SignupBlockRecord: moderation.SignupBlockRecord{Block: b}, State: moderation.ActionActive})
	return b, nil
}

func (f *fakeSignupBlocks) LiftSignupBlock(_ context.Context, id, by string) error {
	for i, v := range f.blocks {
		if v.Block.ID == id {
			if v.Lift == nil {
				f.blocks[i].Lift = &moderation.SignupBlockLift{BlockID: id, LiftedByUserID: by, LiftedAt: registrationsNow.Add(time.Minute)}
				f.blocks[i].State = moderation.ActionLifted
				f.lifted = append(f.lifted, by)
			}
			return nil
		}
	}
	return appmoderation.ErrNotFound
}

// ImportSignupBlocks skips the lines "bad" as invalid and adds the others.
func (f *fakeSignupBlocks) ImportSignupBlocks(_ context.Context, in appmoderation.SignupBlockImportInput) (appmoderation.SignupBlockImportResult, error) {
	if len([]rune(in.Comment)) > moderation.MaxSignupBlockComment {
		return appmoderation.SignupBlockImportResult{}, fmt.Errorf("%w: %w", moderation.ErrInvalidSignupBlock, &moderation.TextError{Field: "comment", Max: moderation.MaxSignupBlockComment})
	}
	entries := moderation.ParseSignupBlockList(in.Text)
	if len(entries) > moderation.MaxSignupBlockImport {
		return appmoderation.SignupBlockImportResult{}, appmoderation.ErrTooManySignupBlocks
	}
	f.imported = append(f.imported, in)
	res := appmoderation.SignupBlockImportResult{Skipped: []appmoderation.SignupBlockImportSkipped{}}
	for _, e := range entries {
		if e.Value == "bad" {
			res.Skipped = append(res.Skipped, appmoderation.SignupBlockImportSkipped{Line: e.Line, Value: e.Value, Reason: moderation.SignupBlockSkipInvalid})
		} else {
			res.Added++
		}
	}
	return res, nil
}

func newSignupBlocksEnv(t *testing.T, perms ...identity.Permission) (*authEnv, *fakeSignupBlocks) {
	t.Helper()
	blocks := &fakeSignupBlocks{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): perms}
		o.SignupBlocks = blocks
	})
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	e.auth.tokens["admin-write"] = bearer("admin:write", aliceAccountID)
	return e, blocks
}

func TestSignupBlocksAPI(t *testing.T) {
	e, blocks := newSignupBlocksEnv(t, identity.PermManageAccounts)
	res := e.adminBody(t, http.MethodPost, "/admin/signup_blocks", "admin-write",
		`{"kind":"email_domain","value":"spam.example","severity":"reject","comment":"wave","expires_in":60}`)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("POST /admin/signup_blocks = %d", res.StatusCode)
	}
	b := decodeJSON[clientapi.AdminSignupBlock](t, res)
	if b.Kind != clientapi.AdminSignupBlockKindEmailDomain || b.Value != "spam.example" || b.Severity != clientapi.AdminSignupBlockSeverityReject ||
		b.Comment == nil || *b.Comment != "wave" || b.ExpiresAt == nil || b.State != clientapi.AdminSignupBlockStateActive || b.LiftedAt != nil {
		t.Errorf("created block = %+v", b)
	}
	if len(blocks.added) != 1 || blocks.added[0].CreatedByUserID != userOf(aliceAccountID) || blocks.added[0].Duration != time.Minute {
		t.Errorf("added = %+v, want one by alice for a minute", blocks.added)
	}
	res = e.adminBody(t, http.MethodPost, "/admin/signup_blocks", "admin-write", `{"kind":"ip_range","value":"192.0.2.0/24","severity":"require_approval"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST a second block = %d", res.StatusCode)
	}
	second := decodeJSON[clientapi.AdminSignupBlock](t, res)

	for name, body := range map[string]string{
		"kind":           `{"kind":"email","value":"x","severity":"reject"}`,
		"severity":       `{"kind":"ip_range","value":"192.0.2.0/24","severity":"maybe"}`,
		"value":          `{"kind":"ip_range","value":"bad","severity":"reject"}`,
		"expires_in":     `{"kind":"ip_range","value":"192.0.2.0/24","severity":"reject","expires_in":0}`,
		"expires_in:max": `{"kind":"ip_range","value":"192.0.2.0/24","severity":"reject","expires_in":10000000000000,"x":"invalid-on-purpose"}`,
		"comment":        `{"kind":"ip_range","value":"192.0.2.0/24","severity":"reject","comment":"` + strings.Repeat("x", 1001) + `"}`,
	} {
		field, _, _ := strings.Cut(name, ":")
		res := e.adminBody(t, http.MethodPost, "/admin/signup_blocks", "admin-write", body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, field) {
			t.Errorf("%s: POST = %d %+v, want 422 naming it", name, res.StatusCode, p)
		}
	}

	res = e.admin(t, http.MethodGet, "/admin/signup_blocks?limit=1", "admin-read", nil)
	page := decodeJSON[clientapi.AdminSignupBlockPage](t, res)
	if res.StatusCode != http.StatusOK || len(page.Items) != 1 || page.Items[0].Id != second.Id || page.NextCursor == nil {
		t.Fatalf("first page = %d %+v", res.StatusCode, page)
	}
	res = e.admin(t, http.MethodGet, "/admin/signup_blocks?limit=1&cursor="+*page.NextCursor, "admin-read", nil)
	page = decodeJSON[clientapi.AdminSignupBlockPage](t, res)
	if res.StatusCode != http.StatusOK || len(page.Items) != 1 || page.Items[0].Id != b.Id || page.NextCursor != nil {
		t.Fatalf("second page = %d %+v", res.StatusCode, page)
	}
	if res := e.admin(t, http.MethodGet, "/admin/signup_blocks?kind=email", "admin-read", nil); res.StatusCode != http.StatusBadRequest {
		t.Errorf("GET with an unknown kind = %d, want 400", res.StatusCode)
	}

	res = e.admin(t, http.MethodPost, "/admin/signup_blocks/"+b.Id+"/lift", "admin-write", nil)
	if lifted := decodeJSON[clientapi.AdminSignupBlock](t, res); res.StatusCode != http.StatusOK || lifted.State != clientapi.AdminSignupBlockStateLifted || lifted.LiftedAt == nil {
		t.Errorf("lift = %d %+v", res.StatusCode, lifted)
	}
	if len(blocks.lifted) != 1 || blocks.lifted[0] != userOf(aliceAccountID) {
		t.Errorf("lifted by %v, want alice", blocks.lifted)
	}
	if res := e.admin(t, http.MethodGet, "/admin/signup_blocks/"+b.Id, "admin-read", nil); res.StatusCode != http.StatusOK {
		t.Errorf("GET a block = %d", res.StatusCode)
	}
	for _, path := range []string{"/admin/signup_blocks/01890a5d-ac96-774b-bcce-b302099a8099", "/admin/signup_blocks/01890a5d-ac96-774b-bcce-b302099a8099/lift"} {
		method, token := http.MethodGet, "admin-read"
		if strings.HasSuffix(path, "/lift") {
			method, token = http.MethodPost, "admin-write"
		}
		if res := e.admin(t, method, path, token, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", method, path, res.StatusCode)
		}
	}
}

func TestImportSignupBlocksAPI(t *testing.T) {
	e, blocks := newSignupBlocksEnv(t, identity.PermManageAccounts)
	res := e.adminBody(t, http.MethodPost, "/admin/signup_blocks/import", "admin-write",
		`{"kind":"email_domain","text":"# list\nspam.example\nbad\nmail.example\n","severity":"reject","comment":"disposable","expires_in":3600}`)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("POST /admin/signup_blocks/import = %d", res.StatusCode)
	}
	got := decodeJSON[clientapi.SignupBlockImportResult](t, res)
	want := []clientapi.SignupBlockImportSkipped{{Line: 3, Value: "bad", Reason: clientapi.SignupBlockImportSkippedReasonInvalid}}
	if got.Added != 2 || !slices.Equal(got.Skipped, want) {
		t.Errorf("import = %+v, want 2 added and line 3 invalid", got)
	}
	if len(blocks.imported) != 1 || blocks.imported[0].CreatedByUserID != userOf(aliceAccountID) || blocks.imported[0].Duration != time.Hour ||
		blocks.imported[0].Comment != "disposable" || blocks.imported[0].Severity != moderation.SignupReject {
		t.Errorf("imported = %+v, want one by alice for an hour", blocks.imported)
	}
	// Nothing skipped is an empty list, not null.
	res = e.adminBody(t, http.MethodPost, "/admin/signup_blocks/import", "admin-write", `{"kind":"ip_range","text":"192.0.2.0/24","severity":"require_approval"}`)
	if got := decodeJSON[clientapi.SignupBlockImportResult](t, res); res.StatusCode != http.StatusOK || got.Added != 1 || got.Skipped == nil {
		t.Errorf("import of one range = %d %+v", res.StatusCode, got)
	}

	for name, body := range map[string]string{
		"kind":           `{"kind":"email","text":"x","severity":"reject"}`,
		"severity":       `{"kind":"ip_range","text":"192.0.2.0/24","severity":"maybe"}`,
		"expires_in":     `{"kind":"ip_range","text":"192.0.2.0/24","severity":"reject","expires_in":0}`,
		"expires_in:max": `{"kind":"ip_range","text":"192.0.2.0/24","severity":"reject","expires_in":10000000000000,"x":"invalid-on-purpose"}`,
		"comment":        `{"kind":"ip_range","text":"192.0.2.0/24","severity":"reject","comment":"` + strings.Repeat("x", 1001) + `"}`,
		"text":           `{"kind":"email_domain","text":"` + strings.Repeat(`a.example\n`, moderation.MaxSignupBlockImport+1) + `","severity":"reject"}`,
	} {
		field, _, _ := strings.Cut(name, ":")
		res := e.adminBody(t, http.MethodPost, "/admin/signup_blocks/import", "admin-write", body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, field) {
			t.Errorf("%s: POST import = %d %+v, want 422 naming it", name, res.StatusCode, p)
		}
	}
	if len(blocks.imported) != 2 {
		t.Errorf("a refused import was run: %d imports", len(blocks.imported))
	}
}

func TestSignupBlocksNeedManageAccounts(t *testing.T) {
	e, blocks := newSignupBlocksEnv(t, identity.PermManageReports)
	for _, c := range []struct{ method, path, token, body string }{
		{http.MethodGet, "/admin/signup_blocks", "admin-read", ""},
		{http.MethodPost, "/admin/signup_blocks", "admin-write", `{"kind":"ip_range","value":"192.0.2.0/24","severity":"reject"}`},
		{http.MethodGet, "/admin/signup_blocks/01890a5d-ac96-774b-bcce-b302099a8057", "admin-read", ""},
		{http.MethodPost, "/admin/signup_blocks/01890a5d-ac96-774b-bcce-b302099a8057/lift", "admin-write", ""},
		{http.MethodPost, "/admin/signup_blocks/import", "admin-write", `{"kind":"email_domain","text":"spam.example","severity":"reject"}`},
	} {
		if res := e.adminBody(t, c.method, c.path, c.token, c.body); res.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s without manage_accounts = %d, want 403", c.method, c.path, res.StatusCode)
		}
	}
	if len(blocks.added) != 0 || len(blocks.lifted) != 0 || len(blocks.imported) != 0 {
		t.Errorf("a refused request changed the blocks: %+v %+v %+v", blocks.added, blocks.lifted, blocks.imported)
	}
}
