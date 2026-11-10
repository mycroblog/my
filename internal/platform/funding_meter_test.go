// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestFundingMeter drives the funding meter on SQLite (FR-905, ADR-0067 §22): off by default;
// percent alone while the costs and the income are both private, amounts only while both are
// public; a change of either publication that leaves the choice invalid turns it off, audited;
// the percentage rounds down to 1%, and a month above its target is covered.
func TestFundingMeter(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	admin := e.user("alice")
	// 10.00 USD a month from 2025: this month's costs, the target while none is set.
	e.want(e.do(http.MethodPost, "/admin/cost_items", admin.token, map[string]any{
		"schedule": "months", "interval": 1, "starts_on": "2025-01-01", "name": "vm", "provider": "Cloud",
		"category": "compute", "price": map[string]string{"amount": "10.00", "currency": "USD"},
	}, nil), http.StatusCreated, "POST /admin/cost_items")
	// 7.29 USD of net income today: 72.9% of the costs.
	now := time.Now().UTC()
	if err := e.app.Finance.RecordIncome(t.Context(), []appfinance.IncomeMovement{{
		Provider: "fake", ProviderID: "txn_1", Kind: "payment", OccurredAt: now, OccurredOn: now.Format(time.DateOnly),
		Gross: "7.60", Fee: "0.31", Net: "7.29", Currency: "USD",
	}}); err != nil {
		t.Fatal(err)
	}
	set := func(values map[string]string, status int) {
		t.Helper()
		e.want(e.do(http.MethodPatch, "/admin/settings", admin.token, map[string]any{"values": values}, nil), status, "PATCH /admin/settings")
	}
	meter := func() (*clientapi.FundingMeter, string) {
		t.Helper()
		res := e.do(http.MethodGet, "/instance", "", nil, nil)
		e.want(res, http.StatusOK, "GET /instance")
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		var info clientapi.Instance
		if err := json.Unmarshal(body, &info); err != nil {
			t.Fatal(err)
		}
		return info.FundingMeter, string(body)
	}
	stored := func() string {
		t.Helper()
		var list clientapi.AdminSettingList
		e.want(e.do(http.MethodGet, "/admin/settings", admin.token, nil, &list), http.StatusOK, "GET /admin/settings")
		for _, item := range list.Items {
			if item.Key == clientapi.InstanceSettingKey(instance.SettingFundingMeterVisibility) {
				return item.Value
			}
		}
		t.Fatal("no funding_meter_visibility")
		return ""
	}

	if m, _ := meter(); m != nil {
		t.Errorf("the meter by default = %+v, want none", m)
	}

	// While both are private, amounts would publish them; percent alone does not.
	set(map[string]string{"funding_meter_visibility": "amounts"}, http.StatusUnprocessableEntity)
	set(map[string]string{"funding_meter_visibility": "percent"}, http.StatusOK)
	m, body := meter()
	if m == nil || m.Visibility != clientapi.FundingMeterVisibilityPercent || m.Percent != 72 || m.Covered || m.Estimated || m.Rail ||
		m.Income != nil || m.Target != nil || m.TargetIsCosts != nil || m.Label != now.Format("2006-01") {
		t.Errorf("the percent meter = %+v", m)
	}
	for _, secret := range []string{"7.29", "10.00", "target"} {
		if strings.Contains(body, secret) {
			t.Errorf("GET /instance under percent shows %q: %s", secret, body)
		}
	}
	set(map[string]string{"funding_meter_rail": "true"}, http.StatusOK)
	if m, _ := meter(); m == nil || !m.Rail {
		t.Errorf("the meter with the rail = %+v", m)
	}

	// Publishing the costs alone turns the percentage off: with them it would give the income away.
	set(map[string]string{"finance_visibility": "totals"}, http.StatusOK)
	if got := stored(); got != "off" {
		t.Errorf("funding_meter_visibility after publishing the costs = %q, want off", got)
	}
	if m, _ := meter(); m != nil {
		t.Errorf("the meter with only the costs public = %+v, want none", m)
	}
	audit, err := e.app.Moderation.AuditLog(t.Context(), appmoderation.AuditQuery{
		TargetKind: moderation.AuditTargetSetting, TargetID: string(instance.SettingFundingMeterVisibility),
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 2 || audit[0].Action != moderation.AuditSettingChanged {
		t.Errorf("the audit of funding_meter_visibility = %+v, want the change to percent and the turn off", audit)
	}
	set(map[string]string{"funding_meter_visibility": "percent"}, http.StatusUnprocessableEntity)
	set(map[string]string{"funding_meter_visibility": "amounts"}, http.StatusUnprocessableEntity)

	// Both public, judged after the whole change: the amounts.
	set(map[string]string{"income_visibility": "totals", "funding_meter_visibility": "amounts"}, http.StatusOK)
	m, _ = meter()
	if m == nil || m.Visibility != clientapi.FundingMeterVisibilityAmounts || m.Percent != 72 || m.Covered ||
		m.Income == nil || *m.Income != (clientapi.Money{Amount: "7.29", Currency: "USD"}) ||
		m.Target == nil || *m.Target != (clientapi.Money{Amount: "10.00", Currency: "USD"}) || m.TargetIsCosts == nil || !*m.TargetIsCosts {
		t.Errorf("the amounts meter = %+v", m)
	}

	// A target of its own, above zero, in the digits of its currency.
	for _, bad := range []map[string]string{
		{"funding_meter_target_amount": "0"},
		{"funding_meter_target_currency": "XYZ"},
		{"funding_meter_target_amount": "0", "funding_meter_target_currency": "USD"},
		{"funding_meter_target_amount": "5.001", "funding_meter_target_currency": "USD"},
		{"funding_meter_target_amount": "5", "funding_meter_target_currency": "XYZ"},
		{"funding_meter_target_amount": "-5", "funding_meter_target_currency": "USD"},
	} {
		set(bad, http.StatusUnprocessableEntity)
	}
	// Without a currency, the amount is in the currency of the month's costs.
	set(map[string]string{"funding_meter_target_amount": "8"}, http.StatusOK)
	m, _ = meter()
	if m == nil || m.Percent != 91 || m.Covered || m.Target == nil || *m.Target != (clientapi.Money{Amount: "8.00", Currency: "USD"}) {
		t.Errorf("the meter against 8 in the currency of the costs = %+v, want 91%%", m)
	}
	set(map[string]string{"funding_meter_target_amount": "8.5", "funding_meter_target_currency": "JPY"}, http.StatusUnprocessableEntity)
	set(map[string]string{"funding_meter_target_amount": "5", "funding_meter_target_currency": "usd"}, http.StatusOK)
	m, _ = meter()
	if m == nil || m.Percent != 145 || !m.Covered || m.Target == nil || *m.Target != (clientapi.Money{Amount: "5.00", Currency: "USD"}) ||
		m.TargetIsCosts == nil || *m.TargetIsCosts {
		t.Errorf("the meter above its target = %+v, want 145%% and covered", m)
	}

	// Taking the income private again turns the amounts off.
	set(map[string]string{"income_visibility": "private"}, http.StatusOK)
	if got := stored(); got != "off" {
		t.Errorf("funding_meter_visibility after hiding the income = %q, want off", got)
	}
}
