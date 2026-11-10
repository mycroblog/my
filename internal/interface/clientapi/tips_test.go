// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	appsupporters "github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

const ownTipID = "01890a5d-ac96-774b-bcce-b302099a8301"

// fakeTips takes tips of 300, 500, and 1000 yen from alice.
type fakeTips struct {
	disabled bool
	startErr error
	starts   []appsupporters.TipInput
	tip      appsupporters.TipView
}

func (f *fakeTips) TipsEnabled(context.Context) (bool, error) { return !f.disabled, nil }

func (f *fakeTips) TipAmounts(context.Context) (string, []int64, error) {
	if f.disabled {
		return "", nil, appsupporters.ErrTipsDisabled
	}
	return "JPY", []int64{300, 500, 1000}, nil
}

func (f *fakeTips) TipSettings(context.Context) (appsupporters.TipSettings, error) {
	return appsupporters.TipSettings{}, nil
}

func (f *fakeTips) SetTips(context.Context, appsupporters.TipSettingsInput, string) (appsupporters.TipSettings, error) {
	return appsupporters.TipSettings{}, nil
}

func (f *fakeTips) StartTip(_ context.Context, in appsupporters.TipInput) (appsupporters.StartedTip, error) {
	f.starts = append(f.starts, in)
	if f.disabled {
		return appsupporters.StartedTip{}, appsupporters.ErrTipsDisabled
	}
	if f.startErr != nil {
		return appsupporters.StartedTip{}, f.startErr
	}
	if !slices.Contains([]int64{300, 500, 1000}, in.AmountMinor) {
		return appsupporters.StartedTip{}, appsupporters.ErrTipAmount
	}
	return appsupporters.StartedTip{TipID: ownTipID, RedirectURL: "https://checkout.example/t/1"}, nil
}

func (f *fakeTips) OwnTip(_ context.Context, userID, tipID string) (appsupporters.TipView, error) {
	if tipID != f.tip.ID || userID != f.tip.UserID {
		return appsupporters.TipView{}, fmt.Errorf("%w: tip %s", appsupporters.ErrNotFound, tipID)
	}
	return f.tip, nil
}

// TestTips checks the public amounts of the tips and the user's own tips (ADR-0162 §17): a 404
// while the server takes none, the amounts in money, a start that returns to the web client,
// the localized problems of an amount that is not offered and of the rate limits, and only the
// user's own tips.
func TestTips(t *testing.T) {
	f := &fakeTips{disabled: true}
	settings := settingsInstance{&fakeSettings{stored: []instance.CurrentSetting{}}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Tips, o.Instance, o.Settings = f, settings, settings.fakeSettings
		o.WebOrigin = "https://app.social.example"
	})
	e.auth.tokens["supporters"] = bearer("read:supporters write:supporters", aliceAccountID)
	if res := e.send(t, http.MethodGet, "/instance/tip_amounts", "", ""); res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /instance/tip_amounts while disabled = %d, want 404", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPost, "/supporter/tips", "supporters", `{"amount":{"amount":"500","currency":"JPY"}}`); res.StatusCode != http.StatusNotFound {
		t.Fatalf("POST /supporter/tips while disabled = %d, want 404", res.StatusCode)
	}
	f.disabled = false
	res := e.send(t, http.MethodGet, "/instance/tip_amounts", "", "")
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "public, max-age=60" {
		t.Fatalf("GET /instance/tip_amounts = %d, Cache-Control %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	amounts := decode[clientapi.TipAmounts](t, res)
	if want := []clientapi.Money{{Amount: "300", Currency: "JPY"}, {Amount: "500", Currency: "JPY"}, {Amount: "1000", Currency: "JPY"}}; !slices.Equal(amounts.Amounts, want) {
		t.Errorf("amounts = %+v, want %+v", amounts.Amounts, want)
	}
	if inst := decode[clientapi.Instance](t, e.send(t, http.MethodGet, "/instance", "", "")); !inst.TipsEnabled {
		t.Error("Instance.tips_enabled = false, want true")
	}

	res = e.sendJSON(t, http.MethodPost, "/supporter/tips", "supporters", `{"amount":{"amount":"500","currency":"JPY"}}`)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("POST /supporter/tips = %d", res.StatusCode)
	}
	if co := decode[clientapi.SupporterTipCheckout](t, res); co.TipId != ownTipID || co.RedirectUrl != "https://checkout.example/t/1" {
		t.Errorf("checkout = %+v", co)
	}
	in := f.starts[len(f.starts)-1]
	if in.UserID != userOf(aliceAccountID) || in.AmountMinor != 500 || in.Currency != "JPY" || in.ReturnURL != "https://app.social.example/settings/supporter/tip" || in.Description == "" {
		t.Errorf("the start's input = %+v", in)
	}
	res = e.sendJSON(t, http.MethodPost, "/supporter/tips", "supporters", `{"amount":{"amount":"700","currency":"JPY"}}`)
	if res.StatusCode != http.StatusUnprocessableEntity || problemType(t, res) != clientapi.ProblemTypeSupporterTipAmount {
		t.Errorf("a tip of 700 = %d, want 422 %s", res.StatusCode, clientapi.ProblemTypeSupporterTipAmount)
	}
	f.startErr = &kernel.RateLimitedError{RetryAfter: 30 * time.Second}
	res = e.sendJSON(t, http.MethodPost, "/supporter/tips", "supporters", `{"amount":{"amount":"500","currency":"JPY"}}`)
	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") != "30" {
		t.Errorf("a tip beyond the limit = %d, Retry-After %q", res.StatusCode, res.Header.Get("Retry-After"))
	}
	f.startErr = &appsupporters.IneligibleError{Reason: appsupporters.IneligibleEmail}
	res = e.sendJSON(t, http.MethodPost, "/supporter/tips", "supporters", `{"amount":{"amount":"500","currency":"JPY"}}`)
	if res.StatusCode != http.StatusUnprocessableEntity || problemType(t, res) != clientapi.ProblemTypeSupporterIneligible {
		t.Errorf("a tip of an unverified user = %d", res.StatusCode)
	}

	paid := time.Date(2026, 11, 10, 3, 0, 0, 0, time.UTC)
	f.tip = appsupporters.TipView{State: appsupporters.TipStatePaid, TipRecord: appsupporters.TipRecord{
		Tip:     appsupporters.Tip{ID: ownTipID, UserID: userOf(aliceAccountID), AmountMinor: 500, Currency: "JPY", CreatedAt: paid.Add(-time.Minute)},
		Payment: &appsupporters.TipPayment{TipID: ownTipID, PaidAt: paid},
	}}
	res = e.send(t, http.MethodGet, "/supporter/tips/"+ownTipID, "supporters", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET own tip = %d", res.StatusCode)
	}
	if got := decode[clientapi.SupporterTip](t, res); got.State != clientapi.SupporterTipStatePaid || got.Amount != (clientapi.Money{Amount: "500", Currency: "JPY"}) ||
		got.PaidAt == nil || !got.PaidAt.Equal(paid) {
		t.Errorf("own tip = %+v", got)
	}
	f.tip.UserID = "someone-else"
	if res := e.send(t, http.MethodGet, "/supporter/tips/"+ownTipID, "supporters", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET another's tip = %d, want 404", res.StatusCode)
	}
}
