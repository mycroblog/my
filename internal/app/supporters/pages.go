// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"log/slog"
	"net/http"
)

// The pages of a PageServer are documents of the web client's payment entry (ADR-0074 §11,
// web/app/pay.html): the adapter decides the page and words what it decides (its title, the
// price, the outcome of a form, what it read of the provider) in PageData, and PageEnv.Render
// writes the entry's shell with that data under the pages' Content-Security-Policy
// (internal/interface/payments). The entry words the rest of the page. No page is HTML of an
// adapter.

// PageRenderer answers a request with the document of a payment page and status.
type PageRenderer func(w http.ResponseWriter, r *http.Request, status int, doc PageDocument)

// PageDocument is a payment page: the language and direction of the document, its title, and
// the data that the entry renders.
type PageDocument struct {
	Lang, Dir, Title string
	Data             PageData
}

// The screens of the payment entry, PageData.Screen.
const (
	PageScreenCheckout = "checkout"
	PageScreenPortal   = "portal"
	PageScreenError    = "error"
)

// PageData is the JSON that the payment entry reads (web/app-features/src/pay/data.ts).
type PageData struct {
	// Screen is the screen to render: PageScreenCheckout, PageScreenPortal, or PageScreenError.
	Screen string `json:"screen"`
	// Title is the page's heading, worded by the adapter (it names the plan of a checkout).
	Title string `json:"title"`
	// TestMode says that the provider's test credentials are in use: no money moves.
	TestMode bool `json:"testMode,omitempty"`
	// Fake says that the page is the fake provider's of a test build (ADR-0015 §6).
	Fake bool `json:"fake,omitempty"`
	// Price is the price of the checkout, worded.
	Price string `json:"price,omitempty"`
	// Error is what went wrong; Notice what went right (a card changed).
	Error  string `json:"error,omitempty"`
	Notice string `json:"notice,omitempty"`
	// Back is where the link back goes; "" shows none.
	Back string `json:"back,omitempty"`
	// Card is the card form of payjp.js (PAY.JP), which tokenizes the card in PAY.JP's iframe.
	Card *PageCardForm `json:"card,omitempty"`
	// Choices is the form of the fake provider's checkout: pay, make the payment fail, or cancel.
	Choices *PageChoices `json:"choices,omitempty"`
	// Portal is what the customer portal shows of the customer; nil on a portal with nothing to
	// show (the fake's).
	Portal *PagePortal `json:"portal,omitempty"`
}

// PageCardForm is a card form: it posts the token of the card that payjp.js made, never the card.
type PageCardForm struct {
	// Action is the path the form posts to; State the signed state it posts back as s.
	Action string `json:"action"`
	State  string `json:"state"`
	// PublicKey is the provider's public key, which payjp.js takes.
	PublicKey string `json:"publicKey"`
	// Email and Name start the fields of the email address and the card holder's name.
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
	// Change is a card change (the portal's) rather than a subscription (the checkout's).
	Change bool `json:"change,omitempty"`
}

// PageChoices is the fake checkout's form, which posts its action (pay, fail, cancel) to Action.
type PageChoices struct {
	Action string `json:"action"`
}

// PagePortal is what the customer portal shows: the card in use ("" for none), whether a
// subscription is paused after a failed payment, and the recent payments.
type PagePortal struct {
	Card     string        `json:"card,omitempty"`
	Paused   bool          `json:"paused,omitempty"`
	Payments []PagePayment `json:"payments"`
}

// PagePayment is a payment of the portal: its date (YYYY-MM-DD), its amount and what of it was
// refunded ("" for nothing), worded.
type PagePayment struct {
	Date     string `json:"date"`
	Amount   string `json:"amount"`
	Refunded string `json:"refunded,omitempty"`
}

// unrendered is the PageRenderer of a PageEnv without one: it answers 503.
func unrendered(log *slog.Logger) PageRenderer {
	return func(w http.ResponseWriter, r *http.Request, _ int, _ PageDocument) {
		if log != nil {
			log.ErrorContext(r.Context(), "a payment page has no renderer")
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
	}
}
