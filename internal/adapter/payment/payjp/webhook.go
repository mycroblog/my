// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

// ParseWebhook implements supporters.PaymentProvider (ADR-0099 §4). PAY.JP authenticates a
// delivery by the account's fixed token only, without a time or a signature of the body, so a
// replayed delivery passes; the job re-reads what it points at, so a replay changes nothing.
func (p *Provider) ParseWebhook(header http.Header, body []byte, _ time.Time) (supporters.Notice, error) {
	got := header.Get(TokenHeader)
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(p.o.WebhookToken)) != 1 {
		return supporters.Notice{}, supporters.ErrInvalidSignature
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

// Resolve implements supporters.PaymentProvider.
func (p *Provider) Resolve(ctx context.Context, n supporters.Notice) (supporters.Change, error) {
	switch n.Kind {
	case supporters.NoticeSubscription:
		ref := n.Subscription
		if ref == "" {
			ref = supporters.SubscriptionRef(n.ObjectID)
		}
		return supporters.Change{Subscriptions: []supporters.SubscriptionRef{ref}}, nil
	case supporters.NoticeCheckout:
		return supporters.Change{Checkout: supporters.CheckoutRef(n.ObjectID)}, nil
	case supporters.NoticeMoney:
		ms, err := p.moneyOf(ctx, n.ObjectID)
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
