// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeAliases keeps the aliases of each account by the rules of the account module.
type fakeAliases struct {
	mu      sync.Mutex
	aliases map[string][]account.Alias
}

func (f *fakeAliases) Aliases(_ context.Context, accountID string) ([]account.Alias, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.aliases[accountID]), nil
}

func (f *fakeAliases) AddAlias(_ context.Context, accountID string, alias appaccount.Actor) (account.Alias, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if alias.IsInstanceActor() {
		return account.Alias{}, false, appaccount.ErrNotFound
	}
	if alias.Account.ID == accountID {
		return account.Alias{}, false, account.ErrSelfAlias
	}
	list := f.aliases[accountID]
	for _, a := range list {
		if a.URI == alias.Account.URI {
			return a, false, nil
		}
	}
	if len(list) >= account.MaxAliases {
		return account.Alias{}, false, account.ErrTooManyAliases
	}
	a := account.Alias{AccountID: accountID, URI: alias.Account.URI, AddedAt: time.Date(2026, 10, 25, 0, 0, len(list), 0, time.UTC)}
	f.aliases[accountID] = append(list, a)
	return a, true, nil
}

func (f *fakeAliases) RemoveAlias(_ context.Context, accountID, uri string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	list := f.aliases[accountID]
	i := slices.IndexFunc(list, func(a account.Alias) bool { return a.URI == uri })
	if i < 0 {
		return false, nil
	}
	f.aliases[accountID] = slices.Delete(list, i, i+1)
	return true, nil
}

// TestAccountAliases checks the aliases of the viewer: an alias is added from a handle or a
// URL, resolved to the actor's id, and listed with its handle; the viewer itself, a handle or URL
// that leads to no actor, and a sixth alias are a 422; and an alias is removed.
func TestAccountAliases(t *testing.T) {
	aliases := &fakeAliases{aliases: map[string][]account.Alias{}}
	resolver := &fakeResolver{}
	var accounts fakeAccounts
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Aliases = aliases
		o.Resolver = resolver
		accounts = o.Accounts.(fakeAccounts)
	})
	alice := accounts[aliceAccountID]
	add := func(acct string) *http.Response {
		t.Helper()
		return e.sendJSON(t, http.MethodPost, "/accounts/aliases", "write", fmt.Sprintf(`{"acct":%q}`, acct))
	}
	wantProblem := func(res *http.Response, typ, what string) {
		t.Helper()
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("%s = %d, want 422", what, res.StatusCode)
		}
		if p := decode[clientapi.Problem](t, res); !strings.HasSuffix(p.Type, typ) {
			t.Errorf("%s: problem type = %q, want %s", what, p.Type, typ)
		}
	}

	// A remote handle is resolved and stored as the actor's id.
	res := add("@bob@remote.example")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("add a handle = %d, want 200", res.StatusCode)
	}
	if list := decode[clientapi.AccountAliasList](t, res); len(list.Aliases) != 1 || list.Aliases[0].Uri != remoteBob.Account.URI {
		t.Fatalf("aliases after adding a handle = %+v, want bob's actor id", list.Aliases)
	}
	// The URL of the same actor, now stored, changes nothing; the alias shows its handle.
	accounts[remoteAccountID] = remoteBob
	if res := add(remoteBob.Account.URI); res.StatusCode != http.StatusOK {
		t.Fatalf("add the same alias by URL = %d, want 200", res.StatusCode)
	} else if list := decode[clientapi.AccountAliasList](t, res); len(list.Aliases) != 1 || list.Aliases[0].Acct == nil ||
		*list.Aliases[0].Acct != "bob@remote.example" {
		t.Errorf("aliases after adding the same alias = %+v, want bob with its handle", list.Aliases)
	}

	wantProblem(add("alice"), clientapi.ProblemTypeInvalidAlias, "add the viewer by handle")
	wantProblem(add(alice.Account.URI), clientapi.ProblemTypeInvalidAlias, "add the viewer by URL")
	wantProblem(add("nobody@remote.example"), clientapi.ProblemTypeInvalidAlias, "add an unresolvable handle")
	wantProblem(add("https://remote.example/notes/1"), clientapi.ProblemTypeInvalidAlias, "add a URL that is no actor")
	wantProblem(add("not a handle"), clientapi.ProblemTypeInvalidAlias, "add something that is neither")

	// Up to 5 aliases; the sixth is refused.
	for i := range account.MaxAliases - 1 {
		aliases.aliases[aliceAccountID] = append(aliases.aliases[aliceAccountID],
			account.Alias{AccountID: aliceAccountID, URI: fmt.Sprintf("https://other.example/users/%d", i), AddedAt: time.Now()})
	}
	if res := add("bob@remote.example"); res.StatusCode != http.StatusOK {
		t.Fatalf("add an alias the viewer has, at the limit = %d, want 200", res.StatusCode)
	}
	aliases.aliases[aliceAccountID] = append(aliases.aliases[aliceAccountID],
		account.Alias{AccountID: aliceAccountID, URI: "https://other.example/users/last", AddedAt: time.Now()})
	if _, err := aliases.RemoveAlias(t.Context(), aliceAccountID, remoteBob.Account.URI); err != nil {
		t.Fatal(err)
	}
	wantProblem(add("bob@remote.example"), clientapi.ProblemTypeTooManyAliases, "add a sixth alias")

	// Removing an alias lists the rest; removing one the viewer does not have changes nothing.
	res = e.sendJSON(t, http.MethodPost, "/accounts/aliases/remove", "write", `{"uri":"https://other.example/users/last"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("remove = %d, want 200", res.StatusCode)
	}
	if list := decode[clientapi.AccountAliasList](t, res); len(list.Aliases) != account.MaxAliases-1 || list.Aliases[0].Acct != nil {
		t.Errorf("aliases after removing one = %+v, want 4 without handles", list.Aliases)
	}
	res = e.sendJSON(t, http.MethodPost, "/accounts/aliases/remove", "write", `{"uri":"https://nowhere.example/users/x"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("remove an unknown alias = %d, want 200", res.StatusCode)
	}
	res = e.send(t, http.MethodGet, "/accounts/aliases", "read", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list = %d, want 200", res.StatusCode)
	}
	if list := decode[clientapi.AccountAliasList](t, res); len(list.Aliases) != account.MaxAliases-1 {
		t.Errorf("aliases = %+v, want 4", list.Aliases)
	}
}
