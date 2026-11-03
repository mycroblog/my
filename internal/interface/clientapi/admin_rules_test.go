// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeRules keeps rules with the semantics of appinstance.Service: rules in effect by
// position, then retired ones.
type fakeRules struct {
	rules []instance.Rule
	now   time.Time
	seq   int
}

func (f *fakeRules) id() string {
	f.seq++
	return fmt.Sprintf("01890a5d-ac96-774b-bcce-b302099a6%03d", f.seq)
}

func (f *fakeRules) find(id string) (int, error) {
	i := slices.IndexFunc(f.rules, func(r instance.Rule) bool { return r.ID == id })
	if i < 0 {
		return 0, fmt.Errorf("rule %s: %w", id, appinstance.ErrNotFound)
	}
	return i, nil
}

func (f *fakeRules) inEffect() []instance.Rule {
	var out []instance.Rule
	for _, r := range f.rules {
		if !r.Retired() {
			out = append(out, r)
		}
	}
	slices.SortStableFunc(out, func(a, b instance.Rule) int { return a.Revision.Position - b.Revision.Position })
	return out
}

func (f *fakeRules) Rules(_ context.Context, includeRetired bool) ([]instance.Rule, error) {
	out := f.inEffect()
	if includeRetired {
		for _, r := range f.rules {
			if r.Retired() {
				out = append(out, r)
			}
		}
	}
	return out, nil
}

func (f *fakeRules) Rule(_ context.Context, id string) (instance.Rule, error) {
	i, err := f.find(id)
	if err != nil {
		return instance.Rule{}, err
	}
	return f.rules[i], nil
}

func (f *fakeRules) AddRule(_ context.Context, in appinstance.AddRuleInput) (instance.Rule, error) {
	if err := instance.ValidateRuleText(in.Text, in.Hint); err != nil {
		return instance.Rule{}, err
	}
	if len(f.inEffect()) >= instance.MaxRules {
		return instance.Rule{}, instance.ErrTooManyRules
	}
	id := f.id()
	r := instance.Rule{ID: id, CreatedAt: f.now, Revision: instance.RuleRevision{
		ID: f.id(), RuleID: id, Position: len(f.inEffect()) + 1, Text: in.Text, Hint: in.Hint, RevisedByUserID: in.CreatedByUserID, RevisedAt: f.now,
	}}
	f.rules = append(f.rules, r)
	return r, nil
}

func (f *fakeRules) ReviseRule(_ context.Context, in appinstance.ReviseRuleInput) (instance.Rule, error) {
	if err := instance.ValidateRuleText(in.Text, in.Hint); err != nil {
		return instance.Rule{}, err
	}
	i, err := f.find(in.RuleID)
	if err != nil {
		return instance.Rule{}, err
	}
	if f.rules[i].Retired() {
		return instance.Rule{}, instance.ErrRuleRetired
	}
	rev := &f.rules[i].Revision
	rev.ID, rev.Text, rev.RevisedByUserID = f.id(), in.Text, in.RevisedByUserID
	if !in.KeepHint {
		rev.Hint = in.Hint
	}
	return f.rules[i], nil
}

func (f *fakeRules) MoveRule(_ context.Context, in appinstance.MoveRuleInput) (instance.Rule, error) {
	if in.Position < 1 {
		return instance.Rule{}, instance.ErrInvalidValue
	}
	i, err := f.find(in.RuleID)
	if err != nil {
		return instance.Rule{}, err
	}
	if f.rules[i].Retired() {
		return instance.Rule{}, instance.ErrRuleRetired
	}
	order := slices.DeleteFunc(f.inEffect(), func(r instance.Rule) bool { return r.ID == in.RuleID })
	order = slices.Insert(order, min(in.Position-1, len(order)), f.rules[i])
	for p, r := range order {
		j, _ := f.find(r.ID)
		f.rules[j].Revision.Position = p + 1
	}
	return f.rules[i], nil
}

func (f *fakeRules) RetireRule(_ context.Context, id, by string) (instance.Rule, error) {
	i, err := f.find(id)
	if err != nil {
		return instance.Rule{}, err
	}
	if !f.rules[i].Retired() {
		f.rules[i].Retirement = &instance.RuleRetirement{RuleID: id, RetiredByUserID: by, RetiredAt: f.now}
	}
	return f.rules[i], nil
}

// newRulesEnv is newAuthEnv with the tokens of newAdminEnv and the rule operations: alice
// holds every permission and bob none.
func newRulesEnv(t *testing.T) (*authEnv, *fakeRules) {
	t.Helper()
	f := &fakeRules{now: adminNow}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.Rules = f
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	return e, f
}

func TestInstanceRuleOperationsNeedAPermission(t *testing.T) {
	e, f := newRulesEnv(t)
	const id = "01890a5d-ac96-774b-bcce-b302099a6999"
	for _, op := range []struct{ method, path, body string }{
		{http.MethodGet, "/admin/rules", ""},
		{http.MethodPost, "/admin/rules", `{"text":"Be kind"}`},
		{http.MethodGet, "/admin/rules/" + id, ""},
		{http.MethodPut, "/admin/rules/" + id, `{"text":"Be kind"}`},
		{http.MethodPost, "/admin/rules/" + id + "/move", `{"position":1}`},
		{http.MethodPost, "/admin/rules/" + id + "/retire", ""},
	} {
		res := e.send(t, op.method, op.path, "bob-admin", op.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
			p.Detail == nil || !strings.Contains(*p.Detail, "manage_rules") {
			t.Errorf("%s %s by bob = %d %+v, want 403 for manage_rules", op.method, op.path, res.StatusCode, p)
		}
	}
	if len(f.rules) != 0 {
		t.Error("a refused operation added a rule")
	}
	res := e.send(t, http.MethodPost, "/admin/rules", "admin-read", `{"text":"Be kind"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
		t.Errorf("create with admin:read = %d %+v, want 403 insufficient-scope", res.StatusCode, p)
	}
}

func TestManageInstanceRules(t *testing.T) {
	e, f := newRulesEnv(t)
	res := e.sendJSON(t, http.MethodPost, "/admin/rules", "admin", `{"text":"Be kind","hint":"No harassment."}`)
	kind := decodeJSON[clientapi.AdminRule](t, res)
	hint := "No harassment."
	want := clientapi.AdminRule{Id: kind.Id, Position: 1, CreatedAt: adminNow, RevisedAt: adminNow, Text: "Be kind", Hint: &hint}
	if res.StatusCode != http.StatusOK || !reflect.DeepEqual(kind, want) {
		t.Fatalf("create = %d %+v, want %+v", res.StatusCode, kind, want)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if r := f.rules[0]; r.Revision.RevisedByUserID != userOf(aliceAccountID) {
		t.Errorf("created by %q, want alice's user", r.Revision.RevisedByUserID)
	}
	spam := decodeJSON[clientapi.AdminRule](t, e.sendJSON(t, http.MethodPost, "/admin/rules", "admin", `{"text":"No spam"}`))

	// Revising keeps the ID and replaces the text and the hint.
	res = e.sendJSON(t, http.MethodPut, "/admin/rules/"+kind.Id, "admin", `{"text":"とても親切に"}`)
	revised := decodeJSON[clientapi.AdminRule](t, res)
	if res.StatusCode != http.StatusOK || revised.Id != kind.Id || revised.Text != "とても親切に" || revised.Hint != nil {
		t.Errorf("revise = %d %+v; want the same rule with the new text and no hint", res.StatusCode, revised)
	}

	res = e.sendJSON(t, http.MethodPost, "/admin/rules/"+spam.Id+"/move", "admin", `{"position":1}`)
	if moved := decodeJSON[clientapi.AdminRule](t, res); res.StatusCode != http.StatusOK || moved.Id != spam.Id || moved.Position != 1 {
		t.Errorf("move = %d %+v, want the spam rule first", res.StatusCode, moved)
	}

	// The public instance lists the rules in effect, the same in every locale.
	instanceRules := func(header map[string]string) []clientapi.InstanceRule {
		t.Helper()
		res := e.get(t, "/instance", header)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET /instance = %d", res.StatusCode)
		}
		return decodeJSON[clientapi.Instance](t, res).Rules
	}
	want2 := []clientapi.InstanceRule{{Id: spam.Id, Text: "No spam"}, {Id: kind.Id, Text: "とても親切に"}}
	for _, header := range []map[string]string{
		{"Accept-Language": "ja-JP"},
		{"Accept-Language": "fr", "Authorization": "Bearer read"}, // alice's locale is ja
		{"Accept-Language": "fr"},
	} {
		if got := instanceRules(header); !reflect.DeepEqual(got, want2) {
			t.Errorf("rules for %v = %+v, want %+v", header, got, want2)
		}
	}

	res = e.send(t, http.MethodPost, "/admin/rules/"+kind.Id+"/retire", "admin", "")
	retired := decodeJSON[clientapi.AdminRule](t, res)
	if res.StatusCode != http.StatusOK || retired.RetiredAt == nil || !retired.RetiredAt.Equal(adminNow) {
		t.Errorf("retire = %d %+v, want retired now", res.StatusCode, retired)
	}
	if got := instanceRules(nil); len(got) != 1 || got[0].Id != spam.Id {
		t.Errorf("rules after retiring = %+v, want only the spam rule", got)
	}
	ids := func(path string) []string {
		t.Helper()
		res := e.admin(t, http.MethodGet, path, "admin-read", nil)
		list := decodeJSON[clientapi.AdminRuleList](t, res)
		var out []string
		for _, r := range list.Items {
			out = append(out, r.Id)
		}
		return out
	}
	if got := ids("/admin/rules"); !reflect.DeepEqual(got, []string{spam.Id}) {
		t.Errorf("list = %q, want the rule in effect", got)
	}
	if got := ids("/admin/rules?include_retired=true"); !reflect.DeepEqual(got, []string{spam.Id, kind.Id}) {
		t.Errorf("list with retired = %q, want the retired rule last", got)
	}
	// Reports still find a retired rule.
	res = e.admin(t, http.MethodGet, "/admin/rules/"+kind.Id, "admin-read", nil)
	if got := decodeJSON[clientapi.AdminRule](t, res); res.StatusCode != http.StatusOK || got.RetiredAt == nil {
		t.Errorf("get retired = %d %+v", res.StatusCode, got)
	}
}

func TestInstanceRuleProblems(t *testing.T) {
	e, f := newRulesEnv(t)
	r, _ := f.AddRule(t.Context(), appinstance.AddRuleInput{Text: "Be kind"})
	retired, _ := f.AddRule(t.Context(), appinstance.AddRuleInput{Text: "Old"})
	_, _ = f.RetireRule(t.Context(), retired.ID, "")
	const unknown = "01890a5d-ac96-774b-bcce-b302099a6999"
	for _, tc := range []struct {
		name, method, path, body string
		status                   int
		// detail is the property that the detail names (in alice's locale, ja).
		detail string
	}{
		{"too long", http.MethodPost, "/admin/rules", `{"text":"` + strings.Repeat("a", 301) + `","x":"invalid-on-purpose"}`, http.StatusUnprocessableEntity, "text"},
		{"line break", http.MethodPost, "/admin/rules", `{"text":"a\nb"}`, http.StatusUnprocessableEntity, "text"},
		{"blank", http.MethodPut, "/admin/rules/" + r.ID, `{"text":"   "}`, http.StatusUnprocessableEntity, "text"},
		{"hint too long", http.MethodPut, "/admin/rules/" + r.ID, `{"text":"a","hint":"` + strings.Repeat("a", 1001) + `","x":"invalid-on-purpose"}`, http.StatusUnprocessableEntity, "hint"},
		{"position 0", http.MethodPost, "/admin/rules/" + r.ID + "/move", `{"position":0,"x":"invalid-on-purpose"}`, http.StatusUnprocessableEntity, "position"},
		{"revise retired", http.MethodPut, "/admin/rules/" + retired.ID, `{"text":"x"}`, http.StatusConflict, ""},
		{"move retired", http.MethodPost, "/admin/rules/" + retired.ID + "/move", `{"position":1}`, http.StatusConflict, ""},
		{"get unknown", http.MethodGet, "/admin/rules/" + unknown, "", http.StatusNotFound, ""},
		{"revise unknown", http.MethodPut, "/admin/rules/" + unknown, `{"text":"x"}`, http.StatusNotFound, ""},
		{"retire unknown", http.MethodPost, "/admin/rules/" + unknown + "/retire", "", http.StatusNotFound, ""},
	} {
		var res *http.Response
		if tc.body == "" {
			res = e.send(t, tc.method, tc.path, "admin", "")
		} else {
			res = e.sendJSON(t, tc.method, tc.path, "admin", tc.body)
		}
		p := decodeProblem(t, res)
		if res.StatusCode != tc.status || (tc.detail != "" && (p.Detail == nil || !strings.Contains(*p.Detail, tc.detail))) {
			t.Errorf("%s: %d %+v, want %d %q", tc.name, res.StatusCode, p, tc.status, tc.detail)
		}
	}
	if got, _ := f.Rule(t.Context(), r.ID); !reflect.DeepEqual(got, r) {
		t.Errorf("rule = %+v, want unchanged %+v", got, r)
	}
}
