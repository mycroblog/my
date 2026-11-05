// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/app/uow"
)

// TranslationHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the translation module's repository on it.
type TranslationHarness struct {
	UnitOfWork uow.UnitOfWork
	Repository translation.Repository
}

// translationIDs are UUIDs, which is what the PostgreSQL schema stores.
func translationID(n int) string {
	return fmt.Sprintf("0197a000-0000-7000-8000-%012d", n)
}

// TranslationRepositoryContract verifies the translation repository of a database adapter
// (ADR-0148 §9, §10): a translation is stored once per revision, target language, and provider,
// with its child rows; deleting by status or by ID takes the child rows along; expired translations
// are listed oldest first; and the usage of an account and of the server adds up by month and is
// deleted by account and by month.
func TranslationRepositoryContract(t *testing.T, newHarness func(t *testing.T) TranslationHarness) {
	t.Helper()
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	full := translation.Stored{
		ID: translationID(1), StatusID: translationID(100), RevisionID: translationID(200), TargetLanguage: "ja", Provider: "deepl",
		SourceLanguage: "en", ContentHTML: "<p>こんにちは</p>", SpoilerText: "注意", CreatedAt: at,
		Attachments: []translation.StoredAttachment{{Position: 0, Description: "猫"}, {Position: 2, Description: "犬"}},
		PollOptions: []translation.StoredPollOption{{Position: 0, Title: "はい"}, {Position: 1, Title: "いいえ"}},
	}
	tx := func(t *testing.T, h TranslationHarness, fn func(ctx context.Context, tx uow.Tx) error) {
		t.Helper()
		ctx := t.Context()
		if err := h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return fn(ctx, tx) }); err != nil {
			t.Fatal(err)
		}
	}
	insert := func(t *testing.T, h TranslationHarness, s translation.Stored) bool {
		t.Helper()
		var inserted bool
		tx(t, h, func(ctx context.Context, tx uow.Tx) error {
			var err error
			inserted, err = h.Repository.InsertTranslation(ctx, tx, s)
			return err
		})
		return inserted
	}
	find := func(t *testing.T, h TranslationHarness, revisionID, target string) []translation.Stored {
		t.Helper()
		got, err := h.Repository.FindTranslations(t.Context(), revisionID, target)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	t.Run("StoresATranslationWithItsChildRows", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		if !insert(t, h, full) {
			t.Fatal("InsertTranslation = false, want true")
		}
		got := find(t, h, full.RevisionID, "ja")
		if len(got) != 1 || !reflect.DeepEqual(got[0], full) {
			t.Errorf("FindTranslations = %+v, want %+v", got, full)
		}
		if got := find(t, h, full.RevisionID, "de"); len(got) != 0 {
			t.Errorf("FindTranslations(de) = %+v", got)
		}
		if got := find(t, h, translationID(201), "ja"); len(got) != 0 {
			t.Errorf("FindTranslations(another revision) = %+v", got)
		}
	})
	t.Run("KeepsTheFirstOfOneRevisionLanguageAndProvider", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		insert(t, h, full)
		again := full
		again.ID, again.ContentHTML = translationID(2), "<p>second</p>"
		again.Attachments = []translation.StoredAttachment{{Position: 1, Description: "second"}}
		if insert(t, h, again) {
			t.Error("InsertTranslation of the same key = true, want false")
		}
		if got := find(t, h, full.RevisionID, "ja"); len(got) != 1 || !reflect.DeepEqual(got[0], full) {
			t.Errorf("FindTranslations = %+v, want the first", got)
		}
		other := full
		other.ID, other.Provider = translationID(3), "libretranslate"
		if !insert(t, h, other) {
			t.Error("InsertTranslation of another provider = false, want true")
		}
		if got := find(t, h, full.RevisionID, "ja"); len(got) != 2 {
			t.Errorf("FindTranslations = %d rows, want 2", len(got))
		}
	})
	t.Run("DeletesTheTranslationsOfAStatus", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		insert(t, h, full)
		kept := full
		kept.ID, kept.StatusID, kept.RevisionID = translationID(4), translationID(101), translationID(204)
		insert(t, h, kept)
		for range 2 { // deleting again changes nothing, as a repeated job does
			tx(t, h, func(ctx context.Context, tx uow.Tx) error {
				return h.Repository.DeleteStatusTranslations(ctx, tx, full.StatusID)
			})
		}
		if got := find(t, h, full.RevisionID, "ja"); len(got) != 0 {
			t.Errorf("FindTranslations of the deleted = %+v", got)
		}
		if got := find(t, h, kept.RevisionID, "ja"); len(got) != 1 {
			t.Errorf("FindTranslations of the other status = %+v", got)
		}
		// The child rows went too: the same ID stores new ones.
		tx(t, h, func(ctx context.Context, tx uow.Tx) error {
			_, err := h.Repository.InsertTranslation(ctx, tx, full)
			return err
		})
	})
	t.Run("ListsAndDeletesTheExpired", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		for i := range 3 {
			s := full
			s.ID, s.RevisionID, s.CreatedAt = translationID(10+i), translationID(210+i), at.Add(time.Duration(i)*time.Hour)
			insert(t, h, s)
		}
		ctx := t.Context()
		got, err := h.Repository.ListExpiredTranslations(ctx, at.Add(90*time.Minute), 10)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{translationID(10), translationID(11)}; !reflect.DeepEqual(got, want) {
			t.Errorf("ListExpiredTranslations = %v, want %v", got, want)
		}
		got, err = h.Repository.ListExpiredTranslations(ctx, at.Add(90*time.Minute), 1)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{translationID(10)}; !reflect.DeepEqual(got, want) {
			t.Errorf("ListExpiredTranslations(limit 1) = %v, want %v", got, want)
		}
		tx(t, h, func(ctx context.Context, tx uow.Tx) error {
			return h.Repository.DeleteTranslations(ctx, tx, got)
		})
		if got := find(t, h, translationID(210), "ja"); len(got) != 0 {
			t.Errorf("FindTranslations of the deleted = %+v", got)
		}
		if got := find(t, h, translationID(211), "ja"); len(got) != 1 {
			t.Errorf("FindTranslations of the other = %+v", got)
		}
	})
	t.Run("AddsUpUsageByMonth", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		alice, bob := translationID(300), translationID(301)
		add := func(account, month string, n int64) {
			t.Helper()
			tx(t, h, func(ctx context.Context, tx uow.Tx) error {
				return h.Repository.AddUsage(ctx, tx, account, month, n)
			})
		}
		usage := func(account, month string) translation.Usage {
			t.Helper()
			u, err := h.Repository.FindUsage(t.Context(), account, month)
			if err != nil {
				t.Fatal(err)
			}
			return u
		}
		if got := usage(alice, "2026-11"); got != (translation.Usage{}) {
			t.Errorf("usage without rows = %+v", got)
		}
		add(alice, "2026-09", 5)
		add(alice, "2026-11", 10)
		add(alice, "2026-11", 20)
		add(bob, "2026-11", 1)
		if got, want := usage(alice, "2026-11"), (translation.Usage{Account: 30, Server: 31}); got != want {
			t.Errorf("usage = %+v, want %+v", got, want)
		}
		tx(t, h, func(ctx context.Context, tx uow.Tx) error {
			return h.Repository.DeleteUsageBefore(ctx, tx, "2026-10")
		})
		if got, want := usage(alice, "2026-09"), (translation.Usage{}); got != want {
			t.Errorf("usage before the month = %+v, want none", got)
		}
		tx(t, h, func(ctx context.Context, tx uow.Tx) error {
			return h.Repository.DeleteAccountUsage(ctx, tx, alice)
		})
		if got, want := usage(alice, "2026-11"), (translation.Usage{Server: 31}); got != want {
			t.Errorf("usage of the erased = %+v, want %+v: the server's stays", got, want)
		}
		if got, want := usage(bob, "2026-11"), (translation.Usage{Account: 1, Server: 31}); got != want {
			t.Errorf("usage of the other = %+v, want %+v", got, want)
		}
	})
}
