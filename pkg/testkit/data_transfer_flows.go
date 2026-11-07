// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The cases of AccountRepositoryContract about the consistency of the data exports and the CSV
// imports (FR-008) under concurrent requests, overlapping job runs, failures, and removals.

// rendezvousOutbox is a recordingOutbox whose EnqueueTx, which a request runs after its check
// and insert and before its commit, waits briefly for a second request to get there too, so
// that requests that do not exclude each other overlap.
type rendezvousOutbox struct {
	*recordingOutbox
	mu      sync.Mutex
	arrived int
	both    chan struct{}
}

func newRendezvousOutbox() *rendezvousOutbox {
	return &rendezvousOutbox{recordingOutbox: &recordingOutbox{}, both: make(chan struct{})}
}

func (o *rendezvousOutbox) EnqueueTx(ctx context.Context, tx uow.Tx, jobs ...queue.NewJob) error {
	o.mu.Lock()
	o.arrived++
	if o.arrived == 2 {
		close(o.both)
	}
	o.mu.Unlock()
	select {
	case <-o.both:
	case <-time.After(200 * time.Millisecond):
	}
	return o.recordingOutbox.EnqueueTx(ctx, tx, jobs...)
}

// exportsAreRequestedOneAtATime checks that concurrent requests of a user start one export.
func exportsAreRequestedOneAtATime(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	x := newExportEnv(e)
	alice := e.register(t, "alice")
	exports := appidentity.NewExports(appidentity.ExportDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Identity, Blobs: x.blobs, Outbox: newRendezvousOutbox(), Accounts: e.accounts,
		Clock: e.clock, IDs: e.ids,
	})
	errs := concurrently(8, func() error {
		_, err := exports.RequestExport(ctx, alice.User.ID)
		return err
	})
	started := 0
	for _, err := range errs {
		switch {
		case err == nil:
			started++
		case !errors.Is(err, appidentity.ErrExportInProgress):
			t.Errorf("RequestExport = %v, want nil or ErrExportInProgress", err)
		}
	}
	list, err := x.exports.ListExports(ctx, alice.User.ID)
	if started != 1 || err != nil || len(list) != 1 {
		t.Errorf("%d requests started, %d exports listed (%v); want one", started, len(list), err)
	}
}

// concurrently runs f n times at once and returns the errors.
func concurrently(n int, f func() error) []error {
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			<-start
			errs[i] = f()
		})
	}
	close(start)
	wg.Wait()
	return errs
}

// exportsDoNotStayPending checks that an export whose build fails is recorded as failed on its
// last attempt even when the blob store cannot remove anything, that a run after the build
// attempts records the failure, and that an export whose job ran out of runs without recording
// anything is recorded as failed after ExportStaleAfter, so that the user may ask again.
func exportsDoNotStayPending(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	x := newExportEnv(e)
	alice, bob := e.register(t, "alice"), e.register(t, "bob")
	h := x.buildHandler(t)

	req, err := x.exports.RequestExport(ctx, alice.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	job := x.outbox.take()[0]
	x.fail, x.deleteErr = true, errors.New("the blob store is down")
	for attempt := 1; attempt < appidentity.ExportBuildAttempts; attempt++ {
		if r := h(ctx, queue.Job{Type: job.Type, Payload: job.Payload, Attempt: attempt}); !r.IsRetry() {
			t.Fatalf("failed attempt %d = %+v, want a retry", attempt, r)
		}
	}
	if r := h(ctx, queue.Job{Type: job.Type, Payload: job.Payload, Attempt: appidentity.ExportBuildAttempts}); !r.IsDone() {
		t.Errorf("last failed attempt = %+v, want done", r)
	}
	if got, err := x.h.Identity.FindDataExport(ctx, req.ID); err != nil || got.State(e.clock.Now()) != identity.ExportStateFailed {
		t.Errorf("export after its last attempt = %+v, %v; want failed", got, err)
	}
	if _, err := x.exports.RequestExport(ctx, alice.User.ID); err != nil {
		t.Errorf("RequestExport after the failure = %v", err)
	}
	x.outbox.take()

	// A run after the build attempts records the failure without building.
	x.fail, x.deleteErr = false, nil
	extra, err := x.exports.RequestExport(ctx, bob.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	job = x.outbox.take()[0]
	if r := h(ctx, queue.Job{Type: job.Type, Payload: job.Payload, Attempt: appidentity.ExportBuildAttempts + 1}); !r.IsDone() {
		t.Errorf("run after the build attempts = %+v, want done", r)
	}
	if got, err := x.h.Identity.FindDataExport(ctx, extra.ID); err != nil || got.State(e.clock.Now()) != identity.ExportStateFailed {
		t.Errorf("export after the build attempts = %+v, %v; want failed", got, err)
	}

	// An export that no run recorded stays pending until ExportStaleAfter, then fails.
	stuck, err := x.exports.RequestExport(ctx, bob.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(appidentity.ExportStaleAfter - time.Minute)
	if _, err := x.exports.ExpireExports(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := x.h.Identity.FindDataExport(ctx, stuck.ID); got.Result != nil {
		t.Errorf("export before ExportStaleAfter = %+v, want pending", got)
	}
	e.clock.Advance(2 * time.Minute)
	if _, err := x.exports.ExpireExports(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := x.h.Identity.FindDataExport(ctx, stuck.ID); err != nil || got.State(e.clock.Now()) != identity.ExportStateFailed {
		t.Errorf("export after ExportStaleAfter = %+v, %v; want failed", got, err)
	}
	if _, err := x.exports.RequestExport(ctx, bob.User.ID); err != nil {
		t.Errorf("RequestExport after the stale export = %v", err)
	}
}

// exportsKeepTheRecordedArchive checks overlapping build attempts: an attempt that finishes
// after another one recorded the archive neither replaces nor removes it, even when it fails
// on its last attempt, and an attempt that finishes after the export was erased leaves neither
// a result nor an archive.
func exportsKeepTheRecordedArchive(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	x := newExportEnv(e)
	alice := e.register(t, "alice")
	h := x.buildHandler(t)

	// overlap runs job as a slow attempt numbered slowAttempt, which returns slowErr after
	// writing its archive, while a second attempt runs in full.
	overlap := func(job queue.NewJob, slowAttempt int, slowErr error) (slow, fast queue.Result) {
		t.Helper()
		entered, release := make(chan struct{}), make(chan struct{})
		var mu sync.Mutex
		calls := 0
		x.write = func(_ context.Context, w io.Writer) error {
			mu.Lock()
			calls++
			first := calls == 1
			mu.Unlock()
			if !first {
				_, err := io.WriteString(w, "the archive of the attempt that finished first")
				return err
			}
			close(entered)
			<-release
			if _, err := io.WriteString(w, "a longer archive of the attempt that started first and finished last"); err != nil {
				return err
			}
			return slowErr
		}
		done := make(chan queue.Result)
		go func() { done <- h(ctx, queue.Job{Type: job.Type, Payload: job.Payload, Attempt: slowAttempt}) }()
		<-entered
		fast = h(ctx, queue.Job{Type: job.Type, Payload: job.Payload, Attempt: 1})
		close(release)
		return <-done, fast
	}
	check := func(id string) {
		t.Helper()
		got, err := x.h.Identity.FindDataExport(ctx, id)
		if err != nil || got.State(e.clock.Now()) != identity.ExportStateReady {
			t.Fatalf("export = %+v, %v; want ready", got, err)
		}
		r, info, err := x.exports.OpenExport(ctx, alice.User.ID, id)
		if err != nil {
			t.Fatalf("OpenExport = %v", err)
		}
		data, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil || int64(len(data)) != got.Result.FileSize || info.Size != got.Result.FileSize {
			t.Errorf("archive = %d bytes (%v), info %d; want the recorded %d", len(data), err, info.Size, got.Result.FileSize)
		}
		if keys := x.archiveKeys(); len(keys) != 1 || keys[0] != got.Result.BlobKey {
			t.Errorf("archives = %v, want only the recorded %s", keys, got.Result.BlobKey)
		}
	}
	expire := func() {
		t.Helper()
		e.clock.Advance(appidentity.ExportTTL)
		if _, err := x.exports.ExpireExports(ctx); err != nil {
			t.Fatal(err)
		}
		if keys := x.archiveKeys(); len(keys) != 0 {
			t.Fatalf("archives after the expiry = %v, want none", keys)
		}
	}

	req, err := x.exports.RequestExport(ctx, alice.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if slow, fast := overlap(x.outbox.take()[0], 1, nil); !slow.IsDone() || !fast.IsDone() {
		t.Fatalf("overlapping attempts = %+v, %+v; want done", slow, fast)
	}
	check(req.ID)

	// A last attempt that fails after another attempt recorded the archive leaves it.
	expire()
	req, err = x.exports.RequestExport(ctx, alice.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if slow, fast := overlap(x.outbox.take()[0], appidentity.ExportBuildAttempts, errors.New("the entry fails")); !slow.IsDone() || !fast.IsDone() {
		t.Fatalf("overlapping attempts = %+v, %+v; want done", slow, fast)
	}
	check(req.ID)

	// An attempt that finishes after the export was erased records nothing.
	expire()
	erased, err := x.exports.RequestExport(ctx, alice.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	job := x.outbox.take()[0]
	target := appaccount.ErasureTarget{AccountID: alice.AccountID, UserID: alice.User.ID}
	x.write = func(ctx context.Context, w io.Writer) error {
		if _, err := x.exports.EraseAccountData(ctx, target, "", 10); err != nil {
			return err
		}
		_, err := io.WriteString(w, "the archive of an erased export")
		return err
	}
	if r := h(ctx, queue.Job{Type: job.Type, Payload: job.Payload, Attempt: 1}); !r.IsDone() {
		t.Errorf("attempt of an erased export = %+v, want done", r)
	}
	if keys := x.archiveKeys(); len(keys) != 0 {
		t.Errorf("archives after the erasure = %v, want none", keys)
	}
	// Recording the export again shows that no result was left behind.
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.h.Identity.InsertDataExport(ctx, tx, erased) }); err != nil {
		t.Fatal(err)
	}
	if got, err := x.h.Identity.FindDataExport(ctx, erased.ID); err != nil || got.Result != nil {
		t.Errorf("export recorded again = %+v, %v; want no result", got, err)
	}
}

// importsStartOneAtATime checks that concurrent imports of a user start one import.
func importsStartOneAtATime(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	x := newImportEnv(e)
	alice := e.register(t, "alice")
	in := appidentity.ImportInput{
		UserID: alice.User.ID, AccountID: alice.AccountID, Kind: identity.ImportBlocks, Mode: identity.ImportMerge, Text: "a@remote.example\n",
	}
	imports := appidentity.NewImports(appidentity.ImportDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Identity, Outbox: newRendezvousOutbox(), Targets: x.targets,
		Host: "my.example", Clock: e.clock, IDs: e.ids, Logger: slog.New(slog.DiscardHandler),
	})
	errs := concurrently(8, func() error {
		_, err := imports.StartImport(ctx, in)
		return err
	})
	started := 0
	for _, err := range errs {
		switch {
		case err == nil:
			started++
		case !errors.Is(err, appidentity.ErrImportInProgress):
			t.Errorf("StartImport = %v, want nil or ErrImportInProgress", err)
		}
	}
	list, err := x.imports.ListImports(ctx, alice.User.ID)
	if started != 1 || err != nil || len(list) != 1 {
		t.Errorf("%d imports started, %d listed (%v); want one", started, len(list), err)
	}
}

// importsCountFailuresPerItem checks that each item has its own ImportItemAttempts: items that
// fail one fewer time than that are carried out however many runs earlier items took, and an
// item that keeps failing is recorded as failed after ImportItemAttempts failures, with each
// retry delayed.
func importsCountFailuresPerItem(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	x := newImportEnv(e)
	alice := e.register(t, "alice")
	x.targets.transient = map[string]int{
		"first@remote.example":  appidentity.ImportItemAttempts - 1,
		"second@remote.example": appidentity.ImportItemAttempts - 1,
	}
	started, err := x.imports.StartImport(ctx, appidentity.ImportInput{
		UserID: alice.User.ID, AccountID: alice.AccountID, Kind: identity.ImportBlocks, Mode: identity.ImportMerge,
		Text: "first@remote.example\nsecond@remote.example\nflaky@remote.example\nlast@remote.example\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	x.run(t)
	got, err := e.h.Identity.FindDataImport(ctx, started.ID)
	if err != nil || !got.Finished() || got.Processed != 4 || got.Failed != 1 {
		t.Errorf("import = %+v, %v; want finished with only the always failing item failed", got, err)
	}
	if x.retries != 3*(appidentity.ImportItemAttempts-1) {
		t.Errorf("%d delayed retries, want %d", x.retries, 3*(appidentity.ImportItemAttempts-1))
	}
}

// importsStopWhenRemoved checks that a batch whose import is removed while it runs records
// nothing more and stops.
func importsStopWhenRemoved(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	x := newImportEnv(e)
	alice := e.register(t, "alice")
	target := appaccount.ErasureTarget{AccountID: alice.AccountID, UserID: alice.User.ID}
	x.targets.onAdd = func(item identity.DataImportItem) {
		if item.Target == "eraser@remote.example" {
			if _, err := x.imports.EraseAccountData(ctx, target, "", 10); err != nil {
				t.Errorf("EraseAccountData = %v", err)
			}
		}
	}
	started, err := x.imports.StartImport(ctx, appidentity.ImportInput{
		UserID: alice.User.ID, AccountID: alice.AccountID, Kind: identity.ImportBlocks, Mode: identity.ImportMerge,
		Text: "first@remote.example\neraser@remote.example\nlast@remote.example\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range x.run(t) {
		if !r.IsDone() {
			t.Errorf("job = %+v, want done", r)
		}
	}
	if len(x.targets.adds) != 2 {
		t.Errorf("adds = %+v, want the two items before the removal", x.targets.adds)
	}
	// Recording the import again shows that no result was left behind.
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.h.Identity.InsertDataImport(ctx, tx, started.DataImport) }); err != nil {
		t.Fatal(err)
	}
	if got, err := e.h.Identity.FindDataImport(ctx, started.ID); err != nil || got.Processed != 0 {
		t.Errorf("import recorded again = %+v, %v; want no results", got, err)
	}
}
