// SPDX-License-Identifier: AGPL-3.0-only

package notificationtest

import (
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
)

// The flows of the notification policy (ADR-0158), part of NotificationFlows.

// recordedSignals is a stream.Publisher that keeps the accounts it signals.
type recordedSignals struct {
	mu       sync.Mutex
	accounts []string
}

// Publish implements stream.Publisher.
func (r *recordedSignals) Publish(e stream.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.Kind == stream.KindNotifications {
		r.accounts = append(r.accounts, e.AccountID)
	}
}

// take returns the accounts signalled since the last take.
func (r *recordedSignals) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.accounts
	r.accounts = nil
	return out
}

func (e *notificationEnv) policy(t *testing.T, accountID string, actions map[notification.Condition]notification.Action) {
	t.Helper()
	var choices []notification.PolicyChoice
	for _, c := range notification.Conditions {
		if a, ok := actions[c]; ok {
			choices = append(choices, notification.PolicyChoice{Condition: c, Action: a})
		}
	}
	if _, err := e.svc.UpdatePolicy(t.Context(), accountID, choices); err != nil {
		t.Fatalf("UpdatePolicy(%s, %v) = %v", accountID, actions, err)
	}
}

func (e *notificationEnv) requestSummary(t *testing.T, accountID string) notification.RequestSummary {
	t.Helper()
	s, err := e.svc.RequestSummary(t.Context(), accountID)
	if err != nil {
		t.Fatalf("RequestSummary(%s) = %v", accountID, err)
	}
	return s
}

func (e *notificationEnv) requestSummaries(t *testing.T, accountID, sourceID string) []summary {
	t.Helper()
	ns, err := e.svc.RequestNotifications(t.Context(), accountID, sourceID, "", 0)
	if err != nil {
		t.Fatalf("RequestNotifications(%s, %s) = %v", accountID, sourceID, err)
	}
	out := []summary{}
	for _, n := range ns {
		out = append(out, summary{Type: n.Type, Source: n.SourceAccountID, StatusID: n.StatusID})
	}
	return out
}

func (e *notificationEnv) postAs(t *testing.T, author, text string, v content.Visibility, inReplyTo string) appcontent.Post {
	t.Helper()
	e.Clock.Advance(time.Second)
	p, err := e.content.Create(t.Context(), appcontent.CreateInput{
		AuthorAccountID: author, TextInput: appcontent.TextInput{Text: text}, Visibility: v, InReplyToID: inReplyTo,
	})
	if err != nil {
		t.Fatalf("Create(%q) = %v", text, err)
	}
	return p
}

// notificationPolicyDefaults checks that a policy that accepts everything, chosen or by default,
// lists every notification and opens no request.
func notificationPolicyDefaults(t *testing.T, e *notificationEnv) {
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	if pol, err := e.svc.Policy(t.Context(), alice); err != nil || pol.Strict() {
		t.Fatalf("Policy of a new account = %+v, %v; want everything accepted", pol, err)
	}
	e.follow(t, bob, alice)
	p := e.post(t, bob, "hi @alice")
	e.policy(t, alice, map[notification.Condition]notification.Action{notification.ConditionNotFollowing: notification.ActionAccept})
	q := e.postAs(t, bob, "@alice psst", content.VisibilityDirect, "")
	checkSummaries(t, e, alice,
		summary{Type: notification.TypeMention, Source: bob, StatusID: q.Status.ID},
		summary{Type: notification.TypeMention, Source: bob, StatusID: p.Status.ID},
		summary{Type: notification.TypeFollow, Source: bob},
	)
	if s := e.requestSummary(t, alice); s != (notification.RequestSummary{}) {
		t.Errorf("RequestSummary = %+v, want none", s)
	}
}

// notificationPolicyFilters checks that a filtered notification waits only in its source's
// request, out of the list and the unread count, and sends no email; that accepting the request
// lists its notifications, unread, and accepts the later ones; and that a followed source is not
// filtered by not_following.
func notificationPolicyFilters(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	e.setting(t, alice, notification.GroupMention, notification.ChannelEmail, true)
	e.policy(t, alice, map[notification.Condition]notification.Action{notification.ConditionNotFollowing: notification.ActionFilter})
	e.follow(t, alice, carol)
	e.outbox.Take()

	post := e.post(t, alice, "mine")
	m := e.post(t, bob, "hi @alice")
	e.favourite(t, bob, post.Status.ID)
	if jobs := e.outbox.Take(); len(jobs) != 0 {
		t.Errorf("jobs of filtered notifications = %+v, want none", jobs)
	}
	c := e.post(t, carol, "hey @alice")
	if jobs := e.outbox.Take(); len(jobs) != 1 || jobs[0].Type != appnotification.JobNotificationEmail {
		t.Errorf("jobs of a followed source's mention = %+v, want its email", jobs)
	}
	checkSummaries(t, e, alice, summary{Type: notification.TypeMention, Source: carol, StatusID: c.Status.ID})
	e.checkUnread(t, alice, 1)
	if s := e.requestSummary(t, alice); s != (notification.RequestSummary{Accounts: 1, Notifications: 2}) {
		t.Errorf("RequestSummary = %+v, want 1 account and 2 notifications", s)
	}
	reqs, err := e.svc.Requests(ctx, alice, "", 0)
	if err != nil || len(reqs) != 1 || reqs[0].SourceAccountID != bob || reqs[0].NotificationsCount != 2 {
		t.Errorf("Requests = %+v, %v; want bob's with 2", reqs, err)
	}
	waiting := []summary{
		{Type: notification.TypeFavourite, Source: bob, StatusID: post.Status.ID},
		{Type: notification.TypeMention, Source: bob, StatusID: m.Status.ID},
	}
	if got := e.requestSummaries(t, alice, bob); !reflect.DeepEqual(got, waiting) {
		t.Errorf("RequestNotifications = %+v, want %+v", got, waiting)
	}
	if err := e.svc.AcceptRequest(ctx, alice, carol); !errors.Is(err, appnotification.ErrNotFound) {
		t.Errorf("AcceptRequest without a request = %v, want ErrNotFound", err)
	}

	if err := e.svc.AcceptRequest(ctx, alice, bob); err != nil {
		t.Fatalf("AcceptRequest = %v", err)
	}
	if jobs := e.outbox.Take(); len(jobs) != 0 {
		t.Errorf("jobs of the accepted notifications = %+v, want none", jobs)
	}
	checkSummaries(t, e, alice, summary{Type: notification.TypeMention, Source: carol, StatusID: c.Status.ID}, waiting[0], waiting[1])
	e.checkUnread(t, alice, 3)
	if s := e.requestSummary(t, alice); s != (notification.RequestSummary{}) {
		t.Errorf("RequestSummary after the acceptance = %+v, want none", s)
	}
	again := e.post(t, bob, "again @alice")
	if jobs := e.outbox.Take(); len(jobs) != 1 {
		t.Errorf("jobs of an accepted source's mention = %+v, want its email", jobs)
	}
	if got := e.list(t, alice, notification.TypeMention); len(got) != 3 || got[0].StatusID != again.Status.ID {
		t.Errorf("mentions after the acceptance = %+v, want bob's new one first", got)
	}
}

// notificationPolicyDismisses checks that dismissing a request hides its notifications for good,
// and that a later filtered notification opens it again.
func notificationPolicyDismisses(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	e.policy(t, alice, map[notification.Condition]notification.Action{notification.ConditionNotFollower: notification.ActionFilter})
	e.post(t, bob, "hi @alice")
	e.signals.take()
	if err := e.svc.DismissRequest(ctx, alice, bob); err != nil {
		t.Fatalf("DismissRequest = %v", err)
	}
	// The streams of alice send the new counts of the requests.
	if got := e.signals.take(); !slices.Equal(got, []string{alice}) {
		t.Errorf("signals of the dismissal = %v, want alice's", got)
	}
	if s := e.requestSummary(t, alice); s != (notification.RequestSummary{}) {
		t.Errorf("RequestSummary after the dismissal = %+v, want none", s)
	}
	if _, err := e.svc.RequestNotifications(ctx, alice, bob, "", 0); !errors.Is(err, appnotification.ErrNotFound) {
		t.Errorf("RequestNotifications after the dismissal = %v, want ErrNotFound", err)
	}
	later := e.post(t, bob, "still @alice")
	if got := e.requestSummaries(t, alice, bob); !reflect.DeepEqual(got, []summary{{Type: notification.TypeMention, Source: bob, StatusID: later.Status.ID}}) {
		t.Errorf("RequestNotifications after a later mention = %+v", got)
	}
	if err := e.svc.AcceptRequest(ctx, alice, bob); err != nil {
		t.Fatalf("AcceptRequest = %v", err)
	}
	if got := e.signals.take(); !slices.Contains(got, alice) {
		t.Errorf("signals of a filtered mention and the acceptance = %v, want alice's", got)
	}
	checkSummaries(t, e, alice, summary{Type: notification.TypeMention, Source: bob, StatusID: later.Status.ID})
}

// notificationPolicyStrictest checks that the strictest action of the conditions met wins, that
// a dropped notification is nowhere, and that a follower is not a stranger to not_follower.
func notificationPolicyStrictest(t *testing.T, e *notificationEnv) {
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	// Every account of the flows is new.
	e.policy(t, alice, map[notification.Condition]notification.Action{
		notification.ConditionNotFollowing: notification.ActionFilter, notification.ConditionNewAccount: notification.ActionDrop,
	})
	e.post(t, bob, "hi @alice")
	checkSummaries(t, e, alice)
	if s := e.requestSummary(t, alice); s != (notification.RequestSummary{}) {
		t.Errorf("RequestSummary of a dropped mention = %+v, want none", s)
	}
	e.policy(t, alice, map[notification.Condition]notification.Action{notification.ConditionNewAccount: notification.ActionAccept})
	e.post(t, bob, "again @alice")
	if s := e.requestSummary(t, alice); s != (notification.RequestSummary{Accounts: 1, Notifications: 1}) {
		t.Errorf("RequestSummary of a filtered mention = %+v, want one", s)
	}

	e.policy(t, alice, map[notification.Condition]notification.Action{
		notification.ConditionNotFollowing: notification.ActionAccept, notification.ConditionNotFollower: notification.ActionDrop,
	})
	e.follow(t, carol, alice)
	p := e.post(t, carol, "hey @alice")
	checkSummaries(t, e, alice,
		summary{Type: notification.TypeMention, Source: carol, StatusID: p.Status.ID},
		summary{Type: notification.TypeFollow, Source: carol},
	)
}

// notificationPolicyPrivateMentions checks unsolicited_private_mention: a direct mention out of
// the blue is filtered, one that answers the recipient's direct post is not, and a public mention
// is not.
func notificationPolicyPrivateMentions(t *testing.T, e *notificationEnv) {
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	e.policy(t, alice, map[notification.Condition]notification.Action{notification.ConditionUnsolicitedPrivateMention: notification.ActionFilter})
	cold := e.postAs(t, bob, "@alice psst", content.VisibilityDirect, "")
	public := e.post(t, carol, "hello @alice")
	started := e.postAs(t, alice, "@carol a secret", content.VisibilityDirect, "")
	answer := e.postAs(t, carol, "@alice tell me", content.VisibilityDirect, started.Status.ID)
	checkSummaries(t, e, alice,
		summary{Type: notification.TypeMention, Source: carol, StatusID: answer.Status.ID},
		summary{Type: notification.TypeMention, Source: carol, StatusID: public.Status.ID},
	)
	if got := e.requestSummaries(t, alice, bob); !reflect.DeepEqual(got, []summary{{Type: notification.TypeMention, Source: bob, StatusID: cold.Status.ID}}) {
		t.Errorf("RequestNotifications of bob = %+v, want the cold mention", got)
	}
}

// notificationPolicyLimited checks limited_account: a limited source that the recipient follows
// is filtered, and the instance's notices are never filtered or dropped.
func notificationPolicyLimited(t *testing.T, e *notificationEnv) {
	alice, bob := e.user(t, "alice"), e.user(t, "bob")
	e.follow(t, alice, bob)
	e.reach.set(bob, true)
	everything := map[notification.Condition]notification.Action{}
	for _, c := range notification.Conditions {
		everything[c] = notification.ActionDrop
	}
	everything[notification.ConditionLimitedAccount] = notification.ActionFilter
	e.policy(t, alice, everything)
	e.post(t, bob, "hi @alice")
	checkSummaries(t, e, alice)
	if s := e.requestSummary(t, alice); s != (notification.RequestSummary{Accounts: 1, Notifications: 1}) {
		t.Errorf("RequestSummary of a limited followed source = %+v, want one", s)
	}

	report := e.IDs.NewID()
	e.project(t, appmoderation.ReportResolved{
		Report:     moderation.Report{ID: report, ReporterAccountID: alice, TargetAccountID: bob},
		Resolution: moderation.ReportResolution{ID: e.IDs.NewID(), ReportID: report, Outcome: moderation.OutcomeActioned, ResolvedAt: e.Clock.Now()},
	})
	checkSummaries(t, e, alice, summary{Type: notification.TypeReportResolved})
}
