// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeOverview answers the counts and the operators of the instance, and counts the reads.
type fakeOverview struct {
	counts    appinstance.Counts
	operators []string
	reads     int
}

func (f *fakeOverview) Counts(context.Context) (appinstance.Counts, error) {
	f.reads++
	return f.counts, nil
}

func (f *fakeOverview) OperatorAccountIDs(context.Context) ([]string, error) {
	f.reads++
	return f.operators, nil
}

const (
	adminRoleID      = "01890a5d-ac96-774b-bcce-b302099a8101"
	goneOperatorID   = "01890a5d-ac96-774b-bcce-b302099a8102"
	unknownAccountID = "01890a5d-ac96-774b-bcce-b302099a8103"
)

func newOverviewEnv(t *testing.T, settings ...instance.CurrentSetting) (*authEnv, *fakeOverview) {
	t.Helper()
	counted := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	overview := &fakeOverview{
		counts: appinstance.Counts{
			Members: 12, DailyPosts: []int{0, 1, 2, 3, 4, 5, 6},
			StartedAt: time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC), CountedAt: counted,
		},
		// alice runs the instance; the second operator was deactivated after the read, and the
		// third account is unknown: neither is listed.
		operators: []string{aliceAccountID, goneOperatorID, unknownAccountID},
	}
	badges := &fakeBadges{choices: map[string][]identity.BadgeChoice{
		userOf(aliceAccountID): {{ProfileBadge: identity.ProfileBadge{RoleID: adminRoleID, Label: "Admin", Color: identity.BadgeRed}, Displayed: true}},
	}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		accounts := o.Accounts.(fakeAccounts)
		gone := account.InitialLifecycleState(goneOperatorID)
		gone.State = account.StateDeactivated
		accounts[goneOperatorID] = appaccount.Actor{
			Account:   account.Account{ID: goneOperatorID, URI: "https://social.example/ap/actors/" + goneOperatorID, CreatedAt: createdAt},
			Local:     &account.LocalAccount{AccountID: goneOperatorID, UserID: userOf(goneOperatorID), Username: "bob"},
			Profile:   &account.ProfileRevision{DisplayName: "Bob"},
			Lifecycle: gone,
		}
		o.Overview, o.Badges = overview, badges
		o.Instance = &fakeInstance{settings: settings}
	})
	return e, overview
}

func TestGetInstanceOverview(t *testing.T) {
	e, _ := newOverviewEnv(t)
	res := e.get(t, "/instance/overview", nil)
	got := decodeJSON[clientapi.InstanceOverview](t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET = %d", res.StatusCode)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("Cache-Control = %q, want public, max-age=60", cc)
	}
	want := clientapi.InstanceCounts{
		Members: 12, DailyPosts: []int{0, 1, 2, 3, 4, 5, 6},
		StartedAt: time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC), CountedAt: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC),
	}
	if got.Counts == nil || !reflect.DeepEqual(*got.Counts, want) {
		t.Errorf("counts = %+v, want %+v", got.Counts, want)
	}
	if got.Operators == nil || len(*got.Operators) != 1 {
		t.Fatalf("operators = %+v, want alice only", got.Operators)
	}
	alice := (*got.Operators)[0]
	wantBadges := []clientapi.AccountBadge{{RoleId: adminRoleID, Label: "Admin", Color: clientapi.BadgeColorRed}}
	if alice.Id != aliceAccountID || alice.Username != "alice" || !reflect.DeepEqual(alice.Badges, wantBadges) {
		t.Errorf("operator = %s %s %+v, want alice with the admin badge", alice.Id, alice.Username, alice.Badges)
	}

	// A signed-in viewer gets the same answer.
	res = e.get(t, "/instance/overview", map[string]string{"Authorization": "Bearer read"})
	if signedIn := decodeJSON[clientapi.InstanceOverview](t, res); !reflect.DeepEqual(signedIn, got) {
		t.Errorf("signed in = %+v, want %+v", signedIn, got)
	}
}

func TestGetInstanceOverviewHidesWhatTheSettingsHide(t *testing.T) {
	off := func(key instance.SettingKey) instance.CurrentSetting {
		return instance.CurrentSetting{Key: key, RevisionID: testkit.NamedID(string(key)), Value: "false"}
	}
	for _, tc := range []struct {
		name                     string
		settings                 []instance.CurrentSetting
		wantCounts, wantOperator bool
	}{
		{"counts off", []instance.CurrentSetting{off(instance.SettingPublicCounts)}, false, true},
		{"operators off", []instance.CurrentSetting{off(instance.SettingPublicOperators)}, true, false},
		{"both off", []instance.CurrentSetting{off(instance.SettingPublicCounts), off(instance.SettingPublicOperators)}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, overview := newOverviewEnv(t, tc.settings...)
			res := e.get(t, "/instance/overview", nil)
			got := decodeJSON[clientapi.InstanceOverview](t, res)
			if res.StatusCode != http.StatusOK {
				t.Fatalf("GET = %d", res.StatusCode)
			}
			if (got.Counts != nil) != tc.wantCounts || (got.Operators != nil) != tc.wantOperator {
				t.Errorf("counts %+v, operators %+v; want counts %v, operators %v", got.Counts, got.Operators, tc.wantCounts, tc.wantOperator)
			}
			reads := 0
			for _, shown := range []bool{tc.wantCounts, tc.wantOperator} {
				if shown {
					reads++
				}
			}
			if overview.reads != reads {
				t.Errorf("read the overview %d times, want %d (a hidden part is not read)", overview.reads, reads)
			}
		})
	}
}
