// SPDX-License-Identifier: AGPL-3.0-only

package fake

import (
	"net/http"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/i18n"
)

// The checkout page of a tip (onetime.go) among the pages of a PageProvider: it shows the
// checkout's line item and amount, and offers to pay, to make the payment fail, or to cancel.

// pageTip is the page of a one-time checkout under supporters.PagesPath, followed by its token.
const pageTip = "tip/"

// openTip returns with p.mu held the request of the one-time checkout of token while it can be
// paid: false for an unknown, expired, or paid one, and done for a paid one.
func (p *Provider) openTip(token string) (req supporters.OneTimeRequest, ref supporters.CheckoutRef, done, ok bool) {
	ref = supporters.CheckoutRef(token)
	c, found := p.oneTimes[ref]
	switch {
	case !found || c.req == nil:
		return supporters.OneTimeRequest{}, ref, false, false
	case c.result.Status == supporters.OneTimePaid:
		return *c.req, ref, true, false
	case c.result.Status == supporters.OneTimeExpired || !p.o.Now().Before(c.expiresAt):
		return *c.req, ref, false, false
	}
	return *c.req, ref, false, true
}

// tipSuccessURL returns the success URL of a one-time checkout with its ref as checkout_id.
func tipSuccessURL(req supporters.OneTimeRequest, ref supporters.CheckoutRef) string {
	return successURL(supporters.CheckoutRequest{SuccessURL: req.SuccessURL}, ref)
}

// tipView returns the view of the checkout page of a tip.
func (h *pages) tipView(r *http.Request, req supporters.OneTimeRequest) view {
	v := h.newView(r, req.Locale)
	v.Title, v.Back = req.Description, req.CancelURL
	v.Price = v.L.ServerPaymentsFakePriceOnce(i18n.ServerPaymentsFakePriceOnceArgs{
		Amount: formatMinor(req.AmountMinor, req.Currency), Currency: req.Currency,
	})
	return v
}

// tipPage shows the checkout of a tip, or goes on to the success URL once it is paid.
func (h *pages) tipPage(w http.ResponseWriter, r *http.Request) {
	h.p.mu.Lock()
	req, ref, done, ok := h.p.openTip(r.PathValue("token"))
	h.p.mu.Unlock()
	switch {
	case done:
		http.Redirect(w, r, tipSuccessURL(req, ref), http.StatusSeeOther) //nolint:gosec // G710: the URL is the checkout's own, which our server gave
		return
	case !ok:
		h.renderError(w, r, http.StatusNotFound, req.CancelURL)
		return
	}
	h.render(w, r, http.StatusOK, "checkout", h.tipView(r, req))
}

// tip takes the checkout form of a tip: pay records the payment and the events of its checkout
// and its money, fail shows the page again with the payment failed, and cancel goes back.
func (h *pages) tip(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPageForm)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "")
		return
	}
	h.p.mu.Lock()
	req, ref, done, ok := h.p.openTip(r.PathValue("token"))
	action := r.PostForm.Get("action")
	if ok && action == actionPay {
		if _, txn, err := h.p.payOneTime(ref); err == nil {
			now := h.p.o.Now().UTC()
			for _, n := range []supporters.Notice{
				{Type: "checkout.completed", Kind: supporters.NoticeCheckout, ObjectID: string(ref)},
				{Type: "money.booked", Kind: supporters.NoticeMoney, ObjectID: txn},
			} {
				n.EventID, n.OccurredAt = h.p.newRef("evt"), now
				h.p.changes = append(h.p.changes, n)
			}
		}
		done, ok = true, false
	}
	h.p.mu.Unlock()
	switch {
	case done:
		http.Redirect(w, r, tipSuccessURL(req, ref), http.StatusSeeOther) //nolint:gosec // G710: the URL is the checkout's own, which our server gave
	case !ok:
		h.renderError(w, r, http.StatusNotFound, req.CancelURL)
	case action == actionCancel:
		http.Redirect(w, r, req.CancelURL, http.StatusSeeOther)
	case action == actionFail:
		v := h.tipView(r, req)
		v.Error = v.L.ServerPaymentsFakeFailed()
		h.render(w, r, http.StatusPaymentRequired, "checkout", v)
	default:
		h.renderError(w, r, http.StatusBadRequest, req.CancelURL)
	}
}
