// SPDX-License-Identifier: AGPL-3.0-only

package moderationtest

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// AllowlistContract verifies the allowlist of federation of a moderation repository by its
// methods alone (ADR-0159 §1, §3): an allow is inserted once per domain and keeps its first row,
// lists by domain, counts, is found, tells which of some domains are allowed, and is deleted with
// what it held; a cutoff is recorded and found, with its domain only when an allow was removed; and
// a severance of a cutoff is recorded once per account and found with the domain of its cutoff.
func AllowlistContract(t *testing.T, newHarness ModerationFactory) {
	t.Helper()
	runModRepo(t, newHarness, []modRepoCase{
		{"KeepsTheAllowsOfDomains", allowlistRepoAllows},
		{"RecordsCutoffsAndTheirSeverances", allowlistRepoCutoffs},
	})
}

func allowlistRepoAllows(t *testing.T, r *modRepo) {
	ctx := t.Context()
	repo := r.h.Moderation
	now := r.clock.Now().Truncate(time.Millisecond)
	a := moderation.DomainAllow{Domain: "b.example", Comment: "partner", CreatedByUserID: r.ids.NewID(), CreatedAt: now}
	cli := moderation.DomainAllow{Domain: "a.example", CreatedAt: now.Add(time.Second)}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, x := range []moderation.DomainAllow{a, cli} {
			if ok, err := repo.InsertDomainAllow(ctx, tx, x); err != nil || !ok {
				t.Errorf("InsertDomainAllow(%s) = %v, %v; want true", x.Domain, ok, err)
			}
		}
		again := a
		again.Comment = "other"
		if ok, err := repo.InsertDomainAllow(ctx, tx, again); err != nil || ok {
			t.Errorf("InsertDomainAllow(%s again) = %v, %v; want false", a.Domain, ok, err)
		}
		if n, err := repo.CountDomainAllows(ctx, tx); err != nil || n != 2 {
			t.Errorf("CountDomainAllows = %d, %v; want 2", n, err)
		}
		got, err := repo.FindDomainAllow(ctx, tx, a.Domain)
		if err != nil || !reflect.DeepEqual(got, a) {
			t.Errorf("FindDomainAllow = %+v, %v; want %+v", got, err, a)
		}
		if _, err := repo.FindDomainAllow(ctx, tx, "none.example"); !errors.Is(err, appmoderation.ErrNotFound) {
			t.Errorf("FindDomainAllow(none) = %v, want ErrNotFound", err)
		}
		return nil
	})
	list, err := repo.ListDomainAllows(ctx)
	if err != nil || !reflect.DeepEqual(list, []moderation.DomainAllow{cli, a}) {
		t.Errorf("ListDomainAllows = %+v, %v; want a.example then b.example", list, err)
	}
	allowed, err := repo.AllowedDomains(ctx, []string{"x.b.example", "b.example", "example", "a.example"})
	if err != nil || !reflect.DeepEqual(allowed, []string{"a.example", "b.example"}) {
		t.Errorf("AllowedDomains = %v, %v; want [a.example b.example]", allowed, err)
	}
	if allowed, err := repo.AllowedDomains(ctx, nil); err != nil || len(allowed) != 0 {
		t.Errorf("AllowedDomains(none) = %v, %v; want none", allowed, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		got, err := repo.DeleteDomainAllow(ctx, tx, a.Domain)
		if err != nil || !reflect.DeepEqual(got, a) {
			t.Errorf("DeleteDomainAllow = %+v, %v; want %+v", got, err, a)
		}
		if _, err := repo.DeleteDomainAllow(ctx, tx, a.Domain); !errors.Is(err, appmoderation.ErrNotFound) {
			t.Errorf("DeleteDomainAllow(again) = %v, want ErrNotFound", err)
		}
		if allowed, err := repo.AllowedDomainsTx(ctx, tx, []string{"a.example", "b.example"}); err != nil || !reflect.DeepEqual(allowed, []string{"a.example"}) {
			t.Errorf("AllowedDomainsTx after the delete = %v, %v; want [a.example]", allowed, err)
		}
		return nil
	})
	existing, err := repo.ExistingAuditTargets(ctx, moderation.AuditTargetDomainAllow, []string{"a.example", "b.example"})
	if err != nil || !reflect.DeepEqual(existing, []string{"a.example"}) {
		t.Errorf("ExistingAuditTargets(domain_allow) = %v, %v; want [a.example]", existing, err)
	}
}

func allowlistRepoCutoffs(t *testing.T, r *modRepo) {
	ctx := t.Context()
	repo := r.h.Moderation
	now := r.clock.Now().Truncate(time.Millisecond)
	switched := moderation.AllowlistCutoff{ID: r.ids.NewID(), Cause: moderation.CutoffScopeSwitched, IssuedByUserID: r.ids.NewID(), OccurredAt: now}
	removed := moderation.AllowlistCutoff{ID: r.ids.NewID(), Cause: moderation.CutoffAllowRemoved, Domain: "gone.example", OccurredAt: now}
	alice := r.ids.NewID()
	sv := moderation.Severance{ID: r.ids.NewID(), AllowlistCutoffID: removed.ID, AccountID: alice, FollowersLost: 1, FollowingLost: 2, OccurredAt: now}
	many := []moderation.Severance{
		{ID: r.ids.NewID(), AllowlistCutoffID: switched.ID, AccountID: alice, FollowersLost: 3, OccurredAt: now},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, c := range []moderation.AllowlistCutoff{switched, removed} {
			if err := repo.InsertAllowlistCutoff(ctx, tx, c); err != nil {
				t.Errorf("InsertAllowlistCutoff(%s): %v", c.Cause, err)
			}
		}
		if ok, err := repo.InsertSeverance(ctx, tx, sv); err != nil || !ok {
			t.Errorf("InsertSeverance(cutoff) = %v, %v; want true", ok, err)
		}
		again := sv
		again.ID = r.ids.NewID()
		if ok, err := repo.InsertSeverance(ctx, tx, again); err != nil || ok {
			t.Errorf("InsertSeverance(same cutoff and account) = %v, %v; want false", ok, err)
		}
		if got, err := repo.InsertSeverances(ctx, tx, many); err != nil || !got[alice] {
			t.Errorf("InsertSeverances(cutoff) = %v, %v; want alice", got, err)
		}
		return nil
	})
	for _, want := range []moderation.AllowlistCutoff{switched, removed} {
		got, err := repo.FindAllowlistCutoff(ctx, want.ID)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("FindAllowlistCutoff = %+v, %v; want %+v", got, err, want)
		}
	}
	if _, err := repo.FindAllowlistCutoff(ctx, r.ids.NewID()); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindAllowlistCutoff(none) = %v, want ErrNotFound", err)
	}
	got, err := repo.FindSeverance(ctx, alice, sv.ID)
	if err != nil || !reflect.DeepEqual(got, moderation.SeveranceRecord{Severance: sv, Domain: "gone.example"}) {
		t.Errorf("FindSeverance(allow removed) = %+v, %v; want %+v with gone.example", got, err, sv)
	}
	all, err := repo.FindSeverances(ctx, alice, []string{many[0].ID})
	if err != nil || !reflect.DeepEqual(all[many[0].ID], moderation.SeveranceRecord{Severance: many[0]}) {
		t.Errorf("FindSeverances(scope switched) = %+v, %v; want %+v without a domain", all, err, many[0])
	}
}
