// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

const (
	refetchRemoteStatusID = "01890a5d-ac96-774b-bcce-b302099a8001"
	refetchLocalStatusID  = "01890a5d-ac96-774b-bcce-b302099a8002"
)

// fakeRefetches records the requested refetches: refetchRemoteStatusID and bobAccountID are remote,
// refetchLocalStatusID and aliceAccountID local, social.example this server.
type fakeRefetches struct{ requested []string }

func (f *fakeRefetches) RefetchStatus(_ context.Context, statusID, byUserID string) error {
	switch statusID {
	case refetchRemoteStatusID:
		f.requested = append(f.requested, "status "+statusID+" by "+byUserID)
		return nil
	case refetchLocalStatusID:
		return appfederation.ErrNotRemote
	}
	return fmt.Errorf("%w: %s", appfederation.ErrNotFound, statusID)
}

func (f *fakeRefetches) RefetchAccount(_ context.Context, accountID, byUserID string) error {
	switch accountID {
	case bobAccountID:
		f.requested = append(f.requested, "account "+accountID+" by "+byUserID)
		return nil
	case aliceAccountID:
		return appfederation.ErrNotRemote
	}
	return appfederation.ErrNotFound
}

func (f *fakeRefetches) RefetchDomain(_ context.Context, domain, byUserID string) (string, error) {
	host, err := appfederation.CanonicalHost(domain)
	switch {
	case err != nil:
		return "", err
	case host == "social.example":
		return "", appfederation.ErrNotRemote
	}
	f.requested = append(f.requested, "domain "+host+" by "+byUserID)
	return host, nil
}

func TestRefetchRemoteStatuses(t *testing.T) {
	refetches := &fakeRefetches{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{
			userOf(aliceAccountID): {identity.PermManageDomains},
			userOf(bobAccountID):   {identity.PermManageReports},
		}
		o.Refetches = refetches
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)

	for _, tc := range []struct {
		path, token string
		status      int
	}{
		{"/admin/statuses/" + refetchRemoteStatusID + "/refetch", "admin", http.StatusAccepted},
		{"/admin/accounts/" + bobAccountID + "/refetch_statuses", "admin", http.StatusAccepted},
		{"/admin/instances/remote.example/refetch_statuses", "admin", http.StatusAccepted},
		{"/admin/statuses/" + refetchLocalStatusID + "/refetch", "admin", http.StatusUnprocessableEntity},
		{"/admin/accounts/" + aliceAccountID + "/refetch_statuses", "admin", http.StatusUnprocessableEntity},
		{"/admin/instances/social.example/refetch_statuses", "admin", http.StatusUnprocessableEntity},
		{"/admin/statuses/01890a5d-ac96-774b-bcce-b302099a8fff/refetch", "admin", http.StatusNotFound},
		{"/admin/accounts/01890a5d-ac96-774b-bcce-b302099a8fff/refetch_statuses", "admin", http.StatusNotFound},
		{"/admin/instances/bad_host!/refetch_statuses", "admin", http.StatusBadRequest},
		{"/admin/statuses/" + refetchRemoteStatusID + "/refetch", "bob-admin", http.StatusForbidden},
		{"/admin/instances/remote.example/refetch_statuses", "admin-read", http.StatusForbidden},
	} {
		res := e.admin(t, http.MethodPost, tc.path, tc.token, nil)
		if res.StatusCode != tc.status {
			t.Errorf("POST %s with %s = %d, want %d", tc.path, tc.token, res.StatusCode, tc.status)
			continue
		}
		if tc.status == http.StatusUnprocessableEntity {
			// The viewer's locale (Japanese) is the language of the problem.
			if p := decodeProblem(t, res); p.Detail == nil || *p.Detail != "取り直せるのは、リモートの投稿・リモートのアカウント・ほかのサーバーだけです。" {
				t.Errorf("POST %s = %+v, want the not-remote detail", tc.path, p)
			}
		}
	}
	user := userOf(aliceAccountID)
	want := []string{"status " + refetchRemoteStatusID + " by " + user, "account " + bobAccountID + " by " + user, "domain remote.example by " + user}
	if fmt.Sprint(refetches.requested) != fmt.Sprint(want) {
		t.Errorf("requested = %v, want %v", refetches.requested, want)
	}
}
