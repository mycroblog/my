// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeSignInLimit answers the impact and records the provider asked about; "missing" is unknown.
type fakeSignInLimit struct {
	asked []string
}

func (*fakeSignInLimit) LoginProvidersOnly(context.Context) (bool, error) { return false, nil }

func (*fakeSignInLimit) RegistrationMode(context.Context) (identity.RegistrationMode, error) {
	return identity.RegistrationOpen, nil
}

func (f *fakeSignInLimit) LoginProvidersOnlyImpact(_ context.Context, _, providerID string) (appidentity.LoginProvidersOnlyImpact, error) {
	f.asked = append(f.asked, providerID)
	if providerID == unknownProviderID {
		return appidentity.LoginProvidersOnlyImpact{}, appidentity.ErrNotFound
	}
	return appidentity.LoginProvidersOnlyImpact{SettingOn: true, UsersWithoutProvider: 3, StartsLimit: true}, nil
}

const (
	knownProviderID   = "0199f0e0-0000-7000-8000-0000000000c9"
	unknownProviderID = "0199f0e0-0000-7000-8000-0000000000ca"
	impactPath        = "/admin/settings/login_providers_only/impact"
)

// TestLoginProvidersOnlyImpactPermissions lets those who change the setting and those who change the
// providers read the count, and nobody else.
func TestLoginProvidersOnlyImpactPermissions(t *testing.T) {
	for _, tc := range []struct {
		name string
		have []identity.Permission
		want int
	}{
		{"manage_settings", []identity.Permission{identity.PermManageSettings}, http.StatusOK},
		{"manage_login_providers", []identity.Permission{identity.PermManageLoginProviders}, http.StatusOK},
		{"neither", []identity.Permission{identity.PermManageJobs}, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limit := &fakeSignInLimit{}
			e := newAuthEnv(t, func(o *clientapi.Options) {
				o.SignInLimit = limit
				o.Permissions = fakePermissions{userOf(aliceAccountID): tc.have}
			})
			e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
			if res := e.send(t, http.MethodGet, impactPath+"?provider_id="+knownProviderID, "admin-read", ""); res.StatusCode != tc.want {
				t.Errorf("GET = %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}

// TestLoginProvidersOnlyImpactOfAProvider passes the provider on, and answers 404 for one that is gone.
func TestLoginProvidersOnlyImpactOfAProvider(t *testing.T) {
	limit := &fakeSignInLimit{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.SignInLimit = limit
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
	})
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	if res := e.send(t, http.MethodGet, impactPath, "admin-read", ""); res.StatusCode != http.StatusOK {
		t.Errorf("without a provider = %d", res.StatusCode)
	}
	if res := e.send(t, http.MethodGet, impactPath+"?provider_id="+knownProviderID, "admin-read", ""); res.StatusCode != http.StatusOK {
		t.Errorf("with a provider = %d", res.StatusCode)
	}
	if res := e.send(t, http.MethodGet, impactPath+"?provider_id="+unknownProviderID, "admin-read", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("with an unknown provider = %d", res.StatusCode)
	}
	if want := []string{"", knownProviderID, unknownProviderID}; len(limit.asked) != 3 || limit.asked[0] != want[0] || limit.asked[1] != want[1] || limit.asked[2] != want[2] {
		t.Errorf("asked = %q, want %q", limit.asked, want)
	}
}
