// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeThemes keeps the theme with the semantics of appinstance.Service: the domain validates it.
type fakeThemes struct {
	now     time.Time
	current instance.ThemeRevision
	sets    []appinstance.SetThemeInput
}

func (f *fakeThemes) Theme(context.Context) (instance.ThemeRevision, bool, error) {
	return f.current, f.current.ID != "" && !f.current.Empty(), nil
}

func (f *fakeThemes) SetTheme(_ context.Context, in appinstance.SetThemeInput) (instance.ThemeRevision, error) {
	tokens, err := instance.NormalizeTheme(in.Tokens, in.CustomCSS)
	if err != nil {
		return instance.ThemeRevision{}, err
	}
	f.sets = append(f.sets, in)
	f.current = instance.ThemeRevision{
		ID: fmt.Sprintf("01890a5d-ac96-774b-bcce-b302099a7%03d", len(f.sets)), Tokens: tokens, CustomCSS: in.CustomCSS,
		RevisedByUserID: in.RevisedByUserID, RevisedAt: f.now,
	}
	return f.current, nil
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
	for _, op := range []struct{ method, body string }{{http.MethodGet, ""}, {http.MethodPut, `{"tokens":[],"custom_css":""}`}} {
		res := e.send(t, op.method, "/admin/theme", "bob-admin", op.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
			p.Detail == nil || !strings.Contains(*p.Detail, "manage_settings") {
			t.Errorf("%s by bob = %d %+v, want 403 for manage_settings", op.method, res.StatusCode, p)
		}
	}
	if len(f.sets) != 0 {
		t.Error("a refused operation saved a theme")
	}
}

func TestManageTheme(t *testing.T) {
	e, f := newThemeEnv(t)
	got := decodeJSON[clientapi.AdminTheme](t, e.admin(t, http.MethodGet, "/admin/theme", "admin-read", nil))
	if got.Revision != nil || got.StylesheetUrl != nil || len(got.Tokens) != 0 || got.CustomCss != "" {
		t.Errorf("theme before saving = %+v", got)
	}
	if inst := decodeJSON[clientapi.Instance](t, e.get(t, "/instance", nil)); inst.Theme != nil {
		t.Errorf("Instance.theme without a theme = %+v", inst.Theme)
	}

	res := e.sendJSON(t, http.MethodPut, "/admin/theme", "admin", `{"tokens":[
		{"token":"radius-control","value":"2px"},
		{"token":"color-surface","light":"#FFFDF5","dark":"#0a0a0a"}
	],"custom_css":".brand { color: red }"}`)
	got = decodeJSON[clientapi.AdminTheme](t, res)
	if res.StatusCode != http.StatusOK || got.Revision == nil || got.StylesheetUrl == nil ||
		*got.StylesheetUrl != "https://social.example/theme/"+*got.Revision+".css" || got.CustomCss != ".brand { color: red }" {
		t.Fatalf("set = %d %+v", res.StatusCode, got)
	}
	if len(got.Tokens) != 2 || got.Tokens[0].Token != "color-surface" || *got.Tokens[0].Light != "#fffdf5" || got.Tokens[0].Value != nil ||
		got.Tokens[1].Token != "radius-control" || *got.Tokens[1].Value != "2px" || got.Tokens[1].Light != nil {
		t.Errorf("tokens = %s", mustJSON(t, got.Tokens))
	}
	if by := f.sets[0].RevisedByUserID; by != userOf(aliceAccountID) {
		t.Errorf("revised by %q", by)
	}
	inst := decodeJSON[clientapi.Instance](t, e.get(t, "/instance", nil))
	if inst.Theme == nil || inst.Theme.Revision != *got.Revision || inst.Theme.StylesheetUrl != *got.StylesheetUrl || len(inst.Theme.Tokens) != 2 {
		t.Errorf("Instance.theme = %+v", inst.Theme)
	}

	// An empty theme resets it: the revision stays, the stylesheet goes.
	got = decodeJSON[clientapi.AdminTheme](t, e.sendJSON(t, http.MethodPut, "/admin/theme", "admin", `{"tokens":[],"custom_css":""}`))
	if got.Revision == nil || got.StylesheetUrl != nil {
		t.Errorf("reset = %+v", got)
	}
	if inst := decodeJSON[clientapi.Instance](t, e.get(t, "/instance", nil)); inst.Theme != nil {
		t.Errorf("Instance.theme after a reset = %+v", inst.Theme)
	}
}

func TestThemeProblems(t *testing.T) {
	e, f := newThemeEnv(t)
	res := e.sendJSON(t, http.MethodPut, "/admin/theme", "admin", `{"tokens":[
		{"token":"palette-blue-600","value":"#000000"},
		{"token":"radius-control","value":"1px; } body { display: none"},
		{"token":"text-body","value":"4rem"}
	],"custom_css":""}`)
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

	res = e.sendJSON(t, http.MethodPut, "/admin/theme", "admin", `{"tokens":[{"token":"color-text","light":"#eeeeee","dark":"#f9fafb"}],"custom_css":""}`)
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
