// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeLinkCardDomainRules keeps the rules by domain with the checks of appinstance.Service, except
// the domain's, which the service makes after the handler canonicalizes it.
type fakeLinkCardDomainRules struct {
	rules    []instance.LinkCardDomainRule
	tooMany  bool
	byUserID string
}

func (f *fakeLinkCardDomainRules) LinkCardDomainRules(context.Context) ([]instance.LinkCardDomainRule, error) {
	return f.rules, nil
}

func (f *fakeLinkCardDomainRules) SetLinkCardDomainRule(_ context.Context, r instance.LinkCardDomainRule, byUserID string) (instance.LinkCardDomainRule, error) {
	if f.tooMany {
		return instance.LinkCardDomainRule{}, instance.ErrTooManyLinkCardDomainRules
	}
	if err := r.Validate(); err != nil {
		return instance.LinkCardDomainRule{}, err
	}
	r.UpdatedAt = adminNow
	f.byUserID = byUserID
	f.rules = slices.DeleteFunc(f.rules, func(old instance.LinkCardDomainRule) bool { return old.Domain == r.Domain })
	f.rules = append(f.rules, r)
	return r, nil
}

func (f *fakeLinkCardDomainRules) RemoveLinkCardDomainRule(_ context.Context, domain, byUserID string) error {
	f.byUserID = byUserID
	f.rules = slices.DeleteFunc(f.rules, func(old instance.LinkCardDomainRule) bool { return old.Domain == domain })
	return nil
}

func newLinkCardDomainsEnv(t *testing.T) (*authEnv, *fakeLinkCardDomainRules) {
	t.Helper()
	f := &fakeLinkCardDomainRules{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
		o.LinkCardDomainRules = f
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	return e, f
}

func TestLinkCardDomainOperationsNeedAPermission(t *testing.T) {
	e, f := newLinkCardDomainsEnv(t)
	f.rules = []instance.LinkCardDomainRule{{Domain: "a.example", Mode: instance.LinkCardNoFetch}}
	for _, op := range []struct{ method, path, body string }{
		{http.MethodGet, "/admin/link_card_domains", ""},
		{http.MethodPost, "/admin/link_card_domains", `{"domain":"b.example","mode":"no_fetch"}`},
		{http.MethodDelete, "/admin/link_card_domains/a.example", ""},
	} {
		res := e.send(t, op.method, op.path, "bob-admin", op.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission ||
			p.Detail == nil || !strings.Contains(*p.Detail, "manage_settings") {
			t.Errorf("%s by bob = %d %+v, want 403 for manage_settings", op.method, res.StatusCode, p)
		}
	}
	res := e.sendJSON(t, http.MethodPost, "/admin/link_card_domains", "admin-read", `{"domain":"b.example","mode":"no_fetch"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
		t.Errorf("post with admin:read = %d %+v, want 403 insufficient-scope", res.StatusCode, p)
	}
	if len(f.rules) != 1 {
		t.Errorf("a refused operation changed the rules: %+v", f.rules)
	}
}

func TestManageLinkCardDomainRules(t *testing.T) {
	e, f := newLinkCardDomainsEnv(t)
	// The domain takes the canonical form of a domain action's.
	res := e.sendJSON(t, http.MethodPost, "/admin/link_card_domains", "admin", `{"domain":" 例え.EXAMPLE. ","mode":"no_images","note":"Heavy"}`)
	saved := decodeJSON[clientapi.LinkCardDomainRule](t, res)
	if res.StatusCode != http.StatusOK || saved.Domain != "xn--r8jz45g.example" || saved.Mode != clientapi.LinkCardDomainModeNoImages ||
		saved.Note != "Heavy" || !saved.UpdatedAt.Equal(adminNow) {
		t.Fatalf("set = %d %+v", res.StatusCode, saved)
	}
	if f.byUserID == "" {
		t.Error("the change does not name the administrator")
	}
	res = e.admin(t, http.MethodGet, "/admin/link_card_domains", "admin-read", nil)
	list := decodeJSON[clientapi.LinkCardDomainRuleList](t, res)
	if res.StatusCode != http.StatusOK || len(list.Items) != 1 || list.Items[0] != saved || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("list = %d %+v (%s)", res.StatusCode, list, res.Header.Get("Cache-Control"))
	}
	res = e.sendJSON(t, http.MethodPost, "/admin/link_card_domains", "admin", `{"domain":"xn--r8jz45g.example","mode":"no_fetch"}`)
	if changed := decodeJSON[clientapi.LinkCardDomainRule](t, res); res.StatusCode != http.StatusOK || changed.Mode != clientapi.LinkCardDomainModeNoFetch || len(f.rules) != 1 {
		t.Errorf("change = %d %+v, rules %+v", res.StatusCode, changed, f.rules)
	}
	if res := e.send(t, http.MethodDelete, "/admin/link_card_domains/XN--R8JZ45G.example", "admin", ""); res.StatusCode != http.StatusNoContent || len(f.rules) != 0 {
		t.Errorf("remove = %d, rules %+v", res.StatusCode, f.rules)
	}
}

func TestLinkCardDomainRuleValidation(t *testing.T) {
	e, f := newLinkCardDomainsEnv(t)
	for name, tc := range map[string]struct{ body, field string }{
		"URL":       {`{"domain":"https://a.example/","mode":"no_fetch"}`, "domain"},
		"space":     {`{"domain":"a b.example","mode":"no_fetch"}`, "domain"},
		"port":      {`{"domain":"a.example:443","mode":"no_fetch"}`, "domain"},
		"mode":      {`{"domain":"a.example","mode":"off","x":"invalid-on-purpose"}`, "mode"},
		"long note": {`{"domain":"a.example","mode":"no_fetch","x":"invalid-on-purpose","note":"` + strings.Repeat("a", instance.MaxLinkCardDomainNote+1) + `"}`, "note"},
	} {
		res := e.sendJSON(t, http.MethodPost, "/admin/link_card_domains", "admin", tc.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, tc.field) {
			t.Errorf("%s = %d %+v, want 422 naming %s", name, res.StatusCode, p, tc.field)
		}
	}
	if len(f.rules) != 0 {
		t.Errorf("an invalid rule was kept: %+v", f.rules)
	}
	f.tooMany = true
	res := e.sendJSON(t, http.MethodPost, "/admin/link_card_domains", "admin", `{"domain":"a.example","mode":"no_fetch"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, "rules") {
		t.Errorf("too many = %d %+v, want 422 naming rules", res.StatusCode, p)
	}
}
