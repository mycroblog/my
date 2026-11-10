// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakePolicies keeps a policy and the requests of bob's filtered notifications in memory, as
// appnotification.Service does.
type fakePolicies struct {
	choices  map[notification.Condition]notification.Action
	waiting  []notification.Notification // bob's filtered notifications of alice, newest first
	accepted bool
	updates  [][]notification.PolicyChoice
}

func (f *fakePolicies) policy() notification.Policy {
	var cs []notification.PolicyChoice
	for c, a := range f.choices {
		cs = append(cs, notification.PolicyChoice{Condition: c, Action: a})
	}
	return notification.NewPolicy(cs)
}

func (f *fakePolicies) Policy(context.Context, string) (notification.Policy, error) {
	return f.policy(), nil
}

func (f *fakePolicies) UpdatePolicy(_ context.Context, _ string, choices []notification.PolicyChoice) (notification.Policy, error) {
	for _, c := range choices {
		if err := c.Validate(); err != nil {
			return notification.Policy{}, err
		}
	}
	f.updates = append(f.updates, choices)
	for _, c := range choices {
		f.choices[c.Condition] = c.Action
	}
	return f.policy(), nil
}

func (f *fakePolicies) request(accountID, sourceID string) (notification.Request, error) {
	if accountID != aliceAccountID || sourceID != bobAccountID || f.accepted || len(f.waiting) == 0 {
		return notification.Request{}, appnotification.ErrNotFound
	}
	return notification.Request{
		RecipientAccountID: aliceAccountID, SourceAccountID: bobAccountID, NotificationsCount: int64(len(f.waiting)),
		LastNotificationID: f.waiting[0].ID, Last: f.waiting[0],
	}, nil
}

func (f *fakePolicies) Requests(_ context.Context, accountID, before string, _ int) ([]notification.Request, error) {
	r, err := f.request(accountID, bobAccountID)
	if err != nil || before != "" && r.LastNotificationID >= before {
		return nil, nil //nolint:nilerr // no request is an empty page
	}
	return []notification.Request{r}, nil
}

func (f *fakePolicies) Request(_ context.Context, accountID, sourceID string) (notification.Request, error) {
	return f.request(accountID, sourceID)
}

func (f *fakePolicies) RequestSummary(_ context.Context, accountID string) (notification.RequestSummary, error) {
	if _, err := f.request(accountID, bobAccountID); err != nil {
		return notification.RequestSummary{}, nil //nolint:nilerr // no request counts nothing
	}
	return notification.RequestSummary{Accounts: 1, Notifications: int64(len(f.waiting))}, nil
}

func (f *fakePolicies) RequestNotifications(_ context.Context, accountID, sourceID, before string, limit int) ([]notification.Notification, error) {
	if _, err := f.request(accountID, sourceID); err != nil {
		return nil, err
	}
	var out []notification.Notification
	for _, n := range f.waiting {
		if (before == "" || n.ID < before) && len(out) < limit {
			out = append(out, n)
		}
	}
	return out, nil
}

func (f *fakePolicies) AcceptRequest(_ context.Context, accountID, sourceID string) error {
	if _, err := f.request(accountID, sourceID); err != nil {
		return err
	}
	f.accepted = true
	return nil
}

func (f *fakePolicies) DismissRequest(_ context.Context, accountID, sourceID string) error {
	if _, err := f.request(accountID, sourceID); err != nil {
		return err
	}
	f.waiting = nil
	return nil
}

func newPolicyEnv(t *testing.T) (*authEnv, *fakePolicies) {
	t.Helper()
	f := &fakePolicies{choices: map[notification.Condition]notification.Action{}}
	e, notifs, _ := newNotificationEnvWithViews(t, func(o *clientapi.Options) { o.NotificationPolicies = f })
	// bob's follow and favourite of alice wait in his request.
	for _, n := range slices.Backward(notifs.ns) {
		if n.RecipientAccountID == aliceAccountID && n.SourceAccountID == bobAccountID && n.Type != notification.TypeMention {
			f.waiting = append(f.waiting, n)
		}
	}
	return e, f
}

func TestNotificationPolicy(t *testing.T) {
	e, f := newPolicyEnv(t)
	got := decode[clientapi.NotificationPolicy](t, e.send(t, http.MethodGet, "/preferences/notification_policy", "read", ""))
	want := clientapi.NotificationPolicy{
		NotFollowing: clientapi.NotificationPolicyActionAccept, NotFollower: clientapi.NotificationPolicyActionAccept,
		NewAccount: clientapi.NotificationPolicyActionAccept, UnsolicitedPrivateMention: clientapi.NotificationPolicyActionAccept,
		LimitedAccount: clientapi.NotificationPolicyActionAccept,
	}
	if got != want {
		t.Errorf("policy = %+v, want every condition accepted", got)
	}
	res := e.send(t, http.MethodPatch, "/preferences/notification_policy", "write", `{"not_following": "filter", "new_account": "drop"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH = %d, want 200", res.StatusCode)
	}
	got = decode[clientapi.NotificationPolicy](t, res)
	want.NotFollowing, want.NewAccount = clientapi.NotificationPolicyActionFilter, clientapi.NotificationPolicyActionDrop
	if got != want {
		t.Errorf("policy after the change = %+v, want %+v", got, want)
	}
	if len(f.updates) != 1 || len(f.updates[0]) != 2 {
		t.Errorf("updates = %+v, want the two named conditions", f.updates)
	}
	res = e.send(t, http.MethodPatch, "/preferences/notification_policy", "write", `{"not_follower": "block"}`)
	if res.StatusCode != http.StatusUnprocessableEntity && res.StatusCode != http.StatusBadRequest {
		t.Errorf("PATCH of an unknown action = %d, want a 4xx problem", res.StatusCode)
	}
	if res := e.send(t, http.MethodPatch, "/preferences/notification_policy", "read", `{}`); res.StatusCode != http.StatusForbidden {
		t.Errorf("PATCH with a read token = %d, want 403", res.StatusCode)
	}
}

func TestNotificationRequests(t *testing.T) {
	e, f := newPolicyEnv(t)
	page := decode[clientapi.NotificationRequestPage](t, e.send(t, http.MethodGet, "/notifications/requests", "read", ""))
	if len(page.Items) != 1 || page.NextCursor != nil {
		t.Fatalf("requests = %+v, want bob's", page)
	}
	r := page.Items[0]
	// The fake presents bob with alice's actor.
	if r.Id != bobAccountID || r.Account.Username != "alice" || r.NotificationsCount != 2 || r.LastNotification == nil || r.LastNotification.Id != f.waiting[0].ID {
		t.Errorf("request = %+v", r)
	}
	if s := decode[clientapi.NotificationRequestSummary](t, e.send(t, http.MethodGet, "/notifications/requests/summary", "read", "")); s.AccountsCount != 1 || s.NotificationsCount != 2 {
		t.Errorf("summary = %+v, want 1 and 2", s)
	}
	if one := decode[clientapi.NotificationRequest](t, e.send(t, http.MethodGet, "/notifications/requests/"+bobAccountID, "read", "")); one.Id != bobAccountID {
		t.Errorf("request of bob = %+v", one)
	}
	ns := decode[clientapi.NotificationPage](t, e.send(t, http.MethodGet, "/notifications/requests/"+bobAccountID+"/notifications?limit=1", "read", ""))
	if want := []string{f.waiting[0].ID}; !slices.Equal(ids(ns), want) || ns.NextCursor == nil {
		t.Fatalf("first page = %v (next %v), want %v and a next page", ids(ns), ns.NextCursor, want)
	}
	ns = decode[clientapi.NotificationPage](t, e.send(t, http.MethodGet, "/notifications/requests/"+bobAccountID+"/notifications?cursor="+*ns.NextCursor, "read", ""))
	if want := []string{f.waiting[1].ID}; !slices.Equal(ids(ns), want) || ns.NextCursor != nil {
		t.Errorf("second page = %v, want %v", ids(ns), want)
	}
	for _, path := range []string{"/notifications/requests/" + aliceAccountID, "/notifications/requests/" + aliceAccountID + "/notifications", "/notifications/requests/not-an-id"} {
		if res := e.send(t, http.MethodGet, path, "read", ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, res.StatusCode)
		}
	}
	if res := e.send(t, http.MethodGet, "/notifications/requests?cursor=nope", "read", ""); res.StatusCode != http.StatusBadRequest {
		t.Errorf("a broken cursor = %d, want 400", res.StatusCode)
	}
	if res := e.send(t, http.MethodPost, "/notifications/requests/"+bobAccountID+"/accept", "read", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("accept with a read token = %d, want 403", res.StatusCode)
	}
	if res := e.send(t, http.MethodPost, "/notifications/requests/"+bobAccountID+"/accept", "write", ""); res.StatusCode != http.StatusNoContent {
		t.Fatalf("accept = %d, want 204", res.StatusCode)
	}
	if !f.accepted {
		t.Error("the request was not accepted")
	}
	for _, op := range []string{"accept", "dismiss"} {
		if res := e.send(t, http.MethodPost, "/notifications/requests/"+bobAccountID+"/"+op, "write", ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s of an accepted request = %d, want 404", op, res.StatusCode)
		}
	}
	page = decode[clientapi.NotificationRequestPage](t, e.send(t, http.MethodGet, "/notifications/requests", "read", ""))
	if len(page.Items) != 0 {
		t.Errorf("requests after the acceptance = %+v, want none", page.Items)
	}
}

func TestDismissNotificationRequest(t *testing.T) {
	e, f := newPolicyEnv(t)
	if res := e.send(t, http.MethodPost, "/notifications/requests/"+bobAccountID+"/dismiss", "write", ""); res.StatusCode != http.StatusNoContent {
		t.Fatalf("dismiss = %d, want 204", res.StatusCode)
	}
	if len(f.waiting) != 0 || f.accepted {
		t.Errorf("after the dismissal: waiting %v, accepted %v", f.waiting, f.accepted)
	}
	if s := decode[clientapi.NotificationRequestSummary](t, e.send(t, http.MethodGet, "/notifications/requests/summary", "read", "")); s.AccountsCount != 0 || s.NotificationsCount != 0 {
		t.Errorf("summary after the dismissal = %+v", s)
	}
}

// TestStreamingNotificationRequests checks that a stream sends the counts of the requests when a
// signal finds them changed, and nothing when they did not change.
func TestStreamingNotificationRequests(t *testing.T) {
	hub := stream.NewHub(stream.HubOptions{})
	f := &fakePolicies{choices: map[notification.Condition]notification.Action{}}
	e, _, _ := newNotificationEnvWithViews(t, func(o *clientapi.Options) { o.Streams = hub; o.NotificationPolicies = f })
	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)

	_, _, r := openStream(t.Context(), t, srv, "read")
	if line := readLine(t, r); line != ": connected" {
		t.Fatalf("first line = %q, want the connected comment", line)
	}
	// A filtered notification arrives: no notification event, the new counts.
	f.waiting = []notification.Notification{{ID: notificationID(7), RecipientAccountID: aliceAccountID, Type: notification.TypeFollow, SourceAccountID: bobAccountID}}
	hub.Publish(stream.Event{Kind: stream.KindNotifications, AccountID: aliceAccountID})
	if line := readLine(t, r); line != "event: notification_requests" {
		t.Fatalf("line = %q, want the counts of the requests", line)
	}
	var sum clientapi.NotificationRequestSummary
	if err := json.Unmarshal([]byte(strings.TrimPrefix(readLine(t, r), "data: ")), &sum); err != nil {
		t.Fatal(err)
	}
	if sum.AccountsCount != 1 || sum.NotificationsCount != 1 {
		t.Errorf("counts = %+v, want 1 and 1", sum)
	}
	// Unchanged counts send nothing; the dismissal sends zero.
	hub.Publish(stream.Event{Kind: stream.KindNotifications, AccountID: aliceAccountID})
	f.waiting = nil
	hub.Publish(stream.Event{Kind: stream.KindNotifications, AccountID: aliceAccountID})
	if line := readLine(t, r); line != "event: notification_requests" {
		t.Fatalf("line = %q, want the counts after the dismissal", line)
	}
	if line := readLine(t, r); line != `data: {"accounts_count":0,"notifications_count":0}` {
		t.Errorf("data = %q, want zero counts", line)
	}
}
