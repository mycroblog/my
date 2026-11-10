// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestSingleUserMode turns a server into a one-person server and back (FR-811, ADR-0152) through
// the admin API: the mode needs exactly one local account, holds the registration closed and the
// public timelines off, refuses new local posts and every way of creating an account, and says so
// in NodeInfo and GET /instance. Turning it off restores the stored settings.
func TestSingleUserMode(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob")
	ctx := t.Context()
	setSingleUser := func(on bool) *http.Response {
		t.Helper()
		v := "false"
		if on {
			v = "true"
		}
		return c.do("alice", http.MethodPatch, "/admin/settings", map[string]any{"values": map[string]string{"single_user": v}}, nil)
	}
	if res := c.do("alice", http.MethodPatch, "/admin/settings", map[string]any{"values": map[string]string{"registration_mode": "open"}}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("registration_mode open = %d", res.StatusCode)
	}
	defaultVisibility := func() clientapi.Visibility {
		t.Helper()
		var prefs clientapi.Preferences
		if res := c.do("alice", http.MethodGet, "/preferences", nil, &prefs); res.StatusCode != http.StatusOK {
			t.Fatalf("GET /preferences = %d", res.StatusCode)
		}
		return prefs.DefaultVisibility
	}
	if res := c.do("alice", http.MethodPatch, "/preferences", map[string]any{"default_visibility": "local"}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("the default visibility local = %d", res.StatusCode)
	}

	// bob is a second local account.
	res := setSingleUser(true)
	var p clientapi.Problem
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeSingleUserRequiresOneAccount || p.Detail == nil || *p.Detail != "This server has 2 local accounts." {
		t.Fatalf("single_user with two accounts = %d %+v", res.StatusCode, p)
	}
	// A deleted account does not count.
	if _, err := c.app.Accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: c.ids["bob"], Kind: account.LifecycleDeleted, InitiatedBy: account.InitiatedBySelf,
	}); err != nil {
		t.Fatalf("RecordLifecycleEvent(deleted) = %v", err)
	}
	if res := setSingleUser(true); res.StatusCode != http.StatusOK {
		t.Fatalf("single_user with one account = %d", res.StatusCode)
	}

	var inst clientapi.Instance
	if res := c.do("", http.MethodGet, "/instance", nil, &inst); res.StatusCode != http.StatusOK || !inst.SingleUser ||
		inst.Registrations.Mode != clientapi.InstanceRegistrationsModeClosed || inst.Timelines.Local != clientapi.InstanceAudienceOff ||
		inst.Timelines.Federated != clientapi.InstanceAudienceOff {
		t.Errorf("GET /instance on a one-person server = %d %+v", res.StatusCode, inst)
	}
	// The landing page shows the operator: the one account that counts, not the deleted bob.
	if inst.SingleUserAccount == nil || inst.SingleUserAccount.Id != c.ids["alice"] {
		t.Errorf("GET /instance single_user_account = %+v, want alice", inst.SingleUserAccount)
	}
	code, doc := apGet(t, c, "/nodeinfo/2.1")
	meta, _ := doc["metadata"].(map[string]any)
	if code != http.StatusOK || doc["openRegistrations"] != false || meta["singleUser"] != true {
		t.Errorf("NodeInfo on a one-person server = %d openRegistrations %v, metadata %v", code, doc["openRegistrations"], meta)
	}
	for _, path := range []string{"/timelines/local", "/timelines/federated", "/accounts/suggestions", "/invites", "/admin/registrations"} {
		res := c.do("alice", http.MethodGet, path, nil, nil)
		var p clientapi.Problem
		if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusNotFound || p.Type != clientapi.ProblemTypeSingleUserMode {
			t.Errorf("GET %s on a one-person server = %d %+v, want the 404 single-user-mode", path, res.StatusCode, p)
		}
	}
	if res := c.do("alice", http.MethodGet, "/timelines/home", nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("home timeline on a one-person server = %d", res.StatusCode)
	}
	// The one person follows hashtags, whose posts reach home from other servers.
	if res := c.do("alice", http.MethodPost, "/tags/cats/follow", nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("following a hashtag on a one-person server = %d", res.StatusCode)
	}

	// No new post is local, and the posting limits leave it out.
	if res := c.do("alice", http.MethodPost, "/statuses", map[string]any{"text": "here", "visibility": "local"}, nil); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("a local post on a one-person server = %d, want 422", res.StatusCode)
	}
	c.post("alice", map[string]any{"text": "here", "visibility": "private"})
	var cred clientapi.CredentialAccount
	if res := c.do("alice", http.MethodGet, "/accounts/verify_credentials", nil, &cred); res.StatusCode != http.StatusOK {
		t.Fatalf("verify_credentials = %d", res.StatusCode)
	}
	for _, v := range cred.Posting.Visibilities {
		if v == clientapi.VisibilityLocal {
			t.Errorf("posting limits on a one-person server = %v, want no local", cred.Posting.Visibilities)
		}
	}
	if got := defaultVisibility(); got != clientapi.VisibilityPrivate {
		t.Errorf("the stored default visibility local on a one-person server reads %s, want private", got)
	}
	if res := c.do("alice", http.MethodPatch, "/preferences", map[string]any{"default_visibility": "local"}, nil); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("the default visibility local on a one-person server = %d, want 422", res.StatusCode)
	}

	// No way creates an account.
	if _, err := c.app.Identity.CreateAdmin(ctx, appidentity.CreateAdminInput{
		Username: "carol", Email: "carol@example.com", Password: "correct horse battery staple",
	}); !errors.Is(err, appidentity.ErrSingleUserMode) {
		t.Errorf("CreateAdmin on a one-person server = %v, want ErrSingleUserMode", err)
	}
	if _, err := c.app.Identity.SignUp(ctx, appidentity.SignUpInput{
		Username: "carol", Email: "carol@example.com", Password: "correct horse battery staple",
	}); !errors.Is(err, appidentity.ErrRegistrationClosed) {
		t.Errorf("SignUp on a one-person server = %v, want ErrRegistrationClosed", err)
	}
	if _, err := c.app.Identity.RegisterLocalUser(ctx, appidentity.RegisterInput{
		Username: "carol", Password: "correct horse battery staple",
	}); !errors.Is(err, appidentity.ErrSingleUserMode) {
		t.Errorf("RegisterLocalUser on a one-person server = %v, want ErrSingleUserMode", err)
	}

	// Turning the mode off restores the stored settings.
	if res := setSingleUser(false); res.StatusCode != http.StatusOK {
		t.Fatalf("turning single_user off = %d", res.StatusCode)
	}
	inst = clientapi.Instance{}
	if res := c.do("", http.MethodGet, "/instance", nil, &inst); res.StatusCode != http.StatusOK || inst.SingleUser ||
		inst.Registrations.Mode != clientapi.InstanceRegistrationsModeOpen || inst.Timelines.Local == clientapi.InstanceAudienceOff {
		t.Errorf("GET /instance after the mode = %d %+v", res.StatusCode, inst)
	}
	if inst.SingleUserAccount != nil {
		t.Errorf("GET /instance after the mode has single_user_account %+v", inst.SingleUserAccount)
	}
	if res := c.do("alice", http.MethodGet, "/timelines/local", nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("local timeline after the mode = %d", res.StatusCode)
	}
	if got := defaultVisibility(); got != clientapi.VisibilityLocal {
		t.Errorf("the default visibility after the mode = %s, want the stored local", got)
	}
	if res := c.do("alice", http.MethodPost, "/statuses", map[string]any{"text": "everyone here", "visibility": "local"}, nil); res.StatusCode != http.StatusCreated {
		t.Errorf("a local post after the mode = %d, want 201", res.StatusCode)
	}
	code, doc = apGet(t, c, "/nodeinfo/2.1")
	meta, _ = doc["metadata"].(map[string]any)
	if _, ok := meta["singleUser"]; code != http.StatusOK || ok {
		t.Errorf("NodeInfo after the mode = %d, metadata %v, want no singleUser", code, meta)
	}
}
