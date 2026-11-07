// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"errors"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/content"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	domainnotification "github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/platform"
)

// autoDeleteApp is an app on a new database, with helpers to register users and change settings.
type autoDeleteApp struct {
	*platform.App
	t *testing.T
}

func newAutoDeleteApp(t *testing.T) autoDeleteApp {
	t.Helper()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return autoDeleteApp{App: app, t: t}
}

func (a autoDeleteApp) register(username string) (accountID, userID string) {
	a.t.Helper()
	reg, err := a.Identity.RegisterLocalUser(a.t.Context(), identity.RegisterInput{
		Username: username, Password: "correct horse battery staple", DisplayName: username,
	})
	if err != nil {
		a.t.Fatal(err)
	}
	return reg.AccountID, reg.User.ID
}

func (a autoDeleteApp) set(values map[instance.SettingKey]string) {
	a.t.Helper()
	var ins []appinstance.SetSettingInput
	for k, v := range values {
		ins = append(ins, appinstance.SetSettingInput{Key: k, Value: v})
	}
	if _, err := a.Instance.SetSettings(a.t.Context(), ins); err != nil {
		a.t.Fatal(err)
	}
}

func (a autoDeleteApp) policy(accountID string) content.AutoDeletePolicy {
	a.t.Helper()
	v, err := a.AutoDelete.AutoDeletePolicy(a.t.Context(), accountID)
	if err != nil {
		a.t.Fatal(err)
	}
	return v.Policy
}

// sameRules reports whether p and q delete the same posts, whenever they were saved.
func sameRules(p, q content.AutoDeletePolicy) bool {
	p.UpdatedAt, q.UpdatedAt = time.Time{}, time.Time{}
	return reflect.DeepEqual(p, q)
}

// TestAutoDeleteServerDefault checks ADR-0151 §2: a new local account receives the server default
// as its own policy when it is created, and a later change of the default leaves the accounts that
// exist as they are, including those that received an earlier default.
func TestAutoDeleteServerDefault(t *testing.T) {
	t.Parallel()
	app := newAutoDeleteApp(t)

	// Without settings the default is the built-in one.
	alice, _ := app.register("alice")
	if got, want := app.policy(alice), content.DefaultAutoDeletePolicy(alice); !sameRules(got, want) {
		t.Errorf("alice's policy = %+v, want the built-in default %+v", got, want)
	}

	app.set(map[instance.SettingKey]string{
		instance.SettingAutoDeleteDefaultEnabled: "true", instance.SettingAutoDeleteDefaultMinAgeDays: "7",
		instance.SettingAutoDeleteDefaultKeepMedia: "true", instance.SettingAutoDeleteDefaultKeepDirect: "false",
		instance.SettingAutoDeleteDefaultKeepPrivate: "true", instance.SettingAutoDeleteDefaultKeepMinBoosts: "10",
	})
	bob, _ := app.register("bob")
	want := content.AutoDeletePolicy{
		AccountID: bob, Enabled: true, MinAgeDays: 7, KeepPinned: true, KeepBookmarked: true, KeepFavourited: true,
		KeepMedia: true, KeepVisibilities: []content.Visibility{content.VisibilityPrivate}, KeepMinBoosts: 10,
	}
	if got := app.policy(bob); !sameRules(got, want) {
		t.Errorf("bob's policy = %+v, want the server default %+v", got, want)
	}
	if got := app.policy(alice); !sameRules(got, content.DefaultAutoDeletePolicy(alice)) {
		t.Errorf("alice's policy changed with the default: %+v", got)
	}

	// A later change of the default changes neither bob nor alice.
	app.set(map[instance.SettingKey]string{instance.SettingAutoDeleteDefaultMinAgeDays: "90"})
	if got := app.policy(bob); got.MinAgeDays != 7 {
		t.Errorf("bob's policy after the default changed = %+v, want 7 days", got)
	}
	carol, _ := app.register("carol")
	if got := app.policy(carol); got.MinAgeDays != 90 || !got.Enabled {
		t.Errorf("carol's policy = %+v, want the new default of 90 days", got)
	}
}

// TestAutoDeleteServerMaximum checks ADR-0151 §3 to §6 through the app: a new account starts within
// the maximum; the maximum refuses a longer policy but not to a holder of keep_posts_longer, whose
// role a supporter plan could carry; the job tells the account that the maximum binds once it loses
// the permission, in the notifications, and holds it to the maximum after the grace period.
func TestAutoDeleteServerMaximum(t *testing.T) {
	t.Parallel()
	app := newAutoDeleteApp(t)
	ctx := t.Context()
	alice, aliceUser := app.register("alice")
	app.set(map[instance.SettingKey]string{
		instance.SettingAutoDeleteMaxAgeDays: "30", instance.SettingAutoDeleteGraceDays: "7",
		instance.SettingAutoDeleteDefaultMinAgeDays: "60",
	})

	// bob signs up under the maximum: the default of 60 days becomes 30, enabled, keeping pins only.
	bob, _ := app.register("bob")
	want := content.AutoDeletePolicy{AccountID: bob, Enabled: true, MinAgeDays: 30, KeepPinned: true, KeepVisibilities: []content.Visibility{}}
	if v, err := app.AutoDelete.AutoDeletePolicy(ctx, bob); err != nil || !sameRules(v.Policy, want) || v.Limit == nil || v.Limit.Bound {
		t.Errorf("bob's policy under the maximum = %+v, %v; want %+v within it", v, err, want)
	}

	longer := content.AutoDeletePolicy{Enabled: true, MinAgeDays: 90}
	if _, err := app.AutoDelete.UpdateAutoDeletePolicy(ctx, alice, longer); !errors.Is(err, content.ErrAutoDeleteOverLimit) {
		t.Errorf("a longer policy under the maximum = %v, want ErrAutoDeleteOverLimit", err)
	}
	// A member role with keep_posts_longer, as a supporter plan's role would be.
	if _, err := app.Identity.CreateRole(ctx, identity.CreateRoleInput{
		Key: "keeper", Position: 10, Permissions: []domainidentity.Permission{domainidentity.PermKeepPostsLonger},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Identity.GrantRole(ctx, identity.RoleChangeInput{UserID: aliceUser, RoleKey: "keeper"}); err != nil {
		t.Fatal(err)
	}
	v, err := app.AutoDelete.UpdateAutoDeletePolicy(ctx, alice, longer)
	if err != nil || v.Limit != nil {
		t.Fatalf("a longer policy with keep_posts_longer = %+v, %v; want saved without a maximum", v, err)
	}
	if _, _, err := app.AutoDelete.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if v, _ := app.AutoDelete.AutoDeletePolicy(ctx, alice); v.Limit != nil {
		t.Errorf("alice with the permission is held to the maximum: %+v", v.Limit)
	}

	// Losing the permission binds her: the job tells her, and the effective policy waits 7 days.
	if _, err := app.Identity.RevokeRole(ctx, identity.RoleChangeInput{UserID: aliceUser, RoleKey: "keeper"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := app.AutoDelete.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	v, err = app.AutoDelete.AutoDeletePolicy(ctx, alice)
	if err != nil || v.Limit == nil || !v.Limit.Bound || v.Limit.EnforceFrom.IsZero() || v.Policy.MinAgeDays != 90 {
		t.Fatalf("alice after losing the permission = %+v, %v; want bound, with the date, and her own policy kept", v, err)
	}
	if days := time.Until(v.Limit.EnforceFrom).Hours() / 24; days < 6.9 || days > 7.1 {
		t.Errorf("enforce_from in %.2f days, want 7", days)
	}
	page, err := app.Notifications.List(ctx, alice, notification.Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var notices int
	for _, n := range page {
		if n.Type == domainnotification.TypeAutoDeleteLimit {
			notices++
		}
	}
	if notices != 1 {
		t.Errorf("alice has %d auto_delete_limit notifications, want 1: %+v", notices, page)
	}
	// Another run tells her nothing more.
	if _, _, err := app.AutoDelete.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if again, _ := app.Notifications.List(ctx, alice, notification.Query{Limit: 10}); len(again) != len(page) {
		t.Errorf("a second run notified again: %d notifications, want %d", len(again), len(page))
	}
}
