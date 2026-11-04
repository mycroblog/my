// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/platform"
)

// TestRoleBadgesStayOnTheProfile checks, through the composition root on SQLite, that a role
// badge that its holder shows is on the account's profile in the client API, and in none of the
// actor's JSON-LD, WebFinger, and NodeInfo (ADR-0070 §12).
func TestRoleBadgesStayOnTheProfile(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	const label = "Badge of supporters 917"
	var accountID string
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		ctx := t.Context()
		reg, err := a.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
			Username: "alice", Password: "correct horse battery staple", DisplayName: "Alice",
		})
		if err != nil {
			t.Fatalf("RegisterLocalUser: %v", err)
		}
		accountID = reg.AccountID
		if _, err := a.Identity.CreateRole(ctx, identity.CreateRoleInput{Key: "supporter", Position: 10}); err != nil {
			t.Fatal(err)
		}
		r, err := a.Identity.UpdateRole(ctx, identity.UpdateRoleInput{
			Key: "supporter", Badge: &identity.BadgeInput{Label: label, Color: domainidentity.BadgeGreen}, SetBadge: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.Identity.GrantRole(ctx, identity.RoleChangeInput{UserID: reg.User.ID, RoleKey: "supporter"}); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Identity.SetBadgeDisplayed(ctx, reg.User.ID, r.ID, true); err != nil {
			t.Fatal(err)
		}
	})

	resp, body := s.do(t, http.MethodGet, "/api/v1/accounts/"+accountID, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"label":"`+label+`"`) {
		t.Errorf("getAccount = %d %s, want the badge %q", resp.StatusCode, body, label)
	}
	for _, req := range []struct {
		path   string
		header map[string]string
	}{
		{"/ap/actors/" + accountID, map[string]string{"Accept": "application/activity+json"}},
		{"/ap/actors/" + accountID, map[string]string{"Accept": `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`}},
		{"/.well-known/webfinger?resource=acct:alice@" + cfg.Host, nil},
		{"/nodeinfo/2.1", nil},
	} {
		resp, body := s.do(t, http.MethodGet, req.path, req.header)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d %s", req.path, resp.StatusCode, body)
			continue
		}
		if strings.Contains(body, label) || strings.Contains(strings.ToLower(body), "badge") {
			t.Errorf("GET %s = %s, want no badge", req.path, body)
		}
	}
}
