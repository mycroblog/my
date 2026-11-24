// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestActorReadsMakeOneQuery checks that one actor, the actors of a page, and a current profile
// are each read with one query, on accounts with every part of an actor (bob without a lifecycle
// state, so in the initial one).
func TestActorReadsMakeOneQuery(t *testing.T) {
	t.Parallel()
	db, _ := newTestDB(t, "", uow.Options{})
	repo := NewRepository(db)
	ctx := t.Context()
	alice, bob := testkit.NamedID("alice"), testkit.NamedID("bob")
	if err := db.Tx(ctx, func(tx uow.Tx) error {
		for _, id := range []string{alice, bob} {
			if err := insertActor(ctx, repo, tx, id); err != nil {
				return err
			}
		}
		return repo.UpsertLifecycleState(ctx, tx, account.LifecycleState{AccountID: alice, State: account.StateDeactivated, Since: time.Now()})
	}); err != nil {
		t.Fatal(err)
	}
	svc := appaccount.NewService(appaccount.Deps{Repository: repo})
	reads := map[string]func(ctx context.Context) error{
		"FindByID": func(ctx context.Context) error {
			a, err := svc.FindByID(ctx, alice)
			if err == nil && (a.Local == nil || a.Handle == nil || a.Profile == nil || len(a.Keys) != 1 || a.Lifecycle.State != account.StateDeactivated) {
				t.Errorf("FindByID = %+v, want every part", a)
			}
			return err
		},
		"FindByIDs": func(ctx context.Context) error {
			actors, err := svc.FindByIDs(ctx, []string{alice, bob, testkit.NamedID("nobody")})
			if err == nil && (len(actors) != 2 || actors[bob].Lifecycle != account.InitialLifecycleState(bob)) {
				t.Errorf("FindByIDs = %+v, want alice and bob, bob in the initial lifecycle state", actors)
			}
			return err
		},
		"FindByIDTx": func(ctx context.Context) error {
			return db.Tx(ctx, func(tx uow.Tx) error { _, err := svc.FindByIDTx(ctx, tx, alice); return err })
		},
		"FindCurrentProfile": func(ctx context.Context) error {
			_, err := repo.FindCurrentProfile(ctx, alice)
			return err
		},
	}
	for name, read := range reads {
		tr := testkit.NewTracer()
		db.SetTracer(tr)
		ctx, parent := tr.Start(ctx, kernel.SpanSpec{Name: name}, kernel.Attrs{})
		if err := read(ctx); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		parent.End("")
		var queries []string
		for _, s := range tr.Spans() {
			if s.Kind == kernel.SpanClient && s.Name != "BEGIN" && s.Name != "COMMIT" { // a transaction's own statements are not reads
				queries = append(queries, s.Name)
			}
		}
		if len(queries) != 1 {
			t.Errorf("%s ran the queries %v, want one", name, queries)
		}
	}
}

// insertActor stores a local actor with every part but a lifecycle state: its local account,
// handle, current profile revision with a source, and a key.
func insertActor(ctx context.Context, repo *Repository, tx uow.Tx, id string) error {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	name := id[len(id)-6:]
	if err := repo.InsertAccount(ctx, tx, account.Account{
		ID: id, URI: "https://" + testkit.ContractHost + "/ap/actors/" + id, Domain: account.LocalDomain, ActorType: account.ActorPerson, CreatedAt: now,
	}); err != nil {
		return err
	}
	if err := repo.InsertLocalAccount(ctx, tx, account.LocalAccount{AccountID: id, UserID: id, Username: name, UsernameNormalized: name}); err != nil {
		return err
	}
	if err := repo.UpsertHandle(ctx, tx, account.Handle{AccountID: id, Username: name, UsernameNormalized: name, Domain: account.LocalDomain}); err != nil {
		return err
	}
	rev := account.ProfileRevision{
		ID: id, AccountID: id, Seq: 1, DisplayName: name, RevisedAt: now,
		Fields: []account.ProfileField{{Position: 0, Name: "site", ValueHTML: "site"}}, Source: &account.ProfileSource{Note: "", FieldValues: []string{"site"}},
	}
	if err := repo.InsertProfileRevision(ctx, tx, rev); err != nil {
		return err
	}
	if err := repo.UpsertProfileHead(ctx, tx, account.ProfileHead{AccountID: id, RevisionID: id, Seq: 1}); err != nil {
		return err
	}
	return repo.InsertKey(ctx, tx, account.Key{
		ID: id, AccountID: id, KeyID: "https://" + testkit.ContractHost + "/ap/actors/" + id + "#main-key", Algorithm: account.KeyEd25519,
		PublicKeyPEM: "pem", Fingerprint: []byte(id), CreatedAt: now,
	}, []byte("sealed"))
}
