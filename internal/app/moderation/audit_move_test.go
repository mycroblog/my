// SPDX-License-Identifier: AGPL-3.0-only

package moderation_test

import (
	"context"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// auditEntries is an appmoderation.AuditRepository that keeps the entries it is given.
type auditEntries struct{ entries []moderation.AuditEntry }

func (a *auditEntries) InsertAuditEntry(_ context.Context, _ uow.Tx, e moderation.AuditEntry) error {
	a.entries = append(a.entries, e)
	return nil
}

func (a *auditEntries) ListAuditEntries(context.Context, appmoderation.AuditQuery, int) ([]moderation.AuditEntry, error) {
	return a.entries, nil
}

func (a *auditEntries) ExistingAuditTargets(context.Context, moderation.AuditTargetKind, []string) ([]string, error) {
	return nil, nil
}

// TestAuditOfLimitedMoves checks that the local move of a limited account to a local one writes
// the audit log entry account_moved, by the user who moved, about the account that moved
// (ADR-0089), and that the other moves write none.
func TestAuditOfLimitedMoves(t *testing.T) {
	repo := &auditEntries{}
	r := &uow.Projectors{}
	appmoderation.RegisterAuditProjector(r, repo)
	at := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	m := appaccount.Move{
		ID: "01890a5d-ac96-774b-bcce-b302099a0001", AccountID: "01890a5d-ac96-774b-bcce-b302099a0002",
		TargetAccountID: "01890a5d-ac96-774b-bcce-b302099a0003", InitiatedBy: appaccount.MoveInitiatedBySelf, OccurredAt: at,
	}
	if err := r.Project(t.Context(), nil, appaccount.Moved{Move: m}); err != nil {
		t.Fatal(err)
	}
	if len(repo.entries) != 0 {
		t.Fatalf("entries of a move without an audit ID = %+v, want none", repo.entries)
	}
	ev := appaccount.Moved{
		Move: m, SourceLimited: true, AuditID: "01890a5d-ac96-774b-bcce-b302099a0004",
		ByUserID: "01890a5d-ac96-774b-bcce-b302099a0005", SourceLabel: "@alice",
	}
	if err := r.Project(t.Context(), nil, ev); err != nil {
		t.Fatal(err)
	}
	want := moderation.AuditEntry{
		ID: ev.AuditID, ActorKind: moderation.AuditActorUser, ActorUserID: ev.ByUserID, Action: moderation.AuditAccountMoved,
		TargetKind: moderation.AuditTargetAccount, TargetID: m.AccountID, TargetLabel: "@alice", SourceEventID: m.ID, OccurredAt: at,
	}
	if len(repo.entries) != 1 || repo.entries[0] != want {
		t.Errorf("entries = %+v, want %+v", repo.entries, want)
	}
}
