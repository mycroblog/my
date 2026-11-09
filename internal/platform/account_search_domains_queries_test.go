// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"net/http"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// accountSearchDomainsQueries counts the round trips of an account search whose candidates are
// on n remote domains.
func accountSearchDomainsQueries(t *testing.T, n int) int {
	t.Helper()
	tracer := testkit.NewTracer()
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, "alice")
	ctx := t.Context()
	for i := range n {
		domain := fmt.Sprintf("d%d.example", i)
		uri := "https://" + domain + "/users/searchee"
		if err := c.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			_, err := c.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
				URI: uri, Domain: domain, Type: "Person", PreferredUsername: "searchee", Name: "Searchee",
				InboxURL: uri + "/inbox", Document: []byte(`{"id":"` + uri + `"}`),
				Handle: appaccount.HandleCheck{Outcome: account.HandleVerified, Username: "searchee", Domain: domain},
			})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	was := ops(tracer)
	var page clientapi.AccountList
	if res := c.do("alice", http.MethodGet, "/accounts/search?q=searchee", nil, &page); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /accounts/search = %d", res.StatusCode)
	}
	count, names := opsSince(tracer, was)
	t.Logf("a search with candidates on %d domains: %d queries, %d accounts: %v", n, count, len(page.Items), names)
	if len(page.Items) != n {
		t.Errorf("the search found %d accounts, want %d", len(page.Items), n)
	}
	return count
}

// TestAccountSearchDomainsQueries checks that the round trips of an account search do not grow
// with the remote domains of its candidates: their suspensions are read together.
func TestAccountSearchDomainsQueries(t *testing.T) {
	t.Parallel()
	small, large := accountSearchDomainsQueries(t, 2), accountSearchDomainsQueries(t, 6)
	if large > small {
		t.Errorf("a search makes %d queries for candidates on 6 domains, more than the %d for 2", large, small)
	}
}
