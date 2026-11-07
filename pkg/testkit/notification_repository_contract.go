// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// NotificationContract verifies the notification repository of a database adapter by its methods
// alone, the port contract of ADR-0153 §1: notifications inserted once per recipient and dedupe
// key, also concurrently, and found by ID; the lists newest and oldest first with their types,
// hidden types, shown announcements, and pages, and the count and the check of one notification
// that read the same; the dismissals, exclusions in force, domain exclusions of a domain and its
// subdomains, and deleted statuses that hide notifications; the read marker, which only moves
// forward; the settings, replaced per group and channel; the error group notices of an hour, also
// added concurrently, and their prune; the projections emptied for a rebuild with the statuses
// notified of; and the subscriptions to new posts and the requests for them. The notification use
// cases on top of it are NotificationFlows, which internal/flowtest/notification runs once on
// SQLite.
func NotificationContract(t *testing.T, newHarness NotificationFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *notificationRepo)
	}{
		{"InsertsAndFindsNotifications", notificationRepoInserts},
		{"InsertsOnceConcurrently", notificationRepoConcurrentInserts},
		{"ListsWithTypesAndPages", notificationRepoLists},
		{"HidesDismissedExcludedAndDeleted", notificationRepoHides},
		{"AdvancesTheReadMarker", notificationRepoReadMarker},
		{"ReplacesSettings", notificationRepoSettings},
		{"CountsErrorGroupNotices", notificationRepoErrorGroupNotices},
		{"EmptiesTheProjections", notificationRepoRebuild},
		{"KeepsSubscriptions", notificationRepoSubscriptions},
		{"KeepsSubscriptionRequests", notificationRepoSubscriptionRequests},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			tc.run(t, &notificationRepo{h: h, repo: h.Notifications, clock: clock, ids: NewIDGenerator(clock)})
		})
	}
}

type notificationRepo struct {
	h     NotificationHarness
	repo  appnotification.Repository
	clock *Clock
	ids   *IDGenerator
}

func (r *notificationRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// insert inserts n with an ID and a time when it has none, a dedupe key of its ID when it has
// none, and reports whether it was inserted; the clock moves on a second.
func (r *notificationRepo) insert(t *testing.T, n notification.Notification) (notification.Notification, bool) {
	t.Helper()
	if n.ID == "" {
		n.ID = r.ids.NewID()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = r.clock.Now()
	}
	if n.DedupeKey == "" {
		n.DedupeKey = string(n.Type) + ":" + n.ID
	}
	var inserted bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		var err error
		inserted, err = r.repo.InsertNotification(ctx, tx, n)
		return err
	})
	r.clock.Advance(time.Second)
	return n, inserted
}

// notify inserts a notification of type typ to recipient from source ("" for the instance).
func (r *notificationRepo) notify(t *testing.T, recipient string, typ notification.Type, source string) notification.Notification {
	t.Helper()
	n, ok := r.insert(t, notification.Notification{RecipientAccountID: recipient, Type: typ, SourceAccountID: source})
	if !ok {
		t.Fatalf("InsertNotification(%+v) inserted nothing", n)
	}
	return n
}

// list returns the IDs of the recipient's visible notifications at the clock's time, newest first.
func (r *notificationRepo) list(t *testing.T, recipient string, types []notification.Type, hidden appnotification.Hidden, before string, limit int) []string {
	t.Helper()
	ns, err := r.repo.ListNotifications(t.Context(), recipient, types, hidden, before, limit, r.clock.Now())
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	return notificationIDs(ns)
}

func notificationIDs(ns []notification.Notification) []string {
	out := []string{}
	for _, n := range ns {
		out = append(out, n.ID)
	}
	return out
}

// sameNotification reports whether got is want as stored, with its times compared as instants.
func sameNotification(got, want notification.Notification) bool {
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.ErrorGroupHour.Equal(want.ErrorGroupHour) {
		return false
	}
	got.CreatedAt, got.ErrorGroupHour = want.CreatedAt, want.ErrorGroupHour
	return reflect.DeepEqual(got, want)
}

func notificationRepoInserts(t *testing.T, r *notificationRepo) {
	alice, bob := r.ids.NewID(), r.ids.NewID()
	mention, ok := r.insert(t, notification.Notification{
		RecipientAccountID: alice, Type: notification.TypeMention, SourceAccountID: bob, SourceDomain: "remote.example",
		StatusID: r.ids.NewID(), DedupeKey: "mention:1",
	})
	if !ok {
		t.Fatal("InsertNotification(mention) inserted nothing")
	}
	others, _ := r.insert(t, notification.Notification{
		RecipientAccountID: alice, Type: notification.TypeAdminErrorGroup, ErrorGroupChange: notification.ErrorGroupOthers,
		ErrorGroupHour: notification.ErrorGroupHour(r.clock.Now()), DedupeKey: "others:1",
	})
	closure, _ := r.insert(t, notification.Notification{
		RecipientAccountID: alice, Type: notification.TypeServiceClosure, ServiceClosureEventID: r.ids.NewID(),
		ServiceClosureChange: notification.ClosureAnnounced, DedupeKey: "closure:1",
	})
	severed, _ := r.insert(t, notification.Notification{
		RecipientAccountID: alice, Type: notification.TypeRelationshipsSevered, SeveranceID: r.ids.NewID(), DedupeKey: "severed:1",
	})
	voice, _ := r.insert(t, notification.Notification{
		RecipientAccountID: alice, Type: notification.TypeVoiceRoomStarted, SourceAccountID: bob, VoiceRoomID: r.ids.NewID(), DedupeKey: "voice:1",
	})
	for _, want := range []notification.Notification{mention, others, closure, severed, voice} {
		var got notification.Notification
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			var err error
			got, err = r.repo.Notification(ctx, tx, want.ID)
			return err
		})
		if !sameNotification(got, want) {
			t.Errorf("Notification(%s) = %+v, want %+v", want.ID, got, want)
		}
	}

	// The same dedupe key inserts nothing for the same recipient, and inserts for another.
	if _, ok := r.insert(t, notification.Notification{RecipientAccountID: alice, Type: notification.TypeMention, SourceAccountID: bob, DedupeKey: "mention:1"}); ok {
		t.Error("InsertNotification of a dedupe key again inserted it")
	}
	if _, ok := r.insert(t, notification.Notification{RecipientAccountID: bob, Type: notification.TypeMention, SourceAccountID: alice, DedupeKey: "mention:1"}); !ok {
		t.Error("InsertNotification of another recipient's dedupe key inserted nothing")
	}
	if got := r.list(t, alice, nil, appnotification.Hidden{}, "", 10); len(got) != 5 {
		t.Errorf("notifications of alice = %v, want 5", got)
	}

	err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		_, err := r.repo.Notification(t.Context(), tx, r.ids.NewID())
		return err
	})
	if !errors.Is(err, appnotification.ErrNotFound) {
		t.Errorf("Notification(unknown) = %v, want ErrNotFound", err)
	}
}

func notificationRepoConcurrentInserts(t *testing.T, r *notificationRepo) {
	alice, bob := r.ids.NewID(), r.ids.NewID()
	const n = 8
	var wg sync.WaitGroup
	inserted := make([]bool, n)
	errs := make([]error, n)
	for i := range n {
		id := r.ids.NewID()
		wg.Go(func() {
			errs[i] = r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
				var err error
				inserted[i], err = r.repo.InsertNotification(t.Context(), tx, notification.Notification{
					ID: id, RecipientAccountID: alice, Type: notification.TypeFavourite, SourceAccountID: bob,
					DedupeKey: "favourite:1", CreatedAt: r.clock.Now(),
				})
				return err
			})
		})
	}
	wg.Wait()
	count := 0
	for i := range n {
		if errs[i] != nil {
			t.Errorf("InsertNotification %d = %v", i, errs[i])
		}
		if inserted[i] {
			count++
		}
	}
	if got := r.list(t, alice, nil, appnotification.Hidden{}, "", 10); count != 1 || len(got) != 1 {
		t.Errorf("%d concurrent inserts of one dedupe key inserted %d, listed %v; want 1", n, count, got)
	}
}

func notificationRepoLists(t *testing.T, r *notificationRepo) {
	ctx := t.Context()
	alice, bob, carol := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	follow := r.notify(t, alice, notification.TypeFollow, bob)
	mention := r.notify(t, alice, notification.TypeMention, bob)
	report := r.notify(t, alice, notification.TypeAdminReport, "")
	shown, _ := r.insert(t, notification.Notification{RecipientAccountID: alice, Type: notification.TypeAnnouncement, AnnouncementID: r.ids.NewID()})
	ended, _ := r.insert(t, notification.Notification{RecipientAccountID: alice, Type: notification.TypeAnnouncement, AnnouncementID: r.ids.NewID()})
	favourite := r.notify(t, alice, notification.TypeFavourite, carol)
	r.notify(t, bob, notification.TypeFollow, alice)
	hidden := appnotification.Hidden{ShownAnnouncements: []string{shown.AnnouncementID}}

	all := []string{favourite.ID, shown.ID, report.ID, mention.ID, follow.ID}
	if got := r.list(t, alice, nil, hidden, "", 10); !slices.Equal(got, all) {
		t.Errorf("ListNotifications = %v, want %v, newest first without the ended announcement", got, all)
	}
	if got := r.list(t, alice, nil, appnotification.Hidden{ShownAnnouncements: []string{shown.AnnouncementID, ended.AnnouncementID}}, "", 10); len(got) != 6 {
		t.Errorf("ListNotifications with both announcements shown = %v, want 6", got)
	}
	if got := r.list(t, alice, nil, hidden, "", 2); !slices.Equal(got, all[:2]) {
		t.Errorf("ListNotifications(limit 2) = %v, want %v", got, all[:2])
	}
	if got := r.list(t, alice, nil, hidden, report.ID, 10); !slices.Equal(got, all[3:]) {
		t.Errorf("ListNotifications(before the report) = %v, want %v", got, all[3:])
	}
	if got := r.list(t, alice, []notification.Type{notification.TypeFollow, notification.TypeFavourite}, hidden, "", 10); !slices.Equal(got, []string{favourite.ID, follow.ID}) {
		t.Errorf("ListNotifications(follow, favourite) = %v", got)
	}
	withoutReports := appnotification.Hidden{Types: []notification.Type{notification.TypeAdminReport}, ShownAnnouncements: hidden.ShownAnnouncements}
	if got := r.list(t, alice, nil, withoutReports, "", 10); !slices.Equal(got, []string{favourite.ID, shown.ID, mention.ID, follow.ID}) {
		t.Errorf("ListNotifications hiding admin_report = %v", got)
	}

	after, err := r.repo.ListNotificationsAfter(ctx, alice, nil, hidden, follow.ID, 2, r.clock.Now())
	if got := notificationIDs(after); err != nil || !slices.Equal(got, []string{mention.ID, report.ID}) {
		t.Errorf("ListNotificationsAfter(the follow, 2) = %v, %v; want the next two, oldest first", got, err)
	}
	after, err = r.repo.ListNotificationsAfter(ctx, alice, []notification.Type{notification.TypeFavourite}, hidden, follow.ID, 10, r.clock.Now())
	if got := notificationIDs(after); err != nil || !slices.Equal(got, []string{favourite.ID}) {
		t.Errorf("ListNotificationsAfter(favourite) = %v, %v", got, err)
	}

	for _, c := range []struct {
		id     string
		hidden appnotification.Hidden
		want   bool
	}{
		{mention.ID, hidden, true}, {ended.ID, hidden, false}, {report.ID, withoutReports, false}, {r.ids.NewID(), hidden, false},
	} {
		if ok, err := r.repo.NotificationListed(ctx, alice, c.id, c.hidden, r.clock.Now()); err != nil || ok != c.want {
			t.Errorf("NotificationListed(%s) = %v, %v; want %v", c.id, ok, err, c.want)
		}
	}
	if ok, err := r.repo.NotificationListed(ctx, bob, mention.ID, hidden, r.clock.Now()); err != nil || ok {
		t.Errorf("NotificationListed(another recipient's) = %v, %v; want false", ok, err)
	}

	if n, err := r.repo.CountNotificationsAfter(ctx, alice, "", hidden, r.clock.Now()); err != nil || n != 5 {
		t.Errorf("CountNotificationsAfter(all) = %d, %v; want 5", n, err)
	}
	if n, err := r.repo.CountNotificationsAfter(ctx, alice, mention.ID, withoutReports, r.clock.Now()); err != nil || n != 2 {
		t.Errorf("CountNotificationsAfter(the mention, without reports) = %d, %v; want 2", n, err)
	}
}

func notificationRepoHides(t *testing.T, r *notificationRepo) {
	ctx := t.Context()
	alice, bob, carol, dave := r.ids.NewID(), r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	fromBob := r.notify(t, alice, notification.TypeFollow, bob)
	fromCarol := r.notify(t, alice, notification.TypeFollow, carol)
	fromDave := r.notify(t, alice, notification.TypeFollow, dave)
	fromRemote, _ := r.insert(t, notification.Notification{
		RecipientAccountID: alice, Type: notification.TypeFollow, SourceAccountID: r.ids.NewID(), SourceDomain: "social.muted.example",
	})
	fromOther, _ := r.insert(t, notification.Notification{
		RecipientAccountID: alice, Type: notification.TypeFollow, SourceAccountID: r.ids.NewID(), SourceDomain: "notmuted.example",
	})
	status := r.ids.NewID()
	aboutStatus, _ := r.insert(t, notification.Notification{RecipientAccountID: alice, Type: notification.TypeFavourite, SourceAccountID: bob, StatusID: status})
	visible := func() []string { return r.list(t, alice, nil, appnotification.Hidden{}, "", 10) }
	if got := visible(); len(got) != 6 {
		t.Fatalf("notifications = %v, want 6", got)
	}

	// A dismissal is recorded once and hides its notification.
	for i, want := range []bool{true, false} {
		var ok bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			var err error
			ok, err = r.repo.InsertDismissal(ctx, tx, notification.Dismissal{NotificationID: fromDave.ID, DismissedAt: r.clock.Now()})
			return err
		})
		if ok != want {
			t.Errorf("InsertDismissal #%d = %v, want %v", i+1, ok, want)
		}
	}
	if has, err := r.statusHasNotifications(t, status); err != nil || !has {
		t.Errorf("StatusHasNotifications = %v, %v; want true", has, err)
	}

	// An exclusion hides its target's notifications while it is in force; upserting it replaces its expiry.
	soon := r.clock.Now().Add(time.Hour)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.repo.UpsertExclusion(ctx, tx, notification.Exclusion{ViewerAccountID: alice, TargetAccountID: bob, Reason: notification.ExcludedBlock}),
			r.repo.UpsertExclusion(ctx, tx, notification.Exclusion{ViewerAccountID: alice, TargetAccountID: carol, Reason: notification.ExcludedMute, ExpiresAt: &soon}),
			r.repo.UpsertExclusion(ctx, tx, notification.Exclusion{ViewerAccountID: carol, TargetAccountID: alice, Reason: notification.ExcludedMute}),
			r.repo.InsertDomainExclusion(ctx, tx, alice, "muted.example"),
			r.repo.InsertDomainExclusion(ctx, tx, alice, "muted.example"),
			r.repo.InsertDeletedStatus(ctx, tx, status),
			r.repo.InsertDeletedStatus(ctx, tx, status),
		)
	})
	if got, want := visible(), []string{fromOther.ID}; !slices.Equal(got, want) {
		t.Errorf("with the exclusions = %v, want %v", got, want)
	}
	if n, err := r.repo.CountNotificationsAfter(ctx, alice, "", appnotification.Hidden{}, r.clock.Now()); err != nil || n != 1 {
		t.Errorf("CountNotificationsAfter with the exclusions = %d, %v; want 1", n, err)
	}
	if ok, err := r.repo.NotificationListed(ctx, alice, aboutStatus.ID, appnotification.Hidden{}, r.clock.Now()); err != nil || ok {
		t.Errorf("NotificationListed(about a deleted status) = %v, %v; want false", ok, err)
	}
	for domain, want := range map[string]bool{"muted.example": true, "a.b.muted.example": true, "notmuted.example": false, "example": false} {
		var got bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			var err error
			got, err = r.repo.DomainExcluded(ctx, tx, alice, domain)
			return err
		})
		if got != want {
			t.Errorf("DomainExcluded(%s) = %v, want %v", domain, got, want)
		}
	}

	// The mute expires; the block and the domain mute go when deleted.
	r.clock.Advance(2 * time.Hour)
	if got, want := visible(), []string{fromOther.ID, fromCarol.ID}; !slices.Equal(got, want) {
		t.Errorf("after the mute expired = %v, want %v", got, want)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		later := r.clock.Now().Add(time.Hour)
		return errors.Join(
			r.repo.UpsertExclusion(ctx, tx, notification.Exclusion{ViewerAccountID: alice, TargetAccountID: carol, Reason: notification.ExcludedMute, ExpiresAt: &later}),
			r.repo.DeleteExclusion(ctx, tx, alice, bob, notification.ExcludedBlock),
			r.repo.DeleteExclusion(ctx, tx, alice, bob, notification.ExcludedMute),
			r.repo.DeleteDomainExclusion(ctx, tx, alice, "muted.example"),
		)
	})
	if got, want := visible(), []string{fromOther.ID, fromRemote.ID, fromBob.ID}; !slices.Equal(got, want) {
		t.Errorf("after the mute was renewed and the block lifted = %v, want %v", got, want)
	}
	if has, err := r.statusHasNotifications(t, r.ids.NewID()); err != nil || has {
		t.Errorf("StatusHasNotifications(another status) = %v, %v; want false", has, err)
	}
}

func (r *notificationRepo) statusHasNotifications(t *testing.T, statusID string) (bool, error) {
	t.Helper()
	var has bool
	err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		var err error
		has, err = r.repo.StatusHasNotifications(t.Context(), tx, statusID)
		return err
	})
	return has, err
}

func notificationRepoReadMarker(t *testing.T, r *notificationRepo) {
	alice := r.ids.NewID()
	if _, err := r.repo.FindReadMarker(t.Context(), alice); !errors.Is(err, appnotification.ErrNotFound) {
		t.Errorf("FindReadMarker before reading = %v, want ErrNotFound", err)
	}
	first, second, third := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	for _, c := range []struct {
		id    string
		moved bool
		at    string
	}{{second, true, second}, {second, false, second}, {first, false, second}, {third, true, third}} {
		var moved bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			var err error
			moved, err = r.repo.AdvanceReadMarker(ctx, tx, notification.ReadMarker{AccountID: alice, LastReadID: c.id})
			return err
		})
		m, err := r.repo.FindReadMarker(t.Context(), alice)
		if moved != c.moved || err != nil || m.LastReadID != c.at || m.AccountID != alice {
			t.Errorf("AdvanceReadMarker(%s) = %v, marker %+v, %v; want %v at %s", c.id, moved, m, err, c.moved, c.at)
		}
	}
}

func notificationRepoSettings(t *testing.T, r *notificationRepo) {
	alice, bob := r.ids.NewID(), r.ids.NewID()
	set := func(s ...notification.Setting) {
		t.Helper()
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.repo.SaveNotificationSettings(ctx, tx, s, r.clock.Now())
		})
		r.clock.Advance(time.Second)
	}
	if got, err := r.repo.NotificationSettings(t.Context(), alice); err != nil || len(got) != 0 {
		t.Errorf("NotificationSettings before choosing = %+v, %v; want none", got, err)
	}
	set(
		notification.Setting{AccountID: alice, Group: notification.GroupMention, Channel: notification.ChannelEmail, Enabled: true},
		notification.Setting{AccountID: alice, Group: notification.GroupFollow, Channel: notification.ChannelInApp, Enabled: false},
		notification.Setting{AccountID: bob, Group: notification.GroupFollow, Channel: notification.ChannelInApp, Enabled: true},
	)
	set(
		notification.Setting{AccountID: alice, Group: notification.GroupMention, Channel: notification.ChannelEmail, Enabled: false},
		notification.Setting{AccountID: alice, Group: notification.GroupMention, Channel: notification.ChannelInApp, Enabled: true},
	)
	want := []notification.Setting{
		{AccountID: alice, Group: notification.GroupFollow, Channel: notification.ChannelInApp, Enabled: false},
		{AccountID: alice, Group: notification.GroupMention, Channel: notification.ChannelEmail, Enabled: false},
		{AccountID: alice, Group: notification.GroupMention, Channel: notification.ChannelInApp, Enabled: true},
	}
	if got, err := r.repo.NotificationSettings(t.Context(), alice); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("NotificationSettings = %+v, %v; want %+v, by group and channel", got, err, want)
	}
	var inTx []notification.Setting
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		var err error
		inTx, err = r.repo.NotificationSettingsTx(ctx, tx, alice)
		return err
	})
	if !reflect.DeepEqual(inTx, want) {
		t.Errorf("NotificationSettingsTx = %+v, want %+v", inTx, want)
	}
	if got, err := r.repo.NotificationSettings(t.Context(), bob); err != nil || len(got) != 1 || !got[0].Enabled {
		t.Errorf("NotificationSettings of bob = %+v, %v", got, err)
	}
}

func notificationRepoErrorGroupNotices(t *testing.T, r *notificationRepo) {
	ctx := t.Context()
	hour := notification.ErrorGroupHour(r.clock.Now())
	next := hour.Add(time.Hour)
	const n = 8
	var wg sync.WaitGroup
	counts := make([]int64, n)
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() {
			errs[i] = r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				var err error
				counts[i], err = r.repo.AddErrorGroupNotice(ctx, tx, hour)
				return err
			})
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("AddErrorGroupNotice %d = %v", i, err)
		}
	}
	slices.Sort(counts)
	if !slices.Equal(counts, []int64{1, 2, 3, 4, 5, 6, 7, 8}) {
		t.Errorf("concurrent AddErrorGroupNotice returned %v, want each count once", counts)
	}
	var c int64
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		var err error
		c, err = r.repo.AddErrorGroupNotice(ctx, tx, next)
		return err
	})
	if c != 1 {
		t.Errorf("AddErrorGroupNotice of the next hour = %d, want 1", c)
	}
	for at, want := range map[time.Time]int64{hour: n, next: 1, next.Add(time.Hour): 0} {
		if got, err := r.repo.ErrorGroupNotices(ctx, at); err != nil || got != want {
			t.Errorf("ErrorGroupNotices(%v) = %d, %v; want %d", at, got, err, want)
		}
	}
	if d, err := r.repo.DeleteErrorGroupNoticesBefore(ctx, next); err != nil || d != 1 {
		t.Errorf("DeleteErrorGroupNoticesBefore(the next hour) = %d, %v; want 1", d, err)
	}
	if got, err := r.repo.ErrorGroupNotices(ctx, hour); err != nil || got != 0 {
		t.Errorf("ErrorGroupNotices of a deleted hour = %d, %v; want 0", got, err)
	}
	if got, err := r.repo.ErrorGroupNotices(ctx, next); err != nil || got != 1 {
		t.Errorf("ErrorGroupNotices of the kept hour = %d, %v; want 1", got, err)
	}
	if d, err := r.repo.DeleteErrorGroupNoticesBefore(ctx, next); err != nil || d != 0 {
		t.Errorf("DeleteErrorGroupNoticesBefore again = %d, %v; want 0", d, err)
	}
}

func notificationRepoRebuild(t *testing.T, r *notificationRepo) {
	alice, bob := r.ids.NewID(), r.ids.NewID()
	s1, s2 := r.ids.NewID(), r.ids.NewID()
	r.insert(t, notification.Notification{RecipientAccountID: alice, Type: notification.TypeFavourite, SourceAccountID: bob, StatusID: s2})
	dismissed, _ := r.insert(t, notification.Notification{RecipientAccountID: bob, Type: notification.TypeMention, SourceAccountID: alice, StatusID: s1})
	r.insert(t, notification.Notification{RecipientAccountID: bob, Type: notification.TypeBoost, SourceAccountID: alice, StatusID: s2})
	r.notify(t, alice, notification.TypeFollow, bob)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.repo.InsertDismissal(ctx, tx, notification.Dismissal{NotificationID: dismissed.ID, DismissedAt: r.clock.Now()})
		return errors.Join(err,
			r.repo.UpsertExclusion(ctx, tx, notification.Exclusion{ViewerAccountID: alice, TargetAccountID: bob, Reason: notification.ExcludedBlock}),
			r.repo.InsertDomainExclusion(ctx, tx, alice, "muted.example"),
			r.repo.InsertDeletedStatus(ctx, tx, s2),
		)
	})
	if got := r.list(t, alice, nil, appnotification.Hidden{}, "", 10); len(got) != 0 {
		t.Fatalf("notifications of alice = %v, want none", got)
	}

	var ids []string
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		var err error
		if ids, err = r.repo.ListNotifiedStatusIDs(ctx, tx); err != nil {
			return err
		}
		return errors.Join(r.repo.DeleteAllExclusions(ctx, tx), r.repo.DeleteAllDeletedStatuses(ctx, tx))
	})
	if want := []string{s1, s2}; !slices.Equal(ids, want) {
		t.Errorf("ListNotifiedStatusIDs = %v, want %v, each once in order", ids, want)
	}
	if got := r.list(t, alice, nil, appnotification.Hidden{}, "", 10); len(got) != 2 {
		t.Errorf("notifications of alice after emptying the projections = %v, want 2", got)
	}
	var excluded bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		var err error
		excluded, err = r.repo.DomainExcluded(ctx, tx, alice, "muted.example")
		return err
	})
	if excluded {
		t.Error("DeleteAllExclusions kept a domain exclusion")
	}
}

func notificationRepoSubscriptions(t *testing.T, r *notificationRepo) {
	ctx := t.Context()
	alice := r.ids.NewID()
	targets := []string{r.ids.NewID(), r.ids.NewID(), r.ids.NewID()}
	subscribe := func(s notification.Subscription) bool {
		t.Helper()
		var ok bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			var err error
			ok, err = r.repo.InsertSubscription(ctx, tx, s)
			return err
		})
		return ok
	}
	// The first two at the same time, the third a second later.
	at := r.clock.Now()
	for i, target := range targets {
		if i == 2 {
			at = at.Add(time.Second)
		}
		if !subscribe(notification.Subscription{AccountID: alice, TargetAccountID: target, CreatedAt: at}) {
			t.Fatalf("InsertSubscription(%d) inserted nothing", i)
		}
	}
	if subscribe(notification.Subscription{AccountID: alice, TargetAccountID: targets[0], CreatedAt: at}) {
		t.Error("InsertSubscription again inserted it")
	}
	subscribers := []string{r.ids.NewID(), r.ids.NewID(), alice}
	slices.Sort(subscribers)
	for _, s := range subscribers {
		if s != alice {
			subscribe(notification.Subscription{AccountID: s, TargetAccountID: targets[0], CreatedAt: at})
		}
	}

	var count int64
	var has, none bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		var err1, err2, err3 error
		count, err1 = r.repo.CountSubscriptions(ctx, tx, alice)
		has, err2 = r.repo.HasSubscribers(ctx, tx, targets[0])
		none, err3 = r.repo.HasSubscribers(ctx, tx, alice)
		return errors.Join(err1, err2, err3)
	})
	if count != 3 || !has || none {
		t.Errorf("CountSubscriptions = %d, HasSubscribers = %v, of a target without = %v; want 3, true, false", count, has, none)
	}
	if ok, err := r.repo.Subscribed(ctx, alice, targets[1]); err != nil || !ok {
		t.Errorf("Subscribed = %v, %v; want true", ok, err)
	}
	if ok, err := r.repo.Subscribed(ctx, targets[1], alice); err != nil || ok {
		t.Errorf("Subscribed the other way = %v, %v; want false", ok, err)
	}

	// Newest first, and by target in descending order at the same time.
	tied := []string{targets[0], targets[1]}
	slices.Sort(tied)
	want := []string{targets[2], tied[1], tied[0]}
	page, err := r.repo.ListSubscriptions(ctx, alice, appnotification.SubscriptionCursor{}, 2)
	if got := subscriptionTargets(page); err != nil || !slices.Equal(got, want[:2]) {
		t.Errorf("ListSubscriptions(2) = %v, %v; want %v", got, err, want[:2])
	}
	if len(page) == 2 {
		if !page[0].CreatedAt.Equal(at) || page[0].AccountID != alice {
			t.Errorf("ListSubscriptions[0] = %+v, want alice's at %v", page[0], at)
		}
		rest, err := r.repo.ListSubscriptions(ctx, alice, appnotification.SubscriptionCursorOf(page[1]), 2)
		if got := subscriptionTargets(rest); err != nil || !slices.Equal(got, want[2:]) {
			t.Errorf("ListSubscriptions after the second = %v, %v; want %v", got, err, want[2:])
		}
	}

	got, err := r.repo.ListSubscribers(ctx, targets[0], "", 2)
	if err != nil || !slices.Equal(got, subscribers[:2]) {
		t.Errorf("ListSubscribers(2) = %v, %v; want %v", got, err, subscribers[:2])
	}
	got, err = r.repo.ListSubscribers(ctx, targets[0], subscribers[1], 10)
	if err != nil || !slices.Equal(got, subscribers[2:]) {
		t.Errorf("ListSubscribers after the second = %v, %v; want %v", got, err, subscribers[2:])
	}
	if n, err := r.repo.CountSubscribers(ctx, targets[0]); err != nil || n != 3 {
		t.Errorf("CountSubscribers = %d, %v; want 3", n, err)
	}

	for i, want := range []bool{true, false} {
		var ok bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			var err error
			ok, err = r.repo.DeleteSubscription(ctx, tx, alice, targets[0])
			return err
		})
		if ok != want {
			t.Errorf("DeleteSubscription #%d = %v, want %v", i+1, ok, want)
		}
	}
	if ok, err := r.repo.Subscribed(ctx, alice, targets[0]); err != nil || ok {
		t.Errorf("Subscribed after the delete = %v, %v; want false", ok, err)
	}
	if n, err := r.repo.CountSubscribers(ctx, targets[0]); err != nil || n != 2 {
		t.Errorf("CountSubscribers after the delete = %d, %v; want 2", n, err)
	}
}

func subscriptionTargets(ss []notification.Subscription) []string {
	out := []string{}
	for _, s := range ss {
		out = append(out, s.TargetAccountID)
	}
	return out
}

func notificationRepoSubscriptionRequests(t *testing.T, r *notificationRepo) {
	ctx := t.Context()
	alice, bob, carol := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	req := notification.SubscriptionRequest{AccountID: alice, TargetAccountID: bob, CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.repo.InsertSubscriptionRequest(ctx, tx, req), r.repo.InsertSubscriptionRequest(ctx, tx, req))
	})
	if ok, err := r.repo.SubscriptionRequested(ctx, alice, bob); err != nil || !ok {
		t.Errorf("SubscriptionRequested = %v, %v; want true", ok, err)
	}
	if ok, err := r.repo.SubscriptionRequested(ctx, alice, carol); err != nil || ok {
		t.Errorf("SubscriptionRequested(another target) = %v, %v; want false", ok, err)
	}
	if ok, err := r.repo.Subscribed(ctx, alice, bob); err != nil || ok {
		t.Errorf("Subscribed with a request = %v, %v; want false", ok, err)
	}
	for i, want := range []bool{true, false} {
		var ok bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			var err error
			ok, err = r.repo.DeleteSubscriptionRequest(ctx, tx, alice, bob)
			return err
		})
		if ok != want {
			t.Errorf("DeleteSubscriptionRequest #%d = %v, want %v", i+1, ok, want)
		}
	}
	if ok, err := r.repo.SubscriptionRequested(ctx, alice, bob); err != nil || ok {
		t.Errorf("SubscriptionRequested after the delete = %v, %v; want false", ok, err)
	}
}
