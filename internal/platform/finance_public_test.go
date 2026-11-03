// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestPublicCostTotals drives the public totals on SQLite (FR-903, ADR-0067 §8): private by
// default, then totals and breakdown, never naming an item or a memo, and stopping at the period
// that holds today; a moderator, without manage_settings, cannot publish them.
func TestPublicCostTotals(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	admin := e.user("alice")
	e.want(e.do(http.MethodPost, "/admin/cost_items", admin.token, map[string]any{
		"schedule": "months", "interval": 1, "starts_on": "2025-01-01", "name": "tokyo-vm-7", "provider": "Cloud",
		"category": "compute", "memo": "bucket-secret", "price": map[string]string{"amount": "10.00", "currency": "USD"},
	}, nil), http.StatusCreated, "POST /admin/cost_items")
	r, err := e.app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{Username: "bob", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Identity.GrantRole(t.Context(), identity.RoleChangeInput{UserID: r.User.ID, RoleKey: domainidentity.RoleModerator}); err != nil {
		t.Fatal(err)
	}
	moderator := issueTokens(t, e.app, r.User.ID, "").AccessToken
	const path = "/instance/cost_totals?granularity=month&from=2025-01-01&to=2025-02-28"
	set := func(token, value string, status int) {
		t.Helper()
		e.want(e.do(http.MethodPatch, "/admin/settings", token, map[string]any{"values": map[string]string{"finance_visibility": value}}, nil),
			status, "PATCH finance_visibility "+value)
	}
	read := func(path string) clientapi.CostTotals {
		t.Helper()
		res := e.do(http.MethodGet, path, "", nil, nil)
		e.want(res, http.StatusOK, "GET "+path)
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"tokyo-vm-7", "bucket-secret"} {
			if strings.Contains(string(body), secret) {
				t.Errorf("GET %s names %s: %s", path, secret, body)
			}
		}
		var out clientapi.CostTotals
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	e.want(e.do(http.MethodGet, path, "", nil, nil), http.StatusNotFound, "GET while private")
	set(moderator, "breakdown", http.StatusForbidden)
	e.want(e.do(http.MethodGet, path, "", nil, nil), http.StatusNotFound, "GET after a forbidden change")

	set(admin.token, "totals", http.StatusOK)
	totals := read(path)
	if len(totals.Periods) != 2 || !slices.Equal(totals.Periods[0].Totals, []clientapi.Money{{Amount: "10.00", Currency: "USD"}}) ||
		len(totals.Periods[0].Breakdown) != 0 || len(totals.Rules.Sources) != 0 {
		t.Errorf("totals = %+v", totals)
	}

	set(admin.token, "breakdown", http.StatusOK)
	totals = read(path)
	if len(totals.Periods) != 2 || len(totals.Periods[0].Breakdown) != 1 || totals.Periods[0].Breakdown[0].Provider != "Cloud" ||
		totals.Periods[0].Breakdown[0].Category != clientapi.CostCategoryCompute {
		t.Errorf("breakdown = %+v", totals)
	}

	// The periods stop at the one that holds today.
	year := time.Now().UTC().Format("2006")
	far := read("/instance/cost_totals?granularity=year&from=2025-01-01&to=2099-12-31")
	if n := len(far.Periods); n == 0 || far.Periods[n-1].Label != year {
		t.Errorf("years until 2099 = %+v, want the last one %s", far.Periods, year)
	}
	if later := read("/instance/cost_totals?granularity=month&from=2099-01-01&to=2099-02-28"); len(later.Periods) != 0 {
		t.Errorf("months of 2099 = %+v, want none", later.Periods)
	}
}
