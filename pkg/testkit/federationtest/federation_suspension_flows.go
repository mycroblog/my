// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/federation"
)

// fedSuspensions is an AccountPolicy that suspends the accounts it holds.
type fedSuspensions struct {
	mu       sync.Mutex
	accounts map[string]bool
}

func (s *fedSuspensions) AccountFederationSuspended(_ context.Context, accountID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accounts[accountID], nil
}

func (s *fedSuspensions) set(accountID string, suspended bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts[accountID] = suspended
}

// suspension sets whether the policy suspends an account and emits the change as the
// moderation module does.
func (e *fedEnv) suspension(t *testing.T, accountID string, suspended bool) {
	t.Helper()
	e.clock.Advance(time.Second)
	e.suspensions.set(accountID, suspended)
	e.emit(t, appmoderation.AccountSuspensionChanged{AccountID: accountID, Suspended: suspended, ChangedAt: e.clock.Now()})
}

// actorUpdates returns the Updates of an actor, oldest first.
func (e *fedEnv) actorUpdates(t *testing.T, accountID string) []federation.OutboundActivity {
	t.Helper()
	all, err := e.h.Federation.ListOutboundActivitiesOf(t.Context(), federation.OutboundAccount, accountID)
	if err != nil {
		t.Fatalf("ListOutboundActivitiesOf: %v", err)
	}
	var out []federation.OutboundActivity
	for _, a := range all {
		if a.Type == federation.ActivityUpdate {
			out = append(out, a)
		}
	}
	return out
}

// fedPropagatesSuspensions checks the Update of a local actor that a suspension and its end
// send (federation.md §11.1, ADR-0058): it goes to the followers and every known shared inbox,
// it carries the emptied actor marked suspended, with its key, while the suspension is in
// effect and the profile after it ends, it goes out while a ban schedules the deletion, and an
// action whose transaction rolls back records nothing.
func fedPropagatesSuspensions(t *testing.T, e *fedEnv) {
	author := e.local()
	e.accounts.mu.Lock()
	actor := e.accounts.actors[author]
	actor.Local = &account.LocalAccount{AccountID: author, Username: "author"}
	actor.Profile = &account.ProfileRevision{
		AccountID: author, Seq: 2, DisplayName: "Author", NoteHTML: "<p>bio</p>",
		Fields: []account.ProfileField{{Name: "site", ValueHTML: "example"}},
	}
	actor.Keys = []account.Key{{
		AccountID: author, KeyID: account.KeyIRI(actor.Account.URI, account.KeyRSASHA256), Algorithm: account.KeyRSASHA256, PublicKeyPEM: "PEM",
	}}
	e.accounts.actors[author] = actor
	e.accounts.mu.Unlock()
	for _, f := range []string{e.remote("a.example", true), e.remote("b.example", false)} {
		e.follow(f, author)
	}
	remote := e.remote("d.example", true) // a server that no follower is on
	want := []string{"https://a.example/inbox", "https://b.example/users/" + e.graph.followers[author][0] + "/inbox", "https://d.example/inbox"}
	slices.Sort(want)

	// The suspension of a remote account sends nothing from here.
	e.suspension(t, remote, true)
	// An action whose transaction rolls back records nothing.
	errRollback := errors.New("rolled back")
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := tx.Emit(t.Context(), appmoderation.AccountSuspensionChanged{AccountID: author, Suspended: true, ChangedAt: e.clock.Now()}); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("rolled-back transaction = %v", err)
	}
	e.drain(t)
	if got := e.actorUpdates(t, author); len(got) != 0 {
		t.Fatalf("Updates before the suspension = %+v, want none", got)
	}

	check := func(update federation.OutboundActivity, suspended bool) {
		t.Helper()
		if got := e.inboxes(t, update.ID); !slices.Equal(got, want) {
			t.Errorf("Update inboxes (suspended %v) = %v, want the followers and every known shared inbox %v", suspended, got, want)
		}
		doc := e.document(t, update.ID)
		person, _ := doc["object"].(map[string]any)
		key, _ := person["publicKey"].(map[string]any)
		if doc["type"] != "Update" || doc["actor"] != e.uri(author) || person["id"] != e.uri(author) || person["inbox"] != e.uri(author)+"/inbox" ||
			key["publicKeyPem"] != "PEM" {
			t.Errorf("Update (suspended %v) = %v, want the author's actor with its key", suspended, doc)
		}
		if suspended {
			if person["suspended"] != true || person["name"] != "" || person["summary"] != "" || len(person["attachment"].([]any)) != 0 {
				t.Errorf("Update object of the suspension = %v, want the emptied actor marked suspended", person)
			}
			return
		}
		if _, ok := person["suspended"]; ok || person["name"] != "Author" || person["summary"] != "<p>bio</p>" || len(person["attachment"].([]any)) != 1 {
			t.Errorf("Update object of the end = %v, want the profile, not suspended", person)
		}
	}

	// The suspension sends the emptied actor.
	e.suspension(t, author, true)
	e.drain(t)
	updates := e.actorUpdates(t, author)
	if len(updates) != 1 || updates[0].ActorAccountID != author {
		t.Fatalf("Updates after the suspension = %+v, want one by the author", updates)
	}
	check(updates[0], true)

	// Its end sends the profile again.
	e.suspension(t, author, false)
	e.drain(t)
	if updates = e.actorUpdates(t, author); len(updates) != 2 {
		t.Fatalf("Updates after the end = %+v, want two", updates)
	}
	check(updates[1], false)

	// A ban schedules the deletion: the account is no longer active, and its suspension still
	// goes out.
	e.accounts.mu.Lock()
	actor = e.accounts.actors[author]
	actor.Lifecycle.State = account.StateDeletionScheduled
	e.accounts.actors[author] = actor
	e.accounts.mu.Unlock()
	e.suspension(t, author, true)
	e.drain(t)
	if updates = e.actorUpdates(t, author); len(updates) != 3 {
		t.Fatalf("Updates after the ban = %+v, want three", updates)
	}
	check(updates[2], true)

	// The deletion at the end of the ban's grace period still sends the Delete of the actor while
	// the account is suspended.
	e.lifecycle(t, author, account.LifecycleDeleted)
	e.drain(t)
	del := e.activity(t, federation.OutboundAccount, author, federation.ActivityDelete)
	if got := e.inboxes(t, del.ID); !slices.Equal(got, want) {
		t.Errorf("Delete inboxes of the suspended actor = %v, want %v", got, want)
	}
}
