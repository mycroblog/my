// SPDX-License-Identifier: AGPL-3.0-only

package timelinetest

import (
	"cmp"
	"context"
	"reflect"
	"slices"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// dm inserts the entry of a direct post of a to the accounts to in the conversation conv, with
// its parties.
func (r *timelineRepo) dm(t *testing.T, conv string, a timelineAuthor, to ...timelineAuthor) apptimeline.Entry {
	t.Helper()
	ids := make([]string, len(to))
	for i, x := range to {
		ids[i] = x.id
	}
	e := r.post(t, a, appcontent.VisibilityDirect, entryOpts{direct: ids})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.AddConversationEntry(ctx, tx, conv, e.ID, append([]string{a.id}, ids...))
	})
	return e
}

// rows returns the whole list of viewer.
func (r *timelineRepo) rows(t *testing.T, viewer timelineAuthor) []apptimeline.ConversationRow {
	t.Helper()
	rows, err := r.repo.ListConversations(t.Context(), apptimeline.ConversationsQuery{ViewerAccountID: viewer.id, Limit: 100})
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	return rows
}

func timelineRepoConversations(t *testing.T, r *timelineRepo) {
	ctx := t.Context()
	alice, bob, carol := r.local(), r.local(), r.remote()
	c1, c2 := r.ids.NewID(), r.ids.NewID()
	e1 := r.dm(t, c1, alice, bob)
	e2 := r.dm(t, c1, bob, alice)
	e3 := r.dm(t, c2, carol, alice)

	if got, want := r.rows(t, alice), []apptimeline.ConversationRow{
		{ConversationID: c2, LastEntryID: e3.ID}, {ConversationID: c1, LastEntryID: e2.ID},
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListConversations(alice) = %+v, want %+v", got, want)
	}
	// Keyset paging by the last entry.
	page, err := r.repo.ListConversations(ctx, apptimeline.ConversationsQuery{ViewerAccountID: alice.id, Before: e3.ID, Limit: 1})
	if err != nil || len(page) != 1 || page[0].ConversationID != c1 {
		t.Errorf("ListConversations(before %s) = %+v, %v; want %s", e3.ID, page, err, c1)
	}
	last, err := r.repo.LastVisibleConversationEntries(ctx, alice.id, []string{c1, c2}, r.clock.Now())
	if want := map[string]apptimeline.LastConversationEntries{
		c1: {ConversationID: c1, LastEntryID: e2.ID, LastOtherEntryID: e2.ID},
		c2: {ConversationID: c2, LastEntryID: e3.ID, LastOtherEntryID: e3.ID},
	}; err != nil || !reflect.DeepEqual(last, want) {
		t.Errorf("LastVisibleConversationEntries = %+v, %v; want %+v", last, err, want)
	}
	// Mutes, blocks, and restrictions of the author and its domain skip a post.
	r.exclude(t, alice, bob, timeline.ExcludedMute, nil)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.UpsertDomainRestriction(ctx, tx, apptimeline.DomainRestriction{Domain: timelineRepoRemote, Restriction: timeline.RestrictProfileView})
	})
	last, err = r.repo.LastVisibleConversationEntries(ctx, alice.id, []string{c1, c2}, r.clock.Now())
	if want := map[string]apptimeline.LastConversationEntries{c1: {ConversationID: c1, LastEntryID: e1.ID}}; err != nil || !reflect.DeepEqual(last, want) {
		t.Errorf("LastVisibleConversationEntries with a mute and a restriction = %+v, %v; want %+v", last, err, want)
	}
	parts, err := r.repo.ConversationParticipants(ctx, alice.id, []string{c1, c2})
	if want := map[string][]string{c1: {bob.id}, c2: {carol.id}}; err != nil || !reflect.DeepEqual(parts, want) {
		t.Errorf("ConversationParticipants = %+v, %v; want %+v", parts, err, want)
	}
	entries, err := r.repo.EntriesByID(ctx, []string{e1.ID, e3.ID, r.ids.NewID()})
	if err != nil || !reflect.DeepEqual(entries, []apptimeline.Entry{e1, e3}) {
		t.Errorf("EntriesByID = %+v, %v; want %+v", entries, err, []apptimeline.Entry{e1, e3})
	}

	// The boundaries: read and hidden up to the last entry; none for a conversation without the
	// account.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if ok, err := r.repo.MarkConversationRead(ctx, tx, alice.id, c1); err != nil || !ok {
			t.Errorf("MarkConversationRead = %v, %v; want true", ok, err)
		}
		if ok, err := r.repo.HideConversation(ctx, tx, alice.id, c2); err != nil || !ok {
			t.Errorf("HideConversation = %v, %v; want true", ok, err)
		}
		if ok, err := r.repo.MarkConversationRead(ctx, tx, bob.id, c2); err != nil || ok {
			t.Errorf("MarkConversationRead of another's = %v, %v; want false", ok, err)
		}
		return nil
	})
	if got, want := r.rows(t, alice), []apptimeline.ConversationRow{{ConversationID: c1, LastEntryID: e2.ID, ReadEntryID: e2.ID}}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListConversations after read and hide = %+v, want %+v", got, want)
	}
	among, err := r.repo.ConversationsAmong(ctx, alice.id, []string{c1, c2, r.ids.NewID()})
	if want := []apptimeline.ConversationRow{
		{ConversationID: c2, LastEntryID: e3.ID, Hidden: true}, {ConversationID: c1, LastEntryID: e2.ID, ReadEntryID: e2.ID},
	}; err != nil || !reflect.DeepEqual(among, want) {
		t.Errorf("ConversationsAmong = %+v, %v; want %+v", among, err, want)
	}

	// A merge moves everything, keeping the newer of each account's rows: the last entry and the
	// hide boundary of c2, the read boundary of c1.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.MergeConversations(ctx, tx, c2, c1) })
	among, err = r.repo.ConversationsAmong(ctx, alice.id, []string{c1, c2})
	if want := []apptimeline.ConversationRow{{ConversationID: c1, LastEntryID: e3.ID, ReadEntryID: e2.ID, Hidden: true}}; err != nil || !reflect.DeepEqual(among, want) {
		t.Errorf("ConversationsAmong after the merge = %+v, %v; want %+v", among, err, want)
	}
	if got := r.rows(t, alice); len(got) != 0 {
		t.Errorf("ListConversations after the merge = %+v, want it hidden", got)
	}
	if got := r.rows(t, carol); len(got) != 1 || got[0].ConversationID != c1 {
		t.Errorf("ListConversations(carol) after the merge = %+v, want %s", got, c1)
	}
	parts, err = r.repo.ConversationParticipants(ctx, alice.id, []string{c1, c2})
	if want := []string{bob.id, carol.id}; err != nil || !slices.Equal(parts[c1], sorted(want)) || len(parts[c2]) != 0 {
		t.Errorf("ConversationParticipants after the merge = %+v, %v; want %v in %s", parts, err, want, c1)
	}

	// Edits replace the parties and deletes take the entries out.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.ReplaceConversationEntryParties(ctx, tx, e3.ID, []string{carol.id, bob.id})
	})
	among, err = r.repo.ConversationsAmong(ctx, alice.id, []string{c1})
	if want := []apptimeline.ConversationRow{{ConversationID: c1, LastEntryID: e2.ID, ReadEntryID: e2.ID, Hidden: true}}; err != nil || !reflect.DeepEqual(among, want) {
		t.Errorf("ConversationsAmong after an edit = %+v, %v; want %+v", among, err, want)
	}
	if got := r.rows(t, bob); len(got) != 1 || got[0].LastEntryID != e3.ID {
		t.Errorf("ListConversations(bob) after an edit = %+v, want the last entry %s", got, e3.ID)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.DeleteConversationEntries(ctx, tx, []string{e2.ID, e3.ID})
	})
	if got := r.rows(t, bob); len(got) != 1 || got[0].LastEntryID != e1.ID {
		t.Errorf("ListConversations(bob) after deletes = %+v, want the last entry %s", got, e1.ID)
	}
	if got := r.rows(t, carol); len(got) != 0 {
		t.Errorf("ListConversations(carol) after deletes = %+v, want none", got)
	}

	snap, err := r.repo.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := sortedParticipants([]apptimeline.ConversationParticipant{
		{ConversationID: c1, EntryID: e1.ID, AccountID: alice.id}, {ConversationID: c1, EntryID: e1.ID, AccountID: bob.id},
	}); !reflect.DeepEqual(snap.ConversationParticipants, want) {
		t.Errorf("Snapshot participants = %+v, want %+v", snap.ConversationParticipants, want)
	}
	if len(snap.ConversationHeads) != 2 {
		t.Errorf("Snapshot heads = %+v, want those of alice and bob", snap.ConversationHeads)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.DeleteProjection(ctx, tx) })
	if snap, err = r.repo.Snapshot(ctx); err != nil || len(snap.ConversationParticipants)+len(snap.ConversationHeads) != 0 {
		t.Errorf("Snapshot after DeleteProjection = %+v, %v; want no conversations", snap, err)
	}
}

func sorted(s []string) []string {
	s = slices.Clone(s)
	slices.Sort(s)
	return s
}

func sortedParticipants(ps []apptimeline.ConversationParticipant) []apptimeline.ConversationParticipant {
	slices.SortFunc(ps, func(a, b apptimeline.ConversationParticipant) int {
		return cmp.Or(cmp.Compare(a.AccountID, b.AccountID), cmp.Compare(a.ConversationID, b.ConversationID), cmp.Compare(a.EntryID, b.EntryID))
	})
	return ps
}
