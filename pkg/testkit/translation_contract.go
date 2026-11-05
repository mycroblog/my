// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
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
		roomy := translation.Limits{Account: 1000, Server: 1000}
		add := func(account, month string, n int64) {
			t.Helper()
			tx(t, h, func(ctx context.Context, tx uow.Tx) error {
				ok, err := h.Repository.AddUsageWithin(ctx, tx, account, month, n, roomy)
				if err == nil && !ok {
					t.Errorf("add %d for %s in %s: over the limits", n, account, month)
				}
				return err
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
	t.Run("AddsUsageOnlyWithinTheLimits", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		alice, bob := translationID(310), translationID(311)
		const month = "2026-11"
		errOver := errors.New("over")
		// add ends the transaction with an error when it is over, as the limiter does.
		add := func(account string, n int64, limits translation.Limits) bool {
			t.Helper()
			err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
				ok, err := h.Repository.AddUsageWithin(t.Context(), tx, account, month, n, limits)
				if err != nil {
					return err
				}
				if !ok {
					return errOver
				}
				return nil
			})
			if err != nil && !errors.Is(err, errOver) {
				t.Fatal(err)
			}
			return err == nil
		}
		usage := func(account string) translation.Usage {
			t.Helper()
			u, err := h.Repository.FindUsage(t.Context(), account, month)
			if err != nil {
				t.Fatal(err)
			}
			return u
		}
		limits := translation.Limits{Account: 10, Server: 15}
		if add(alice, 11, limits) {
			t.Error("a first request over the account's limit passed")
		}
		if got := usage(alice); got != (translation.Usage{}) {
			t.Errorf("usage after the refusal = %+v, want none", got)
		}
		if !add(alice, 10, limits) {
			t.Error("a request up to the account's limit was refused")
		}
		if add(alice, 1, limits) {
			t.Error("a request past the account's limit passed")
		}
		if got, want := usage(alice), (translation.Usage{Account: 10, Server: 10}); got != want {
			t.Errorf("usage = %+v, want %+v", got, want)
		}
		if add(bob, 6, limits) {
			t.Error("a request past the server's limit passed")
		}
		if got, want := usage(bob), (translation.Usage{Server: 10}); got != want {
			t.Errorf("usage after the server's refusal = %+v, want %+v: the account's row rolled back", got, want)
		}
		if !add(bob, 5, limits) {
			t.Error("a request up to the server's limit was refused")
		}
		if add(bob, 1, translation.Limits{Account: 0, Server: 100}) || add(bob, 1, translation.Limits{Account: 100, Server: 0}) {
			t.Error("a limit of 0 let a character through")
		}
	})
	t.Run("ConcurrentAddsStayWithinTheLimits", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		const month, each, requests = "2026-11", int64(3), 8
		limits := translation.Limits{Account: 1000, Server: 10}
		errOver := errors.New("over")
		var wg sync.WaitGroup
		for i := range requests {
			wg.Go(func() {
				err := h.UnitOfWork.Tx(context.WithoutCancel(t.Context()), func(tx uow.Tx) error {
					ok, err := h.Repository.AddUsageWithin(context.WithoutCancel(t.Context()), tx, translationID(330+i), month, each, limits)
					if err == nil && !ok {
						return errOver
					}
					return err
				})
				if err != nil && !errors.Is(err, errOver) {
					t.Errorf("add: %v", err)
				}
			})
		}
		wg.Wait()
		u, err := h.Repository.FindServerUsage(t.Context(), month)
		if err != nil {
			t.Fatal(err)
		}
		if u.Characters != 9 {
			t.Errorf("characters = %d, want 9: three requests of %d within %d", u.Characters, each, limits.Server)
		}
	})
	t.Run("CountsCacheLookupsByMonth", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		lookup := func(month string, hit bool) {
			t.Helper()
			tx(t, h, func(ctx context.Context, tx uow.Tx) error {
				return h.Repository.AddCacheLookup(ctx, tx, month, hit)
			})
		}
		server := func(month string) translation.ServerUsage {
			t.Helper()
			u, err := h.Repository.FindServerUsage(t.Context(), month)
			if err != nil {
				t.Fatal(err)
			}
			return u
		}
		if got := server("2026-11"); got != (translation.ServerUsage{}) {
			t.Errorf("server usage without a row = %+v", got)
		}
		lookup("2026-11", true)
		lookup("2026-11", true)
		lookup("2026-11", false)
		lookup("2026-10", false)
		tx(t, h, func(ctx context.Context, tx uow.Tx) error {
			ok, err := h.Repository.AddUsageWithin(ctx, tx, translationID(320), "2026-11", 7, translation.Limits{Account: 7, Server: 7})
			if err == nil && !ok {
				t.Error("the first characters of a month with lookups were refused")
			}
			return err
		})
		if got, want := server("2026-11"), (translation.ServerUsage{Characters: 7, CacheHits: 2, CacheMisses: 1}); got != want {
			t.Errorf("server usage = %+v, want %+v", got, want)
		}
		tx(t, h, func(ctx context.Context, tx uow.Tx) error {
			return h.Repository.DeleteUsageBefore(ctx, tx, "2026-11")
		})
		if got := server("2026-10"); got != (translation.ServerUsage{}) {
			t.Errorf("server usage of a deleted month = %+v", got)
		}
	})
}
