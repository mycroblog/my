// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// SignupBlockContract verifies the sign-up block list of a database adapter's moderation
// repository by its methods alone, the port contract of ADR-0153 §1: a block is read back as
// inserted and lifted once; the blocks list newest first with their lifts, by kind, in effect, and
// in keyset pages; the blocks in effect (neither lifted nor expired) that may match a sign-up are
// those of its email's domains, of its email's hash, and every IP range, and those of a kind
// among some values; and the purge lists the blocks that ended before a time, oldest first and in
// batches, except one made by the ban of a held account, and deletes them with their lifts. The
// use cases on top of it are SignupBlockFlows, which internal/flowtest/moderation runs once on
// SQLite.
func SignupBlockContract(t *testing.T, newHarness ModerationFactory) {
	t.Helper()
	runModRepo(t, newHarness, []modRepoCase{
		{"InsertsAndLiftsOnce", signupRepoInsertsAndLifts},
		{"ListsWithFiltersAndPages", signupRepoLists},
		{"FindsTheBlocksInEffect", signupRepoInEffect},
		{"PurgesTheEndedBlocks", signupRepoPurges},
	})
}

// signupBlock inserts a block of kind and value, changed by edit, a second before the next.
func (r *modRepo) signupBlock(t *testing.T, kind moderation.SignupBlockKind, value string, edit func(*moderation.SignupBlock)) moderation.SignupBlock {
	t.Helper()
	b := moderation.SignupBlock{ID: r.ids.NewID(), Kind: kind, Value: value, Severity: moderation.SignupReject, CreatedAt: r.clock.Now()}
	if edit != nil {
		edit(&b)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Moderation.InsertSignupBlock(ctx, tx, b) })
	r.clock.Advance(time.Second)
	return b
}

func (r *modRepo) liftSignupBlock(t *testing.T, id string) moderation.SignupBlockLift {
	t.Helper()
	l := moderation.SignupBlockLift{BlockID: id, LiftedByUserID: NamedID("mod-1"), LiftedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Moderation.InsertSignupBlockLift(ctx, tx, l) })
	r.clock.Advance(time.Second)
	return l
}

func signupBlockIDs(bs []moderation.SignupBlock) []string {
	var out []string
	for _, b := range bs {
		out = append(out, b.ID)
	}
	slices.Sort(out)
	return out
}

func signupRepoInsertsAndLifts(t *testing.T, r *modRepo) {
	ctx := t.Context()
	b := r.signupBlock(t, moderation.SignupBlockEmailDomain, "spam.example", func(b *moderation.SignupBlock) {
		b.Severity, b.Comment, b.ModerationActionID = moderation.SignupRequireApproval, "a wave of spam", r.ids.NewID()
		b.CreatedByUserID, b.ExpiresAt = NamedID("mod-1"), r.at(time.Hour)
	})
	var got moderation.SignupBlock
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		got, err = r.h.Moderation.SignupBlock(ctx, tx, b.ID)
		return err
	})
	if !reflect.DeepEqual(got, b) {
		t.Errorf("SignupBlock = %+v, want %+v", got, b)
	}
	if rec, err := r.h.Moderation.FindSignupBlockRecord(ctx, b.ID); err != nil || !reflect.DeepEqual(rec, moderation.SignupBlockRecord{Block: b}) {
		t.Errorf("FindSignupBlockRecord = %+v, %v; want the block without a lift", rec, err)
	}
	l := r.liftSignupBlock(t, b.ID)
	if rec, err := r.h.Moderation.FindSignupBlockRecord(ctx, b.ID); err != nil || rec.Lift == nil || *rec.Lift != l {
		t.Errorf("FindSignupBlockRecord after the lift = %+v, %v; want the lift %+v", rec, err, l)
	}
	err := r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.InsertSignupBlockLift(ctx, tx, moderation.SignupBlockLift{BlockID: b.ID, LiftedAt: r.clock.Now()})
	})
	if !errors.Is(err, appmoderation.ErrAlreadyRecorded) {
		t.Errorf("InsertSignupBlockLift again = %v, want ErrAlreadyRecorded", err)
	}
	unknown := r.ids.NewID()
	if _, err := r.h.Moderation.FindSignupBlockRecord(ctx, unknown); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindSignupBlockRecord(unknown) = %v, want ErrNotFound", err)
	}
	err = r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.h.Moderation.SignupBlock(ctx, tx, unknown)
		return err
	})
	if !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("SignupBlock(unknown) = %v, want ErrNotFound", err)
	}
}

func signupRepoLists(t *testing.T, r *modRepo) {
	ctx := t.Context()
	domain := r.signupBlock(t, moderation.SignupBlockEmailDomain, "spam.example", nil)
	lifted := r.signupBlock(t, moderation.SignupBlockEmailDomain, "lifted.example", nil)
	expiring := r.signupBlock(t, moderation.SignupBlockIPRange, "192.0.2.0/24", func(b *moderation.SignupBlock) { b.ExpiresAt = r.at(time.Hour) })
	hash := r.signupBlock(t, moderation.SignupBlockEmailHash, "0123456789abcdef", nil)
	l := r.liftSignupBlock(t, lifted.ID)
	now := r.clock.Now()
	later := now.Add(time.Hour)
	ids := func(recs []moderation.SignupBlockRecord) []string {
		var out []string
		for _, rec := range recs {
			out = append(out, rec.Block.ID)
		}
		return out
	}
	for _, c := range []struct {
		name  string
		q     appmoderation.SignupBlockQuery
		limit int
		want  []string
	}{
		{"every block", appmoderation.SignupBlockQuery{}, 10, []string{hash.ID, expiring.ID, lifted.ID, domain.ID}},
		{"of a kind", appmoderation.SignupBlockQuery{Kind: moderation.SignupBlockEmailDomain}, 10, []string{lifted.ID, domain.ID}},
		{"in effect", appmoderation.SignupBlockQuery{InEffectAt: &now}, 10, []string{hash.ID, expiring.ID, domain.ID}},
		{"in effect after the end", appmoderation.SignupBlockQuery{InEffectAt: &later}, 10, []string{hash.ID, domain.ID}},
		{"a page", appmoderation.SignupBlockQuery{}, 2, []string{hash.ID, expiring.ID}},
		{"the next page", appmoderation.SignupBlockQuery{Before: expiring.ID}, 2, []string{lifted.ID, domain.ID}},
	} {
		got, err := r.h.Moderation.ListSignupBlocks(ctx, c.q, c.limit)
		if err != nil || !slices.Equal(ids(got), c.want) {
			t.Errorf("ListSignupBlocks(%s) = %v, %v; want %v", c.name, ids(got), err, c.want)
		}
	}
	got, err := r.h.Moderation.ListSignupBlocks(ctx, appmoderation.SignupBlockQuery{Before: expiring.ID}, 1)
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], moderation.SignupBlockRecord{Block: lifted, Lift: &l}) {
		t.Errorf("ListSignupBlocks = %+v, %v; want the block with its lift", got, err)
	}
}

func signupRepoInEffect(t *testing.T, r *modRepo) {
	ctx := t.Context()
	parent := r.signupBlock(t, moderation.SignupBlockEmailDomain, "spam.example", nil)
	sub := r.signupBlock(t, moderation.SignupBlockEmailDomain, "mail.spam.example", func(b *moderation.SignupBlock) { b.Severity = moderation.SignupRequireApproval })
	other := r.signupBlock(t, moderation.SignupBlockEmailDomain, "other.example", nil)
	lifted := r.signupBlock(t, moderation.SignupBlockEmailDomain, "example", nil)
	r.liftSignupBlock(t, lifted.ID)
	expiring := r.signupBlock(t, moderation.SignupBlockEmailHash, "0123456789abcdef", func(b *moderation.SignupBlock) { b.ExpiresAt = r.at(time.Minute) })
	otherHash := r.signupBlock(t, moderation.SignupBlockEmailHash, "fedcba9876543210", nil)
	ipRange := r.signupBlock(t, moderation.SignupBlockIPRange, "192.0.2.0/24", nil)
	now := r.clock.Now()

	match := appmoderation.SignupBlockMatch{Domains: moderation.SignupDomains("mail.spam.example"), EmailHash: expiring.Value}
	got, err := r.h.Moderation.SignupBlocksInEffect(ctx, match, now)
	if want := signupBlockIDs([]moderation.SignupBlock{parent, sub, expiring, ipRange}); err != nil || !slices.Equal(signupBlockIDs(got), want) {
		t.Errorf("SignupBlocksInEffect = %v, %v; want %v", signupBlockIDs(got), err, want)
	}
	for _, b := range got {
		if b.ID == sub.ID && !reflect.DeepEqual(b, sub) {
			t.Errorf("SignupBlocksInEffect has %+v, want %+v", b, sub)
		}
	}
	got, err = r.h.Moderation.SignupBlocksInEffect(ctx, appmoderation.SignupBlockMatch{Domains: []string{"other.example", "example"}}, expiring.ExpiresAt.Add(time.Second))
	if want := signupBlockIDs([]moderation.SignupBlock{other, ipRange}); err != nil || !slices.Equal(signupBlockIDs(got), want) {
		t.Errorf("SignupBlocksInEffect(no hash, after the end) = %v, %v; want %v", signupBlockIDs(got), err, want)
	}

	for _, c := range []struct {
		kind   moderation.SignupBlockKind
		values []string
		at     time.Time
		want   []moderation.SignupBlock
	}{
		{moderation.SignupBlockEmailHash, []string{expiring.Value, otherHash.Value, "0000"}, now, []moderation.SignupBlock{expiring, otherHash}},
		{moderation.SignupBlockEmailHash, []string{expiring.Value}, *expiring.ExpiresAt, nil},
		{moderation.SignupBlockEmailDomain, []string{"example", "spam.example"}, now, []moderation.SignupBlock{parent}},
		{moderation.SignupBlockIPRange, []string{"spam.example"}, now, nil},
	} {
		var got []moderation.SignupBlock
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			got, err = r.h.Moderation.SignupBlocksInEffectOf(ctx, tx, c.kind, c.values, c.at)
			return err
		})
		if !slices.Equal(signupBlockIDs(got), signupBlockIDs(c.want)) {
			t.Errorf("SignupBlocksInEffectOf(%s, %v) = %v, want %v", c.kind, c.values, signupBlockIDs(got), signupBlockIDs(c.want))
		}
	}
}

func signupRepoPurges(t *testing.T, r *modRepo) {
	ctx := t.Context()
	held := r.ids.NewID()
	ban := r.action(t, held, moderation.ActionBan, nil)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.InsertRetentionHold(ctx, tx, moderation.RetentionHold{ID: r.ids.NewID(), AccountID: held, Reason: "court order", CreatedAt: r.clock.Now()})
	})
	lifted := r.signupBlock(t, moderation.SignupBlockEmailDomain, "lifted.example", nil)
	r.liftSignupBlock(t, lifted.ID)
	expired := r.signupBlock(t, moderation.SignupBlockEmailDomain, "expired.example", func(b *moderation.SignupBlock) { b.ExpiresAt = r.at(time.Second) })
	ofHeld := r.signupBlock(t, moderation.SignupBlockEmailHash, "0123456789abcdef", func(b *moderation.SignupBlock) {
		b.ModerationActionID, b.ExpiresAt = ban.ID, r.at(time.Second)
	})
	inEffect := r.signupBlock(t, moderation.SignupBlockEmailDomain, "spam.example", nil)
	r.clock.Advance(time.Minute)
	later := r.signupBlock(t, moderation.SignupBlockEmailDomain, "later.example", nil)
	r.liftSignupBlock(t, later.ID)

	list := func(before time.Time, limit int) []string {
		t.Helper()
		var ids []string
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			ids, err = r.h.Moderation.ListSignupBlocksToPurge(ctx, tx, before, limit)
			return err
		})
		return ids
	}
	cutoff := later.CreatedAt
	if got := list(cutoff, 10); !slices.Equal(got, []string{lifted.ID, expired.ID}) {
		t.Errorf("ListSignupBlocksToPurge = %v, want the lifted and the expired blocks, oldest first", got)
	}
	if got := list(cutoff, 1); !slices.Equal(got, []string{lifted.ID}) {
		t.Errorf("ListSignupBlocksToPurge(limit 1) = %v, want the oldest", got)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.PurgeSignupBlocks(ctx, tx, []string{lifted.ID, expired.ID})
	})
	for _, id := range []string{lifted.ID, expired.ID} {
		if _, err := r.h.Moderation.FindSignupBlockRecord(ctx, id); !errors.Is(err, appmoderation.ErrNotFound) {
			t.Errorf("FindSignupBlockRecord(purged) = %v, want ErrNotFound", err)
		}
	}
	for _, b := range []moderation.SignupBlock{ofHeld, inEffect, later} {
		if _, err := r.h.Moderation.FindSignupBlockRecord(ctx, b.ID); err != nil {
			t.Errorf("FindSignupBlockRecord(%s) = %v, want it kept", b.Value, err)
		}
	}
	if got := list(r.clock.Now().Add(time.Hour), 10); !slices.Equal(got, []string{later.ID}) {
		t.Errorf("ListSignupBlocksToPurge later = %v, want the block lifted later only", got)
	}
}
