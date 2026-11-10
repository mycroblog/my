// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/pkg/testkit"
)

// AccountRepositoryContract verifies the account and identity repositories of a database adapter
// by their methods alone, the port contract of ADR-0153 §1. Of the account repository: accounts
// found by ID and by URI, with one instance actor; local accounts with one normalized username,
// found by account and by user; handles; profile revisions with their fields, the head and its
// rebuild sources; keys with their sealed private halves; the parts of many actors read at once, as
// the reads of one read them; lifecycle events and states, the due deletions and deactivations,
// and the newest reactivation; the counts and lists of local
// accounts and of the accounts of a domain; and the reads inside a transaction. Of the identity
// repository: users, password credentials with their removal, sessions with their revocations
// and activity, TOTP enrollments with their recovery codes used once, passkeys with their
// revocations, the OAuth clients, grants, codes, and tokens (with the disablement of a client and
// the revocation of its tokens), the data exports and CSV imports with their results recorded
// once, the email suppressions derived from the failures and lifts, the service closure emails
// recorded once per user, and the reads inside a transaction. The use cases on top of it
// are AccountFlows, which internal/flowtest/identity runs once on SQLite.
func AccountRepositoryContract(t *testing.T, newHarness AccountFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *identityRepo)
	}{
		{"FindsAccounts", accountRepoAccounts},
		{"KeepsOneInstanceActor", accountRepoInstanceActor},
		{"KeepsOneUsernamePerLocalAccount", accountRepoLocalAccounts},
		{"KeepsProfileRevisionsAndTheirHead", accountRepoProfiles},
		{"KeepsKeys", accountRepoKeys},
		{"ReadsTheActorPartsOfManyAccounts", accountRepoActorParts},
		{"KeepsLifecycleEventsAndStates", accountRepoLifecycle},
		{"ListsDueDeletionsAndDeactivations", accountRepoDue},
		{"ListsAndCountsAccounts", accountRepoLists},
		{"ReadsAccountsInATransaction", accountRepoTxReads},
		{"KeepsUsersAndPasswords", identityRepoPasswords},
		{"KeepsSessions", identityRepoSessions},
		{"UsesTOTPRecoveryCodesOnce", identityRepoTOTP},
		{"KeepsPasskeys", identityRepoPasskeys},
		{"ReadsSignInMethodsInATransaction", identityRepoTxReads},
		{"KeepsOAuthClientsAndTokens", oauthRepoClients},
		{"DisablesOAuthClientsAndRevokesTheirTokens", oauthRepoDisablement},
		{"KeepsDataExports", dataRepoExports},
		{"KeepsDataImports", dataRepoImports},
		{"DerivesEmailSuppressions", emailRepoSuppressions},
		{"RecordsServiceClosureEmailsOnce", emailRepoClosureEmails},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newIdentityRepo(t, newHarness)) })
	}
}

// profile inserts the next profile revision of an account, with two fields, and points the head
// at it, as the account module's projector does; it returns the revision.
func (r *identityRepo) profile(t *testing.T, accountID string, seq int, name string) account.ProfileRevision {
	t.Helper()
	r.clock.Advance(time.Second)
	rev := account.ProfileRevision{
		ID: r.ids.NewID(), AccountID: accountID, Seq: seq, DisplayName: name, NoteHTML: "<p>note of " + name + "</p>",
		Discoverable: true, Indexable: true, RevisedAt: r.clock.Now(),
		Fields: []account.ProfileField{{Position: 0, Name: "site", ValueHTML: "<a>site</a>"}, {Position: 1, Name: "pronouns", ValueHTML: "they"}},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Accounts.InsertProfileRevision(ctx, tx, rev); err != nil {
			return err
		}
		return r.h.Accounts.UpsertProfileHead(ctx, tx, account.ProfileHead{AccountID: accountID, RevisionID: rev.ID, Seq: seq})
	})
	return rev
}

// lifecycleEvent records a lifecycle event of an account and returns it.
func (r *identityRepo) lifecycleEvent(t *testing.T, accountID string, kind account.LifecycleKind, by account.Initiator) account.LifecycleEvent {
	t.Helper()
	r.clock.Advance(time.Second)
	e := account.LifecycleEvent{ID: r.ids.NewID(), AccountID: accountID, Kind: kind, InitiatedBy: by, OccurredAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.InsertLifecycleEvent(ctx, tx, e) })
	return e
}

func accountRepoAccounts(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	id := r.remoteAccount(t, "remote.example", "alfred")
	a, err := r.h.Accounts.FindAccount(ctx, id)
	if err != nil || a.ID != id || a.Domain != "remote.example" || a.ActorType != account.ActorPerson {
		t.Fatalf("FindAccount = %+v, %v", a, err)
	}
	if got, err := r.h.Accounts.FindAccountByURI(ctx, a.URI); err != nil || got.ID != id {
		t.Errorf("FindAccountByURI = %+v, %v", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Accounts.Account(ctx, tx, id); err != nil || got.ID != id || !got.CreatedAt.Equal(a.CreatedAt) {
			t.Errorf("Account = %+v, %v", got, err)
		}
		if _, err := r.h.Accounts.Account(ctx, tx, r.ids.NewID()); !errors.Is(err, appaccount.ErrNotFound) {
			t.Errorf("Account(unknown) = %v, want ErrNotFound", err)
		}
		return nil
	})
	if _, err := r.h.Accounts.FindAccount(ctx, r.ids.NewID()); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindAccount(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Accounts.FindAccountByURI(ctx, "https://remote.example/users/nobody"); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindAccountByURI(unknown) = %v, want ErrNotFound", err)
	}
	if got, err := r.h.Accounts.FindAccountsByURIs(ctx, []string{a.URI, "https://remote.example/users/nobody"}); err != nil || len(got) != 1 || got[a.URI] != a {
		t.Errorf("FindAccountsByURIs = %+v, %v; want %+v alone, as FindAccountByURI", got, err, a)
	}
	if h, err := r.h.Accounts.FindHandle(ctx, "alfred", "remote.example"); err != nil || h.AccountID != id {
		t.Errorf("FindHandle = %+v, %v", h, err)
	}
	if h, err := r.h.Accounts.FindHandleOf(ctx, id); err != nil || h.Username != "alfred" || h.Domain != "remote.example" {
		t.Errorf("FindHandleOf = %+v, %v", h, err)
	}
	if _, err := r.h.Accounts.FindHandle(ctx, "alfred", "other.example"); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindHandle(another domain) = %v, want ErrNotFound", err)
	}
	known, other := appaccount.HandleName{UsernameNormalized: "alfred", Domain: "remote.example"}, appaccount.HandleName{UsernameNormalized: "alfred", Domain: "other.example"}
	want, _ := r.h.Accounts.FindHandle(ctx, "alfred", "remote.example")
	if got, err := r.h.Accounts.FindHandles(ctx, []appaccount.HandleName{known, other}); err != nil || len(got) != 1 || got[known] != want {
		t.Errorf("FindHandles = %+v, %v; want %+v alone, as FindHandle", got, err, want)
	}
	// A handle moves with an upsert, and the projection is rebuilt from nothing.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Accounts.UpsertHandle(ctx, tx, account.Handle{AccountID: id, Username: "Alfie", UsernameNormalized: "alfie", Domain: "remote.example"})
	})
	if h, err := r.h.Accounts.FindHandleOf(ctx, id); err != nil || h.UsernameNormalized != "alfie" {
		t.Errorf("FindHandleOf after the upsert = %+v, %v", h, err)
	}
	if hs, err := r.h.Accounts.ListHandles(ctx); err != nil || len(hs) != 1 {
		t.Errorf("ListHandles = %+v, %v; want one", hs, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.DeleteAllHandles(ctx, tx) })
	if hs, err := r.h.Accounts.ListHandles(ctx); err != nil || len(hs) != 0 {
		t.Errorf("ListHandles after DeleteAllHandles = %+v, %v; want none", hs, err)
	}
}

func accountRepoInstanceActor(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	if _, err := r.h.Accounts.FindInstanceActor(ctx); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindInstanceActor before any = %v, want ErrNotFound", err)
	}
	actor := func() account.Account {
		id := r.ids.NewID()
		return account.Account{ID: id, URI: "https://" + testkit.ContractHost + "/ap/actors/" + id, Domain: account.LocalDomain, ActorType: account.ActorApplication, CreatedAt: r.clock.Now()}
	}
	first := actor()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.InsertAccount(ctx, tx, first) })
	err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.h.Accounts.InsertAccount(ctx, tx, actor()) })
	if !errors.Is(err, appaccount.ErrInstanceActorExists) {
		t.Errorf("a second instance actor = %v, want ErrInstanceActorExists", err)
	}
	if got, err := r.h.Accounts.FindInstanceActor(ctx); err != nil || got.ID != first.ID {
		t.Errorf("FindInstanceActor = %+v, %v; want %s", got, err, first.ID)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Accounts.FindInstanceActorTx(ctx, tx); err != nil || got.ID != first.ID {
			t.Errorf("FindInstanceActorTx = %+v, %v", got, err)
		}
		return nil
	})
	// A remote Application actor is not the instance actor.
	remote := account.Account{ID: r.ids.NewID(), URI: "https://remote.example/actor", Domain: "remote.example", ActorType: account.ActorApplication, CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.InsertAccount(ctx, tx, remote) })
}

func accountRepoLocalAccounts(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	user := r.user(t)
	id := r.localAccount(t, user, "Alice")
	la := account.LocalAccount{AccountID: id, UserID: user, Username: "Alice", UsernameNormalized: "alice"}
	if got, err := r.h.Accounts.FindLocalAccount(ctx, id); err != nil || got != la {
		t.Errorf("FindLocalAccount = %+v, %v; want %+v", got, err, la)
	}
	if got, err := r.h.Accounts.FindLocalAccountByUser(ctx, user); err != nil || got != la {
		t.Errorf("FindLocalAccountByUser = %+v, %v; want %+v", got, err, la)
	}
	if _, err := r.h.Accounts.FindLocalAccountByUser(ctx, r.ids.NewID()); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindLocalAccountByUser(unknown) = %v, want ErrNotFound", err)
	}
	if h, err := r.h.Accounts.FindHandle(ctx, "alice", account.LocalDomain); err != nil || h.AccountID != id {
		t.Errorf("FindHandle(local) = %+v, %v", h, err)
	}
	other := r.user(t)
	err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		aid := r.ids.NewID()
		if err := r.h.Accounts.InsertAccount(ctx, tx, account.Account{ID: aid, URI: "https://" + testkit.ContractHost + "/ap/actors/" + aid, Domain: account.LocalDomain, ActorType: account.ActorPerson, CreatedAt: r.clock.Now()}); err != nil {
			return err
		}
		return r.h.Accounts.InsertLocalAccount(ctx, tx, account.LocalAccount{AccountID: aid, UserID: other, Username: "ALICE", UsernameNormalized: "alice"})
	})
	if !errors.Is(err, appaccount.ErrUsernameTaken) {
		t.Errorf("InsertLocalAccount with a taken username = %v, want ErrUsernameTaken", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := r.h.Accounts.ListLocalAccounts(ctx, tx)
		if err != nil || !reflect.DeepEqual(got, []account.LocalAccount{la}) {
			t.Errorf("ListLocalAccounts = %+v, %v; want alice alone (the failed insert wrote nothing)", got, err)
		}
		return nil
	})
}

func accountRepoProfiles(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	id := r.localAccount(t, r.user(t), "alice")
	if _, err := r.h.Accounts.FindCurrentProfile(ctx, id); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindCurrentProfile before a revision = %v, want ErrNotFound", err)
	}
	first := r.profile(t, id, 1, "First")
	second := r.profile(t, id, 2, "Second")
	got, err := r.h.Accounts.FindCurrentProfile(ctx, id)
	if err != nil || got.ID != second.ID || got.DisplayName != "Second" || got.NoteHTML != second.NoteHTML || !got.Discoverable ||
		!reflect.DeepEqual(got.Fields, second.Fields) || !got.RevisedAt.Equal(second.RevisedAt) {
		t.Errorf("FindCurrentProfile = %+v, %v; want %+v", got, err, second)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if h, err := r.h.Accounts.ProfileHead(ctx, tx, id); err != nil || h != (account.ProfileHead{AccountID: id, RevisionID: second.ID, Seq: 2}) {
			t.Errorf("ProfileHead = %+v, %v", h, err)
		}
		if _, err := r.h.Accounts.ProfileHead(ctx, tx, r.ids.NewID()); !errors.Is(err, appaccount.ErrNotFound) {
			t.Errorf("ProfileHead(unknown) = %v, want ErrNotFound", err)
		}
		if got, err := r.h.Accounts.FindCurrentProfileTx(ctx, tx, id); err != nil || got.ID != second.ID {
			t.Errorf("FindCurrentProfileTx = %+v, %v", got, err)
		}
		seqs, err := r.h.Accounts.ListProfileRevisionSeqs(ctx, tx)
		want := []account.ProfileHead{{AccountID: id, RevisionID: first.ID, Seq: 1}, {AccountID: id, RevisionID: second.ID, Seq: 2}}
		if err != nil || !reflect.DeepEqual(seqs, want) {
			t.Errorf("ListProfileRevisionSeqs = %+v, %v; want %+v", seqs, err, want)
		}
		return r.h.Accounts.DeleteAllProfileHeads(ctx, tx)
	})
	if heads, err := r.h.Accounts.ListProfileHeads(ctx); err != nil || len(heads) != 0 {
		t.Errorf("ListProfileHeads after DeleteAllProfileHeads = %+v, %v; want none", heads, err)
	}
	if used, err := r.h.Accounts.IsMediaReferenced(ctx, r.ids.NewID()); err != nil || used {
		t.Errorf("IsMediaReferenced(unused) = %t, %v; want false", used, err)
	}
}

func accountRepoKeys(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	id := r.localAccount(t, r.user(t), "alice")
	var keys []account.Key
	for i, alg := range []account.KeyAlgorithm{account.KeyRSASHA256, account.KeyEd25519} {
		r.clock.Advance(time.Second)
		k := account.Key{ID: r.ids.NewID(), AccountID: id, KeyID: account.KeyIRI("https://"+testkit.ContractHost+"/ap/actors/"+id, alg), Algorithm: alg,
			PublicKeyPEM: "-----BEGIN PUBLIC KEY-----\nkey " + string(rune('a'+i)) + "\n-----END PUBLIC KEY-----\n", Fingerprint: []byte{byte(i), 1, 2}, CreatedAt: r.clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.h.Accounts.InsertKey(ctx, tx, k, []byte("sealed "+k.ID))
		})
		keys = append(keys, k)
	}
	remote := r.remoteAccount(t, "remote.example", "")
	rk := account.Key{ID: r.ids.NewID(), AccountID: remote, KeyID: "https://remote.example/key", Algorithm: account.KeyRSASHA256, PublicKeyPEM: "pem", Fingerprint: []byte{9}, CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.InsertKey(ctx, tx, rk, nil) })

	got, err := r.h.Accounts.ListKeys(ctx, id)
	if err != nil || len(got) != 2 || got[0].ID != keys[0].ID || got[1].ID != keys[1].ID || got[0].KeyID != keys[0].KeyID ||
		got[1].Algorithm != account.KeyEd25519 || got[0].PublicKeyPEM != keys[0].PublicKeyPEM || !slices.Equal(got[0].Fingerprint, keys[0].Fingerprint) {
		t.Errorf("ListKeys = %+v, %v; want the two keys, oldest first", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Accounts.ListKeysTx(ctx, tx, id); err != nil || len(got) != 2 {
			t.Errorf("ListKeysTx = %+v, %v; want two", got, err)
		}
		return nil
	})
	if sealed, err := r.h.Accounts.FindSealedPrivateKey(ctx, keys[1].ID); err != nil || string(sealed) != "sealed "+keys[1].ID {
		t.Errorf("FindSealedPrivateKey = %q, %v", sealed, err)
	}
	if _, err := r.h.Accounts.FindSealedPrivateKey(ctx, rk.ID); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindSealedPrivateKey of a remote key = %v, want ErrNotFound", err)
	}
	// The keys with their sealed private keys, as ListKeys and FindSealedPrivateKey read them.
	withSealed, err := r.h.Accounts.ListKeysWithSealed(ctx, id)
	if err != nil || len(withSealed) != 2 {
		t.Fatalf("ListKeysWithSealed = %+v, %v; want the two keys", withSealed, err)
	}
	for i, k := range withSealed {
		if k.Key.ID != got[i].ID || k.Key.KeyID != got[i].KeyID || k.Key.Algorithm != got[i].Algorithm || k.Key.PublicKeyPEM != got[i].PublicKeyPEM ||
			!slices.Equal(k.Key.Fingerprint, got[i].Fingerprint) || !k.Key.CreatedAt.Equal(got[i].CreatedAt) || string(k.Sealed) != "sealed "+got[i].ID {
			t.Errorf("ListKeysWithSealed[%d] = %+v; want %+v sealed", i, k, got[i])
		}
	}
	if remoteKeys, err := r.h.Accounts.ListKeysWithSealed(ctx, remote); err != nil || len(remoteKeys) != 1 || remoteKeys[0].Key.ID != rk.ID || remoteKeys[0].Sealed != nil {
		t.Errorf("ListKeysWithSealed(remote) = %+v, %v; want its key without a private key", remoteKeys, err)
	}
}

// accountRepoActorParts reads the parts of several accounts at once, the duplicates and the
// unknown IDs among them, and finds each part as the reads of one account find it.
func accountRepoActorParts(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	aliceUser := r.user(t)
	alice := r.localAccount(t, aliceUser, "alice")
	bob := r.remoteAccount(t, "remote.example", "bob")
	carol := r.remoteAccount(t, "remote.example", "") // no handle, profile, key, or state
	r.profile(t, alice, 1, "First")
	r.clock.Advance(time.Second)
	rev := account.ProfileRevision{
		ID: r.ids.NewID(), AccountID: alice, Seq: 2, DisplayName: "Alice", NoteHTML: "<p>hi</p>", RevisedAt: r.clock.Now(),
		Fields: []account.ProfileField{{Position: 0, Name: "site", ValueHTML: "<a>site</a>"}, {Position: 1, Name: "pronouns", ValueHTML: "they"}},
		Source: &account.ProfileSource{Note: "hi", FieldValues: []string{"https://alice.example", "they"}},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Accounts.InsertProfileRevision(ctx, tx, rev); err != nil {
			return err
		}
		return r.h.Accounts.UpsertProfileHead(ctx, tx, account.ProfileHead{AccountID: alice, RevisionID: rev.ID, Seq: 2})
	})
	r.profile(t, bob, 1, "Bob")
	for i, id := range []string{alice, alice, bob} {
		r.clock.Advance(time.Second)
		k := account.Key{ID: r.ids.NewID(), AccountID: id, KeyID: "https://example/key/" + string(rune('a'+i)), Algorithm: account.KeyRSASHA256,
			PublicKeyPEM: "pem " + string(rune('a'+i)), Fingerprint: []byte{byte(i)}, CreatedAt: r.clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.InsertKey(ctx, tx, k, nil) })
	}
	r.lifecycle(t, alice, account.StateDeactivated)
	unknown := r.ids.NewID()

	parts, err := r.h.Accounts.FindActorParts(ctx, []string{alice, bob, carol, unknown, alice})
	if err != nil {
		t.Fatalf("FindActorParts: %v", err)
	}
	same := func(id, part string, got any, ok bool, want any, err error) {
		t.Helper()
		switch {
		case errors.Is(err, appaccount.ErrNotFound):
			if ok {
				t.Errorf("FindActorParts: %s of %s = %+v, want none", part, id, got)
			}
		case err != nil:
			t.Fatalf("%s of %s: %v", part, id, err)
		case !ok || !reflect.DeepEqual(got, want):
			t.Errorf("FindActorParts: %s of %s = %+v (%t), want %+v", part, id, got, ok, want)
		}
	}
	for _, id := range []string{alice, bob, carol, unknown} {
		a, ok := parts.Accounts[id]
		want, err := r.h.Accounts.FindAccount(ctx, id)
		same(id, "account", a, ok, want, err)
		la, ok := parts.Locals[id]
		wantLocal, err := r.h.Accounts.FindLocalAccount(ctx, id)
		same(id, "local account", la, ok, wantLocal, err)
		h, ok := parts.Handles[id]
		wantHandle, err := r.h.Accounts.FindHandleOf(ctx, id)
		same(id, "handle", h, ok, wantHandle, err)
		p, ok := parts.Profiles[id]
		wantProfile, err := r.h.Accounts.FindCurrentProfile(ctx, id)
		same(id, "profile", p, ok, wantProfile, err)
		ls, ok := parts.Lifecycle[id]
		wantState, err := r.h.Accounts.FindLifecycleState(ctx, id)
		same(id, "lifecycle state", ls, ok, wantState, err)
		keys, err := r.h.Accounts.ListKeys(ctx, id)
		if err != nil || len(parts.Keys[id]) != len(keys) || len(keys) > 0 && !reflect.DeepEqual(parts.Keys[id], keys) {
			t.Errorf("FindActorParts: keys of %s = %+v, want %+v (%v)", id, parts.Keys[id], keys, err)
		}
	}
	if p := parts.Profiles[alice]; p.ID != rev.ID || p.Source == nil || p.Source.Note != "hi" || !slices.Equal(p.Source.FieldValues, rev.Source.FieldValues) {
		t.Errorf("FindActorParts: profile of alice = %+v, want the current revision with its source", p)
	}
	if len(parts.Keys[alice]) != 2 {
		t.Errorf("FindActorParts: keys of alice = %+v, want two", parts.Keys[alice])
	}
	want, err := r.h.Accounts.FindLocalAccountByUser(ctx, aliceUser)
	if got, gerr := r.h.Accounts.FindLocalAccountsByUsers(ctx, []string{aliceUser, r.ids.NewID()}); err != nil || gerr != nil || len(got) != 1 || got[aliceUser] != want {
		t.Errorf("FindLocalAccountsByUsers = %+v, %v; want %+v (%v) alone, as FindLocalAccountByUser", got, gerr, want, err)
	}
	if got, err := r.h.Accounts.FindAccounts(ctx, []string{alice, bob, unknown}); err != nil || len(got) != 2 || got[alice] != parts.Accounts[alice] {
		t.Errorf("FindAccounts = %+v, %v; want alice and bob, as FindAccount", got, err)
	}
	states, err := r.h.Accounts.FindLifecycleStates(ctx, []string{alice, bob, unknown})
	if err != nil {
		t.Errorf("FindLifecycleStates = %v", err)
	}
	for _, id := range []string{alice, bob, unknown} {
		got, ok := states[id]
		want, err := r.h.Accounts.FindLifecycleState(ctx, id)
		if (err == nil) != ok || ok && got != want {
			t.Errorf("FindLifecycleStates[%s] = %+v (%t); want %+v (%v), as FindLifecycleState", id, got, ok, want, err)
		}
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		accounts, err := r.h.Accounts.FindAccountsTx(ctx, tx, []string{alice, bob, unknown})
		if err == nil && (len(accounts) != 2 || accounts[alice] != parts.Accounts[alice]) {
			t.Errorf("FindAccountsTx = %+v; want alice and bob, as FindAccounts", accounts)
		}
		inTx, err2 := r.h.Accounts.FindLifecycleStatesTx(ctx, tx, []string{alice, bob, unknown})
		if err2 == nil && !maps.Equal(inTx, states) {
			t.Errorf("FindLifecycleStatesTx = %+v; want %+v, as FindLifecycleStates", inTx, states)
		}
		return errors.Join(err, err2)
	})
	if empty, err := r.h.Accounts.FindActorParts(ctx, nil); err != nil || len(empty.Accounts) != 0 {
		t.Errorf("FindActorParts(nil) = %+v, %v; want nothing", empty, err)
	}
}

func accountRepoLifecycle(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	id := r.localAccount(t, r.user(t), "alice")
	if _, err := r.h.Accounts.FindLifecycleState(ctx, id); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindLifecycleState of an active account = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Accounts.FindLatestReactivation(ctx, id); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindLatestReactivation before any = %v, want ErrNotFound", err)
	}
	deactivated := r.lifecycleEvent(t, id, account.LifecycleDeactivated, account.InitiatedBySelf)
	reactivated := r.lifecycleEvent(t, id, account.LifecycleReactivated, account.InitiatedBySelf)
	if at, err := r.h.Accounts.FindLatestReactivation(ctx, id); err != nil || !at.Equal(reactivated.OccurredAt) {
		t.Errorf("FindLatestReactivation = %v, %v; want %v", at, err, reactivated.OccurredAt)
	}
	due := r.clock.Now().Add(30 * 24 * time.Hour)
	state := account.LifecycleState{AccountID: id, State: account.StateDeletionScheduled, Since: r.clock.Now(), DeletionDueAt: &due}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if at, err := r.h.Accounts.FindLatestReactivationTx(ctx, tx, id); err != nil || !at.Equal(reactivated.OccurredAt) {
			t.Errorf("FindLatestReactivationTx = %v, %v", at, err)
		}
		if err := r.h.Accounts.UpsertLifecycleState(ctx, tx, state); err != nil {
			return err
		}
		got, err := r.h.Accounts.LifecycleState(ctx, tx, id)
		if err != nil || got.State != account.StateDeletionScheduled || got.DeletionDueAt == nil || !got.DeletionDueAt.Equal(due) {
			t.Errorf("LifecycleState = %+v, %v; want %+v", got, err, state)
		}
		if got, err := r.h.Accounts.FindLifecycleStateTx(ctx, tx, id); err != nil || got.State != account.StateDeletionScheduled {
			t.Errorf("FindLifecycleStateTx = %+v, %v", got, err)
		}
		events, err := r.h.Accounts.ListLifecycleEvents(ctx, tx)
		if err != nil || len(events) != 2 || events[0].ID != deactivated.ID || events[1].Kind != account.LifecycleReactivated {
			t.Errorf("ListLifecycleEvents = %+v, %v; want the two events in order", events, err)
		}
		return nil
	})
	if got, err := r.h.Accounts.FindLifecycleState(ctx, id); err != nil || got.State != account.StateDeletionScheduled {
		t.Errorf("FindLifecycleState = %+v, %v", got, err)
	}
	remote := r.remoteAccount(t, "remote.example", "")
	if _, err := r.h.Accounts.NewestRemoteDeletion(ctx, nil, remote); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("NewestRemoteDeletion before any = %v, want ErrNotFound", err)
	}
	r.lifecycleEvent(t, remote, account.LifecycleRemoteDeleted, account.InitiatedBySystem)
	r.lifecycleEvent(t, remote, account.LifecycleRemoteDeleted, account.InitiatedByRemote)
	if by, err := r.h.Accounts.NewestRemoteDeletion(ctx, nil, remote); err != nil || by != account.InitiatedByRemote {
		t.Errorf("NewestRemoteDeletion = %q, %v; want the newest, remote", by, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.DeleteAllLifecycleStates(ctx, tx) })
	if states, err := r.h.Accounts.ListLifecycleStates(ctx); err != nil || len(states) != 0 {
		t.Errorf("ListLifecycleStates after DeleteAllLifecycleStates = %+v, %v; want none", states, err)
	}
}

func accountRepoDue(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	now := r.clock.Now()
	schedule := func(dueIn time.Duration) string {
		id := r.remoteAccount(t, "remote.example", "")
		due := now.Add(dueIn)
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.h.Accounts.UpsertLifecycleState(ctx, tx, account.LifecycleState{AccountID: id, State: account.StateDeletionScheduled, Since: now, DeletionDueAt: &due})
		})
		return id
	}
	late, early := schedule(-time.Hour), schedule(-2*time.Hour)
	schedule(time.Hour)
	got, err := r.h.Accounts.ListDueDeletions(ctx, now, 10)
	if err != nil || len(got) != 2 || got[0].AccountID != early || got[1].AccountID != late {
		t.Errorf("ListDueDeletions = %+v, %v; want the two due, earliest first", got, err)
	}
	if got, err := r.h.Accounts.ListDueDeletions(ctx, now, 1); err != nil || len(got) != 1 || got[0].AccountID != early {
		t.Errorf("ListDueDeletions(1) = %+v, %v; want the earliest", got, err)
	}
	deactivate := func(since time.Time) string {
		id := r.remoteAccount(t, "remote.example", "")
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			return r.h.Accounts.UpsertLifecycleState(ctx, tx, account.LifecycleState{AccountID: id, State: account.StateDeactivated, Since: since})
		})
		return id
	}
	newer, older := deactivate(now.Add(-time.Hour)), deactivate(now.Add(-2*time.Hour))
	deactivate(now.Add(time.Hour))
	if got, err := r.h.Accounts.ListDueDeactivations(ctx, now, 10); err != nil || len(got) != 2 || got[0].AccountID != older || got[1].AccountID != newer {
		t.Errorf("ListDueDeactivations = %+v, %v; want the two, longest deactivated first", got, err)
	}
}

func accountRepoLists(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice := r.localAccount(t, r.user(t), "alice")
	bob := r.localAccount(t, r.user(t), "bob")
	carol := r.localAccount(t, r.user(t), "carol")
	sub := r.remoteAccount(t, "a.remote.example", "")
	top := r.remoteAccount(t, "remote.example", "")
	other := r.remoteAccount(t, "other.example", "")
	r.lifecycle(t, carol, account.StateDeleted)
	if n, err := r.h.Accounts.CountLocalAccounts(ctx); err != nil || n != 2 {
		t.Errorf("CountLocalAccounts = %d, %v; want 2 (not the deleted one)", n, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if n, err := r.h.Accounts.CountLocalAccountsTx(ctx, tx); err != nil || n != 2 {
			t.Errorf("CountLocalAccountsTx = %d, %v; want 2", n, err)
		}
		if got, err := r.h.Accounts.AccountsOnDomain(ctx, tx, "remote.example", []string{other, top, sub, alice}); err != nil || !slices.Equal(got, []string{sub, top}) {
			t.Errorf("AccountsOnDomain = %v, %v; want the domain and its subdomain", got, err)
		}
		return nil
	})
	if got, err := r.h.Accounts.ListLocalAccountIDs(ctx, "", 2); err != nil || !slices.Equal(got, []string{alice, bob}) {
		t.Errorf("ListLocalAccountIDs = %v, %v", got, err)
	}
	if got, err := r.h.Accounts.ListLocalAccountIDs(ctx, bob, 10); err != nil || !slices.Equal(got, []string{carol}) {
		t.Errorf("ListLocalAccountIDs(after bob) = %v, %v", got, err)
	}
	if got, err := r.h.Accounts.ListAccountIDsOnDomain(ctx, "remote.example", "", 10); err != nil || !slices.Equal(got, []string{sub, top}) {
		t.Errorf("ListAccountIDsOnDomain = %v, %v", got, err)
	}
	if got, err := r.h.Accounts.ListAccountIDsOnDomain(ctx, "remote.example", sub, 10); err != nil || !slices.Equal(got, []string{top}) {
		t.Errorf("ListAccountIDsOnDomain(after) = %v, %v", got, err)
	}
	unknown := r.ids.NewID()
	if got, err := r.h.Accounts.ExistingAccountIDs(ctx, []string{other, unknown, alice}); err != nil || !slices.Equal(got, []string{alice, other}) {
		t.Errorf("ExistingAccountIDs = %v, %v", got, err)
	}
	if ok, err := r.h.Accounts.AnyAccountOnDomain(ctx, "other.example", []string{alice, other}); err != nil || !ok {
		t.Errorf("AnyAccountOnDomain = %t, %v; want true", ok, err)
	}
	if ok, err := r.h.Accounts.AnyAccountOnDomain(ctx, "other.example", []string{alice, top}); err != nil || ok {
		t.Errorf("AnyAccountOnDomain(none) = %t, %v; want false", ok, err)
	}
}

func accountRepoTxReads(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	user := r.user(t)
	err := r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		id := r.ids.NewID()
		a := account.Account{ID: id, URI: "https://" + testkit.ContractHost + "/ap/actors/" + id, Domain: account.LocalDomain, ActorType: account.ActorPerson, CreatedAt: r.clock.Now()}
		la := account.LocalAccount{AccountID: id, UserID: user, Username: "dora", UsernameNormalized: "dora"}
		if err := r.h.Accounts.InsertAccount(ctx, tx, a); err != nil {
			return err
		}
		if err := r.h.Accounts.InsertLocalAccount(ctx, tx, la); err != nil {
			return err
		}
		if err := r.h.Accounts.UpsertHandle(ctx, tx, account.Handle{AccountID: id, Username: "dora", UsernameNormalized: "dora", Domain: account.LocalDomain}); err != nil {
			return err
		}
		if got, err := r.h.Accounts.FindAccountTx(ctx, tx, id); err != nil || got.ID != id {
			t.Errorf("FindAccountTx = %+v, %v", got, err)
		}
		if got, err := r.h.Accounts.FindLocalAccountTx(ctx, tx, id); err != nil || got != la {
			t.Errorf("FindLocalAccountTx = %+v, %v", got, err)
		}
		if got, err := r.h.Accounts.FindLocalAccountByUserTx(ctx, tx, user); err != nil || got != la {
			t.Errorf("FindLocalAccountByUserTx = %+v, %v", got, err)
		}
		if got, err := r.h.Accounts.FindHandleOfTx(ctx, tx, id); err != nil || got.Username != "dora" {
			t.Errorf("FindHandleOfTx = %+v, %v", got, err)
		}
		if _, err := r.h.Accounts.FindAccountTx(ctx, tx, r.ids.NewID()); !errors.Is(err, appaccount.ErrNotFound) {
			t.Errorf("FindAccountTx(unknown) = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
