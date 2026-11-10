// SPDX-License-Identifier: AGPL-3.0-only

package notificationtest

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// The contract of PolicyRepository (ADR-0158), part of NotificationContract.

// notificationRepoPolicies checks that the policy choices are saved, replaced per condition, and
// read for one account and for many.
func notificationRepoPolicies(t *testing.T, r *notificationRepo) {
	alice, bob := r.ids.NewID(), r.ids.NewID()
	save := func(choices ...notification.PolicyChoice) {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.SavePolicyChoices(ctx, tx, choices) })
	}
	at := r.clock.Now()
	save(
		notification.PolicyChoice{AccountID: alice, Condition: notification.ConditionNotFollowing, Action: notification.ActionFilter, UpdatedAt: at},
		notification.PolicyChoice{AccountID: alice, Condition: notification.ConditionNewAccount, Action: notification.ActionDrop, UpdatedAt: at},
		notification.PolicyChoice{AccountID: bob, Condition: notification.ConditionLimitedAccount, Action: notification.ActionFilter, UpdatedAt: at},
	)
	later := at.Add(time.Minute)
	save(notification.PolicyChoice{AccountID: alice, Condition: notification.ConditionNotFollowing, Action: notification.ActionAccept, UpdatedAt: later})
	got, err := r.repo.PolicyChoices(t.Context(), alice)
	if err != nil {
		t.Fatal(err)
	}
	want := []notification.PolicyChoice{
		{AccountID: alice, Condition: notification.ConditionNewAccount, Action: notification.ActionDrop, UpdatedAt: at},
		{AccountID: alice, Condition: notification.ConditionNotFollowing, Action: notification.ActionAccept, UpdatedAt: later},
	}
	if !samePolicyChoices(got, want) {
		t.Errorf("PolicyChoices = %+v, want %+v", got, want)
	}
	var of map[string][]notification.PolicyChoice
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		var err error
		of, err = r.repo.PolicyChoicesOfTx(ctx, tx, []string{alice, bob, r.ids.NewID()})
		return err
	})
	if len(of) != 2 || !samePolicyChoices(of[alice], want) || len(of[bob]) != 1 || of[bob][0].Action != notification.ActionFilter {
		t.Errorf("PolicyChoicesOfTx = %+v", of)
	}
}

func samePolicyChoices(got, want []notification.PolicyChoice) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		g, w := got[i], want[i]
		if g.AccountID != w.AccountID || g.Condition != w.Condition || g.Action != w.Action || !g.UpdatedAt.Equal(w.UpdatedAt) {
			return false
		}
	}
	return true
}

// filterInto inserts a notification of typ to recipient from source and records that the policy
// filtered it, deriving the request again, as the projectors do.
func (r *notificationRepo) filterInto(t *testing.T, recipient string, typ notification.Type, source string) notification.Notification {
	t.Helper()
	n := r.notify(t, recipient, typ, source)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.InsertFilterings(ctx, tx, []notification.Notification{n}); err != nil {
			return err
		}
		return r.repo.RefreshRequests(ctx, tx, []appnotification.RequestPair{{RecipientAccountID: recipient, SourceAccountID: source}})
	})
	return n
}

// requests returns the recipient's requests, each without its newest notification once it is
// checked to be the one the request names.
func (r *notificationRepo) requests(t *testing.T, recipient, before string, limit int) []notification.Request {
	t.Helper()
	got, err := r.repo.Requests(t.Context(), recipient, before, limit, r.clock.Now())
	if err != nil {
		t.Fatalf("Requests: %v", err)
	}
	for i := range got {
		got[i] = withoutLast(t, got[i])
	}
	return got
}

func withoutLast(t *testing.T, req notification.Request) notification.Request {
	t.Helper()
	if req.Last.ID != req.LastNotificationID || req.Last.SourceAccountID != req.SourceAccountID || req.Last.RecipientAccountID != req.RecipientAccountID {
		t.Errorf("request %+v carries the notification %+v", req, req.Last)
	}
	req.Last = notification.Notification{}
	return req
}

func (r *notificationRepo) requestNotifications(t *testing.T, recipient, source string) []string {
	t.Helper()
	ns, err := r.repo.RequestNotifications(t.Context(), recipient, source, "", 10)
	if err != nil {
		t.Fatalf("RequestNotifications: %v", err)
	}
	return notificationIDs(ns)
}

func (r *notificationRepo) refresh(t *testing.T, recipient, source string) {
	t.Helper()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.RefreshRequests(ctx, tx, []appnotification.RequestPair{{RecipientAccountID: recipient, SourceAccountID: source}})
	})
}

// notificationRepoRequests checks that a filtered notification is out of the list, the count,
// and the check of one notification, and waits in its source's request, which counts it and
// pages newest first, until the recipient accepts the source: then it is listed, and the request
// is gone.
func notificationRepoRequests(t *testing.T, r *notificationRepo) {
	ctx := t.Context()
	alice, stranger, other, friend := r.ids.NewID(), r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	listed := r.notify(t, alice, notification.TypeFollow, friend)
	first := r.filterInto(t, alice, notification.TypeMention, stranger)
	second := r.filterInto(t, alice, notification.TypeFavourite, stranger)
	third := r.filterInto(t, alice, notification.TypeFollow, other)

	if got := r.list(t, alice, nil, appnotification.Hidden{}, "", 10); !slices.Equal(got, []string{listed.ID}) {
		t.Errorf("list = %v, want only %s", got, listed.ID)
	}
	if n, err := r.repo.CountNotificationsAfter(ctx, alice, "", appnotification.Hidden{}, r.clock.Now()); err != nil || n != 1 {
		t.Errorf("CountNotificationsAfter = %d, %v; want 1", n, err)
	}
	if ok, err := r.repo.NotificationListed(ctx, alice, first.ID, appnotification.Hidden{}, r.clock.Now()); err != nil || ok {
		t.Errorf("NotificationListed(filtered) = %v, %v", ok, err)
	}
	// The filtered notifications all come after the listed one.
	after, err := r.repo.ListNotificationsAfter(ctx, alice, nil, appnotification.Hidden{}, listed.ID, 10, r.clock.Now())
	if err != nil || len(after) != 0 {
		t.Errorf("ListNotificationsAfter = %v, %v", notificationIDs(after), err)
	}

	want := []notification.Request{
		{RecipientAccountID: alice, SourceAccountID: other, NotificationsCount: 1, LastNotificationID: third.ID},
		{RecipientAccountID: alice, SourceAccountID: stranger, NotificationsCount: 2, LastNotificationID: second.ID},
	}
	if got := r.requests(t, alice, "", 10); !reflect.DeepEqual(got, want) {
		t.Errorf("Requests = %+v, want %+v", got, want)
	}
	if got := r.requests(t, alice, "", 1); !reflect.DeepEqual(got, want[:1]) {
		t.Errorf("Requests page 1 = %+v", got)
	}
	if got := r.requests(t, alice, third.ID, 1); !reflect.DeepEqual(got, want[1:]) {
		t.Errorf("Requests page 2 = %+v", got)
	}
	if got, err := r.repo.Request(ctx, alice, stranger, r.clock.Now()); err != nil || withoutLast(t, got) != want[1] {
		t.Errorf("Request = %+v, %v", got, err)
	}
	if _, err := r.repo.Request(ctx, alice, friend, r.clock.Now()); !errors.Is(err, appnotification.ErrNotFound) {
		t.Errorf("Request(no request) = %v, want ErrNotFound", err)
	}
	if s, err := r.repo.RequestSummary(ctx, alice, r.clock.Now()); err != nil || s != (notification.RequestSummary{Accounts: 2, Notifications: 3}) {
		t.Errorf("RequestSummary = %+v, %v", s, err)
	}
	if got := r.requestNotifications(t, alice, stranger); !slices.Equal(got, []string{second.ID, first.ID}) {
		t.Errorf("RequestNotifications = %v", got)
	}

	var accepted bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		a := notification.AcceptedSource{RecipientAccountID: alice, SourceAccountID: stranger, CreatedAt: r.clock.Now()}
		if err := r.repo.InsertAcceptedSource(ctx, tx, a); err != nil {
			return err
		}
		if err := r.repo.InsertAcceptedSource(ctx, tx, a); err != nil { // again changes nothing
			return err
		}
		if err := r.repo.RefreshRequests(ctx, tx, []appnotification.RequestPair{{RecipientAccountID: alice, SourceAccountID: stranger}}); err != nil {
			return err
		}
		var err error
		accepted, err = r.repo.SourceAcceptedTx(ctx, tx, alice, stranger)
		return err
	})
	if !accepted {
		t.Error("SourceAcceptedTx = false after the acceptance")
	}
	if got := r.list(t, alice, nil, appnotification.Hidden{}, "", 10); !slices.Equal(got, []string{second.ID, first.ID, listed.ID}) {
		t.Errorf("list after the acceptance = %v", got)
	}
	if got := r.requests(t, alice, "", 10); !reflect.DeepEqual(got, want[:1]) {
		t.Errorf("Requests after the acceptance = %+v", got)
	}
	if got := r.requestNotifications(t, alice, stranger); len(got) != 0 {
		t.Errorf("RequestNotifications after the acceptance = %v", got)
	}
	var tx notification.Request
	r.tx(t, func(ctx context.Context, txn uow.Tx) error {
		var err error
		if tx, err = r.repo.RequestTx(ctx, txn, alice, stranger); errors.Is(err, appnotification.ErrNotFound) {
			return nil
		}
		return err
	})
	if tx != (notification.Request{}) {
		t.Errorf("RequestTx after the acceptance = %+v", tx)
	}
}

// notificationRepoRequestDismissals checks that a dismissal takes the notifications up to its
// last one out of the request and, also after an acceptance, out of the list, while a later
// filtered notification opens the request again; and that the request of a source that the
// recipient excludes is hidden but still found inside a transaction.
func notificationRepoRequestDismissals(t *testing.T, r *notificationRepo) {
	ctx := t.Context()
	alice, stranger, muted := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	r.filterInto(t, alice, notification.TypeMention, stranger)
	old := r.filterInto(t, alice, notification.TypeMention, stranger)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.InsertRequestDismissal(ctx, tx, notification.RequestDismissal{
			ID: r.ids.NewID(), RecipientAccountID: alice, SourceAccountID: stranger, LastNotificationID: old.ID, DismissedAt: r.clock.Now(),
		})
	})
	r.refresh(t, alice, stranger)
	if got := r.requests(t, alice, "", 10); len(got) != 0 {
		t.Errorf("Requests after the dismissal = %+v", got)
	}
	later := r.filterInto(t, alice, notification.TypeBoost, stranger)
	want := []notification.Request{{RecipientAccountID: alice, SourceAccountID: stranger, NotificationsCount: 1, LastNotificationID: later.ID}}
	if got := r.requests(t, alice, "", 10); !reflect.DeepEqual(got, want) {
		t.Errorf("Requests after a later notification = %+v, want %+v", got, want)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.InsertAcceptedSource(ctx, tx, notification.AcceptedSource{RecipientAccountID: alice, SourceAccountID: stranger, CreatedAt: r.clock.Now()})
	})
	r.refresh(t, alice, stranger)
	if got := r.list(t, alice, nil, appnotification.Hidden{}, "", 10); !slices.Equal(got, []string{later.ID}) {
		t.Errorf("list after the acceptance = %v, want only the notification after the dismissal", got)
	}

	hidden := r.filterInto(t, alice, notification.TypeFollow, muted)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.UpsertExclusion(ctx, tx, notification.Exclusion{ViewerAccountID: alice, TargetAccountID: muted, Reason: notification.ExcludedMute})
	})
	if got := r.requests(t, alice, "", 10); len(got) != 0 {
		t.Errorf("Requests with the source muted = %+v", got)
	}
	if _, err := r.repo.Request(ctx, alice, muted, r.clock.Now()); !errors.Is(err, appnotification.ErrNotFound) {
		t.Errorf("Request of a muted source = %v, want ErrNotFound", err)
	}
	if s, err := r.repo.RequestSummary(ctx, alice, r.clock.Now()); err != nil || s != (notification.RequestSummary{}) {
		t.Errorf("RequestSummary with the source muted = %+v, %v", s, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.repo.RequestTx(ctx, tx, alice, muted)
		if err == nil && got.LastNotificationID != hidden.ID {
			t.Errorf("RequestTx of a muted source = %+v", got)
		}
		return err
	})
}

// notificationRepoRebuildRequests checks that RebuildRequests derives the requests that the
// refreshes kept.
func notificationRepoRebuildRequests(t *testing.T, r *notificationRepo) {
	alice, bob, stranger, accepted := r.ids.NewID(), r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	r.filterInto(t, alice, notification.TypeMention, stranger)
	r.filterInto(t, bob, notification.TypeMention, stranger)
	r.filterInto(t, alice, notification.TypeFollow, accepted)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.InsertAcceptedSource(ctx, tx, notification.AcceptedSource{RecipientAccountID: alice, SourceAccountID: accepted, CreatedAt: r.clock.Now()})
	})
	r.refresh(t, alice, accepted)
	before := append(r.requests(t, alice, "", 10), r.requests(t, bob, "", 10)...)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.RebuildRequests(ctx, tx) })
	after := append(r.requests(t, alice, "", 10), r.requests(t, bob, "", 10)...)
	if len(before) != 2 || !reflect.DeepEqual(before, after) {
		t.Errorf("Requests rebuilt = %+v, want %+v", after, before)
	}
}
