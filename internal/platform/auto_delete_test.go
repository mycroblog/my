// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
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
	p, err := a.AutoDelete.AutoDeletePolicy(a.t.Context(), accountID)
	if err != nil {
		a.t.Fatal(err)
	}
	p.UpdatedAt = p.UpdatedAt.UTC()
	return p
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
