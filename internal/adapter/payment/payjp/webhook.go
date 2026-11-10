// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

// ParseWebhook implements supporters.PaymentProvider (ADR-0074 §12). PAY.JP authenticates a
// delivery by the account's fixed token only, without a time or a signature of the body, so a
// replayed delivery passes; the job re-reads what it points at, so a replay changes nothing.
// The events of API v1 and v2 come to the same endpoint with the same token, and are told apart
// by their shape (isV2Event).
func (p *Provider) ParseWebhook(header http.Header, body []byte, _ time.Time) (supporters.Notice, error) {
	got := header.Get(TokenHeader)
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(p.o.WebhookToken)) != 1 {
		return supporters.Notice{}, supporters.ErrInvalidSignature
	}
	if isV2Event(body) {
		return p.parseV2(body)
	}
	var ev event
	if err := json.Unmarshal(body, &ev); err != nil || ev.ID == "" || ev.Type == "" {
		return supporters.Notice{}, fmt.Errorf("payjp: %w: the body is not an event", supporters.ErrInvalidPaymentResponse)
	}
	if ev.Livemode != (p.mode == supporters.ModeLive) {
		return supporters.Notice{}, supporters.ErrModeMismatch
	}
	return noticeOf(ev), nil
}

// noticeOf maps an event to a notice: the subscription's events and a failed charge of one
// point at the subscription; a captured charge points at the charge, and a refund, a
// chargeback, and a statement at the event itself, which GetMoney reads back.
func noticeOf(ev event) supporters.Notice {
	var obj eventObject
	_ = json.Unmarshal(ev.Data, &obj)
	n := supporters.Notice{EventID: ev.ID, Type: ev.Type, Kind: supporters.NoticeIgnored, ObjectID: obj.ID, OccurredAt: unixTime(ev.Created)}
	sub := obj.Subscription
	if sub == "" {
		sub = obj.Metadata[metaSubscription]
	}
	switch {
	case strings.HasPrefix(ev.Type, "subscription."):
		n.Kind, n.Subscription = supporters.NoticeSubscription, supporters.SubscriptionRef(obj.ID)
	case ev.Type == "charge.failed" && sub != "":
		n.Kind, n.ObjectID, n.Subscription = supporters.NoticeSubscription, sub, supporters.SubscriptionRef(sub)
	case ev.Type == "charge.succeeded":
		n.Kind, n.Subscription = supporters.NoticeMoney, supporters.SubscriptionRef(sub)
	case ev.Type == "charge.refunded":
		n.Kind, n.ObjectID, n.Subscription = supporters.NoticeMoney, ev.ID, supporters.SubscriptionRef(sub)
	case ev.Type == "dispute.created", ev.Type == "statement.created":
		n.Kind, n.ObjectID = supporters.NoticeMoney, ev.ID
	}
	return n
}

// parseV2 maps an event of API v2 to a notice (ADR-0162 §11): a Checkout Session's completion
// and expiry point at the session; a PaymentFlow's success, a refund's events, and a dispute's
// creation at their object, whose transactions moneyOfV2 reads; the others are ignored.
func (p *Provider) parseV2(body []byte) (supporters.Notice, error) {
	var ev eventV2
	if err := json.Unmarshal(body, &ev); err != nil || ev.ID == "" || ev.Type == "" {
		return supporters.Notice{}, fmt.Errorf("payjp: %w: the body is not an event", supporters.ErrInvalidPaymentResponse)
	}
	if ev.Livemode != (p.mode == supporters.ModeLive) {
		return supporters.Notice{}, supporters.ErrModeMismatch
	}
	var obj eventObject
	_ = json.Unmarshal(ev.Data, &obj)
	n := supporters.Notice{EventID: ev.ID, Type: ev.Type, Kind: supporters.NoticeIgnored, ObjectID: obj.ID, OccurredAt: ev.CreatedAt.UTC()}
	switch {
	case ev.Type == "checkout.session.completed", ev.Type == "checkout.session.expired":
		n.Kind = supporters.NoticeCheckout
	case ev.Type == "payment_flow.succeeded", strings.HasPrefix(ev.Type, "refund."), ev.Type == evDisputeCreated:
		n.Kind = supporters.NoticeMoney
	}
	return n, nil
}

// Resolve implements supporters.PaymentProvider. A checkout's notice is of a one-time checkout
// when PAY.JP has a Checkout Session of one payment by its ID, and of a subscription's checkout
// otherwise (the pages of API v1 send none).
func (p *Provider) Resolve(ctx context.Context, n supporters.Notice) (supporters.Change, error) {
	switch n.Kind {
	case supporters.NoticeSubscription:
		ref := n.Subscription
		if ref == "" {
			ref = supporters.SubscriptionRef(n.ObjectID)
		}
		return supporters.Change{Subscriptions: []supporters.SubscriptionRef{ref}}, nil
	case supporters.NoticeCheckout:
		ref := supporters.CheckoutRef(n.ObjectID)
		if _, _, _, ok := parseCheckoutRef(ref); ok || ref == "" {
			return supporters.Change{Checkout: ref}, nil
		}
		_, err := p.getSession(ctx, ref)
		switch {
		case err == nil:
			return supporters.Change{OneTimeCheckout: ref}, nil
		case errors.Is(err, supporters.ErrPaymentNotFound):
			return supporters.Change{Checkout: ref}, nil
		}
		return supporters.Change{}, err
	case supporters.NoticeMoney:
		read := p.moneyOf
		if strings.HasPrefix(n.Type, "payment_flow.") || strings.HasPrefix(n.Type, "refund.") {
			read = p.moneyOfV2 // types of API v2 alone
		}
		ms, err := read(ctx, n.ObjectID)
		if err != nil {
			return supporters.Change{}, err
		}
		ch := supporters.Change{Money: ms}
		for _, m := range ms {
			if m.Subscription != "" {
				ch.Subscriptions = append(ch.Subscriptions, m.Subscription)
			}
		}
		return ch, nil
	}
	return supporters.Change{}, nil
}
