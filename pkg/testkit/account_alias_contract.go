// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/federation"
)

// recordedActorUpdates returns how many Updates of the actor accountID the outbound projectors recorded
// and enqueued so far.
func (e *moveEnv) recordedActorUpdates(t *testing.T, accountID string) int {
	t.Helper()
	e.jobs.mu.Lock()
	pending := slices.Clone(e.jobs.pending)
	e.jobs.mu.Unlock()
	n := 0
	for _, j := range pending {
		if j.Type != appfederation.JobFanOut {
			continue
		}
		var p struct {
			ID string `json:"outbound_activity_id"`
		}
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			t.Fatal(err)
		}
		act, err := e.h.Federation.FindOutboundActivity(t.Context(), p.ID)
		if err != nil {
			t.Fatalf("FindOutboundActivity(%s): %v", p.ID, err)
		}
		if act.Type == federation.ActivityUpdate && act.ObjectKind == federation.OutboundAccount && act.ObjectID == accountID {
			n++
		}
	}
	return n
}

// aliasesAddAndRemove checks the aliases of a local account (ADR-0089): an alias is stored once
// by its actor's id, at most account.MaxAliases of them, never the account itself or of a remote
// account; every change records the Update of the actor, unless the federation mode does not
// publish.
func aliasesAddAndRemove(t *testing.T, e *moveEnv) {
	ctx := t.Context()
	alice := e.register(t, "alice").AccountID
	self, err := e.accounts.FindByID(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	actor := func(uri string) appaccount.Actor {
		t.Helper()
		a, err := e.accounts.FindByID(ctx, e.remote(t, uri))
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	old := actor(moveSource)
	if _, added, err := e.accounts.AddAlias(ctx, alice, old); err != nil || !added {
		t.Fatalf("AddAlias = %v, %v, want added", added, err)
	}
	if _, added, err := e.accounts.AddAlias(ctx, alice, old); err != nil || added {
		t.Errorf("AddAlias again = %v, %v, want nothing added", added, err)
	}
	if ok, err := e.accounts.HasAlias(ctx, alice, moveSource); err != nil || !ok {
		t.Errorf("HasAlias = %v, %v, want true", ok, err)
	}
	if _, _, err := e.accounts.AddAlias(ctx, alice, self); !errors.Is(err, account.ErrSelfAlias) {
		t.Errorf("AddAlias(self) = %v, want ErrSelfAlias", err)
	}
	if _, _, err := e.accounts.AddAlias(ctx, old.Account.ID, self); !errors.Is(err, appaccount.ErrNotLocal) {
		t.Errorf("AddAlias of a remote account = %v, want ErrNotLocal", err)
	}
	for i := range account.MaxAliases - 1 {
		if _, _, err := e.accounts.AddAlias(ctx, alice, actor(fmt.Sprintf("https://other.example/users/%d", i))); err != nil {
			t.Fatalf("AddAlias #%d: %v", i+2, err)
		}
	}
	if _, _, err := e.accounts.AddAlias(ctx, alice, actor("https://other.example/users/last")); !errors.Is(err, account.ErrTooManyAliases) {
		t.Errorf("AddAlias beyond the limit = %v, want ErrTooManyAliases", err)
	}
	if _, added, err := e.accounts.AddAlias(ctx, alice, old); err != nil || added {
		t.Errorf("AddAlias of an alias it has, at the limit = %v, %v, want nothing added", added, err)
	}
	list, err := e.accounts.Aliases(ctx, alice)
	if err != nil || len(list) != account.MaxAliases || list[0].URI != moveSource || list[0].AccountID != alice {
		t.Fatalf("Aliases = %+v, %v, want %d starting with %s", list, err, account.MaxAliases, moveSource)
	}
	if removed, err := e.accounts.RemoveAlias(ctx, alice, moveSource); err != nil || !removed {
		t.Errorf("RemoveAlias = %v, %v, want removed", removed, err)
	}
	if removed, err := e.accounts.RemoveAlias(ctx, alice, moveSource); err != nil || removed {
		t.Errorf("RemoveAlias again = %v, %v, want nothing removed", removed, err)
	}
	if ok, err := e.accounts.HasAlias(ctx, alice, moveSource); err != nil || ok {
		t.Errorf("HasAlias after RemoveAlias = %v, %v, want false", ok, err)
	}
	// Five additions and one removal, each an Update of the actor.
	if got := e.recordedActorUpdates(t, alice); got != account.MaxAliases+1 {
		t.Errorf("Updates of the actor = %d, want %d", got, account.MaxAliases+1)
	}
	// A mode that does not publish records none.
	e.mode.mode = federation.ModeInbound
	if _, added, err := e.accounts.AddAlias(ctx, alice, old); err != nil || !added {
		t.Fatalf("AddAlias in mode inbound = %v, %v, want added", added, err)
	}
	if got := e.recordedActorUpdates(t, alice); got != account.MaxAliases+1 {
		t.Errorf("Updates of the actor after a change in mode inbound = %d, want still %d", got, account.MaxAliases+1)
	}
}

// movesToLocalAlias checks that a Move to a local account is recorded when the account names
// the source among its aliases, and dropped as alias_missing when it does not.
func movesToLocalAlias(t *testing.T, e *moveEnv) {
	ctx := t.Context()
	alice := e.register(t, "alice").AccountID
	target, err := e.accounts.FindByID(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	source := e.remote(t, moveSource)
	if got := e.receive(t, moveDoc(moveSource, moveSource, target.Account.URI, 1)); got != appfederation.InboxDropped {
		t.Fatalf("Move to a local account without the alias = %q, want dropped", got)
	}
	if got := e.moveOutcomes(appfederation.MoveOutcomeAliasMissing); got != 1 {
		t.Errorf("alias_missing Moves = %v, want 1", got)
	}
	src, err := e.accounts.FindByID(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.accounts.AddAlias(ctx, alice, src); err != nil {
		t.Fatal(err)
	}
	if got := e.receive(t, moveDoc(moveSource, moveSource, target.Account.URI, 2)); got != appfederation.InboxHandled {
		t.Fatalf("Move to a local account with the alias = %q, want handled", got)
	}
	if st, err := e.accounts.MoveState(ctx, source); err != nil || st.TargetAccountID != alice {
		t.Errorf("MoveState = %+v, %v, want the move to %s", st, err, alice)
	}
}
