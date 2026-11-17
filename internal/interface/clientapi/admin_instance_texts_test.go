// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeTexts keeps instance texts with the semantics of appinstance.Service, on fakeInstance so
// that GET /instance reads what the admin operations write.
type fakeTexts struct {
	inst *fakeInstance
	now  time.Time
	seq  int
	sets []appinstance.SetTextInput
}

func (f *fakeTexts) Texts(context.Context) ([]instance.Text, error) {
	var out []instance.Text
	for _, d := range instance.TextDefinitions() {
		if body := f.inst.texts[d.Key]; body != "" {
			out = append(out, instance.Text{Key: d.Key, Body: body, RevisedAt: f.now})
		}
	}
	return out, nil
}

func (f *fakeTexts) SetText(_ context.Context, in appinstance.SetTextInput) (instance.TextRevision, error) {
	def, err := instance.LookupText(in.Key)
	if err != nil {
		return instance.TextRevision{}, err
	}
	if err := def.Validate(in.Body); err != nil {
		return instance.TextRevision{}, err
	}
	f.sets = append(f.sets, in)
	f.seq++
	if f.inst.texts == nil {
		f.inst.texts = map[instance.TextKey]string{}
	}
	f.inst.texts[in.Key] = in.Body
	return instance.TextRevision{
		ID: fmt.Sprintf("01890a5d-ac96-774b-bcce-b302099a7%03d", f.seq), Key: in.Key, Body: in.Body,
		RevisedByUserID: in.RevisedByUserID, RevisedAt: f.now,
	}, nil
}

// newTextsEnv is newAuthEnv with the tokens of newAdminEnv, the instance texts, and the paths of
// the document pages: alice holds every permission and bob none.
func newTextsEnv(t *testing.T) (*authEnv, *fakeTexts) {
	t.Helper()
	f := &fakeTexts{inst: &fakeInstance{}, now: adminNow}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.Instance, o.InstanceTexts = f.inst, f
		o.TermsPath, o.PrivacyPolicyPath = "/about/terms", "/about/privacy"
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	return e, f
}

func TestInstanceTextOperationsNeedAPermission(t *testing.T) {
	e, f := newTextsEnv(t)
	for _, op := range []struct{ method, path, body string }{
		{http.MethodGet, "/admin/instance_texts", ""},
		{http.MethodPut, "/admin/instance_texts/terms", `{"body":"Be kind"}`},
	} {
		res := e.send(t, op.method, op.path, "bob-admin", op.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
			p.Detail == nil || !strings.Contains(*p.Detail, "manage_settings") {
			t.Errorf("%s %s by bob = %d %+v, want 403 for manage_settings", op.method, op.path, res.StatusCode, p)
		}
	}
	res := e.sendJSON(t, http.MethodPut, "/admin/instance_texts/terms", "admin-read", `{"body":"Be kind"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
		t.Errorf("set with admin:read = %d %+v, want 403 insufficient-scope", res.StatusCode, p)
	}
	if len(f.sets) != 0 {
		t.Error("a refused operation set a text")
	}
}

func TestManageInstanceTexts(t *testing.T) {
	e, f := newTextsEnv(t)
	instanceLinks := func(header map[string]string) (terms, privacy *string) {
		t.Helper()
		res := e.get(t, "/instance", header)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET /instance = %d", res.StatusCode)
		}
		got := decodeJSON[clientapi.Instance](t, res)
		return got.TermsUrl, got.PrivacyPolicyUrl
	}
	if terms, privacy := instanceLinks(nil); terms != nil || privacy != nil {
		t.Errorf("links without documents = %v %v, want none", terms, privacy)
	}

	res := e.sendJSON(t, http.MethodPut, "/admin/instance_texts/terms", "admin", `{"body":"# 利用規約\n\n親切に。"}`)
	got := decodeJSON[clientapi.AdminInstanceText](t, res)
	want := clientapi.AdminInstanceText{Key: clientapi.InstanceTextKeyTerms, Body: "# 利用規約\n\n親切に。", RevisedAt: adminNow}
	if res.StatusCode != http.StatusOK || !reflect.DeepEqual(got, want) {
		t.Fatalf("set = %d %+v, want %+v", res.StatusCode, got, want)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if by := f.sets[0].RevisedByUserID; by != userOf(aliceAccountID) {
		t.Errorf("revised by %q, want alice's user", by)
	}
	if got := decodeJSON[clientapi.AdminInstanceText](t, e.sendJSON(t, http.MethodPut, "/admin/instance_texts/name", "admin", `{"body":"Example Social"}`)); got.Body != "Example Social" {
		t.Errorf("set name = %+v", got)
	}

	res = e.admin(t, http.MethodGet, "/admin/instance_texts", "admin-read", nil)
	list := decodeJSON[clientapi.AdminInstanceTextList](t, res)
	wantList := []clientapi.AdminInstanceText{
		{Key: clientapi.InstanceTextKeyName, Body: "Example Social", RevisedAt: adminNow},
		want,
	}
	if res.StatusCode != http.StatusOK || !reflect.DeepEqual(list.Items, wantList) {
		t.Errorf("list = %d %+v, want %+v", res.StatusCode, list.Items, wantList)
	}

	// GET /instance links to the terms for readers of every locale.
	for _, lang := range []string{"ja", "en"} {
		if terms, privacy := instanceLinks(map[string]string{"Accept-Language": lang}); terms == nil ||
			*terms != "https://social.example/about/terms" || privacy != nil {
			t.Errorf("links for %s = %v %v, want the terms only", lang, terms, privacy)
		}
	}

	// Clearing the text removes the link.
	res = e.sendJSON(t, http.MethodPut, "/admin/instance_texts/terms", "admin", `{"body":""}`)
	if got := decodeJSON[clientapi.AdminInstanceText](t, res); res.StatusCode != http.StatusOK || got.Body != "" {
		t.Errorf("clear = %d %+v", res.StatusCode, got)
	}
	if terms, _ := instanceLinks(map[string]string{"Accept-Language": "ja"}); terms != nil {
		t.Errorf("terms link after clearing = %q, want none", *terms)
	}
}

func TestInstanceTextProblems(t *testing.T) {
	e, f := newTextsEnv(t)
	for _, tc := range []struct {
		name, path, body string
		status           int
		// field is the property that the problem names.
		field string
	}{
		{"unknown key", "/admin/instance_texts/motto", `{"body":"x"}`, http.StatusNotFound, ""},
		{"too long", "/admin/instance_texts/name", `{"body":"` + strings.Repeat("a", 101) + `"}`, http.StatusUnprocessableEntity, "body"},
		{"line break", "/admin/instance_texts/name", `{"body":"a\nb"}`, http.StatusUnprocessableEntity, "body"},
		{"blank name", "/admin/instance_texts/name", `{"body":"   "}`, http.StatusUnprocessableEntity, "body"},
		{"NUL", "/admin/instance_texts/terms", `{"body":"a\u0000b"}`, http.StatusUnprocessableEntity, "body"},
	} {
		res := e.send(t, http.MethodPut, tc.path, "admin", tc.body)
		p := decodeProblem(t, res)
		if res.StatusCode != tc.status || (tc.field != "" && (p.Field == nil || *p.Field != tc.field)) {
			t.Errorf("%s: %d %+v, want %d %q", tc.name, res.StatusCode, p, tc.status, tc.field)
		}
	}
	if len(f.sets) != 0 {
		t.Errorf("a refused text was set: %+v", f.sets)
	}
}
