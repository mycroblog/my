// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/blob"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The cases of AccountRepositoryContract about a user's own account (FR-008): its deletion at the
// user's request and the exports of its data.

// accountDeletesAtTheOwnersRequest checks that a user deletes their account with their password:
// a wrong password changes nothing, the right one records a deleted lifecycle event initiated by
// the account itself and ends the user's sessions, and a second deletion is refused.
func accountDeletesAtTheOwnersRequest(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	appidentity.RegisterLifecycleProjectors(e.projectors, e.h.Identity, e.accounts, e.clock)
	alice := e.register(t, "alice")
	bob := e.register(t, "bob")
	state := func(id string) account.LifecycleState {
		t.Helper()
		a, err := e.accounts.FindByID(ctx, id)
		if err != nil {
			t.Fatalf("FindByID = %v", err)
		}
		return a.Lifecycle
	}
	if err := e.identity.DeleteOwnAccount(ctx, alice.User.ID, appidentity.Confirmation{Password: "wrong password"}); !errors.Is(err, appidentity.ErrWrongPassword) {
		t.Fatalf("DeleteOwnAccount(wrong password) = %v, want ErrWrongPassword", err)
	}
	if s := state(alice.AccountID); s.State != account.StateActive {
		t.Fatalf("state after a wrong password = %+v, want active", s)
	}
	if err := e.identity.DeleteOwnAccount(ctx, alice.User.ID, appidentity.Confirmation{Password: "correct horse battery staple"}); err != nil {
		t.Fatalf("DeleteOwnAccount = %v", err)
	}
	if s := state(alice.AccountID); s.State != account.StateDeleted || !s.Since.Equal(e.clock.Now()) {
		t.Errorf("state after the deletion = %+v, want deleted now", s)
	}
	if s := state(bob.AccountID); s.State != account.StateActive {
		t.Errorf("another account's state = %+v, want active", s)
	}
	if err := e.identity.DeleteOwnAccount(ctx, alice.User.ID, appidentity.Confirmation{Password: "correct horse battery staple"}); !errors.Is(err, appaccount.ErrAlreadyDeleted) {
		t.Errorf("second DeleteOwnAccount = %v, want ErrAlreadyDeleted", err)
	}
	instance, err := e.accounts.EnsureInstanceActor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.accounts.DeleteBySelf(ctx, instance.ID); !errors.Is(err, appaccount.ErrNotLocal) {
		t.Errorf("DeleteBySelf(instance actor) = %v, want ErrNotLocal", err)
	}
}

// exportEnv runs the data exports on an accountEnv with an in-memory blob store.
type exportEnv struct {
	*accountEnv
	blobs   *MemoryBlobStore
	outbox  *recordingOutbox
	exports *appidentity.Exports
	// fail makes the archive's entry fail.
	fail bool
	// write, when set, writes the archive's entry instead.
	write func(ctx context.Context, w io.Writer) error
	// deleteErr makes the removals of blobs fail.
	deleteErr error
}

// exportBlobs is the blob store of an exportEnv, whose removals fail with its deleteErr.
type exportBlobs struct {
	*MemoryBlobStore
	x *exportEnv
}

// Delete implements blob.Store.
func (b exportBlobs) Delete(ctx context.Context, key string) error {
	if b.x.deleteErr != nil {
		return b.x.deleteErr
	}
	return b.MemoryBlobStore.Delete(ctx, key)
}

func newExportEnv(e *accountEnv) *exportEnv {
	x := &exportEnv{accountEnv: e, blobs: NewMemoryBlobStore(e.clock), outbox: &recordingOutbox{}}
	x.exports = appidentity.NewExports(appidentity.ExportDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Identity, Blobs: exportBlobs{MemoryBlobStore: x.blobs, x: x}, Outbox: x.outbox,
		Accounts: e.accounts,
		Entries: []appidentity.ArchiveEntry{{Name: "account.txt", Write: func(ctx context.Context, accountID string, w io.Writer) error {
			if x.fail {
				return errors.New("the entry fails")
			}
			if x.write != nil {
				return x.write(ctx, w)
			}
			_, err := io.WriteString(w, "account "+accountID)
			return err
		}}},
		Clock: e.clock, IDs: e.ids, Logger: slog.New(slog.DiscardHandler),
	})
	return x
}

// buildHandler returns the handler of the build jobs.
func (x *exportEnv) buildHandler(t *testing.T) queue.Handler {
	t.Helper()
	reg := &queue.JobHandlerRegistry{}
	x.exports.RegisterJobs(reg, queue.NewScheduler(nil, x.clock, nil))
	h, _, ok := reg.Lookup(appidentity.JobBuildExport)
	if !ok {
		t.Fatal("no handler of the build")
	}
	return h
}

// archiveKeys returns the keys of the stored archives.
func (x *exportEnv) archiveKeys() []string {
	var keys []string
	for _, k := range x.blobs.Keys() {
		if strings.HasPrefix(k, "exports/") {
			keys = append(keys, k)
		}
	}
	return keys
}

// build runs the queued build jobs with attempt as their attempt number.
func (x *exportEnv) build(t *testing.T, attempt int) {
	t.Helper()
	reg := &queue.JobHandlerRegistry{}
	x.exports.RegisterJobs(reg, queue.NewScheduler(nil, x.clock, nil))
	h, _, ok := reg.Lookup(appidentity.JobBuildExport)
	if !ok {
		t.Fatal("no handler of the build")
	}
	for _, j := range x.outbox.take() {
		if r := h(t.Context(), queue.Job{Type: j.Type, Payload: j.Payload, Attempt: attempt}); !r.IsDone() {
			t.Fatalf("build attempt %d = %+v, want done", attempt, r)
		}
	}
}

func exportsBuildAndExpireArchives(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	x := newExportEnv(e)
	alice, bob := e.register(t, "alice"), e.register(t, "bob")

	req, err := x.exports.RequestExport(ctx, alice.User.ID)
	if err != nil {
		t.Fatalf("RequestExport = %v", err)
	}
	if req.State(e.clock.Now()) != identity.ExportStatePending {
		t.Errorf("new export = %+v, want pending", req)
	}
	if _, err := x.exports.RequestExport(ctx, alice.User.ID); !errors.Is(err, appidentity.ErrExportInProgress) {
		t.Errorf("RequestExport while pending = %v, want ErrExportInProgress", err)
	}
	if _, _, err := x.exports.OpenExport(ctx, alice.User.ID, req.ID); !errors.Is(err, appidentity.ErrExportNotReady) {
		t.Errorf("OpenExport(pending) = %v, want ErrExportNotReady", err)
	}
	if jobs := x.outbox.jobs; len(jobs) != 1 || jobs[0].Type != appidentity.JobBuildExport {
		t.Fatalf("queued jobs = %+v, want one build", jobs)
	}

	e.clock.Advance(time.Minute)
	x.build(t, 1)
	list, err := x.exports.ListExports(ctx, alice.User.ID)
	if err != nil || len(list) != 1 || list[0].Result == nil || list[0].State(e.clock.Now()) != identity.ExportStateReady {
		t.Fatalf("ListExports after the build = %+v, %v; want one ready export", list, err)
	}
	res := list[0].Result
	if !res.CompletedAt.Equal(e.clock.Now()) || !res.ExpiresAt.Equal(e.clock.Now().Add(appidentity.ExportTTL)) || res.FileSize <= 0 {
		t.Errorf("result = %+v", res)
	}
	// Running the build again keeps the first result.
	if err := x.exports.BuildExport(ctx, req.ID); err != nil {
		t.Errorf("BuildExport again = %v", err)
	}
	r, info, err := x.exports.OpenExport(ctx, alice.User.ID, req.ID)
	if err != nil {
		t.Fatalf("OpenExport = %v", err)
	}
	data, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil || int64(len(data)) != info.Size || info.Size != res.FileSize {
		t.Fatalf("archive = %d bytes, %v; want %d", len(data), err, res.FileSize)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(zr.File) != 1 || zr.File[0].Name != "account.txt" {
		t.Fatalf("archive entries = %+v, %v; want account.txt", zr, err)
	}
	f, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(f)
	if string(body) != "account "+alice.AccountID {
		t.Errorf("account.txt = %q", body)
	}
	if _, _, err := x.exports.OpenExport(ctx, bob.User.ID, req.ID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("OpenExport(another user) = %v, want ErrNotFound", err)
	}

	// A failed build is retried, and recorded as failed on its last attempt.
	failed, err := x.exports.RequestExport(ctx, bob.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	x.fail = true
	reg := &queue.JobHandlerRegistry{}
	x.exports.RegisterJobs(reg, queue.NewScheduler(nil, e.clock, nil))
	h, _, _ := reg.Lookup(appidentity.JobBuildExport)
	job := x.outbox.take()[0]
	payload, _ := json.Marshal(appidentity.ExportJob{ExportID: failed.ID})
	if string(job.Payload) != string(payload) {
		t.Errorf("payload = %s, want %s", job.Payload, payload)
	}
	if r := h(ctx, queue.Job{Type: job.Type, Payload: job.Payload, Attempt: 1}); !r.IsRetry() {
		t.Errorf("first failed attempt = %+v, want a retry", r)
	}
	if r := h(ctx, queue.Job{Type: job.Type, Payload: job.Payload, Attempt: appidentity.ExportBuildAttempts}); !r.IsDone() {
		t.Errorf("last failed attempt = %+v, want done", r)
	}
	got, err := x.h.Identity.FindDataExport(ctx, failed.ID)
	if err != nil || got.State(e.clock.Now()) != identity.ExportStateFailed || got.Result.BlobKey != "" {
		t.Errorf("failed export = %+v, %v; want failed without an archive", got, err)
	}
	// After a failure, the user may ask again.
	if _, err := x.exports.RequestExport(ctx, bob.User.ID); err != nil {
		t.Errorf("RequestExport after a failure = %v", err)
	}

	// Past the expiry the archive cannot be downloaded, and the cleanup removes it.
	e.clock.Advance(appidentity.ExportTTL)
	if _, _, err := x.exports.OpenExport(ctx, alice.User.ID, req.ID); !errors.Is(err, appidentity.ErrExportNotReady) {
		t.Errorf("OpenExport(expired) = %v, want ErrExportNotReady", err)
	}
	if n, err := x.exports.ExpireExports(ctx); err != nil || n != 2 {
		t.Errorf("ExpireExports = %d, %v; want 2", n, err)
	}
	if _, err := x.h.Identity.FindDataExport(ctx, req.ID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindDataExport(expired) = %v, want ErrNotFound", err)
	}
	if _, err := x.blobs.Head(ctx, res.BlobKey); !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("archive after the expiry: %v, want ErrNotFound", err)
	}
	if n, err := x.exports.ExpireExports(ctx); err != nil || n != 0 {
		t.Errorf("ExpireExports again = %d, %v; want 0", n, err)
	}
}

func exportsAreErasedWithTheAccount(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	x := newExportEnv(e)
	alice, bob := e.register(t, "alice"), e.register(t, "bob")
	var keys []string
	for range 2 {
		ex, err := x.exports.RequestExport(ctx, alice.User.ID)
		if err != nil {
			t.Fatal(err)
		}
		x.build(t, 1)
		got, err := x.h.Identity.FindDataExport(ctx, ex.ID)
		if err != nil || got.Result == nil {
			t.Fatalf("FindDataExport = %+v, %v", got, err)
		}
		keys = append(keys, got.Result.BlobKey)
	}
	kept, err := x.exports.RequestExport(ctx, bob.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	target := appaccount.ErasureTarget{AccountID: alice.AccountID, UserID: alice.User.ID}
	step, err := x.exports.EraseAccountData(ctx, target, "", 1)
	if err != nil || step.Done {
		t.Fatalf("EraseAccountData(limit 1) = %+v, %v; want not done", step, err)
	}
	if step, err = x.exports.EraseAccountData(ctx, target, step.Next, 10); err != nil || !step.Done {
		t.Fatalf("EraseAccountData = %+v, %v; want done", step, err)
	}
	if list, err := x.exports.ListExports(ctx, alice.User.ID); err != nil || len(list) != 0 {
		t.Errorf("exports after the erasure = %+v, %v; want none", list, err)
	}
	for _, k := range keys {
		if _, err := x.blobs.Head(ctx, k); !errors.Is(err, blob.ErrNotFound) {
			t.Errorf("archive %s after the erasure: %v, want ErrNotFound", k, err)
		}
	}
	if _, err := x.h.Identity.FindDataExport(ctx, kept.ID); err != nil {
		t.Errorf("another user's export after the erasure: %v", err)
	}
}
