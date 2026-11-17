// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeThemes keeps the draft and the publications with the semantics of appinstance.Service: the
// domain validates a set, publishing checks it again, and restoring publishes the set before.
type fakeThemes struct {
	now          time.Time
	sets         []instance.ThemeSet
	publications []instance.ThemeRevision
	by           []string
}

func (f *fakeThemes) id() string {
	return fmt.Sprintf("01890a5d-ac96-774b-bcce-b302099a7%03d", len(f.sets)+len(f.publications)+1)
}

func (f *fakeThemes) Theme(context.Context) (instance.ThemeRevision, bool, error) {
	if len(f.publications) == 0 {
		return instance.ThemeRevision{}, false, nil
	}
	cur := f.publications[len(f.publications)-1]
	return cur, !cur.Empty(), nil
}

func (f *fakeThemes) ThemeDraft(context.Context) (instance.ThemeSet, bool, error) {
	if len(f.sets) == 0 {
		return instance.ThemeSet{}, false, nil
	}
	return f.sets[len(f.sets)-1], true, nil
}

func (f *fakeThemes) SaveThemeDraft(_ context.Context, in appinstance.SaveThemeDraftInput) (instance.ThemeSet, error) {
	tokens, err := instance.NormalizeTheme(in.Tokens)
	if err != nil {
		return instance.ThemeSet{}, err
	}
	set := instance.ThemeSet{ID: f.id(), Tokens: tokens, SavedByUserID: in.SavedByUserID, SavedAt: f.now}
	f.sets = append(f.sets, set)
	f.by = append(f.by, in.SavedByUserID)
	return set, nil
}

func (f *fakeThemes) publish(set instance.ThemeSet, by string) (instance.ThemeRevision, error) {
	if _, err := instance.NormalizeTheme(set.Tokens); err != nil {
		return instance.ThemeRevision{}, err
	}
	rev := instance.ThemeRevision{ThemePublication: instance.ThemePublication{ID: f.id(), SetID: set.ID, PublishedByUserID: by, PublishedAt: f.now}, Set: set}
	f.publications = append(f.publications, rev)
	f.by = append(f.by, by)
	return rev, nil
}

func (f *fakeThemes) PublishTheme(ctx context.Context, by string) (instance.ThemeRevision, error) {
	draft, ok, _ := f.ThemeDraft(ctx)
	if !ok {
		return instance.ThemeRevision{}, appinstance.ErrNotFound
	}
	return f.publish(draft, by)
}

func (f *fakeThemes) RestoreTheme(_ context.Context, by string) (instance.ThemeRevision, error) {
	for i := len(f.publications) - 2; i >= 0; i-- {
		if f.publications[i].SetID != f.publications[len(f.publications)-1].SetID {
			return f.publish(f.publications[i].Set, by)
		}
	}
	return instance.ThemeRevision{}, appinstance.ErrNotFound
}

func newThemeEnv(t *testing.T) (*authEnv, *fakeThemes) {
	t.Helper()
	f := &fakeThemes{now: adminNow}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.Themes = f
		o.ThemeStylesheetPath = func(id string) string { return "/theme/" + id + ".css" }
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	return e, f
}

func TestThemeOperationsNeedAPermission(t *testing.T) {
	e, f := newThemeEnv(t)
	for _, op := range []struct{ method, path, body string }{
		{http.MethodGet, "/admin/theme", ""},
		{http.MethodPut, "/admin/theme/draft", `{"tokens":[]}`},
		{http.MethodGet, "/admin/theme/draft/stylesheet", ""},
		{http.MethodPost, "/admin/theme/publish", ""},
		{http.MethodPost, "/admin/theme/restore", ""},
	} {
		res := e.send(t, op.method, op.path, "bob-admin", op.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
			p.Detail == nil || !strings.Contains(*p.Detail, "manage_settings") {
			t.Errorf("%s %s by bob = %d %+v, want 403 for manage_settings", op.method, op.path, res.StatusCode, p)
		}
	}
	if len(f.by) != 0 {
		t.Error("a refused operation saved or published a theme")
	}
}

// The draft is saved, previewed, and published apart from the theme in effect, and restoring
// publishes the set before.
func TestManageTheme(t *testing.T) {
	e, f := newThemeEnv(t)
	got := decodeJSON[clientapi.AdminTheme](t, e.admin(t, http.MethodGet, "/admin/theme", "admin-read", nil))
	if got.Draft != nil || got.Published != nil {
		t.Errorf("theme before saving = %+v", got)
	}
	for _, path := range []string{"/admin/theme/draft/stylesheet", "/admin/theme/publish", "/admin/theme/restore"} {
		method := http.MethodPost
		if strings.HasSuffix(path, "stylesheet") {
			method = http.MethodGet
		}
		if res := e.send(t, method, path, "admin", ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s before a draft = %d, want 404", path, res.StatusCode)
		}
	}

	res := e.sendJSON(t, http.MethodPut, "/admin/theme/draft", "admin", `{"tokens":[
		{"token":"radius-control","value":"2px"},
		{"token":"color-surface","light":"#FFFDF5","dark":"#0a0a0a"}
	]}`)
	got = decodeJSON[clientapi.AdminTheme](t, res)
	if res.StatusCode != http.StatusOK || got.Draft == nil || got.Published != nil {
		t.Fatalf("save draft = %d %+v", res.StatusCode, got)
	}
	if tokens := got.Draft.Tokens; len(tokens) != 2 || tokens[0].Token != "color-surface" || *tokens[0].Light != "#fffdf5" || tokens[0].Value != nil ||
		tokens[1].Token != "radius-control" || *tokens[1].Value != "2px" || tokens[1].Light != nil || len(got.Draft.StaleTokens) != 0 {
		t.Errorf("draft = %s", mustJSON(t, got.Draft))
	}
	if inst := decodeJSON[clientapi.Instance](t, e.get(t, "/instance", nil)); inst.Theme != nil {
		t.Errorf("Instance.theme of a draft = %+v", inst.Theme)
	}

	// The preview is the draft's stylesheet, never cached.
	res = e.send(t, http.MethodGet, "/admin/theme/draft/stylesheet", "admin-read", "")
	css, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/css") ||
		res.Header.Get("Cache-Control") != "no-store" || !strings.Contains(string(css), "--color-surface: light-dark(#fffdf5, #0a0a0a);") {
		t.Errorf("preview = %d %v\n%s", res.StatusCode, res.Header, css)
	}

	got = decodeJSON[clientapi.AdminTheme](t, e.send(t, http.MethodPost, "/admin/theme/publish", "admin", ""))
	if got.Published == nil || got.Published.StylesheetUrl == nil ||
		*got.Published.StylesheetUrl != "https://social.example/theme/"+got.Published.Revision+".css" || got.Published.Set.Id != got.Draft.Id {
		t.Fatalf("publish = %s", mustJSON(t, got))
	}
	first := got.Published.Revision
	if by := f.by[len(f.by)-1]; by != userOf(aliceAccountID) {
		t.Errorf("published by %q", by)
	}
	inst := decodeJSON[clientapi.Instance](t, e.get(t, "/instance", nil))
	if inst.Theme == nil || inst.Theme.Revision != first || inst.Theme.StylesheetUrl != *got.Published.StylesheetUrl || len(inst.Theme.Tokens) != 2 {
		t.Errorf("Instance.theme = %+v", inst.Theme)
	}

	// An empty draft, published, resets the theme: the revision stays, the stylesheet goes.
	e.sendJSON(t, http.MethodPut, "/admin/theme/draft", "admin", `{"tokens":[]}`)
	got = decodeJSON[clientapi.AdminTheme](t, e.send(t, http.MethodPost, "/admin/theme/publish", "admin", ""))
	if got.Published == nil || got.Published.Revision == first || got.Published.StylesheetUrl != nil {
		t.Errorf("reset = %s", mustJSON(t, got))
	}
	if inst := decodeJSON[clientapi.Instance](t, e.get(t, "/instance", nil)); inst.Theme != nil {
		t.Errorf("Instance.theme after a reset = %+v", inst.Theme)
	}

	// Restoring publishes the first set again, under a new revision; the draft stays.
	got = decodeJSON[clientapi.AdminTheme](t, e.send(t, http.MethodPost, "/admin/theme/restore", "admin", ""))
	if got.Published == nil || got.Published.Revision == first || len(got.Published.Set.Tokens) != 2 || got.Draft == nil || len(got.Draft.Tokens) != 0 {
		t.Errorf("restore = %s", mustJSON(t, got))
	}
}

func TestThemeShowsStaleOverrides(t *testing.T) {
	e, f := newThemeEnv(t)
	tokens, stale, err := instance.ReadThemeSet([]byte(`{"version":1,"tokens":[{"token":"elevation-card","value":"none"},{"token":"radius-control","value":"2px"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	f.sets = append(f.sets, instance.ThemeSet{ID: f.id(), Tokens: tokens, Stale: stale, SavedAt: adminNow})
	got := decodeJSON[clientapi.AdminTheme](t, e.admin(t, http.MethodGet, "/admin/theme", "admin-read", nil))
	if got.Draft == nil || len(got.Draft.Tokens) != 1 || len(got.Draft.StaleTokens) != 1 ||
		got.Draft.StaleTokens[0].Token != "elevation-card" || got.Draft.StaleTokens[0].Reason != "unknown_token" || *got.Draft.StaleTokens[0].Value != "none" {
		t.Errorf("draft = %s", mustJSON(t, got.Draft))
	}
}

func TestThemeProblems(t *testing.T) {
	e, f := newThemeEnv(t)
	res := e.sendJSON(t, http.MethodPut, "/admin/theme/draft", "admin", `{"tokens":[
		{"token":"palette-blue-600","value":"#000000"},
		{"token":"radius-control","value":"1px; } body { display: none"},
		{"token":"text-body","value":"4rem"}
	]}`)
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", res.StatusCode)
	}
	p := decodeJSON[clientapi.ThemeProblem](t, res)
	if p.Type != clientapi.ProblemTypeInvalidTheme || p.Title == "" || len(themeErrors(p)) < 3 {
		t.Fatalf("problem = %s", mustJSON(t, p))
	}
	codes := []clientapi.ThemeErrorCode{themeErrors(p)[0].Code, themeErrors(p)[1].Code, themeErrors(p)[2].Code}
	if codes[0] != "unknown_token" || codes[1] != "invalid_value" || codes[2] != "out_of_range" ||
		*themeErrors(p)[2].Min != 0.625 || *themeErrors(p)[2].Max != 3 {
		t.Errorf("errors = %s", mustJSON(t, themeErrors(p)))
	}

	res = e.sendJSON(t, http.MethodPut, "/admin/theme/draft", "admin", `{"tokens":[{"token":"color-text","light":"#eeeeee","dark":"#f9fafb"}]}`)
	p = decodeJSON[clientapi.ThemeProblem](t, res)
	if res.StatusCode != http.StatusUnprocessableEntity || len(themeErrors(p)) < 3 {
		t.Fatalf("low contrast = %d %s", res.StatusCode, mustJSON(t, p))
	}
	e0 := themeErrors(p)[0]
	if e0.Code != "low_contrast" || *e0.Token != "color-text" || *e0.Background != "color-surface" || *e0.Scheme != "light" ||
		*e0.Minimum != 4.5 || *e0.Ratio >= 4.5 {
		t.Errorf("contrast error = %s", mustJSON(t, e0))
	}
	if len(f.sets) != 0 {
		t.Error("an invalid theme was saved")
	}

	// A draft that no longer meets its contrast pairs is not published.
	f.sets = append(f.sets, instance.ThemeSet{ID: f.id(), Tokens: []instance.ThemeTokenValue{{Token: "color-text", Light: "#ffffff", Dark: "#ffffff"}}})
	res = e.send(t, http.MethodPost, "/admin/theme/publish", "admin", "")
	if p := decodeJSON[clientapi.ThemeProblem](t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeInvalidTheme {
		t.Errorf("publish of a failing draft = %d %s", res.StatusCode, mustJSON(t, p))
	}
	if len(f.publications) != 0 {
		t.Error("a failing draft was published")
	}
}

// themeErrors returns the reasons of an invalid-theme problem, none when it has none.
func themeErrors(p clientapi.ThemeProblem) []clientapi.ThemeError {
	if p.ThemeErrors == nil {
		return nil
	}
	return *p.ThemeErrors
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
