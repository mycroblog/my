// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"errors"
	"net/http"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// TestSignupBlocksThroughTheAPI drives the sign-up block list on the real database (FR-719): the
// blocks that moderators add through the API refuse the sign-ups they match or make them wait for
// a moderator, a lifted or expired block no longer matches, a ban blocks the banned user's email
// address, and every addition and lift is audited.
func TestSignupBlocksThroughTheAPI(t *testing.T) {
	t.Parallel()
	c := newStatusesClientWith(t, nil, platform.AppOptions{}, "carol")
	ctx := t.Context()
	if _, err := c.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{
		Key: instance.SettingRegistrationMode, Value: string(instance.RegistrationInvite),
	}); err != nil {
		t.Fatal(err)
	}
	invite, err := c.app.Identity.CreateInvite(ctx, appidentity.CreateInviteInput{})
	if err != nil {
		t.Fatal(err)
	}
	signUp := func(name, email, ip string) (appidentity.SignedUp, error) {
		return c.app.Identity.SignUp(ctx, appidentity.SignUpInput{
			Username: name, Email: email, Password: "correct horse battery staple", InviteCode: invite.Code, IP: ip,
		})
	}
	add := func(body map[string]any, want int) clientapi.AdminSignupBlock {
		t.Helper()
		var b clientapi.AdminSignupBlock
		if res := c.do("carol", http.MethodPost, "/admin/signup_blocks", body, &b); res.StatusCode != want {
			t.Fatalf("POST /admin/signup_blocks %v = %d, want %d", body, res.StatusCode, want)
		}
		return b
	}

	domain := add(map[string]any{"kind": "email_domain", "value": "例え.jp", "severity": "reject", "comment": "spam wave"}, http.StatusCreated)
	if domain.Value != "xn--r8jz45g.jp" || domain.State != clientapi.AdminSignupBlockStateActive || domain.Comment == nil || *domain.Comment != "spam wave" {
		t.Errorf("domain block = %+v", domain)
	}
	ip := add(map[string]any{"kind": "ip_range", "value": "192.0.2.7/24", "severity": "require_approval", "expires_in": 3600}, http.StatusCreated)
	if ip.Value != "192.0.2.0/24" || ip.ExpiresAt == nil {
		t.Errorf("ip block = %+v", ip)
	}
	add(map[string]any{"kind": "ip_range", "value": "not a range", "severity": "reject"}, http.StatusUnprocessableEntity)

	// reject refuses the sign-up and writes nothing; require_approval starts it pending in invite
	// mode, which otherwise approves at once.
	if _, err := signUp("dave", "dave@mx.xn--r8jz45g.jp", ""); !errors.Is(err, appidentity.ErrSignUpRefused) {
		t.Errorf("sign-up from the blocked domain = %v, want ErrSignUpRefused", err)
	}
	pending, err := signUp("erin", "erin@example.com", "192.0.2.99")
	if err != nil || pending.State != domainidentity.RegistrationPending {
		t.Errorf("sign-up from the blocked range = %+v, %v; want pending", pending.State, err)
	}
	approved, err := signUp("frank", "frank@example.com", "198.51.100.1")
	if err != nil || approved.State != domainidentity.RegistrationApproved {
		t.Errorf("sign-up from elsewhere = %+v, %v; want approved", approved.State, err)
	}

	// A lifted block no longer matches; the username of the refused sign-up was never taken.
	var lifted clientapi.AdminSignupBlock
	if res := c.do("carol", http.MethodPost, "/admin/signup_blocks/"+domain.Id+"/lift", nil, &lifted); res.StatusCode != http.StatusOK ||
		lifted.State != clientapi.AdminSignupBlockStateLifted || lifted.LiftedAt == nil {
		t.Errorf("lift = %d %+v, want lifted", res.StatusCode, lifted)
	}
	if _, err := signUp("dave", "dave@mx.xn--r8jz45g.jp", ""); err != nil {
		t.Errorf("sign-up after the lift = %v", err)
	}
	var page clientapi.AdminSignupBlockPage
	if res := c.do("carol", http.MethodGet, "/admin/signup_blocks?active=true", nil, &page); res.StatusCode != http.StatusOK ||
		len(page.Items) != 1 || page.Items[0].Id != ip.Id {
		t.Errorf("active blocks = %d %+v, want the ip block", res.StatusCode, page.Items)
	}
	if res := c.do("carol", http.MethodGet, "/admin/signup_blocks/"+ip.Id, nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("GET a block = %d", res.StatusCode)
	}
	if res := c.do("carol", http.MethodGet, "/admin/signup_blocks/"+c.ids["carol"], nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET an unknown block = %d, want 404", res.StatusCode)
	}

	// A ban with block_email rejects new sign-ups with the banned user's address, +tags included.
	var action clientapi.AdminAccountAction
	if res := c.do("carol", http.MethodPost, "/admin/accounts/"+approved.AccountID+"/actions",
		map[string]any{"kind": "ban", "reason_category": "spam", "block_email": true}, &action); res.StatusCode != http.StatusCreated {
		t.Fatalf("ban with block_email = %d", res.StatusCode)
	}
	if res := c.do("carol", http.MethodPost, "/admin/accounts/"+approved.AccountID+"/actions",
		map[string]any{"kind": "suspend", "block_email": true}, nil); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("suspend with block_email = %d, want 422", res.StatusCode)
	}
	if _, err := signUp("frank2", "Frank+again@example.com", ""); !errors.Is(err, appidentity.ErrSignUpRefused) {
		t.Errorf("sign-up with the banned address = %v, want ErrSignUpRefused", err)
	}
	if res := c.do("carol", http.MethodGet, "/admin/signup_blocks?kind=email_hash", nil, &page); res.StatusCode != http.StatusOK ||
		len(page.Items) != 1 || page.Items[0].ModerationActionId == nil || *page.Items[0].ModerationActionId != action.Id ||
		page.Items[0].Value != moderation.EmailHash("frank@example.com") {
		t.Errorf("email_hash blocks = %d %+v, want the ban's", res.StatusCode, page.Items)
	}

	me, err := c.app.Accounts.FindByID(ctx, c.ids["carol"])
	if err != nil {
		t.Fatal(err)
	}
	audit, err := c.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetSignupBlock}, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []moderation.AuditAction{moderation.AuditSignupBlockAdded, moderation.AuditSignupBlockLifted, moderation.AuditSignupBlockAdded, moderation.AuditSignupBlockAdded}
	if len(audit) != len(want) {
		t.Fatalf("audit log of the blocks = %+v, want %v", audit, want)
	}
	for i, a := range want {
		if audit[i].Action != a || audit[i].ActorUserID != me.Local.UserID {
			t.Errorf("entry %d = %+v, want %s by carol", i, audit[i], a)
		}
	}
	if audit[1].TargetID != domain.Id || audit[1].TargetLabel != "email_domain xn--r8jz45g.jp" {
		t.Errorf("lift entry = %+v", audit[1])
	}
}

// TestImportSignupBlocksThroughTheAPI imports a list of email domains on the real database: the
// U-labels are stored in A-labels, the lines it cannot add are reported, the blocks refuse the
// sign-ups they match, and each block added is audited.
func TestImportSignupBlocksThroughTheAPI(t *testing.T) {
	t.Parallel()
	c := newStatusesClientWith(t, nil, platform.AppOptions{}, "carol")
	ctx := t.Context()
	var res clientapi.SignupBlockImportResult
	body := map[string]any{
		"kind": "email_domain", "severity": "reject",
		"text": "# disposable\n例え.jp\nspam.example,2026-10-01\n192.0.2.1\nSPAM.example\n",
	}
	if r := c.do("carol", http.MethodPost, "/admin/signup_blocks/import", body, &res); r.StatusCode != http.StatusOK {
		t.Fatalf("POST /admin/signup_blocks/import = %d", r.StatusCode)
	}
	want := []clientapi.SignupBlockImportSkipped{
		{Line: 4, Value: "192.0.2.1", Reason: clientapi.SignupBlockImportSkippedReasonInvalid},
		{Line: 5, Value: "SPAM.example", Reason: clientapi.SignupBlockImportSkippedReasonDuplicate},
	}
	if res.Added != 2 || len(res.Skipped) != len(want) || res.Skipped[0] != want[0] || res.Skipped[1] != want[1] {
		t.Errorf("import = %+v, want 2 added, %+v", res, want)
	}
	for _, email := range []string{"a@mx.xn--r8jz45g.jp", "b@spam.example"} {
		if _, err := c.app.Identity.SignUp(ctx, appidentity.SignUpInput{
			Username: "imported", Email: email, Password: "correct horse battery staple",
		}); !errors.Is(err, appidentity.ErrSignUpRefused) {
			t.Errorf("sign-up with %s = %v, want ErrSignUpRefused", email, err)
		}
	}
	// The same list again adds nothing.
	if r := c.do("carol", http.MethodPost, "/admin/signup_blocks/import", body, &res); r.StatusCode != http.StatusOK || res.Added != 0 ||
		len(res.Skipped) != 4 || res.Skipped[0].Reason != clientapi.SignupBlockImportSkippedReasonAlreadyBlocked {
		t.Errorf("import again = %d %+v, want everything skipped", r.StatusCode, res)
	}
	audit, err := c.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetSignupBlock}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 2 || audit[0].Action != moderation.AuditSignupBlockAdded || audit[1].Action != moderation.AuditSignupBlockAdded {
		t.Errorf("audit log of the import = %+v, want two additions", audit)
	}
}
