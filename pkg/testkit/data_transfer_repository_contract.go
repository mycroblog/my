// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The cases of AccountRepositoryContract on the data exports and the CSV imports of the identity
// repository.

// export requests a data export of userID and returns it.
func (r *identityRepo) export(t *testing.T, userID string) identity.DataExport {
	t.Helper()
	r.clock.Advance(time.Second)
	e := identity.DataExport{ID: r.ids.NewID(), UserID: userID, RequestedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Identity.InsertDataExport(ctx, tx, e) })
	return e
}

// exportResult records the result of an export and reports whether it did.
func (r *identityRepo) exportResult(t *testing.T, res identity.DataExportResult) bool {
	t.Helper()
	var ok bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		ok, err = r.h.Identity.InsertDataExportResult(ctx, tx, res)
		return err
	})
	return ok
}

func dataRepoExports(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice, bob := r.user(t), r.user(t)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if pending, err := r.h.Identity.LockPendingDataExports(ctx, tx, alice); err != nil || pending {
			t.Errorf("LockPendingDataExports before any = %t, %v; want false", pending, err)
		}
		return nil
	})
	first := r.export(t, alice)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if pending, err := r.h.Identity.LockPendingDataExports(ctx, tx, alice); err != nil || !pending {
			t.Errorf("LockPendingDataExports with a pending export = %t, %v; want true", pending, err)
		}
		return nil
	})
	ready := identity.DataExportResult{ExportID: first.ID, Outcome: identity.ExportReady, BlobKey: "exports/" + first.ID, FileSize: 42, ExpiresAt: r.clock.Now().Add(time.Hour), CompletedAt: r.clock.Now()}
	if !r.exportResult(t, ready) {
		t.Error("InsertDataExportResult = false, want the first result recorded")
	}
	failed := ready
	failed.Outcome, failed.BlobKey, failed.FileSize = identity.ExportFailed, "", 0
	if r.exportResult(t, failed) {
		t.Error("InsertDataExportResult of a second result = true, want the first to stand")
	}
	got, err := r.h.Identity.FindDataExport(ctx, first.ID)
	if err != nil || got.Result == nil || got.Result.Outcome != identity.ExportReady || got.Result.BlobKey != ready.BlobKey || got.Result.FileSize != 42 || !got.Result.ExpiresAt.Equal(ready.ExpiresAt) {
		t.Errorf("FindDataExport = %+v, %v; want the ready result", got, err)
	}
	second := r.export(t, alice)
	r.export(t, bob)
	if list, err := r.h.Identity.ListDataExports(ctx, alice, 10); err != nil || len(list) != 2 || list[0].ID != second.ID || list[0].Result != nil || list[1].ID != first.ID {
		t.Errorf("ListDataExports = %+v, %v; want alice's two, newest first", list, err)
	}
	if ids, err := r.h.Identity.ListStalePendingDataExports(ctx, r.clock.Now().Add(time.Second), 1); err != nil || !slices.Equal(ids, []string{second.ID}) {
		t.Errorf("ListStalePendingDataExports(1) = %v, %v; want the oldest pending", ids, err)
	}
	if list, err := r.h.Identity.ListExpiredDataExports(ctx, r.clock.Now().Add(2*time.Hour), 10); err != nil || len(list) != 1 || list[0].ID != first.ID {
		t.Errorf("ListExpiredDataExports = %+v, %v; want the first", list, err)
	}
	if list, err := r.h.Identity.ListExpiredDataExports(ctx, r.clock.Now(), 10); err != nil || len(list) != 0 {
		t.Errorf("ListExpiredDataExports before the expiry = %+v, %v; want none", list, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for range 2 { // deleting a missing export succeeds
			if err := r.h.Identity.DeleteDataExport(ctx, tx, first.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if _, err := r.h.Identity.FindDataExport(ctx, first.ID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindDataExport after the deletion = %v, want ErrNotFound", err)
	}
	err = r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := r.h.Identity.InsertDataExportResult(ctx, tx, ready)
		return err
	})
	if !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("InsertDataExportResult of a deleted export = %v, want ErrNotFound", err)
	}
}

func dataRepoImports(t *testing.T, r *identityRepo) {
	ctx := t.Context()
	alice := r.user(t)
	account := r.localAccount(t, alice, "alice")
	r.clock.Advance(time.Second)
	imp := identity.DataImport{ID: r.ids.NewID(), UserID: alice, AccountID: account, Kind: identity.ImportFollowing, Mode: identity.ImportMerge, CreatedAt: r.clock.Now()}
	items := []identity.DataImportItem{
		{ImportID: imp.ID, Position: 0, Action: identity.ImportAdd, Target: "a@remote.example", ShowBoosts: true},
		{ImportID: imp.ID, Position: 1, Action: identity.ImportAdd, Target: "b@remote.example", Notify: true},
		{ImportID: imp.ID, Position: 2, Action: identity.ImportAdd, Target: "c@remote.example"},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if unfinished, err := r.h.Identity.LockUnfinishedDataImports(ctx, tx, alice); err != nil || unfinished {
			t.Errorf("LockUnfinishedDataImports before any = %t, %v; want false", unfinished, err)
		}
		if err := r.h.Identity.InsertDataImport(ctx, tx, imp); err != nil {
			return err
		}
		return r.h.Identity.InsertDataImportItems(ctx, tx, items)
	})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if unfinished, err := r.h.Identity.LockUnfinishedDataImports(ctx, tx, alice); err != nil || !unfinished {
			t.Errorf("LockUnfinishedDataImports with pending items = %t, %v; want true", unfinished, err)
		}
		for i, want := range []int{1, 2} {
			n, err := r.h.Identity.InsertDataImportItemFailure(ctx, tx, imp.ID, 1, r.clock.Now())
			if err != nil {
				return err
			}
			if n != want {
				t.Errorf("InsertDataImportItemFailure #%d = %d, want %d", i+1, n, want)
			}
		}
		if err := r.h.Identity.InsertDataImportResult(ctx, tx, identity.DataImportResult{ImportID: imp.ID, Position: 0, Outcome: identity.ImportDone, ProcessedAt: r.clock.Now()}); err != nil {
			return err
		}
		failed := identity.DataImportResult{ImportID: imp.ID, Position: 1, Outcome: identity.ImportFailed, Failure: identity.ImportFailureUnresolvable, ProcessedAt: r.clock.Now()}
		if err := r.h.Identity.InsertDataImportResult(ctx, tx, failed); err != nil {
			return err
		}
		// A second result of an item does nothing.
		return r.h.Identity.InsertDataImportResult(ctx, tx, identity.DataImportResult{ImportID: imp.ID, Position: 1, Outcome: identity.ImportDone, ProcessedAt: r.clock.Now()})
	})
	got, err := r.h.Identity.FindDataImport(ctx, imp.ID)
	if err != nil || got.ID != imp.ID || got.Kind != identity.ImportFollowing || got.Total != 3 || got.Processed != 2 || got.Failed != 1 {
		t.Errorf("FindDataImport = %+v, %v; want 3 items, 2 processed, 1 failed", got, err)
	}
	if list, err := r.h.Identity.ListDataImports(ctx, alice, 10); err != nil || len(list) != 1 || list[0].Total != 3 {
		t.Errorf("ListDataImports = %+v, %v", list, err)
	}
	pending, err := r.h.Identity.ListPendingDataImportItems(ctx, imp.ID, 10)
	if err != nil || len(pending) != 1 || pending[0] != items[2] {
		t.Errorf("ListPendingDataImportItems = %+v, %v; want the third item", pending, err)
	}
	if ids, err := r.h.Identity.ListDataImportsCreatedBefore(ctx, r.clock.Now().Add(time.Second), 10); err != nil || !slices.Equal(ids, []string{imp.ID}) {
		t.Errorf("ListDataImportsCreatedBefore = %v, %v", ids, err)
	}
	if ids, err := r.h.Identity.ListDataImportsCreatedBefore(ctx, imp.CreatedAt, 10); err != nil || len(ids) != 0 {
		t.Errorf("ListDataImportsCreatedBefore(its creation) = %v, %v; want none", ids, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for range 2 { // deleting a missing import succeeds
			if err := r.h.Identity.DeleteDataImport(ctx, tx, imp.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if _, err := r.h.Identity.FindDataImport(ctx, imp.ID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindDataImport after the deletion = %v, want ErrNotFound", err)
	}
	err = r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return r.h.Identity.InsertDataImportResult(ctx, tx, identity.DataImportResult{ImportID: imp.ID, Position: 2, Outcome: identity.ImportDone, ProcessedAt: r.clock.Now()})
	})
	if !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("InsertDataImportResult of a deleted import = %v, want ErrNotFound", err)
	}
	err = r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := r.h.Identity.InsertDataImportItemFailure(ctx, tx, imp.ID, 2, r.clock.Now())
		return err
	})
	if !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("InsertDataImportItemFailure of a deleted import = %v, want ErrNotFound", err)
	}
}
