// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeNotifications keeps notifications in memory with the semantics of
// appnotification.Service: newest first, dismissals, and a forward-only read marker.
type fakeNotifications struct {
	ns        []notification.Notification // ascending by ID
	dismissed map[string]bool
	markers   map[string]string
	queries   []appnotification.Query
	err       error
}

func (f *fakeNotifications) visible(accountID string) []notification.Notification {
	var out []notification.Notification
	for _, n := range slices.Backward(f.ns) {
		if n.RecipientAccountID == accountID && !f.dismissed[n.ID] {
			out = append(out, n)
		}
	}
	return out
}

func (f *fakeNotifications) List(_ context.Context, accountID string, q appnotification.Query) ([]notification.Notification, error) {
	f.queries = append(f.queries, q)
	if f.err != nil {
		return nil, f.err
	}
	for _, t := range q.Types {
		if !t.Valid() {
			return nil, notification.ErrInvalidType
		}
	}
	var out []notification.Notification
	for _, n := range f.visible(accountID) {
		if (q.Before == "" || n.ID < q.Before) && (len(q.Types) == 0 || slices.Contains(q.Types, n.Type)) && len(out) < q.Limit {
			out = append(out, n)
		}
	}
	return out, nil
}

func (f *fakeNotifications) ReadMarker(_ context.Context, accountID string) (notification.ReadMarker, error) {
	return notification.ReadMarker{AccountID: accountID, LastReadID: f.markers[accountID]}, nil
}

func (f *fakeNotifications) UnreadCount(_ context.Context, accountID string) (int64, error) {
	var n int64
	for _, x := range f.visible(accountID) {
		if x.ID > f.markers[accountID] {
			n++
		}
	}
	return n, nil
}

func (f *fakeNotifications) own(accountID, id string) error {
	for _, n := range f.ns {
		if n.ID == id && n.RecipientAccountID == accountID {
			return nil
		}
	}
	return appnotification.ErrNotFound
}

func (f *fakeNotifications) MarkRead(ctx context.Context, accountID, lastReadID string) (notification.ReadMarker, error) {
	if err := f.own(accountID, lastReadID); err != nil {
		return notification.ReadMarker{}, err
	}
	if lastReadID > f.markers[accountID] {
		f.markers[accountID] = lastReadID
	}
	return f.ReadMarker(ctx, accountID)
}

// ErrorGroupOthers folds 4 groups into every notification of the others.
func (f *fakeNotifications) ErrorGroupOthers(_ context.Context, n notification.Notification) (int64, error) {
	if n.ErrorGroupChange != notification.ErrorGroupOthers {
		return 0, nil
	}
	return 4, nil
}

func (f *fakeNotifications) Dismiss(_ context.Context, accountID, id string) error {
	if err := f.own(accountID, id); err != nil {
		return err
	}
	f.dismissed[id] = true
	return nil
}

const (
	bobAccountID = "01890a5d-ac96-774b-bcce-b302099a8058"
	statusID     = "01890a5d-ac96-774b-bcce-b302099a9000"
	// hiddenStatusID is a status that alice may not see.
	hiddenStatusID = "01890a5d-ac96-774b-bcce-b302099a9001"
)

// notificationID returns the ID of the i-th notification; IDs grow with i.
func notificationID(i int) string { return fmt.Sprintf("01890a5d-ac96-774b-bcce-b302099a%04d", i) }

func newNotificationEnv(t *testing.T) (*authEnv, *fakeNotifications) {
	e, f, _ := newNotificationEnvWithViews(t)
	return e, f
}

// newNotificationEnvWithViews is newNotificationEnv that also returns the statuses that alice
// may see: only statusID.
func newNotificationEnvWithViews(t *testing.T) (*authEnv, *fakeNotifications, *fakeViews) {
	t.Helper()
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	f := &fakeNotifications{dismissed: map[string]bool{}, markers: map[string]string{}}
	types := []notification.Type{notification.TypeFollow, notification.TypeMention, notification.TypeFavourite, notification.TypeModerationNotice}
	for i, typ := range types {
		n := notification.Notification{
			ID: notificationID(i + 1), RecipientAccountID: aliceAccountID, Type: typ, SourceAccountID: bobAccountID,
			CreatedAt: at.Add(time.Duration(i) * time.Minute),
		}
		switch typ {
		case notification.TypeMention:
			n.StatusID = hiddenStatusID
		case notification.TypeFavourite:
			n.StatusID = statusID
		case notification.TypeModerationNotice:
			n.SourceAccountID, n.ModerationActionID = "", actionID(3)
		}
		f.ns = append(f.ns, n)
	}
	// A notification of bob, which alice may neither see nor act on.
	f.ns = append(f.ns, notification.Notification{
		ID: notificationID(9), RecipientAccountID: bobAccountID, Type: notification.TypeFollow, SourceAccountID: aliceAccountID, CreatedAt: at,
	})
	views := &fakeViews{statuses: map[string]appcontent.StatusView{statusID: {
		Status: content.Status{
			ID: statusID, URI: "https://social.example/ap/statuses/" + statusID,
			AuthorAccountID: aliceAccountID, IsLocal: true, Visibility: content.VisibilityPublic, PublishedAt: at,
		},
		Revision: content.Revision{Seq: 1, Language: "en", RevisedAt: at, Bodies: []content.Body{{Language: "en", ContentHTML: "<p>hi</p>"}}},
		Author:   aliceActor,
		Counters: content.Counters{StatusID: statusID, FavouritesCount: 1},
		Relation: &appcontent.ViewerRelation{Author: true},
	}}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Notifications = f
		o.TimelineReader = views
		o.OwnModeration = &fakeOwnModeration{on: map[string][]appmoderation.TargetActionView{aliceAccountID: aliceActions()}}
		o.Accounts.(fakeAccounts)[bobAccountID] = o.Accounts.(fakeAccounts)[aliceAccountID]
	})
	return e, f, views
}

func (e *authEnv) send(t *testing.T, method, path, token, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, clientapi.BasePath+path, strings.NewReader(body))
	req.Header.Set("Accept-Language", "en")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return e.spec.Serve(t, e.h, req)
}

func decode[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func ids(page clientapi.NotificationPage) []string {
	out := []string{}
	for _, n := range page.Items {
		out = append(out, n.Id)
	}
	return out
}

// TestListModeratorNotifications checks the moderator notifications: an admin_report names its
// report and no account, and an admin_sign_up presents the account of the user who signed up.
func TestListModeratorNotifications(t *testing.T) {
	const reportID = "01890a5d-ac96-774b-bcce-b302099a9100"
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	f := &fakeNotifications{dismissed: map[string]bool{}, markers: map[string]string{}, ns: []notification.Notification{
		{ID: notificationID(1), RecipientAccountID: aliceAccountID, Type: notification.TypeAdminReport, ReportID: reportID, CreatedAt: at},
		{ID: notificationID(2), RecipientAccountID: aliceAccountID, Type: notification.TypeAdminSignUp, SubjectUserID: userOf(bobAccountID), CreatedAt: at},
	}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Notifications = f
		o.UserAccounts = fakeUserAccounts{}
		o.Accounts.(fakeAccounts)[bobAccountID] = o.Accounts.(fakeAccounts)[aliceAccountID]
	})
	res := e.send(t, http.MethodGet, "/notifications", "read", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	page := decode[clientapi.NotificationPage](t, res)
	if len(page.Items) != 2 {
		t.Fatalf("items = %+v, want 2", page.Items)
	}
	signUp, report := page.Items[0], page.Items[1]
	if signUp.Type != clientapi.NotificationTypeAdminSignUp || signUp.Account == nil || signUp.ReportId != nil {
		t.Errorf("admin_sign_up = %+v, want the account of the user who signed up", signUp)
	}
	if report.Type != clientapi.NotificationTypeAdminReport || report.Account != nil || report.ReportId == nil || *report.ReportId != reportID {
		t.Errorf("admin_report = %+v, want report %s and no account", report, reportID)
	}
}

// TestListErrorGroupNotifications checks the admin_error_group notifications: a new and a
// regressed group name the group, and the others say how many groups they fold.
func TestListErrorGroupNotifications(t *testing.T) {
	const groupID = "01890a5d-ac96-774b-bcce-b302099a9200"
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	f := &fakeNotifications{dismissed: map[string]bool{}, markers: map[string]string{}, ns: []notification.Notification{
		{ID: notificationID(1), RecipientAccountID: aliceAccountID, Type: notification.TypeAdminErrorGroup, ErrorGroupChange: notification.ErrorGroupCreated, ErrorGroupID: groupID, CreatedAt: at},
		{ID: notificationID(2), RecipientAccountID: aliceAccountID, Type: notification.TypeAdminErrorGroup, ErrorGroupChange: notification.ErrorGroupRegressed, ErrorGroupID: groupID, CreatedAt: at},
		{ID: notificationID(3), RecipientAccountID: aliceAccountID, Type: notification.TypeAdminErrorGroup, ErrorGroupChange: notification.ErrorGroupOthers, ErrorGroupHour: at, CreatedAt: at},
	}}
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Notifications = f })
	res := e.send(t, http.MethodGet, "/notifications", "read", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	page := decode[clientapi.NotificationPage](t, res)
	if len(page.Items) != 3 {
		t.Fatalf("items = %+v, want 3", page.Items)
	}
	others, regressed, created := page.Items[0], page.Items[1], page.Items[2]
	if others.Type != clientapi.NotificationTypeAdminErrorGroup || others.Account != nil || others.ErrorGroup == nil ||
		others.ErrorGroup.Change != clientapi.NotificationErrorGroupChangeOthers || others.ErrorGroup.GroupId != nil ||
		others.ErrorGroup.Count == nil || *others.ErrorGroup.Count != 4 {
		t.Errorf("others = %+v", others)
	}
	for _, n := range []clientapi.Notification{regressed, created} {
		if n.ErrorGroup == nil || n.ErrorGroup.GroupId == nil || *n.ErrorGroup.GroupId != groupID || n.ErrorGroup.Count != nil {
			t.Errorf("%s = %+v, want group %s", n.Type, n.ErrorGroup, groupID)
		}
	}
	if regressed.ErrorGroup.Change != clientapi.NotificationErrorGroupChangeRegressed || created.ErrorGroup.Change != clientapi.NotificationErrorGroupChangeCreated {
		t.Errorf("changes = %s, %s", regressed.ErrorGroup.Change, created.ErrorGroup.Change)
	}
}

// fakeSeverances holds severances by ID.
type fakeSeverances map[string]moderation.SeveranceRecord

func (f fakeSeverances) SeveranceOf(_ context.Context, accountID, id string) (moderation.SeveranceRecord, error) {
	sv, ok := f[id]
	if !ok || sv.AccountID != accountID {
		return moderation.SeveranceRecord{}, appmoderation.ErrNotFound
	}
	return sv, nil
}

// TestListSeveranceNotifications checks the relationships_severed notifications: one of a
// domain names the domain and has no account, one of an account presents the suspended
// account, and one whose severance is gone has neither.
func TestListSeveranceNotifications(t *testing.T) {
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	severed := func(i int) notification.Notification {
		return notification.Notification{
			ID: notificationID(i), RecipientAccountID: aliceAccountID, Type: notification.TypeRelationshipsSevered,
			SeveranceID: testkit.NamedID(fmt.Sprintf("severance-%d", i)), CreatedAt: at,
		}
	}
	f := &fakeNotifications{dismissed: map[string]bool{}, markers: map[string]string{}, ns: []notification.Notification{severed(1), severed(2), severed(3)}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Notifications = f
		o.Severances = fakeSeverances{
			testkit.NamedID("severance-1"): {Severance: moderation.Severance{ID: testkit.NamedID("severance-1"), DomainActionID: testkit.NamedID("domain-action"), AccountID: aliceAccountID, FollowersLost: 2, FollowingLost: 1}, Domain: "bad.example"},
			testkit.NamedID("severance-2"): {Severance: moderation.Severance{ID: testkit.NamedID("severance-2"), ActionID: testkit.NamedID("action"), AccountID: aliceAccountID, FollowersLost: 1}, TargetAccountID: bobAccountID},
		}
		o.Accounts.(fakeAccounts)[bobAccountID] = o.Accounts.(fakeAccounts)[aliceAccountID]
	})
	res := e.send(t, http.MethodGet, "/notifications", "read", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	page := decode[clientapi.NotificationPage](t, res)
	if len(page.Items) != 3 {
		t.Fatalf("items = %+v, want 3", page.Items)
	}
	gone, ofAccount, ofDomain := page.Items[0], page.Items[1], page.Items[2]
	if sv := ofDomain.RelationshipSeverance; ofDomain.Account != nil || sv == nil || sv.Kind != clientapi.RelationshipSeveranceKindDomainSuspension ||
		sv.Domain == nil || *sv.Domain != "bad.example" || sv.FollowersLost != 2 || sv.FollowingLost != 1 {
		t.Errorf("severance of a domain = %+v (%+v), want bad.example, 2 and 1, and no account", ofDomain, ofDomain.RelationshipSeverance)
	}
	if sv := ofAccount.RelationshipSeverance; ofAccount.Account == nil || sv == nil || sv.Kind != clientapi.RelationshipSeveranceKindAccountSuspension ||
		sv.Domain != nil || sv.FollowersLost != 1 || sv.FollowingLost != 0 {
		t.Errorf("severance of an account = %+v (%+v), want the account, 1 and 0", ofAccount, ofAccount.RelationshipSeverance)
	}
	if gone.Type != clientapi.NotificationTypeRelationshipsSevered || gone.RelationshipSeverance != nil || gone.Account != nil {
		t.Errorf("severance that is gone = %+v, want neither the severance nor an account", gone)
	}
}

func TestListNotifications(t *testing.T) {
	e, f, views := newNotificationEnvWithViews(t)

	res := e.send(t, http.MethodGet, "/notifications", "read", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	page := decode[clientapi.NotificationPage](t, res)
	if got, want := ids(page), []string{notificationID(4), notificationID(3), notificationID(2), notificationID(1)}; !slices.Equal(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
	if page.NextCursor != nil {
		t.Errorf("next_cursor = %q on the last page", *page.NextCursor)
	}
	if got := f.queries[0]; got.Limit != 21 || got.Before != "" || got.Types != nil {
		t.Errorf("query = %+v, want the default limit and one more", got)
	}
	notice, fav, mention := page.Items[0], page.Items[1], page.Items[2]
	if notice.Type != "moderation_notice" || notice.Account != nil || notice.Status != nil {
		t.Errorf("moderation notice = %+v, want neither account nor status", notice)
	}
	// The notice carries its action, labelled in alice's language.
	if a := notice.ModerationAction; a == nil || a.Id != actionID(3) || a.Kind != clientapi.AdminAccountActionKindSuspend ||
		!strings.HasPrefix(a.KindLabel, "凍結") || a.ReasonCategoryLabel != "法的な理由" {
		t.Errorf("the action of the moderation notice = %+v", notice.ModerationAction)
	}
	if fav.ModerationAction != nil {
		t.Errorf("favourite has the moderation action %+v", fav.ModerationAction)
	}
	if fav.Type != "favourite" || fav.Account == nil || fav.Account.Username != "alice" ||
		!fav.CreatedAt.Equal(time.Date(2026, 10, 15, 0, 2, 0, 0, time.UTC)) {
		t.Errorf("favourite = %+v", fav)
	}
	if fav.Status == nil || fav.Status.Id != statusID || fav.Status.Content != "<p>hi</p>" || fav.Status.FavouritesCount != 1 {
		t.Errorf("favourite status = %+v, want the embedded status", fav.Status)
	}
	// The post that alice may not see is left out; the notification stays.
	if mention.Type != "mention" || mention.Status != nil || mention.Account == nil {
		t.Errorf("mention = %+v, want the account without the hidden status", mention)
	}
	// The posts of the page are read in one call, as alice sees them.
	if len(views.requests) != 1 || !slices.Equal(views.requests[0], []string{statusID, hiddenStatusID}) {
		t.Errorf("Views calls = %v, want one call with both statuses", views.requests)
	}
	if want := (appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: aliceAccountID}); len(views.viewers) != 1 || views.viewers[0] != want {
		t.Errorf("Views viewers = %+v, want %+v", views.viewers, want)
	}

	// Pages follow next_cursor until the last one, which has none.
	var got []string
	cursor := ""
	for range 3 {
		path := "/notifications?limit=3"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		res := e.send(t, http.MethodGet, path, "read", "")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s = %d", path, res.StatusCode)
		}
		page := decode[clientapi.NotificationPage](t, res)
		got = append(got, ids(page)...)
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if want := []string{notificationID(4), notificationID(3), notificationID(2), notificationID(1)}; !slices.Equal(got, want) {
		t.Errorf("paged ids = %v, want %v", got, want)
	}

	// types keeps only the given types.
	res = e.send(t, http.MethodGet, "/notifications?types=follow&types=mention", "read", "")
	if page := decode[clientapi.NotificationPage](t, res); !slices.Equal(ids(page), []string{notificationID(2), notificationID(1)}) {
		t.Errorf("filtered ids = %v", ids(page))
	}
}

func TestListNotificationsProblems(t *testing.T) {
	e, f := newNotificationEnv(t)
	for _, path := range []string{"/notifications?types=poke", "/notifications?limit=0", "/notifications?limit=41", "/notifications?limit=x"} {
		res := e.send(t, http.MethodGet, path, "read", "")
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", path, res.StatusCode)
		}
		// alice's locale is ja.
		if p := decodeProblem(t, res); p.Detail == nil || !strings.Contains(*p.Detail, "パラメータ") || res.Header.Get("Content-Language") != "ja" {
			t.Errorf("%s: problem = %+v (detail %v)", path, p, p.Detail)
		}
	}
	if res := e.send(t, http.MethodGet, "/notifications", "read-accounts", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("without read:notifications = %d, want 403", res.StatusCode)
	}
	f.err = errors.New("database down")
	if res := e.send(t, http.MethodGet, "/notifications", "read", ""); res.StatusCode != http.StatusInternalServerError {
		t.Errorf("failing list = %d, want 500", res.StatusCode)
	}
}

func TestNotificationMarker(t *testing.T) {
	e, _ := newNotificationEnv(t)
	marker := func(res *http.Response) clientapi.NotificationMarker {
		t.Helper()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
		if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", cc)
		}
		return decode[clientapi.NotificationMarker](t, res)
	}

	if m := marker(e.send(t, http.MethodGet, "/notifications/marker", "read", "")); m.LastReadId != nil || m.UnreadCount != 4 {
		t.Errorf("initial marker = %+v, want none read and 4 unread", m)
	}
	body := func(id string) string { return `{"last_read_id":"` + id + `"}` }
	if m := marker(e.send(t, http.MethodPut, "/notifications/marker", "write", body(notificationID(3)))); m.LastReadId == nil || *m.LastReadId != notificationID(3) || m.UnreadCount != 1 {
		t.Errorf("marker after reading 3 = %+v", m)
	}
	// The marker never moves back.
	if m := marker(e.send(t, http.MethodPut, "/notifications/marker", "write", body(notificationID(1)))); *m.LastReadId != notificationID(3) || m.UnreadCount != 1 {
		t.Errorf("marker after reading 1 = %+v, want it still at 3", m)
	}
	if m := marker(e.send(t, http.MethodGet, "/notifications/marker", "read", "")); *m.LastReadId != notificationID(3) {
		t.Errorf("marker = %+v", m)
	}

	// Another account's notification, or none at all, is not found.
	for _, id := range []string{notificationID(9), notificationID(7)} {
		res := e.send(t, http.MethodPut, "/notifications/marker", "write", body(id))
		if res.StatusCode != http.StatusNotFound || res.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("marking %s = %d, want 404", id, res.StatusCode)
		}
	}
	if res := e.send(t, http.MethodPut, "/notifications/marker", "write", "{"); res.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed body = %d, want 400", res.StatusCode)
	}
	if res := e.send(t, http.MethodPut, "/notifications/marker", "read", body(notificationID(3))); res.StatusCode != http.StatusForbidden {
		t.Errorf("without write:notifications = %d, want 403", res.StatusCode)
	}
}

func TestDismissNotification(t *testing.T) {
	e, _ := newNotificationEnv(t)
	for range 2 { // dismissing again changes nothing
		if res := e.send(t, http.MethodPost, "/notifications/"+notificationID(2)+"/dismiss", "write", ""); res.StatusCode != http.StatusNoContent {
			t.Fatalf("dismiss = %d, want 204", res.StatusCode)
		}
	}
	page := decode[clientapi.NotificationPage](t, e.send(t, http.MethodGet, "/notifications", "read", ""))
	if want := []string{notificationID(4), notificationID(3), notificationID(1)}; !slices.Equal(ids(page), want) {
		t.Errorf("ids after dismissing 2 = %v, want %v", ids(page), want)
	}
	if m := decode[clientapi.NotificationMarker](t, e.send(t, http.MethodGet, "/notifications/marker", "read", "")); m.UnreadCount != 3 {
		t.Errorf("unread count = %d, want 3", m.UnreadCount)
	}

	res := e.send(t, http.MethodPost, "/notifications/"+notificationID(9)+"/dismiss", "write", "")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("dismissing bob's notification = %d, want 404", res.StatusCode)
	}
	if p := decodeProblem(t, res); p.Type != clientapi.ProblemTypeBlank || p.Status != http.StatusNotFound {
		t.Errorf("problem = %+v", p)
	}
}
