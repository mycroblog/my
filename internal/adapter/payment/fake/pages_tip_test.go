// SPDX-License-Identifier: AGPL-3.0-only

package fake

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/supporters"
)

// The page of a tip shows its line item and amount, and paying it records the payment, its money,
// and the events of both, then goes on to the success URL with the checkout's ID.
func TestTipPage(t *testing.T) {
	e := newPagesEnv(t)
	c, err := e.p.StartOneTimeCheckout(t.Context(), supporters.OneTimeRequest{
		LocalID: "tip-1", AmountMinor: 500, Currency: "JPY", Description: "Tip for my.example", Locale: "en", Key: "tip:tip-1",
		SuccessURL: "https://" + pagesHost + "/settings/supporter/tip?tip=tip-1&checkout=success",
		CancelURL:  "https://" + pagesHost + "/settings/supporter/tip?tip=tip-1&checkout=canceled",
	})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(c.RedirectURL)
	if err != nil || u.Host != pagesHost || !strings.Contains(u.Path, pageTip) {
		t.Fatalf("RedirectURL = %q, want a tip page on %s", c.RedirectURL, pagesHost)
	}
	page := e.get(u.Path)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Tip for my.example") || !strings.Contains(page.Body.String(), "500 JPY") {
		t.Fatalf("GET the tip page = %d\n%s", page.Code, page.Body.String())
	}
	if res := e.post(u.Path, actionCancel, ""); res.Code != http.StatusSeeOther || !strings.Contains(res.Header().Get("Location"), "checkout=canceled") {
		t.Errorf("cancel = %d %q", res.Code, res.Header().Get("Location"))
	}
	res := e.post(u.Path, actionPay, "")
	if res.Code != http.StatusSeeOther || !strings.Contains(res.Header().Get("Location"), "checkout_id="+url.QueryEscape(string(c.Ref))) {
		t.Fatalf("pay = %d %q", res.Code, res.Header().Get("Location"))
	}
	got, err := e.p.GetOneTimeCheckout(t.Context(), c.Ref)
	if err != nil || got.Status != supporters.OneTimePaid || got.Payment == "" || got.AmountMinor != 500 {
		t.Fatalf("GetOneTimeCheckout after paying = %+v, %v", got, err)
	}
	money, err := e.p.ListMoney(t.Context(), e.now.Add(-1), "")
	if err != nil || len(money.Movements) != 1 || money.Movements[0].Payment != got.Payment || money.Movements[0].Gross != "500" {
		t.Errorf("money after paying = %+v, %v", money, err)
	}
	changes, err := e.p.ListChanges(t.Context(), e.now.Add(-1), "")
	if err != nil || len(changes.Notices) != 2 {
		t.Errorf("changes after paying = %+v, %v, want the checkout's and the money's", changes, err)
	}
	if again := e.get(u.Path); again.Code != http.StatusSeeOther {
		t.Errorf("GET a paid tip page = %d, want the success URL", again.Code)
	}
}
