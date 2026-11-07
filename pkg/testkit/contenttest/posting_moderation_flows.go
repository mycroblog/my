// SPDX-License-Identifier: AGPL-3.0-only

package contenttest

import (
	"errors"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/pkg/testkit"
)

// postingModeration checks the effects of the moderation actions on statuses (data-model §5.2):
// delete_statuses deletes a status into a tombstone that names the action, like the author's
// deletion, once; mark_statuses_sensitive marks a status until the action is lifted, and a
// status marked by two actions stays marked until both are.
func postingModeration(t *testing.T, e *postingEnv) {
	ctx := t.Context()
	alice := e.user(t, "alice")
	p := e.postText(t, alice, "spam", content.VisibilityPublic)
	marked := e.postText(t, alice, "gore", content.VisibilityPublic)
	e.events.Take()

	e.Clock.Advance(time.Minute)
	at := e.Clock.Now()
	tx := func(fn func(tx uow.Tx) error) error {
		t.Helper()
		return e.H.UnitOfWork.Tx(ctx, fn)
	}
	if err := tx(func(tx uow.Tx) error {
		return e.content.DeleteByModeration(ctx, tx, p.Status, testkit.NamedID("action-1"), at)
	}); err != nil {
		t.Fatalf("DeleteByModeration = %v", err)
	}
	want := content.Deletion{
		StatusID: p.Status.ID, URI: p.Status.URI, AuthorAccountID: alice, Origin: content.DeletedByModeration,
		ModerationActionID: testkit.NamedID("action-1"), DeletedAt: at,
	}
	if got, err := e.repo.FindDeletion(ctx, p.Status.ID); err != nil || got != want {
		t.Errorf("deletion = %+v, %v; want %+v", got, err, want)
	}
	if _, err := e.content.Find(ctx, p.Status.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Find after the deletion = %v, want ErrNotFound", err)
	}
	if events := e.events.Take(); len(events) != 1 {
		t.Errorf("events = %v, want one StatusDeleted", EventTypes(events))
	} else if ev, ok := events[0].(appcontent.StatusDeleted); !ok || ev.Deletion != want {
		t.Errorf("event = %+v, want the deletion", events[0])
	}
	err := tx(func(tx uow.Tx) error {
		return e.content.DeleteByModeration(ctx, tx, p.Status, testkit.NamedID("action-2"), at)
	})
	if !errors.Is(err, appcontent.ErrAlreadyDeleted) {
		t.Errorf("DeleteByModeration again = %v, want ErrAlreadyDeleted", err)
	}

	overridden := func(want bool) {
		t.Helper()
		if got, err := e.repo.SensitivityOverridden(ctx, marked.Status.ID); err != nil || got != want {
			t.Errorf("SensitivityOverridden = %v, %v; want %v", got, err, want)
		}
	}
	mark := func(action string) {
		t.Helper()
		if err := tx(func(tx uow.Tx) error { return e.content.MarkSensitiveByModeration(ctx, tx, marked.Status.ID, action) }); err != nil {
			t.Fatalf("MarkSensitiveByModeration(%s) = %v", action, err)
		}
	}
	unmark := func(action string) {
		t.Helper()
		if err := tx(func(tx uow.Tx) error { return e.content.ClearSensitivityOf(ctx, tx, action) }); err != nil {
			t.Fatalf("ClearSensitivityOf(%s) = %v", action, err)
		}
	}
	overridden(false)
	mark(testkit.NamedID("action-3"))
	mark(testkit.NamedID("action-3")) // again: nothing changes
	mark(testkit.NamedID("action-4"))
	overridden(true)
	unmark(testkit.NamedID("action-3"))
	overridden(true)
	unmark(testkit.NamedID("action-4"))
	overridden(false)
}
