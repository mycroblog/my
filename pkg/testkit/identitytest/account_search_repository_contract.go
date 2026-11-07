// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// AccountSearchContract verifies the account_search_terms projection of a database adapter's
// account repository (ADR-0092 §1, FR-307) by its methods alone, the port contract of ADR-0153
// §1: the terms of an account are replaced as a whole and deleted with an empty set; the terms in
// a range come in term and account ID order with the domain of their account, at most limit of
// them; the accounts of a username term and the accounts with a handle come in ID order; and the
// table is emptied for its rebuild. The search on top of it, with the graph and moderation
// modules, is AccountSearchFlows, which internal/flowtest/identity runs once on SQLite.
func AccountSearchContract(t *testing.T, newHarness AccountSearchFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *identityRepo)
	}{
		{"ReplacesTheTermsOfAnAccount", accountSearchRepoReplaces},
		{"ReadsTermsInARange", accountSearchRepoRange},
		{"ListsAccountsOfUsernamesAndHandles", accountSearchRepoAccounts},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newIdentityRepo(t, func(t *testing.T, opts uow.Options) AccountHarness {
				h := newHarness(t, opts)
				return AccountHarness{UnitOfWork: h.UnitOfWork, Identity: h.Identity, Accounts: h.Accounts}
			})
			tc.run(t, r)
		})
	}
}

// terms replaces the search terms of accountID.
func (r *identityRepo) terms(t *testing.T, accountID string, terms ...account.SearchTerm) {
	t.Helper()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Accounts.ReplaceSearchTerms(ctx, tx, accountID, terms)
	})
}

func accountSearchRepoReplaces(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice := r.localAccount(t, r.user(t), "alice")
	bob := r.remoteAccount(t, "remote.example", "bob")
	term := func(id string, kind account.SearchTermKind, s string, local bool) account.SearchTerm {
		return account.SearchTerm{AccountID: id, Kind: kind, Term: s, IsLocal: local}
	}
	r.terms(t, alice, term(alice, account.SearchTermUsername, "alice", true), term(alice, account.SearchTermDisplayName, "old", true))
	r.terms(t, bob, term(bob, account.SearchTermUsername, "bob", false), term(bob, account.SearchTermUsername, "bob@remote.example", false))
	r.terms(t, alice, term(alice, account.SearchTermUsername, "alice", true), term(alice, account.SearchTermInfix, "lice", true), term(alice, account.SearchTermDisplayName, "new", true))
	got, err := r.h.Accounts.ListSearchTerms(ctx)
	want := []account.SearchTerm{
		term(alice, account.SearchTermDisplayName, "new", true),
		term(alice, account.SearchTermInfix, "lice", true),
		term(alice, account.SearchTermUsername, "alice", true),
		term(bob, account.SearchTermUsername, "bob", false),
		term(bob, account.SearchTermUsername, "bob@remote.example", false),
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ListSearchTerms = %+v, %v; want %+v", got, err, want)
	}
	r.terms(t, alice)
	if got, err := r.h.Accounts.ListSearchTerms(ctx); err != nil || len(got) != 2 || got[0].AccountID != bob {
		t.Errorf("ListSearchTerms after replacing alice's with none = %+v, %v; want bob's", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.DeleteAllSearchTerms(ctx, tx) })
	if got, err := r.h.Accounts.ListSearchTerms(ctx); err != nil || len(got) != 0 {
		t.Errorf("ListSearchTerms after DeleteAllSearchTerms = %+v, %v; want none", got, err)
	}
}

func accountSearchRepoRange(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	a1 := r.localAccount(t, r.user(t), "ann")
	a2 := r.remoteAccount(t, "remote.example", "anna")
	a3 := r.remoteAccount(t, "other.example", "ann")
	r.terms(t, a1, account.SearchTerm{AccountID: a1, Kind: account.SearchTermUsername, Term: "ann", IsLocal: true})
	r.terms(t, a2, account.SearchTerm{AccountID: a2, Kind: account.SearchTermUsername, Term: "anna"})
	r.terms(t, a3, account.SearchTerm{AccountID: a3, Kind: account.SearchTermUsername, Term: "ann"}, account.SearchTerm{AccountID: a3, Kind: account.SearchTermDisplayName, Term: "bo"})
	got, err := r.h.Accounts.SearchTerms(ctx, "ann", "ann\U0010FFFF", 10)
	want := []account.SearchTerm{
		{AccountID: a1, Kind: account.SearchTermUsername, Term: "ann", IsLocal: true, Domain: account.LocalDomain},
		{AccountID: a3, Kind: account.SearchTermUsername, Term: "ann", Domain: "other.example"},
		{AccountID: a2, Kind: account.SearchTermUsername, Term: "anna", Domain: "remote.example"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("SearchTerms(ann) = %+v, %v; want %+v", got, err, want)
	}
	if got, err := r.h.Accounts.SearchTerms(ctx, "ann", "ann\U0010FFFF", 2); err != nil || !reflect.DeepEqual(got, want[:2]) {
		t.Errorf("SearchTerms(ann, 2) = %+v, %v; want the first two", got, err)
	}
	if got, err := r.h.Accounts.SearchTerms(ctx, "anna", "annb", 10); err != nil || !reflect.DeepEqual(got, want[2:]) {
		t.Errorf("SearchTerms([anna, annb)) = %+v, %v; want anna", got, err)
	}
}

func accountSearchRepoAccounts(t *testing.T, r *identityRepo) {
	a := r.localAccount(t, r.user(t), "same")
	b := r.remoteAccount(t, "remote.example", "same")
	noHandle := r.remoteAccount(t, "remote.example", "")
	r.terms(t, b, account.SearchTerm{AccountID: b, Kind: account.SearchTermUsername, Term: "same"})
	r.terms(t, a, account.SearchTerm{AccountID: a, Kind: account.SearchTermUsername, Term: "same", IsLocal: true})
	r.terms(t, noHandle, account.SearchTerm{AccountID: noHandle, Kind: account.SearchTermDisplayName, Term: "same"})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		want := []string{a, b}
		slices.Sort(want)
		if got, err := r.h.Accounts.ListAccountIDsWithUsernameTerm(ctx, tx, "same"); err != nil || !slices.Equal(got, want) {
			t.Errorf("ListAccountIDsWithUsernameTerm = %v, %v; want %v (not the display name)", got, err, want)
		}
		if got, err := r.h.Accounts.ListHandleAccountIDs(ctx, tx); err != nil || !slices.Equal(got, want) {
			t.Errorf("ListHandleAccountIDs = %v, %v; want %v", got, err, want)
		}
		return nil
	})
}
