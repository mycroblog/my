// SPDX-License-Identifier: AGPL-3.0-only

package notificationtest

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// notificationRepoThreadMutes checks the thread mutes (conversation_mutes): a mute is inserted
// once and deleted once; the muters of a conversation and the mutes of accounts and conversations
// are read back; and a merge moves the mutes of the merged conversation to the one it was merged
// into, one per account, again without change.
func notificationRepoThreadMutes(t *testing.T, r *notificationRepo) {
	alice, bob, carol := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	c1, c2, c3 := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	at := r.clock.Now().Truncate(time.Millisecond)
	mute := func(account, conversation string) bool {
		t.Helper()
		var inserted bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			inserted, err = r.repo.InsertConversationMute(ctx, tx, notification.ConversationMute{AccountID: account, ConversationID: conversation, CreatedAt: at})
			return err
		})
		return inserted
	}
	muters := func(conversation string, accounts ...string) []string {
		t.Helper()
		var got map[string]bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			got, err = r.repo.ConversationMutersAmong(ctx, tx, conversation, accounts)
			return err
		})
		return slices.Sorted(maps.Keys(got))
	}
	sorted := func(ids ...string) []string { return slices.Sorted(slices.Values(ids)) }

	if !mute(alice, c1) || mute(alice, c1) {
		t.Fatal("InsertConversationMute twice = not (true, false)")
	}
	mute(bob, c1)
	mute(bob, c2)
	mute(carol, c2)
	if got, want := muters(c1, alice, bob, carol), sorted(alice, bob); !slices.Equal(got, want) {
		t.Errorf("muters of c1 = %v, want %v", got, want)
	}
	if got := muters(c1, carol); len(got) != 0 {
		t.Errorf("muters of c1 among carol = %v, want none", got)
	}
	if got := muters(c1); len(got) != 0 {
		t.Errorf("muters of c1 among nobody = %v, want none", got)
	}

	mutes, err := r.repo.ListConversationMutes(t.Context(), []string{alice, bob}, []string{c1, c2, c3})
	if err != nil {
		t.Fatal(err)
	}
	var pairs [][2]string
	for _, m := range mutes {
		if !m.CreatedAt.Equal(at) {
			t.Errorf("mute %s/%s created at %v, want %v", m.AccountID, m.ConversationID, m.CreatedAt, at)
		}
		pairs = append(pairs, [2]string{m.AccountID, m.ConversationID})
	}
	slices.SortFunc(pairs, func(a, b [2]string) int { return strings.Compare(a[0]+a[1], b[0]+b[1]) })
	want := [][2]string{{alice, c1}, {bob, c1}, {bob, c2}}
	slices.SortFunc(want, func(a, b [2]string) int { return strings.Compare(a[0]+a[1], b[0]+b[1]) })
	if !slices.Equal(pairs, want) {
		t.Errorf("ListConversationMutes = %v, want %v", pairs, want)
	}

	// c2 merges into c1: carol's mute moves, bob keeps one; merging again changes nothing.
	for range 2 {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.MergeConversationMutes(ctx, tx, c2, c1) })
	}
	if got, want := muters(c1, alice, bob, carol), sorted(alice, bob, carol); !slices.Equal(got, want) {
		t.Errorf("muters of c1 after the merge = %v, want %v", got, want)
	}
	if got := muters(c2, alice, bob, carol); len(got) != 0 {
		t.Errorf("muters of the merged c2 = %v, want none", got)
	}

	var deleted [2]bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		if deleted[0], err = r.repo.DeleteConversationMute(ctx, tx, bob, c1); err != nil {
			return err
		}
		deleted[1], err = r.repo.DeleteConversationMute(ctx, tx, bob, c1)
		return err
	})
	if deleted != [2]bool{true, false} {
		t.Errorf("DeleteConversationMute twice = %v, want [true false]", deleted)
	}
	if got, want := muters(c1, alice, bob, carol), sorted(alice, carol); !slices.Equal(got, want) {
		t.Errorf("muters of c1 after bob unmutes = %v, want %v", got, want)
	}
}
