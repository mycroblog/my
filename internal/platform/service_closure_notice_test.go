// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/supporters"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	domain "github.com/mycroblog/my/internal/domain/supporters"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestServiceClosureNoticePeriod drives the notice period of the service closure against a real
// database (FR-808, ADR-0088 §2): the announcement keeps the setting open but closes the
// registration (sign-ups and invites are refused, and the withdrawal opens it again); its start
// rejects the pending application, cancels the live supporter subscription at the end of its
// period at the fake provider and tells its subscriber, and refuses new subscriptions with 409;
// every local user gets a service_closure of the announcement, the change of its dates, and the
// withdrawal; and once the closing date has passed, the administrators get
// admin_service_closure_due.
func TestServiceClosureNoticePeriod(t *testing.T) {
	t.Parallel()
	e, prov := newSupportersEnv(t)
	ctx := t.Context()
	admin := e.user("alice")
	e.want(e.do(http.MethodPost, "/admin/roles", admin.token, map[string]any{"key": "supporter", "position": 10, "permissions": []string{}}, nil),
		http.StatusOK, "POST /admin/roles")
	var plan clientapi.AdminSupporterPlan
	e.want(e.do(http.MethodPost, "/admin/supporter_plans", admin.token, map[string]any{
		"role": "supporter", "name": "Supporter", "monthly_price": map[string]string{"amount": "500", "currency": "JPY"},
		"annual_discount_percent": 0, "emoji_limit": 0, "rank": 1, "visibility": "public",
	}, &plan), http.StatusOK, "POST /admin/supporter_plans")
	e.want(e.do(http.MethodPut, "/admin/supporter_program", admin.token, map[string]any{"enabled": true}, nil), http.StatusOK, "enable")
	carol, dave := e.user("carol"), e.user("dave")
	subID := subscribeOnFake(t, e.app, prov, carol.id, plan.Id, "sub_n")

	setMode := func(mode instance.RegistrationMode) {
		t.Helper()
		if _, err := e.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingRegistrationMode, Value: string(mode)}); err != nil {
			t.Fatal(err)
		}
	}
	signUp := func(name, invite string) error {
		_, err := e.app.Identity.SignUp(ctx, appidentity.SignUpInput{
			Username: name, Email: name + "@example.com", Password: "correct horse battery staple", InviteCode: invite,
		})
		return err
	}
	registrationMode := func() clientapi.InstanceRegistrationsMode {
		t.Helper()
		var inst clientapi.Instance
		e.want(e.do(http.MethodGet, "/instance", "", nil, &inst), http.StatusOK, "GET /instance")
		return inst.Registrations.Mode
	}
	setMode(instance.RegistrationApproval)
	if err := signUp("erin", ""); err != nil {
		t.Fatal(err)
	}
	pending, err := e.app.Identity.ListRegistrations(ctx, domainidentity.RegistrationPending, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending registrations before the announcement = %+v, %v", pending, err)
	}
	erin := pending[0].Application.UserID

	// The announcement. Its closing date is in the future: no administrator is asked to close yet.
	closes := time.Now().Add(45 * 24 * time.Hour).UTC().Truncate(time.Millisecond)
	e.want(e.do(http.MethodPut, "/admin/closure/announcement", admin.token, map[string]any{"closes_at": closes}, nil), http.StatusOK, "announce")

	if st, err := e.app.Identity.Registration(ctx, erin); err != nil || st.State.State != domainidentity.RegistrationRejected {
		t.Errorf("erin's registration after the announcement = %+v, %v", st.State, err)
	}
	setMode(instance.RegistrationOpen)
	if err := signUp("frank", ""); !errors.Is(err, appidentity.ErrRegistrationClosed) {
		t.Errorf("SignUp in open mode during the notice = %v, want ErrRegistrationClosed", err)
	}
	if got := registrationMode(); got != clientapi.InstanceRegistrationsModeClosed {
		t.Errorf("registrations.mode during the notice = %s, want closed", got)
	}
	if s, err := e.app.Instance.Settings(ctx); err != nil || s.RegistrationMode() != instance.RegistrationOpen {
		t.Errorf("the setting during the notice = %v, %v; it keeps its value", s.RegistrationMode(), err)
	}
	setMode(instance.RegistrationInvite)
	invite, err := e.app.Identity.CreateInvite(ctx, appidentity.CreateInviteInput{})
	if err != nil {
		t.Fatal(err)
	}
	if err := signUp("grace", invite.Code); !errors.Is(err, appidentity.ErrRegistrationClosed) {
		t.Errorf("SignUp with an invite during the notice = %v, want ErrRegistrationClosed", err)
	}

	v, err := e.app.Supporters.Subscription(ctx, subID)
	if err != nil || v.Current.State != domain.StateCanceling || v.Current.Reason != domain.ReasonServiceClosed {
		t.Errorf("carol's subscription after the announcement = %+v, %v", v.Current, err)
	}
	if s, _ := prov.Subscription("sub_n"); !s.CancelAtPeriodEnd || s.Status != supporters.StatusActive {
		t.Errorf("the provider's subscription = %+v", s)
	}
	if typ := e.want(e.do(http.MethodPost, "/supporter/checkout", dave.token, map[string]any{"plan_id": plan.Id, "interval": "month"}, nil),
		http.StatusConflict, "checkout during the notice"); typ != clientapi.ProblemTypeServiceClosing {
		t.Errorf("checkout during the notice: %s, want service-closing", typ)
	}

	// A change of the dates to the past, then the withdrawal.
	past := time.Now().Add(-time.Minute).UTC().Truncate(time.Millisecond)
	e.want(e.do(http.MethodPut, "/admin/closure/announcement", admin.token, map[string]any{"closes_at": past}, nil), http.StatusOK, "reschedule")
	if got := e.closureNotices(admin, clientapi.NotificationTypeAdminServiceClosureDue); len(got) != 1 || got[0].ServiceClosure == nil ||
		got[0].ServiceClosure.ClosesAt == nil || !got[0].ServiceClosure.ClosesAt.Equal(past) {
		t.Errorf("admin_service_closure_due after the closing date = %+v", got)
	}
	e.want(e.do(http.MethodDelete, "/admin/closure/announcement", admin.token, nil, nil), http.StatusOK, "withdraw")

	for _, u := range []testUser{admin, carol, dave} {
		got := e.closureNotices(u, clientapi.NotificationTypeServiceClosure)
		var changes []clientapi.ServiceClosureNoticeChange
		for _, n := range got {
			if n.ServiceClosure == nil || n.ServiceClosure.Change == nil || n.Account != nil {
				t.Fatalf("service_closure = %+v", n)
			}
			changes = append(changes, *n.ServiceClosure.Change)
		}
		want := []clientapi.ServiceClosureNoticeChange{clientapi.ServiceClosureNoticeChangeWithdrawn, clientapi.ServiceClosureNoticeChangeRescheduled, clientapi.ServiceClosureNoticeChangeAnnounced}
		if len(changes) != len(want) || changes[0] != want[0] || changes[1] != want[1] || changes[2] != want[2] {
			t.Errorf("service_closure of %s = %v, want %v", u.id, changes, want)
		}
		if got[2].ServiceClosure.ClosesAt == nil || !got[2].ServiceClosure.ClosesAt.Equal(closes) || got[0].ServiceClosure.ClosesAt != nil {
			t.Errorf("the dates of the service_closure of %s = %+v, %+v", u.id, got[2].ServiceClosure, got[0].ServiceClosure)
		}
	}
	if got := e.closureNotices(carol, clientapi.NotificationTypeSupporterServiceClosure); len(got) != 1 || got[0].SupporterSubscription == nil ||
		got[0].SupporterSubscription.Id != subID {
		t.Errorf("supporter_service_closure of carol = %+v", got)
	}
	if got := e.closureNotices(dave, clientapi.NotificationTypeSupporterServiceClosure); len(got) != 0 {
		t.Errorf("supporter_service_closure of dave, who does not subscribe = %+v", got)
	}

	// The withdrawal opens the registration again, with the setting it kept.
	setMode(instance.RegistrationOpen)
	if got := registrationMode(); got != clientapi.InstanceRegistrationsModeOpen {
		t.Errorf("registrations.mode after the withdrawal = %s, want open", got)
	}
	if err := signUp("frank", ""); err != nil {
		t.Errorf("SignUp after the withdrawal = %v", err)
	}
}

// closureNotices returns the notifications of type of u, newest first.
func (e *accountsEnv) closureNotices(u testUser, typ clientapi.NotificationType) []clientapi.Notification {
	e.t.Helper()
	var page clientapi.NotificationPage
	e.want(e.do(http.MethodGet, "/notifications?types="+string(typ), u.token, nil, &page), http.StatusOK, "GET /notifications")
	return page.Items
}
