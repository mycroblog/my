// SPDX-License-Identifier: AGPL-3.0-only

package moderationtest

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/pkg/testkit"
)

// DomainListContract verifies the subscriptions to domain block lists of a database adapter's
// moderation repository by its methods alone, the port contract of ADR-0153 §1: a subscription is
// read back with its newest revision, its head, its last fetch, and its deletion, which is
// recorded once; the subscriptions that are not deleted list newest first in keyset pages; the
// fetches list newest first; the entries are written over and removed by domain or all at once;
// the actions of a subscription in effect are those it issued, neither lifted nor expired, on a
// domain or every one, and an action names its subscription; and the rejections read back by ID.
// The use cases on top of it are DomainListFlows, which internal/flowtest/moderation runs once on
// SQLite.
func DomainListContract(t *testing.T, newHarness ModerationFactory) {
	t.Helper()
	runModRepo(t, newHarness, []modRepoCase{
		{"ReadsBackTheSubscription", domainListRepoSubscription},
		{"KeepsTheEntries", domainListRepoEntries},
		{"KeepsTheActionsAndRejections", domainListRepoActions},
	})
}

func (r *modRepo) domainListSubscription(t *testing.T, url string) moderation.DomainListSubscriptionRecord {
	t.Helper()
	now := r.clock.Now()
	sub := moderation.DomainListSubscription{ID: r.ids.NewID(), URL: url, Format: moderation.DomainListMastodonCSV, CreatedByUserID: testkit.NamedID("mod-1"), CreatedAt: now}
	rev := moderation.DomainListRevision{ID: r.ids.NewID(), SubscriptionID: sub.ID, Settings: moderation.DefaultDomainListSettings("List"), RevisedByUserID: sub.CreatedByUserID, RevisedAt: now}
	head := moderation.DomainListHead{SubscriptionID: sub.ID}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Moderation.InsertDomainListSubscription(ctx, tx, sub); err != nil {
			return err
		}
		if err := r.h.Moderation.InsertDomainListRevision(ctx, tx, rev); err != nil {
			return err
		}
		return r.h.Moderation.PutDomainListHead(ctx, tx, head)
	})
	r.clock.Advance(time.Second)
	return moderation.DomainListSubscriptionRecord{Subscription: sub, Revision: rev, Head: head}
}

func domainListRepoSubscription(t *testing.T, r *modRepo) {
	ctx := t.Context()
	repo := r.h.Moderation
	a := r.domainListSubscription(t, "https://lists.example/a.csv")
	got, err := repo.DomainListSubscription(ctx, nil, a.Subscription.ID)
	if err != nil || !reflect.DeepEqual(got, a) {
		t.Errorf("DomainListSubscription = %+v, %v; want %+v", got, err, a)
	}
	if _, err := repo.DomainListSubscription(ctx, nil, r.ids.NewID()); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("DomainListSubscription(unknown) = %v; want ErrNotFound", err)
	}
	// A new revision is in effect; a fetch and the head are read back with it.
	rev := moderation.DomainListRevision{ID: r.ids.NewID(), SubscriptionID: a.Subscription.ID, Settings: a.Revision.Settings, RevisedAt: r.clock.Now()}
	rev.Settings.Mode, rev.Settings.SuspendAs, rev.Settings.ApplyRejectMedia, rev.Settings.PublicCommentFromList = moderation.DomainListAutomatic, moderation.DomainListAsLimit, false, true
	fetch := moderation.DomainListFetch{
		ID: r.ids.NewID(), SubscriptionID: a.Subscription.ID, Outcome: moderation.DomainListFailed, HTTPStatus: 404,
		ErrorCode: moderation.DomainListErrorHTTPStatus, EntryCount: 3, FetchedAt: r.clock.Now(),
	}
	head := moderation.DomainListHead{SubscriptionID: a.Subscription.ID, ETag: `"e"`, LastModified: "Tue, 10 Nov 2026 09:00:00 GMT", Digest: "abc", ConsecutiveFailures: 2, LastFetchID: fetch.ID}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := repo.LockDomainListSubscription(ctx, tx, a.Subscription.ID); err != nil {
			return err
		}
		if err := repo.InsertDomainListRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := repo.InsertDomainListFetch(ctx, tx, fetch); err != nil {
			return err
		}
		return repo.PutDomainListHead(ctx, tx, head)
	})
	want := a
	want.Revision, want.Head, want.LastFetch = rev, head, &fetch
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := repo.DomainListSubscription(ctx, tx, a.Subscription.ID)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("DomainListSubscription in tx = %+v, %v; want %+v", got, err, want)
		}
		return nil
	})
	r.clock.Advance(time.Second)
	ok := moderation.DomainListFetch{ID: r.ids.NewID(), SubscriptionID: a.Subscription.ID, Outcome: moderation.DomainListApplied, HTTPStatus: 200, EntryCount: 1, FetchedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return repo.InsertDomainListFetch(ctx, tx, ok) })
	fetches, err := repo.ListDomainListFetches(ctx, a.Subscription.ID, "", 10)
	if err != nil || !reflect.DeepEqual(fetches, []moderation.DomainListFetch{ok, fetch}) {
		t.Errorf("ListDomainListFetches = %+v, %v; want newest first", fetches, err)
	}
	if fetches, err := repo.ListDomainListFetches(ctx, a.Subscription.ID, ok.ID, 10); err != nil || len(fetches) != 1 || fetches[0].ID != fetch.ID {
		t.Errorf("ListDomainListFetches(before) = %+v, %v; want the older one", fetches, err)
	}

	b := r.domainListSubscription(t, "https://lists.example/b.csv")
	c := r.domainListSubscription(t, "https://lists.example/c.csv")
	page, err := repo.ListDomainListSubscriptions(ctx, "", 2)
	if err != nil || len(page) != 2 || page[0].Subscription.ID != c.Subscription.ID || page[1].Subscription.ID != b.Subscription.ID {
		t.Errorf("ListDomainListSubscriptions = %+v, %v; want c, b", page, err)
	}
	if page, err = repo.ListDomainListSubscriptions(ctx, b.Subscription.ID, 2); err != nil || len(page) != 1 || !reflect.DeepEqual(page[0], want) {
		t.Errorf("ListDomainListSubscriptions(before b) = %+v, %v; want a as read", page, err)
	}
	del := moderation.DomainListDeletion{SubscriptionID: b.Subscription.ID, LiftActions: true, DeletedByUserID: testkit.NamedID("mod-1"), DeletedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return repo.InsertDomainListDeletion(ctx, tx, del) })
	if err := r.txErr(t, func(ctx context.Context, tx uow.Tx) error { return repo.InsertDomainListDeletion(ctx, tx, del) }); !errors.Is(err, appmoderation.ErrAlreadyRecorded) {
		t.Errorf("InsertDomainListDeletion twice = %v; want ErrAlreadyRecorded", err)
	}
	if got, err := repo.DomainListSubscription(ctx, nil, b.Subscription.ID); err != nil || got.Deletion == nil || !reflect.DeepEqual(*got.Deletion, del) {
		t.Errorf("DomainListSubscription(deleted) = %+v, %v; want the deletion", got, err)
	}
	if page, err = repo.ListDomainListSubscriptions(ctx, "", 10); err != nil || len(page) != 2 {
		t.Errorf("ListDomainListSubscriptions = %d, %v; want the 2 not deleted", len(page), err)
	}
}

func domainListRepoEntries(t *testing.T, r *modRepo) {
	ctx := t.Context()
	repo := r.h.Moderation
	s := r.domainListSubscription(t, "https://lists.example/e.csv")
	id := s.Subscription.ID
	f := r.ids.NewID()
	entries := []moderation.DomainListEntry{
		{SubscriptionID: id, Domain: "a.example", Severity: moderation.DomainBlockSuspend, RejectMedia: true, PublicComment: "Spam", SinceFetchID: f},
		{SubscriptionID: id, Domain: "b.example", Severity: "block", RejectReports: true, Obfuscate: true, SinceFetchID: f},
		{SubscriptionID: id, Domain: "c.example", Severity: moderation.DomainBlockSilence, SinceFetchID: f},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return repo.PutDomainListEntries(ctx, tx, entries) })
	if got, err := repo.DomainListEntries(ctx, nil, id); err != nil || !reflect.DeepEqual(got, entries) {
		t.Errorf("DomainListEntries = %+v, %v; want %+v", got, err, entries)
	}
	changed := entries[0]
	changed.Severity, changed.PublicComment, changed.SinceFetchID = moderation.DomainBlockSilence, "", r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := repo.PutDomainListEntries(ctx, tx, []moderation.DomainListEntry{changed}); err != nil {
			return err
		}
		return repo.DeleteDomainListEntries(ctx, tx, id, []string{"c.example"})
	})
	if got, ok, err := repo.DomainListEntry(ctx, nil, id, "a.example"); err != nil || !ok || !reflect.DeepEqual(got, changed) {
		t.Errorf("DomainListEntry(a) = %+v, %v, %v; want %+v", got, ok, err, changed)
	}
	if _, ok, err := repo.DomainListEntry(ctx, nil, id, "c.example"); err != nil || ok {
		t.Errorf("DomainListEntry(removed) = %v, %v; want none", ok, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return repo.DeleteDomainListEntries(ctx, tx, id, nil) })
	if got, err := repo.DomainListEntries(ctx, nil, id); err != nil || len(got) != 0 {
		t.Errorf("DomainListEntries after removing all = %+v, %v", got, err)
	}
}

func domainListRepoActions(t *testing.T, r *modRepo) {
	ctx := t.Context()
	repo := r.h.Moderation
	s := r.domainListSubscription(t, "https://lists.example/x.csv")
	id := s.Subscription.ID
	now := r.clock.Now()
	action := func(domain string, kind moderation.DomainActionKind, expires *time.Time) moderation.DomainAction {
		return moderation.DomainAction{ID: r.ids.NewID(), Domain: domain, Kind: kind, IssuedAt: now, ExpiresAt: expires}
	}
	a := action("a.example", moderation.DomainSuspend, nil)
	b := action("b.example", moderation.DomainLimit, nil)
	lifted := action("a.example", moderation.DomainRejectMedia, nil)
	expired := action("a.example", moderation.DomainLimit, r.at(time.Hour))
	human := action("a.example", moderation.DomainRejectReports, nil)
	fetch := r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, x := range []moderation.DomainAction{a, b, lifted, expired, human} {
			if err := repo.InsertDomainAction(ctx, tx, x); err != nil {
				return err
			}
		}
		for _, x := range []moderation.DomainAction{a, b, lifted, expired} {
			if err := repo.InsertSubscriptionDomainAction(ctx, tx, moderation.SubscriptionDomainAction{ActionID: x.ID, SubscriptionID: id, FetchID: fetch}); err != nil {
				return err
			}
		}
		return repo.InsertDomainActionLift(ctx, tx, moderation.DomainLift{ActionID: lifted.ID, LiftedAt: now})
	})
	later := now.Add(2 * time.Hour)
	ids := func(as []moderation.DomainAction) []string {
		out := make([]string, len(as))
		for i, x := range as {
			out[i] = x.ID
		}
		return out
	}
	if got, err := repo.SubscriptionDomainActionsInEffect(ctx, nil, id, "", later); err != nil || !reflect.DeepEqual(ids(got), []string{a.ID, b.ID}) || !reflect.DeepEqual(got[0], a) {
		t.Errorf("SubscriptionDomainActionsInEffect = %+v, %v; want a and b", got, err)
	}
	if got, err := repo.SubscriptionDomainActionsInEffect(ctx, nil, id, "a.example", now); err != nil || !reflect.DeepEqual(ids(got), []string{a.ID, expired.ID}) {
		t.Errorf("SubscriptionDomainActionsInEffect(a.example, before the expiry) = %+v, %v; want a and the one not expired yet", got, err)
	}
	if got, ok, err := repo.SubscriptionOfDomainAction(ctx, nil, a.ID); err != nil || !ok || got != (moderation.SubscriptionDomainAction{ActionID: a.ID, SubscriptionID: id, FetchID: fetch}) {
		t.Errorf("SubscriptionOfDomainAction(a) = %+v, %v, %v", got, ok, err)
	}
	if _, ok, err := repo.SubscriptionOfDomainAction(ctx, nil, human.ID); err != nil || ok {
		t.Errorf("SubscriptionOfDomainAction(a person's) = %v, %v; want none", ok, err)
	}
	if rec, err := repo.FindDomainActionRecord(ctx, a.ID); err != nil || rec.SubscriptionID != id {
		t.Errorf("FindDomainActionRecord(a) = %+v, %v; want the subscription", rec, err)
	}
	if recs, err := repo.ListDomainActions(ctx, appmoderation.DomainActionQuery{Domain: "a.example"}, 10); err != nil || len(recs) != 4 || recs[0].SubscriptionID != "" || recs[1].SubscriptionID != id {
		t.Errorf("ListDomainActions(a.example) = %+v, %v; want the person's newest, without a subscription", recs, err)
	}
	rejections := []moderation.DomainListRejection{
		{ID: r.ids.NewID(), SubscriptionID: id, Change: moderation.DomainListIssue, Domain: "c.example", Kind: moderation.DomainSuspend, EntrySinceFetchID: fetch, RejectedByUserID: testkit.NamedID("mod-1"), RejectedAt: now},
		{ID: r.ids.NewID(), SubscriptionID: id, Change: moderation.DomainListLift, Domain: "b.example", Kind: moderation.DomainLimit, ActionID: b.ID, RejectedAt: now},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, x := range rejections {
			if err := repo.InsertDomainListRejection(ctx, tx, x); err != nil {
				return err
			}
		}
		return nil
	})
	if got, err := repo.DomainListRejections(ctx, nil, id); err != nil || !reflect.DeepEqual(got, rejections) {
		t.Errorf("DomainListRejections = %+v, %v; want %+v", got, err, rejections)
	}
}
