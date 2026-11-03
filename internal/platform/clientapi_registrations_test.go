// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"database/sql"
	"net/http"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// TestRegistrationReviewThroughTheAPI drives the registration review on the real database
// (FR-721): the pending sign-ups are listed page by page with their reasons, a moderator's
// approval and rejection are recorded in registration_decisions and the audit log, and deciding
// a decided registration again changes nothing.
func TestRegistrationReviewThroughTheAPI(t *testing.T) {
	t.Parallel()
	path := newDatabase(t, true)
	c := newStatusesClientWith(t, map[string]string{"MY_DATABASE_URL": path}, platform.AppOptions{}, "carol")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	if _, err := c.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{
		Key: instance.SettingRegistrationMode, Value: string(instance.RegistrationApproval),
	}); err != nil {
		t.Fatal(err)
	}
	signUp := func(name, reason string) appidentity.SignedUp {
		t.Helper()
		s, err := c.app.Identity.SignUp(ctx, appidentity.SignUpInput{
			Username: name, Email: name + "@example.com", Password: "correct horse battery staple", Reason: reason,
		})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	dave, erin := signUp("dave", "I like ducks."), signUp("erin", "")
	me, err := c.app.Accounts.FindByID(ctx, c.ids["carol"])
	if err != nil {
		t.Fatal(err)
	}
	carol := me.Local.UserID

	var first clientapi.AdminRegistrationPage
	if res := c.do("carol", http.MethodGet, "/admin/registrations?limit=1", nil, &first); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/registrations = %d", res.StatusCode)
	}
	if len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatalf("first page = %+v, want one registration and a cursor", first)
	}
	d := first.Items[0]
	if d.UserId != dave.User.ID || d.State != clientapi.RegistrationStatePending || d.Reason == nil || *d.Reason != "I like ducks." ||
		d.Username == nil || *d.Username != "dave" || d.AccountId == nil || *d.AccountId != dave.AccountID || d.Email == nil || *d.Email != "dave@example.com" {
		t.Errorf("first registration = %+v, want dave's", d)
	}
	var second clientapi.AdminRegistrationPage
	if res := c.do("carol", http.MethodGet, "/admin/registrations?limit=1&cursor="+*first.NextCursor, nil, &second); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/registrations (second page) = %d", res.StatusCode)
	}
	if len(second.Items) != 1 || second.Items[0].UserId != erin.User.ID || second.Items[0].Reason != nil || second.NextCursor != nil {
		t.Fatalf("second page = %+v, want erin's registration alone", second)
	}

	decide := func(user, action string, body any) clientapi.AdminRegistration {
		t.Helper()
		var got clientapi.AdminRegistration
		if res := c.do("carol", http.MethodPost, "/admin/registrations/"+user+"/"+action, body, &got); res.StatusCode != http.StatusOK {
			t.Fatalf("POST %s %s = %d", action, user, res.StatusCode)
		}
		return got
	}
	if got := decide(dave.User.ID, "approve", map[string]any{"note": "welcome"}); got.State != clientapi.RegistrationStateApproved {
		t.Errorf("approved = %+v", got)
	}
	if got := decide(erin.User.ID, "reject", nil); got.State != clientapi.RegistrationStateRejected {
		t.Errorf("rejected = %+v", got)
	}
	// Deciding again changes nothing.
	if got := decide(dave.User.ID, "reject", nil); got.State != clientapi.RegistrationStateApproved {
		t.Errorf("dave after a second decision = %+v, want approved", got)
	}
	if got := decide(erin.User.ID, "approve", nil); got.State != clientapi.RegistrationStateRejected {
		t.Errorf("erin after a second decision = %+v, want rejected", got)
	}
	var left clientapi.AdminRegistrationPage
	if res := c.do("carol", http.MethodGet, "/admin/registrations", nil, &left); res.StatusCode != http.StatusOK || len(left.Items) != 0 {
		t.Errorf("pending after the decisions = %d %+v, want none", res.StatusCode, left.Items)
	}
	if res := c.do("carol", http.MethodPost, "/admin/registrations/"+carol+"/approve", nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("approving a user without a registration = %d, want 404", res.StatusCode)
	}

	type decision struct{ user, decision, by, note string }
	rows, err := db.QueryContext(ctx, `SELECT a.user_id, d.decision, COALESCE(d.decided_by_user_id, ''), COALESCE(d.note, '')
		FROM registration_decisions d JOIN registration_applications a ON a.id = d.application_id ORDER BY d.id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []decision
	for rows.Next() {
		var r decision
		if err := rows.Scan(&r.user, &r.decision, &r.by, &r.note); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []decision{{dave.User.ID, "approved", carol, "welcome"}, {erin.User.ID, "rejected", carol, ""}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("registration_decisions = %+v, want %+v", got, want)
	}

	audit, err := c.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetUser}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 2 || audit[1].Action != moderation.AuditRegistrationApproved || audit[1].TargetID != dave.User.ID ||
		audit[0].Action != moderation.AuditRegistrationRejected || audit[0].TargetID != erin.User.ID ||
		audit[0].ActorUserID != carol || audit[1].ActorUserID != carol {
		t.Errorf("audit log of the users = %+v, want dave approved and erin rejected by carol", audit)
	}
}
