// SPDX-License-Identifier: AGPL-3.0-only

package notificationtest

import (
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/notification"
)

// notificationThreadMutes checks the thread mutes (ADR-0158 §8): a muted conversation, the
// recipient's own thread included, notifies the recipient of no reply, mention, favourite,
// boost, or quote, while the other conversations and the other recipients still notify; a mute
// moves with a merge of its conversation into another; MutedAmong tells the muted statuses; and
// unmuting notifies again of what comes next, not of what came meanwhile.
func notificationThreadMutes(t *testing.T, e *notificationEnv) {
	ctx := t.Context()
	alice, bob, carol := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "carol")
	thread := e.post(t, alice, "a thread that spreads").Status
	other := e.post(t, alice, "another thread").Status
	// alice mutes the conversation of other, which then merges into that of thread: the mute
	// follows it.
	if err := e.mutes.Mute(ctx, alice, other.ID); err != nil {
		t.Fatalf("Mute(alice, other) = %v", err)
	}
	if err := e.mutes.Mute(ctx, alice, other.ID); err != nil {
		t.Fatalf("Mute(alice, other) again = %v", err)
	}
	e.merge(t, thread, other)
	muted, err := e.mutes.MutedAmong(ctx, []string{alice, bob}, []string{thread.ID, other.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := slices.Sorted(maps.Keys(muted[alice])); !slices.Equal(got, slices.Sorted(slices.Values([]string{thread.ID, other.ID}))) || len(muted[bob]) != 0 {
		t.Errorf("MutedAmong after the merge = %v, want alice's both statuses only", muted)
	}

	e.reply(t, bob, thread, "@alice @carol what a thread")
	e.favourite(t, bob, thread.ID)
	e.Clock.Advance(time.Second)
	if _, err := e.interactions.Boost(ctx, bob, thread.ID); err != nil {
		t.Fatalf("Boost = %v", err)
	}
	quoting := appcontent.StatusRef{ID: e.IDs.NewID(), AuthorAccountID: bob, IsLocal: true, Visibility: content.VisibilityPublic}
	quoted := appcontent.RefOf(thread)
	e.project(t, appcontent.QuoteRecorded{
		Quote: content.Quote{StatusID: quoting.ID, QuotedURI: thread.URI, Form: appcontent.QuoteFormLegacy}, Status: quoting, Quoted: &quoted, RecordedAt: e.Clock.Now(),
	})
	checkSummaries(t, e, alice)
	// carol does not mute the thread, and alice's other threads still notify her.
	if got := e.summaries(t, carol); len(got) != 1 || got[0].Type != notification.TypeMention {
		t.Errorf("notifications of carol = %+v, want the mention", got)
	}
	elsewhere := e.post(t, alice, "elsewhere").Status
	e.favourite(t, bob, elsewhere.ID)
	checkSummaries(t, e, alice, summary{Type: notification.TypeFavourite, Source: bob, StatusID: elsewhere.ID})

	// Unmuting by any status of the conversation notifies of what comes next.
	if err := e.mutes.Unmute(ctx, alice, other.ID); err != nil {
		t.Fatalf("Unmute(alice) = %v", err)
	}
	e.favourite(t, carol, thread.ID)
	checkSummaries(t, e, alice,
		summary{Type: notification.TypeFavourite, Source: carol, StatusID: thread.ID},
		summary{Type: notification.TypeFavourite, Source: bob, StatusID: elsewhere.ID})

	if err := e.mutes.Mute(ctx, alice, e.IDs.NewID()); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Mute(unknown status) = %v, want ErrNotFound", err)
	}
}

// reply posts a public reply of author to parent with text.
func (e *notificationEnv) reply(t *testing.T, author string, parent content.Status, text string) content.Status {
	t.Helper()
	e.Clock.Advance(time.Second)
	p, err := e.content.Create(t.Context(), appcontent.CreateInput{
		AuthorAccountID: author, TextInput: appcontent.TextInput{Text: text}, Visibility: content.VisibilityPublic, InReplyToID: parent.ID,
	})
	if err != nil {
		t.Fatalf("Create(reply %q) = %v", text, err)
	}
	return p.Status
}

// merge merges the conversation of from into that of into, as the content module does when a
// reply links them, emitting ConversationMerged.
func (e *notificationEnv) merge(t *testing.T, into, from content.Status) {
	t.Helper()
	of, err := e.nh.Statuses.StatusConversations(t.Context(), []string{into.ID, from.ID})
	if err != nil {
		t.Fatal(err)
	}
	m := content.ConversationMerge{MergedConversationID: of[from.ID].ID, IntoConversationID: of[into.ID].ID, StatusID: from.ID, MergedAt: e.Clock.Now()}
	if m.MergedConversationID == m.IntoConversationID {
		t.Fatalf("statuses %s and %s are in one conversation already", into.ID, from.ID)
	}
	err = e.nh.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := e.nh.Statuses.MergeConversation(t.Context(), tx, m); err != nil {
			return err
		}
		return tx.Emit(t.Context(), appcontent.ConversationMerged{Merge: m})
	})
	if err != nil {
		t.Fatalf("merge conversation %s into %s: %v", m.MergedConversationID, m.IntoConversationID, err)
	}
}
