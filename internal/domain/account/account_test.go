// SPDX-License-Identifier: AGPL-3.0-only

package account_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/account"
)

const (
	accountA  = "01890a5d-ac96-7000-8000-000000000001"
	accountB  = "01890a5d-ac96-7000-8000-000000000002"
	revision1 = "01890a5d-ac96-7000-8000-000000000011"
	revision2 = "01890a5d-ac96-7000-8000-000000000012"
	revision3 = "01890a5d-ac96-7000-8000-000000000013"
)

func TestValidateUsername(t *testing.T) {
	for _, ok := range []string{"a", "Alice_01", strings.Repeat("z", account.MaxUsernameLength)} {
		if err := account.ValidateUsername(ok); err != nil {
			t.Errorf("ValidateUsername(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "a b", "a-b", "a.b", "ålice", "@alice", strings.Repeat("z", account.MaxUsernameLength+1)} {
		if err := account.ValidateUsername(bad); !errors.Is(err, account.ErrInvalidUsername) {
			t.Errorf("ValidateUsername(%q) = %v, want ErrInvalidUsername", bad, err)
		}
	}
	if got := account.NormalizeUsername("Alice_01"); got != "alice_01" {
		t.Errorf("NormalizeUsername = %q", got)
	}
}

func TestURIs(t *testing.T) {
	id := "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f"
	actor := account.ActorURI("social.example.org", id)
	if actor != "https://social.example.org/ap/actors/"+id {
		t.Errorf("ActorURI = %q", actor)
	}
	if got := account.KeyIRI(actor, account.KeyRSASHA256); got != actor+"#main-key" {
		t.Errorf("KeyIRI(rsa) = %q", got)
	}
	if got := account.KeyIRI(actor, account.KeyEd25519); got != actor+"#ed25519-key" {
		t.Errorf("KeyIRI(ed25519) = %q", got)
	}
}

func TestLifecycleApply(t *testing.T) {
	t0 := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	due := t0.Add(30 * 24 * time.Hour)
	ev := func(kind account.LifecycleKind, at int, due *time.Time) account.LifecycleEvent {
		return account.LifecycleEvent{AccountID: accountA, Kind: kind, OccurredAt: t0.Add(time.Duration(at) * time.Hour), DeletionDueAt: due}
	}
	steps := []struct {
		e    account.LifecycleEvent
		want account.LifecycleStateName
	}{
		{ev(account.LifecycleDeactivated, 1, nil), account.StateDeactivated},
		{ev(account.LifecycleReactivated, 2, nil), account.StateActive},
		{ev(account.LifecycleDeletionScheduled, 3, &due), account.StateDeletionScheduled},
		{ev(account.LifecycleDeletionCanceled, 4, nil), account.StateActive},
		{ev(account.LifecycleDeleted, 5, nil), account.StateDeleted},
		{ev(account.LifecycleReactivated, 6, nil), account.StateDeleted}, // deletion is final
	}
	s := account.InitialLifecycleState(accountA)
	if s.State != account.StateActive {
		t.Fatalf("initial state = %v", s.State)
	}
	for i, step := range steps {
		s = s.Apply(step.e)
		if s.State != step.want {
			t.Fatalf("step %d (%s): state = %s, want %s", i, step.e.Kind, s.State, step.want)
		}
		if (s.State == account.StateDeletionScheduled) != (s.DeletionDueAt != nil) {
			t.Fatalf("step %d: deletion due %v in state %s", i, s.DeletionDueAt, s.State)
		}
	}
	if want := t0.Add(5 * time.Hour); !s.Since.Equal(want) {
		t.Errorf("since = %v, want the time of the deletion %v", s.Since, want)
	}
	if got := account.InitialLifecycleState(accountB).Apply(ev(account.LifecycleRemoteDeleted, 1, nil)); got.State != account.StateDeleted || got.AccountID != accountB {
		t.Errorf("remote_deleted: %+v", got)
	}
}

func TestLifecycleActive(t *testing.T) {
	for state, want := range map[account.LifecycleStateName]bool{
		account.StateActive: true, account.StateDeactivated: false, account.StateDeletionScheduled: false, account.StateDeleted: false,
	} {
		if got := (account.LifecycleState{State: state}).Active(); got != want {
			t.Errorf("%s: Active() = %v, want %v", state, got, want)
		}
	}
	if !account.InitialLifecycleState(accountA).Active() {
		t.Error("a new account is not active")
	}
}

func TestProfileSupersedes(t *testing.T) {
	if !(account.ProfileRevision{ID: revision1, Seq: 1}).Supersedes(account.ProfileHead{}) {
		t.Error("a first revision does not supersede an empty head")
	}
	head := account.ProfileHead{RevisionID: revision2, Seq: 2}
	if (account.ProfileRevision{ID: revision1, Seq: 1}).Supersedes(head) || !(account.ProfileRevision{ID: revision3, Seq: 3}).Supersedes(head) {
		t.Error("Supersedes does not follow seq")
	}
}
