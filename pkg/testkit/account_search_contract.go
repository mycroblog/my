// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"slices"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// AccountSearchHarness is one database under test: an empty database with the product schema,
// its UnitOfWork, and the identity, account, graph, and moderation repositories on it.
type AccountSearchHarness struct {
	UnitOfWork uow.UnitOfWork
	Identity   appidentity.Repository
	Accounts   appaccount.Repository
	Graph      appgraph.Repository
	Moderation appmoderation.Repository
}

// AccountSearchFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type AccountSearchFactory func(t *testing.T, opts uow.Options) AccountSearchHarness

// AccountSearchContract verifies the search of accounts (ADR-0092 §1, FR-307) on a database
// adapter, through appaccount.PrefixSearch wired to the graph and moderation modules as the
// composition root wires them: the account_search_terms projection matches the username,
// username@domain, and the display name whole and word by word (NFKC and lowercase, without
// :shortcode:s, a CJK name from its start), and the middle of a discoverable local account's
// username and display name; a local account that is not discoverable matches only its whole
// username, or a viewer that follows it; the results come exact match first, then matches from
// the start of a name, then followed, then local, then by followers; blocks in both directions, mutes in effect, deactivated,
// deleted, and suspended accounts, and accounts of a suspended domain are left out, but not
// accounts that are not indexable; profile revisions, handle checks, and a suspension and its
// lift update the projection; and a rebuild derives the same rows.
func AccountSearchContract(t *testing.T, newHarness AccountSearchFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *accountSearchEnv)
	}{
		{"MatchesUsernamesHandlesAndDisplayNames", accountSearchMatches},
		{"OrdersExactFollowedLocalFollowers", accountSearchOrders},
		{"MatchesInfixesAndDiscoverability", accountSearchInfixes},
		{"LeavesOutHiddenAccounts", accountSearchLeavesOut},
		{"FollowsProfilesHandlesAndSuspensions", accountSearchFollowsChanges},
		{"RebuildEqualsIncremental", accountSearchRebuilds},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newAccountSearchEnv(t, newHarness)) })
	}
}

type accountSearchEnv struct {
	*accountEnv
	h      AccountSearchHarness
	graph  *appgraph.Service
	mod    *appmoderation.Service
	remote *appaccount.Service
	terms  *appaccount.SearchTermProjector
	search *appaccount.PrefixSearch

	viewer string
}

func newAccountSearchEnv(t *testing.T, newHarness AccountSearchFactory) *accountSearchEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	base := newAccountEnvOn(AccountHarness{UnitOfWork: h.UnitOfWork, Identity: h.Identity, Accounts: h.Accounts}, projectors)
	e := &accountSearchEnv{accountEnv: base, h: h}
	appgraph.RegisterProjectors(projectors, h.Graph)
	e.graph = appgraph.NewService(appgraph.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Graph, Accounts: base.accounts, Clock: base.clock, IDs: base.ids, Host: contractHost,
	})
	e.mod = appmoderation.NewService(appmoderation.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Moderation, Accounts: base.accounts, Clock: base.clock, IDs: base.ids,
		Lifecycles: base.accounts,
	})
	e.remote = appaccount.NewService(appaccount.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Accounts, Clock: base.clock, IDs: base.ids,
		Sealer: kernel.NewSealer(NewKeyManager("account-search")), Host: contractHost, Sanitizer: remoteSanitizer{},
	})
	accounts := appmoderation.NewAccountPolicy(h.Moderation, base.accounts, base.clock)
	domains := appmoderation.NewFederationPolicy(h.Moderation, base.clock)
	e.terms = appaccount.NewSearchTermProjector(h.Accounts, accounts.SuspendedTx)
	e.terms.Register(projectors)
	projectors.Register(appmoderation.EventAccountSuspensionChanged, func(ctx context.Context, tx uow.Tx, ev uow.Event) error {
		return e.terms.Refresh(ctx, tx, ev.(appmoderation.AccountSuspensionChanged).AccountID)
	})
	e.search = appaccount.NewPrefixSearch(appaccount.SearchDeps{
		Repository: h.Accounts,
		Relations: func(ctx context.Context, viewerID string, ids []string) (map[string]appaccount.SearchRelation, error) {
			rels, err := e.graph.CandidateRelations(ctx, viewerID, ids)
			out := make(map[string]appaccount.SearchRelation, len(rels))
			for id, r := range rels {
				out[id] = appaccount.SearchRelation(r)
			}
			return out, err
		},
		DomainSuspended: domains.FederationSuspended,
	})
	if _, err := base.accounts.EnsureInstanceActor(t.Context()); err != nil {
		t.Fatal(err)
	}
	e.viewer = e.local(t, "viewer", "Viewer")
	return e
}

// local registers a local user with a display name and returns its account ID.
func (e *accountSearchEnv) local(t *testing.T, username, displayName string) string {
	t.Helper()
	id := e.register(t, username).AccountID
	e.rename(t, id, displayName, true)
	return id
}

// rename revises a local profile's display name and indexable; the profile stays discoverable.
func (e *accountSearchEnv) rename(t *testing.T, id, displayName string, indexable bool) {
	t.Helper()
	e.revise(t, id, appaccount.ProfileInput{DisplayName: displayName, Indexable: indexable, Discoverable: true})
}

// revise records a new revision of a local profile.
func (e *accountSearchEnv) revise(t *testing.T, id string, in appaccount.ProfileInput) {
	t.Helper()
	e.clock.Advance(time.Second)
	if _, err := e.accounts.ReviseProfile(t.Context(), id, in); err != nil {
		t.Fatalf("ReviseProfile(%s) = %v", id, err)
	}
}

// remoteActor stores a remote actor with a display name and the outcome of its handle check
// (username@domain), and returns its account ID.
func (e *accountSearchEnv) remoteActor(t *testing.T, uri, domain, username, displayName string, outcome account.HandleOutcome) string {
	t.Helper()
	e.clock.Advance(time.Second)
	a, err := e.remote.SaveRemoteActor(t.Context(), appaccount.RemoteActorInput{
		URI: uri, Domain: domain, Type: "Person", PreferredUsername: username, Name: displayName,
		InboxURL: uri + "/inbox", Document: []byte(`{"id":"` + uri + `","name":"` + displayName + `","t":"` + e.clock.Now().String() + `"}`),
		Handle: appaccount.HandleCheck{Outcome: outcome, Username: username, Domain: domain},
	})
	if err != nil {
		t.Fatalf("SaveRemoteActor(%s) = %v", uri, err)
	}
	return a.Account.ID
}

func (e *accountSearchEnv) follow(t *testing.T, follower, followee string) {
	t.Helper()
	e.clock.Advance(time.Second)
	if _, err := e.graph.Follow(t.Context(), follower, followee); err != nil {
		t.Fatalf("Follow(%s, %s) = %v", follower, followee, err)
	}
}

func (e *accountSearchEnv) suspend(t *testing.T, accountID string) string {
	t.Helper()
	e.clock.Advance(time.Second)
	a, err := e.mod.IssueAccountAction(t.Context(), appmoderation.AccountActionInput{
		TargetAccountID: accountID, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam,
	})
	if err != nil {
		t.Fatalf("IssueAccountAction(suspend %s) = %v", accountID, err)
	}
	return a.ID
}

// want checks the IDs that the viewer's search for q returns with limit.
func (e *accountSearchEnv) want(t *testing.T, q string, limit int, want ...string) {
	t.Helper()
	got, err := e.search.Search(t.Context(), e.viewer, q, limit)
	if err != nil {
		t.Fatalf("Search(%q) = %v", q, err)
	}
	if want == nil {
		want = []string{}
	}
	if !slices.Equal(got, want) {
		t.Errorf("Search(%q, %d) = %v, want %v", q, limit, got, want)
	}
}

func (e *accountSearchEnv) allTerms(t *testing.T) []account.SearchTerm {
	t.Helper()
	terms, err := e.h.Accounts.ListSearchTerms(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return terms
}

func accountSearchMatches(t *testing.T, e *accountSearchEnv) {
	alice := e.local(t, "alice", "Alice Liddell")
	alicia := e.local(t, "alicia", "アリス・リデル")
	alfred := e.remoteActor(t, "https://remote.example/users/alfred", "remote.example", "Alfred", "Alfred :blobcat: Pennyworth", account.HandleVerified)
	e.remoteActor(t, "https://remote.example/users/unverified", "remote.example", "alfonso", "Alfonso", account.HandleMismatch)

	e.want(t, "ali", 20, alice, alicia)
	e.want(t, "@ALICE", 20, alice)
	e.want(t, "ＡＬＩＣＥ", 20, alice)
	e.want(t, "liddell", 20, alice)
	e.want(t, "alice lid", 20, alice)
	e.want(t, "アリ", 20, alicia)
	e.want(t, "リデル", 20, alicia)
	e.want(t, "alf", 20, alfred)
	e.want(t, "@alfred@remote", 20, alfred)
	e.want(t, "alfred@remote.example", 20, alfred)
	e.want(t, "alfred@other", 20)
	e.want(t, "pennyworth", 20, alfred)
	e.want(t, "alfred pennyworth", 20, alfred)
	e.want(t, "blobcat", 20)
	e.want(t, ":blobcat:", 20)
	e.want(t, "zzz", 20)
	for _, q := range []string{"", "@", "   ", string(make([]rune, appaccount.MaxSearchQueryLength+1))} {
		if _, err := e.search.Search(t.Context(), e.viewer, q, 8); err == nil {
			t.Errorf("Search(%q) = nil error, want ErrInvalidSearchQuery", q)
		}
	}
	instance, err := e.accounts.InstanceActor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range e.allTerms(t) {
		if term.AccountID == instance.Account.ID {
			t.Errorf("the instance actor has the term %+v", term)
		}
	}
}

func accountSearchOrders(t *testing.T, e *accountSearchEnv) {
	alice := e.local(t, "alice", "Alice")
	alicia := e.local(t, "alicia", "Alicia")
	alma := e.local(t, "alma", "Alma")
	al := e.remoteActor(t, "https://other.example/users/al", "other.example", "al", "Someone", account.HandleVerified)
	alfred := e.remoteActor(t, "https://remote.example/users/alfred", "remote.example", "alfred", "Alfred", account.HandleVerified)
	for _, name := range []string{"bob", "carol"} {
		e.follow(t, e.local(t, name, name), alice)
	}
	e.follow(t, e.viewer, alma)

	// al matches exactly; the viewer follows alma; alice and alicia are local, and alice has more
	// followers; alfred is remote.
	e.want(t, "al", 20, al, alma, alice, alicia, alfred)
	e.want(t, "al", 2, al, alma)
	e.want(t, "al", 0, al, alma, alice, alicia, alfred)
}

func accountSearchInfixes(t *testing.T, e *accountSearchEnv) {
	alice := e.local(t, "alice", "Alice Liddell")
	licorice := e.local(t, "licorice", "Licorice")
	e.remoteActor(t, "https://remote.example/users/alfred", "remote.example", "alfred", "Alfred", account.HandleVerified)
	hidden := e.local(t, "hidden", "Hidden One")
	e.revise(t, hidden, appaccount.ProfileInput{DisplayName: "Hidden One"})

	// The middle of a local username and display name; a remote name only from its start.
	e.want(t, "iddel", 20, alice)
	e.want(t, "ce lid", 20, alice)
	e.want(t, "lfred", 20)
	// licorice matches from the start, alice in the middle.
	e.want(t, "lic", 20, licorice, alice)

	// hidden is not discoverable: only its whole username finds it, until the viewer follows it.
	e.want(t, "hid", 20)
	e.want(t, "one", 20)
	e.want(t, "idden", 20)
	e.want(t, "@hidden", 20, hidden)
	e.follow(t, e.viewer, hidden)
	e.want(t, "hid", 20, hidden)
	e.want(t, "one", 20)
}

func accountSearchLeavesOut(t *testing.T, e *accountSearchEnv) {
	ctx := t.Context()
	blocked := e.local(t, "blocked", "x")
	blocker := e.local(t, "blocker", "x")
	muted := e.local(t, "blmuted", "x")
	expired := e.local(t, "blexpired", "x")
	suspended := e.local(t, "blsuspended", "x")
	deactivated := e.local(t, "bldeactivated", "x")
	unindexed := e.local(t, "blunindexed", "x")
	e.rename(t, unindexed, "x", false)
	gone := e.remoteActor(t, "https://remote.example/users/blgone", "remote.example", "blgone", "x", account.HandleVerified)
	banned := e.remoteActor(t, "https://suspended.example/users/blbanned", "suspended.example", "blbanned", "x", account.HandleVerified)
	// Before anything hides them: the local accounts, then the remote ones, by username.
	e.want(t, "bl", 20, deactivated, expired, muted, blocked, blocker, suspended, unindexed, banned, gone)

	e.clock.Advance(time.Second)
	if _, err := e.graph.Block(ctx, e.viewer, blocked); err != nil {
		t.Fatal(err)
	}
	if _, err := e.graph.Block(ctx, blocker, e.viewer); err != nil {
		t.Fatal(err)
	}
	if _, err := e.graph.Mute(ctx, e.viewer, muted, appgraph.MuteInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.graph.Mute(ctx, e.viewer, expired, appgraph.MuteInput{Duration: time.Minute}); err != nil {
		t.Fatal(err)
	}
	e.suspend(t, suspended)
	if _, err := e.accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: deactivated, Kind: account.LifecycleDeactivated, InitiatedBy: account.InitiatedBySelf,
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.remote.MarkRemoteDeleted(ctx, tx, gone) }); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Second)
	if _, err := e.mod.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "suspended.example", Kind: moderation.DomainSuspend}); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(2 * time.Minute)

	// The mute of blexpired has expired; blunindexed is not indexable.
	e.want(t, "bl", 20, expired, unindexed)
}

func accountSearchFollowsChanges(t *testing.T, e *accountSearchEnv) {
	ctx := t.Context()
	alice := e.local(t, "alice", "Alice Liddell")
	e.rename(t, alice, "Queen of Hearts", true)
	e.want(t, "liddell", 20)
	e.want(t, "queen", 20, alice)
	e.want(t, "hearts", 20, alice)

	uri := "https://remote.example/users/alfred"
	alfred := e.remoteActor(t, uri, "remote.example", "alfred", "Alfred", account.HandleVerified)
	e.want(t, "alfred", 20, alfred)
	e.remoteActor(t, uri, "remote.example", "alfred", "Alfred", account.HandleMismatch)
	e.want(t, "alfred", 20)
	e.remoteActor(t, uri, "remote.example", "alfred", "Alfred", account.HandleVerified)
	e.want(t, "alfred", 20, alfred)
	// Another actor verifies the same handle and takes it.
	taker := e.remoteActor(t, "https://remote.example/users/alfred-new", "remote.example", "alfred", "Alfred", account.HandleVerified)
	e.want(t, "alfred@remote.example", 20, taker)
	for _, term := range e.allTerms(t) {
		if term.AccountID == alfred {
			t.Errorf("the account that lost its handle has the term %+v", term)
		}
	}

	action := e.suspend(t, alice)
	e.want(t, "queen", 20)
	e.clock.Advance(time.Second)
	if err := e.mod.LiftAccountAction(ctx, action, "", "lifted"); err != nil {
		t.Fatal(err)
	}
	e.want(t, "queen", 20, alice)
}

func accountSearchRebuilds(t *testing.T, e *accountSearchEnv) {
	alice := e.local(t, "alice", "Alice Liddell")
	e.local(t, "alicia", "アリス")
	e.remoteActor(t, "https://remote.example/users/alfred", "remote.example", "alfred", "Alfred :blobcat: Pennyworth", account.HandleVerified)
	e.remoteActor(t, "https://remote.example/users/alfonso", "remote.example", "alfonso", "Alfonso", account.HandleMismatch)
	e.suspend(t, alice)
	before := e.allTerms(t)
	if len(before) == 0 {
		t.Fatal("no search terms")
	}
	if err := e.terms.Rebuild(t.Context(), e.h.UnitOfWork); err != nil {
		t.Fatalf("Rebuild = %v", err)
	}
	if after := e.allTerms(t); !slices.Equal(after, before) {
		t.Errorf("rebuilt terms = %+v, want %+v", after, before)
	}
}
