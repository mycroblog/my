// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/blob"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The export of a user's data (FR-008, data-model §4.1): the user asks for an archive, a job
// builds it into blob storage, and the user downloads it until it expires. The archive is a ZIP
// file whose entries the composition root chooses (ExportDeps.Entries): the actor, the posts as an
// ActivityStreams outbox, and the user's own moderation records (ADR-0038).

const (
	// JobBuildExport builds the archive of a data export. Its payload is ExportJob.
	JobBuildExport = "identity.build_export"
	// JobExpireExports removes the expired archives and their records.
	JobExpireExports = "identity.expire_exports"
	// ExportExpireInterval is the period of the JobExpireExports schedule.
	ExportExpireInterval = time.Hour
	// ExportTTL is how long an archive (or the record of a failed build) stays.
	ExportTTL = 7 * 24 * time.Hour
	// ExportBuildAttempts is the number of runs of JobBuildExport that build the archive before
	// the export is recorded as failed.
	ExportBuildAttempts = 5
	// exportFailureAttempts is the number of further runs of JobBuildExport that only record the
	// failure, when recording it on the last build attempt fails.
	exportFailureAttempts = 3
	// ExportStaleAfter is how long after its request an export without a result is recorded as
	// failed by JobExpireExports: its build job ran out of runs without recording an outcome.
	ExportStaleAfter = 24 * time.Hour
	// ExportListLimit is the number of exports ListExports returns.
	ExportListLimit = 10
	// exportExpireBatch bounds the exports that one run of JobExpireExports removes.
	exportExpireBatch = 100
	// exportContentType is the media type of the archives.
	exportContentType = "application/zip"
)

var (
	// ErrExportInProgress reports a request for an export while another one is being built.
	ErrExportInProgress = errors.New("identity: a data export is being built")
	// ErrExportNotReady reports a download of an export that has no archive: it is being built,
	// failed, or expired.
	ErrExportNotReady = errors.New("identity: the data export has no archive")
)

// ExportRepository is the persistence of the data exports. The database adapters' identity
// repositories implement it and pass testkit.AccountRepositoryContract.
type ExportRepository interface {
	InsertDataExport(ctx context.Context, tx uow.Tx, e identity.DataExport) error
	// LockPendingDataExports makes the requests of a user's exports and imports that run tx
	// wait for each other until tx ends, and reports whether the user has an export without a
	// result.
	LockPendingDataExports(ctx context.Context, tx uow.Tx, userID string) (bool, error)
	// InsertDataExportResult records the end of a build and reports whether it did: a second
	// result of the same export does nothing, so that the first one stands. It fails with
	// ErrNotFound when the export is gone, and keeps the export from being deleted until tx
	// ends.
	InsertDataExportResult(ctx context.Context, tx uow.Tx, r identity.DataExportResult) (bool, error)
	// ListStalePendingDataExports returns the IDs of up to limit exports without a result that
	// were requested before t, oldest first.
	ListStalePendingDataExports(ctx context.Context, t time.Time, limit int) ([]string, error)
	// FindDataExport returns an export with its result. It fails with ErrNotFound when there is
	// none.
	FindDataExport(ctx context.Context, id string) (identity.DataExport, error)
	// ListDataExports returns up to limit exports of a user with their results, newest first.
	ListDataExports(ctx context.Context, userID string, limit int) ([]identity.DataExport, error)
	// ListExpiredDataExports returns up to limit exports whose result expires at or before t,
	// oldest expiry first.
	ListExpiredDataExports(ctx context.Context, t time.Time, limit int) ([]identity.DataExport, error)
	// DeleteDataExport deletes an export with its result; deleting a missing one succeeds. It
	// waits for a transaction that is recording the export's result.
	DeleteDataExport(ctx context.Context, tx uow.Tx, id string) error
}

// ArchiveEntry is a file of the archive: Write writes the content of Name for the local account
// accountID to w.
type ArchiveEntry struct {
	Name  string
	Write func(ctx context.Context, accountID string, w io.Writer) error
}

// ExportDeps are the ports of Exports.
type ExportDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository ExportRepository
	Blobs      blob.Store
	// Outbox enqueues the build of an export in the transaction of the request.
	Outbox queue.Outbox
	// Accounts finds the account whose data an export holds.
	Accounts *account.Service
	// Entries are the files of the archive, in order.
	Entries []ArchiveEntry
	Clock   kernel.Clock
	IDs     kernel.IDGenerator
	// Logger logs the blobs that could not be removed; nil means slog.Default().
	Logger *slog.Logger
	// Reporter receives a panic of the goroutine that writes an archive (kernel.Go); nil only
	// panics again.
	Reporter kernel.ErrorReporter
}

// Exports runs the data exports.
type Exports struct{ d ExportDeps }

// NewExports returns the data exports on d.
func NewExports(d ExportDeps) *Exports {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Exports{d: d}
}

// ExportJob is the payload of JobBuildExport.
type ExportJob struct {
	ExportID string `json:"export_id"`
}

// RequestExport records a user's request for an export of their data and enqueues its build in
// the same transaction. While another export of the user is being built, it fails with
// ErrExportInProgress; concurrent requests of a user check and insert one at a time, so that a
// user has at most one export being built.
func (x *Exports) RequestExport(ctx context.Context, userID string) (identity.DataExport, error) {
	e := identity.DataExport{ID: x.d.IDs.NewID(), UserID: userID, RequestedAt: kernel.Normalize(x.d.Clock.Now())}
	payload, err := json.Marshal(ExportJob{ExportID: e.ID})
	if err != nil {
		return identity.DataExport{}, err
	}
	err = x.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		pending, err := x.d.Repository.LockPendingDataExports(ctx, tx, userID)
		if err != nil {
			return err
		}
		if pending {
			return ErrExportInProgress
		}
		if err := x.d.Repository.InsertDataExport(ctx, tx, e); err != nil {
			return err
		}
		return x.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{
			Type: JobBuildExport, Payload: payload, IdempotencyKey: JobBuildExport + ":" + e.ID,
		})
	})
	if err != nil {
		return identity.DataExport{}, err
	}
	return e, nil
}

// ListExports returns the latest ExportListLimit exports of a user, newest first.
func (x *Exports) ListExports(ctx context.Context, userID string) ([]identity.DataExport, error) {
	return x.d.Repository.ListDataExports(ctx, userID, ExportListLimit)
}

// OpenExport opens the archive of a user's export; close the reader when done. An export of
// another user fails with ErrNotFound, and one without an archive now with ErrExportNotReady.
func (x *Exports) OpenExport(ctx context.Context, userID, exportID string) (io.ReadCloser, blob.Info, error) {
	e, err := x.d.Repository.FindDataExport(ctx, exportID)
	if err != nil {
		return nil, blob.Info{}, err
	}
	if e.UserID != userID {
		return nil, blob.Info{}, fmt.Errorf("%w: export %s is another user's", ErrNotFound, exportID)
	}
	if e.State(x.d.Clock.Now()) != identity.ExportStateReady {
		return nil, blob.Info{}, ErrExportNotReady
	}
	r, info, err := x.d.Blobs.Get(ctx, e.Result.BlobKey)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, blob.Info{}, ErrExportNotReady
	}
	return r, info, err
}

// Now returns the current time of the exports' clock, for presenting their states.
func (x *Exports) Now() time.Time { return x.d.Clock.Now() }

// RegisterJobs registers the build of the exports and the hourly removal of the expired ones.
func (x *Exports) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.RegisterWithPolicy(JobBuildExport, x.handleBuild, queue.RetryPolicy{MaxAttempts: ExportBuildAttempts + exportFailureAttempts})
	reg.Register(JobExpireExports, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := x.ExpireExports(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	sched.Add(queue.Schedule{Name: JobExpireExports, Every: ExportExpireInterval, Job: queue.NewJob{Type: JobExpireExports}})
}

// errExportOutOfAttempts is the error of the runs of JobBuildExport after ExportBuildAttempts.
var errExportOutOfAttempts = errors.New("identity: the build of the export ran out of attempts")

// handleBuild builds the archive in the first ExportBuildAttempts runs and records the export as
// failed when the last of them fails. The runs after it only record the failure, and an export
// that none of them recorded is recorded as failed by ExpireExports after ExportStaleAfter, so
// that a failed export does not stay pending.
func (x *Exports) handleBuild(ctx context.Context, j queue.Job) queue.Result {
	var p ExportJob
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.ExportID == "" {
		return queue.Fail(fmt.Errorf("identity: invalid %s payload: %w", JobBuildExport, err))
	}
	err := errExportOutOfAttempts
	if j.Attempt <= ExportBuildAttempts {
		err = x.BuildExport(ctx, p.ExportID)
	}
	switch {
	case err == nil, errors.Is(err, ErrNotFound):
		return queue.Done()
	case j.Attempt >= ExportBuildAttempts:
		if ferr := x.recordFailure(ctx, p.ExportID); ferr != nil {
			return queue.Retry(0, errors.Join(err, ferr))
		}
		x.d.Logger.ErrorContext(ctx, "identity: a data export failed", "export_id", p.ExportID, "error", err)
		return queue.Done()
	default:
		return queue.Retry(0, err)
	}
}

// exportKey returns a new private blob key for an archive of an export. Each build attempt
// writes its own key, which its result records, so that overlapping attempts never write or
// remove each other's archive.
func (x *Exports) exportKey(e identity.DataExport) string {
	return "exports/" + e.UserID + "/" + e.ID + "/" + x.d.IDs.NewID() + ".zip"
}

// BuildExport builds the archive of an export into blob storage and records it as ready. An
// export that has a result already is left as it is, so running it again is safe; an unknown
// export fails with ErrNotFound. An archive that does not become the export's result (another
// attempt recorded one first, or the export is gone) is removed again.
func (x *Exports) BuildExport(ctx context.Context, exportID string) error {
	e, err := x.d.Repository.FindDataExport(ctx, exportID)
	if err != nil {
		return err
	}
	if e.Result != nil {
		return nil
	}
	la, err := x.d.Accounts.LocalAccountOfUser(ctx, e.UserID)
	if err != nil {
		return err
	}
	key := x.exportKey(e)
	pr, pw := io.Pipe()
	kernel.Go(ctx, "identity.export.write", x.d.Reporter, func(ctx context.Context) { pw.CloseWithError(x.writeArchive(ctx, la.AccountID, pw)) })
	info, err := x.d.Blobs.Put(ctx, key, pr, blob.PutOptions{ContentType: exportContentType})
	_ = pr.CloseWithError(errors.New("identity: the archive was not read to its end"))
	if err != nil {
		x.discard(ctx, key)
		return fmt.Errorf("identity: build export %s: %w", exportID, err)
	}
	now := kernel.Normalize(x.d.Clock.Now())
	res := identity.DataExportResult{
		ExportID: e.ID, Outcome: identity.ExportReady, BlobKey: key, FileSize: info.Size,
		ExpiresAt: now.Add(ExportTTL), CompletedAt: now,
	}
	recorded := false
	err = x.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		recorded, err = x.d.Repository.InsertDataExportResult(ctx, tx, res)
		return err
	})
	switch {
	case err == nil && recorded:
		return nil
	case err == nil, errors.Is(err, ErrNotFound):
		x.discard(ctx, key)
		return err
	}
	// The commit may have succeeded all the same: the archive goes only when it is not the
	// result.
	got, ferr := x.d.Repository.FindDataExport(ctx, exportID)
	if errors.Is(ferr, ErrNotFound) || (ferr == nil && (got.Result == nil || got.Result.BlobKey != key)) {
		x.discard(ctx, key)
	}
	return err
}

// discard removes an archive that is no export's result; a failure leaves it in blob storage
// and is logged.
func (x *Exports) discard(ctx context.Context, key string) {
	if err := x.d.Blobs.Delete(ctx, key); err != nil {
		x.d.Logger.WarnContext(ctx, "identity: an unused data export archive could not be removed", "key", key, "error", err)
	}
}

// writeArchive writes the ZIP archive of accountID's data to w.
func (x *Exports) writeArchive(ctx context.Context, accountID string, w io.Writer) error {
	zw := zip.NewWriter(w)
	modified := x.d.Clock.Now()
	for _, entry := range x.d.Entries {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: entry.Name, Method: zip.Deflate, Modified: modified})
		if err != nil {
			return err
		}
		if err := entry.Write(ctx, accountID, f); err != nil {
			return fmt.Errorf("identity: write %s of the export: %w", entry.Name, err)
		}
	}
	return zw.Close()
}

// recordFailure records an export as failed in one short transaction; a result recorded first
// stands, and a missing export is done. Each build attempt removes its own archive, so there is
// none to remove here.
func (x *Exports) recordFailure(ctx context.Context, exportID string) error {
	now := kernel.Normalize(x.d.Clock.Now())
	res := identity.DataExportResult{ExportID: exportID, Outcome: identity.ExportFailed, ExpiresAt: now.Add(ExportTTL), CompletedAt: now}
	err := x.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := x.d.Repository.InsertDataExportResult(ctx, tx, res)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// ExpireExports records as failed up to exportExpireBatch exports that have had no result for
// ExportStaleAfter, then removes up to exportExpireBatch expired exports: the archive first,
// outside any transaction, then the records, so that a failed removal of the blob is retried by
// the next run. It returns how many exports it removed.
func (x *Exports) ExpireExports(ctx context.Context) (int, error) {
	now := kernel.Normalize(x.d.Clock.Now())
	stale, err := x.d.Repository.ListStalePendingDataExports(ctx, now.Add(-ExportStaleAfter), exportExpireBatch)
	if err != nil {
		return 0, err
	}
	for _, id := range stale {
		if err := x.recordFailure(ctx, id); err != nil {
			return 0, err
		}
		x.d.Logger.ErrorContext(ctx, "identity: a data export was not built in time", "export_id", id)
	}
	es, err := x.d.Repository.ListExpiredDataExports(ctx, now, exportExpireBatch)
	if err != nil {
		return 0, err
	}
	for i, e := range es {
		if err := x.remove(ctx, e); err != nil {
			return i, err
		}
	}
	return len(es), nil
}

// remove deletes the archive of an export, then its records.
func (x *Exports) remove(ctx context.Context, e identity.DataExport) error {
	if e.Result != nil && e.Result.BlobKey != "" {
		if err := x.d.Blobs.Delete(ctx, e.Result.BlobKey); err != nil {
			return fmt.Errorf("identity: remove the archive of export %s: %w", e.ID, err)
		}
	}
	return x.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return x.d.Repository.DeleteDataExport(ctx, tx, e.ID) })
}

// EraseAccountData implements account.DataEraser: the deleted account's user loses every export
// with its archive (data-model §8).
func (x *Exports) EraseAccountData(ctx context.Context, t account.ErasureTarget, _ string, limit int) (account.ErasureStep, error) {
	if t.UserID == "" {
		return account.ErasureStep{Done: true}, nil
	}
	es, err := x.d.Repository.ListDataExports(ctx, t.UserID, limit)
	if err != nil {
		return account.ErasureStep{}, err
	}
	for _, e := range es {
		if err := x.remove(ctx, e); err != nil {
			return account.ErasureStep{}, err
		}
	}
	return account.ErasureStep{Done: len(es) < limit}, nil
}
