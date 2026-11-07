// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The cases of AccountRepositoryContract about the CSV imports (FR-008): an import records its
// items with its first job, the jobs carry them out in batches, an overwrite removes what the
// file does not list, and old imports are removed with their items and results.

// fakeImportTargets keeps what each account has of each kind in memory. Addresses of the form
// "missing@..." do not resolve, and "flaky@..." fails with an error worth retrying.
type fakeImportTargets struct {
	mu   sync.Mutex
	has  map[string][]appidentity.ImportTarget // by account and kind
	adds []identity.DataImportItem
	// transient are the targets that fail with an error worth retrying, and how many more times.
	transient map[string]int
	// onAdd, when set, runs on each add of a target that resolves.
	onAdd func(identity.DataImportItem)
}

func (f *fakeImportTargets) slot(accountID string, kind identity.ImportKind) string {
	return accountID + "/" + string(kind)
}

func (f *fakeImportTargets) set(accountID string, kind identity.ImportKind, targets ...appidentity.ImportTarget) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.has == nil {
		f.has = map[string][]appidentity.ImportTarget{}
	}
	f.has[f.slot(accountID, kind)] = targets
}

func (f *fakeImportTargets) List(_ context.Context, accountID string, kind identity.ImportKind) ([]appidentity.ImportTarget, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]appidentity.ImportTarget(nil), f.has[f.slot(accountID, kind)]...), nil
}

func (f *fakeImportTargets) Add(_ context.Context, accountID string, kind identity.ImportKind, item identity.DataImportItem) error {
	switch {
	case strings.HasPrefix(item.Target, "missing@"):
		return &appidentity.ImportItemError{Failure: identity.ImportFailureUnresolvable, Err: errors.New("no such account")}
	case strings.HasPrefix(item.Target, "flaky@"):
		return errors.New("the database is busy")
	}
	f.mu.Lock()
	if f.transient[item.Target] > 0 {
		f.transient[item.Target]--
		f.mu.Unlock()
		return errors.New("the remote server is busy")
	}
	f.adds = append(f.adds, item)
	onAdd := f.onAdd
	f.mu.Unlock()
	if onAdd != nil {
		onAdd(item)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	slot := f.slot(accountID, kind)
	for _, t := range f.has[slot] {
		if strings.EqualFold(t.Row.Target, item.Target) {
			return nil
		}
	}
	if f.has == nil {
		f.has = map[string][]appidentity.ImportTarget{}
	}
	f.has[slot] = append(f.has[slot], appidentity.ImportTarget{ID: NamedID("id-" + item.Target), Row: identity.ExportRow{Target: item.Target}})
	return nil
}

func (f *fakeImportTargets) Remove(_ context.Context, accountID string, kind identity.ImportKind, targetID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	slot := f.slot(accountID, kind)
	kept := f.has[slot][:0:0]
	for _, t := range f.has[slot] {
		if t.ID != targetID {
			kept = append(kept, t)
		}
	}
	f.has[slot] = kept
	return nil
}

// importEnv runs the imports on an accountEnv with fakeImportTargets.
type importEnv struct {
	*accountEnv
	outbox  *recordingOutbox
	targets *fakeImportTargets
	imports *appidentity.Imports
	// retries counts the delayed jobs that run has run.
	retries int
}

func newImportEnv(e *accountEnv) *importEnv {
	x := &importEnv{accountEnv: e, outbox: &recordingOutbox{}, targets: &fakeImportTargets{}}
	x.imports = appidentity.NewImports(appidentity.ImportDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Identity, Outbox: x.outbox, Targets: x.targets,
		Host: "my.example", Clock: e.clock, IDs: e.ids, Logger: slog.New(slog.DiscardHandler),
	})
	return x
}

// run runs the queued jobs of the imports, and the jobs they enqueue, without waiting for their
// delays, and returns the results of the last round. A job that asks for a retry runs again
// with the next attempt number, as the queue runs it, up to the queue's default attempts.
func (x *importEnv) run(t *testing.T) []queue.Result {
	t.Helper()
	reg := &queue.JobHandlerRegistry{}
	x.imports.RegisterJobs(reg, queue.NewScheduler(nil, x.clock, nil))
	h, _, ok := reg.Lookup(appidentity.JobProcessImport)
	if !ok {
		t.Fatal("no handler of the import jobs")
	}
	type queued struct {
		job     queue.NewJob
		attempt int
	}
	var pending []queued
	var results []queue.Result
	for round := 0; round <= 100; round++ {
		for _, j := range x.outbox.take() {
			if !j.RunAt.IsZero() {
				if !j.RunAt.After(x.clock.Now()) {
					t.Errorf("job %s runs at %v, want a delay", j.IdempotencyKey, j.RunAt)
				}
				x.retries++
			}
			pending = append(pending, queued{job: j, attempt: 1})
		}
		if len(pending) == 0 {
			return results
		}
		results = results[:0]
		jobs := pending
		pending = nil
		for _, q := range jobs {
			r := h(t.Context(), queue.Job{Type: q.job.Type, Payload: q.job.Payload, Attempt: q.attempt})
			if r.IsRetry() && q.attempt < queue.DefaultMaxAttempts {
				pending = append(pending, queued{job: q.job, attempt: q.attempt + 1})
			}
			results = append(results, r)
		}
	}
	t.Fatal("the import jobs did not end")
	return nil
}

func importsCarryOutCSVFiles(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	x := newImportEnv(e)
	alice, bob := e.register(t, "alice"), e.register(t, "bob")
	var csv strings.Builder
	csv.WriteString("Account address,Show boosts,Notify on new posts,Languages\n")
	for i := range 25 {
		fmt.Fprintf(&csv, "user%d@remote.example,true,false,\n", i)
	}
	csv.WriteString("missing@remote.example,false,true,\nnot an address\n")
	in := appidentity.ImportInput{
		UserID: alice.User.ID, AccountID: alice.AccountID, Kind: identity.ImportFollowing, Mode: identity.ImportMerge, Text: csv.String(),
	}

	p, err := x.imports.PreviewImport(ctx, in)
	if err != nil || len(p.Entries) != 26 || len(p.Skipped) != 1 || p.Present != 0 || len(p.Removals) != 0 {
		t.Fatalf("PreviewImport = %+v, %v; want 26 entries and 1 skipped line", p, err)
	}
	started, err := x.imports.StartImport(ctx, in)
	if err != nil || started.Total != 26 || started.Finished() {
		t.Fatalf("StartImport = %+v, %v; want 26 items to do", started, err)
	}
	if _, err := x.imports.StartImport(ctx, in); !errors.Is(err, appidentity.ErrImportInProgress) {
		t.Errorf("StartImport while in progress = %v, want ErrImportInProgress", err)
	}
	if jobs := x.outbox.jobs; len(jobs) != 1 || jobs[0].Type != appidentity.JobProcessImport || jobs[0].IdempotencyKey == "" {
		t.Fatalf("queued jobs = %+v, want one keyed job", jobs)
	}
	for _, r := range x.run(t) {
		if !r.IsDone() {
			t.Errorf("job = %+v, want done", r)
		}
	}
	list, err := x.imports.ListImports(ctx, alice.User.ID)
	if err != nil || len(list) != 1 || list[0].ID != started.ID || list[0].Total != 26 || list[0].Processed != 26 || list[0].Failed != 1 ||
		list[0].Kind != identity.ImportFollowing || list[0].Mode != identity.ImportMerge || !list[0].CreatedAt.Equal(e.clock.Now()) {
		t.Fatalf("ListImports = %+v, %v; want the finished import with one failure", list, err)
	}
	if len(x.targets.adds) != 25 || x.targets.adds[0].Target != "user0@remote.example" || !x.targets.adds[0].ShowBoosts || x.targets.adds[0].Notify {
		t.Errorf("adds = %d, first %+v; want the 25 resolvable entries with their settings", len(x.targets.adds), x.targets.adds)
	}
	// Another user sees none of it, and a finished import lets a new one start.
	if list, _ := x.imports.ListImports(ctx, bob.User.ID); len(list) != 0 {
		t.Errorf("bob's imports = %+v, want none", list)
	}

	// An overwrite removes what the file does not list, after adding what it does.
	in.Text, in.Mode = "user0@remote.example\nUSER1@Remote.Example\nnew@remote.example\n", identity.ImportOverwrite
	p, err = x.imports.PreviewImport(ctx, in)
	if err != nil || len(p.Entries) != 3 || p.Present != 2 || len(p.Removals) != 23 {
		t.Fatalf("PreviewImport(overwrite) = %d entries, %d present, %d removals, %v; want 3, 2, 23", len(p.Entries), p.Present, len(p.Removals), err)
	}
	over, err := x.imports.StartImport(ctx, in)
	if err != nil || over.Total != 26 {
		t.Fatalf("StartImport(overwrite) = %+v, %v; want 3 adds and 23 removals", over, err)
	}
	x.run(t)
	has, _ := x.targets.List(ctx, alice.AccountID, identity.ImportFollowing)
	var got []string
	for _, h := range has {
		got = append(got, h.Row.Target)
	}
	if strings.Join(got, " ") != "user0@remote.example user1@remote.example new@remote.example" {
		t.Errorf("following after the overwrite = %v", got)
	}

	// An error worth retrying stops the batch, and a delayed job retries it; after
	// ImportItemAttempts failures the item is recorded as failed.
	in = appidentity.ImportInput{UserID: bob.User.ID, AccountID: bob.AccountID, Kind: identity.ImportBlocks, Mode: identity.ImportMerge,
		Text: "flaky@remote.example\nok@remote.example\n"}
	flaky, err := x.imports.StartImport(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := x.imports.ProcessImport(ctx, flaky.ID); err != nil {
		t.Fatalf("first run = %v", err)
	}
	if jobs := x.outbox.jobs; len(jobs) != 2 || !jobs[1].RunAt.After(e.clock.Now()) {
		t.Fatalf("jobs after the first run = %+v, want the first job and a delayed retry", jobs)
	}
	if got, _ := e.h.Identity.FindDataImport(ctx, flaky.ID); got.Processed != 0 {
		t.Errorf("after a retry = %+v, want nothing processed", got)
	}
	x.run(t)
	if got, err := e.h.Identity.FindDataImport(ctx, flaky.ID); err != nil || !got.Finished() || got.Failed != 1 || got.Processed != 2 {
		t.Errorf("after the last attempt = %+v, %v; want finished with one failure", got, err)
	}

	// An empty file finishes at once, with no job.
	in.Text = ""
	if empty, err := x.imports.StartImport(ctx, in); err != nil || empty.Total != 0 || !empty.Finished() || len(x.outbox.jobs) != 0 {
		t.Errorf("StartImport(empty) = %+v, %v, jobs %d; want a finished import and no job", empty, err, len(x.outbox.jobs))
	}

	// The export writes what the account has, as Mastodon does.
	x.targets.set(bob.AccountID, identity.ImportMutes, appidentity.ImportTarget{ID: NamedID("1"), Row: identity.ExportRow{Target: "noisy@remote.example", HideNotifications: true}})
	var b bytes.Buffer
	if err := x.imports.WriteExport(ctx, bob.AccountID, identity.ImportMutes, &b); err != nil ||
		b.String() != "Account address,Hide notifications\nnoisy@remote.example,true\n" {
		t.Errorf("WriteExport = %q, %v", b.String(), err)
	}

	// Imports go after ImportTTL.
	e.clock.Advance(appidentity.ImportTTL + time.Second)
	if n, err := x.imports.ExpireImports(ctx); err != nil || n != 4 {
		t.Errorf("ExpireImports = %d, %v; want 4", n, err)
	}
	if _, err := e.h.Identity.FindDataImport(ctx, started.ID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindDataImport(expired) = %v, want ErrNotFound", err)
	}
	if pending, err := e.h.Identity.ListPendingDataImportItems(ctx, started.ID, 10); err != nil || len(pending) != 0 {
		t.Errorf("items of an expired import = %+v, %v; want none", pending, err)
	}
	// A job of a removed import does nothing.
	if err := x.imports.ProcessImport(ctx, started.ID); err != nil {
		t.Errorf("ProcessImport(removed) = %v", err)
	}
}

func importsAreErasedWithTheAccount(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	x := newImportEnv(e)
	alice, bob := e.register(t, "alice"), e.register(t, "bob")
	for range 2 {
		if _, err := x.imports.StartImport(ctx, appidentity.ImportInput{
			UserID: alice.User.ID, AccountID: alice.AccountID, Kind: identity.ImportBookmarks, Mode: identity.ImportMerge,
			Text: "https://remote.example/notes/1\n",
		}); err != nil {
			t.Fatal(err)
		}
		x.run(t)
	}
	kept, err := x.imports.StartImport(ctx, appidentity.ImportInput{
		UserID: bob.User.ID, AccountID: bob.AccountID, Kind: identity.ImportBlocks, Mode: identity.ImportMerge, Text: "a@remote.example\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	target := appaccount.ErasureTarget{AccountID: alice.AccountID, UserID: alice.User.ID}
	step, err := x.imports.EraseAccountData(ctx, target, "", 1)
	if err != nil || step.Done {
		t.Fatalf("EraseAccountData(limit 1) = %+v, %v; want not done", step, err)
	}
	if step, err = x.imports.EraseAccountData(ctx, target, step.Next, 10); err != nil || !step.Done {
		t.Fatalf("EraseAccountData = %+v, %v; want done", step, err)
	}
	if list, err := x.imports.ListImports(ctx, alice.User.ID); err != nil || len(list) != 0 {
		t.Errorf("imports after the erasure = %+v, %v; want none", list, err)
	}
	if _, err := e.h.Identity.FindDataImport(ctx, kept.ID); err != nil {
		t.Errorf("another user's import after the erasure: %v", err)
	}
}
