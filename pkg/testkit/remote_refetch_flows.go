// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// remoteRefetches checks the refetch of stored remote posts (FR-407, federation.md §6.4): a
// refetch records a newer revision and the counts of a poll as a received Update does, deletes a
// post that is gone, and leaves a post that cannot be fetched or that names another author; the
// requests for a post, an account, and a domain are audited and walk their posts within bounds.
func remoteRefetches(t *testing.T, e *remoteStatusEnv) {
	ctx := t.Context()
	const alice, bob = "https://remote.example/users/alice", "https://remote.example/users/bob"
	e.follower(t, alice)
	e.follower(t, bob)
	published := remoteNow.Add(-2 * time.Hour)
	note := func(id, author string, fields map[string]any) map[string]any {
		return remoteNote(id, author, published, fields)
	}
	create := func(id, author string) string {
		t.Helper()
		e.expect(t, remoteActivity(t, "Create", id+"/activity", author, note(id, author, nil)), appfederation.InboxHandled)
		return e.post(t, id).Status.ID
	}
	edited, gone, tomb, foreign, failing := alice+"/notes/edited", alice+"/notes/gone", alice+"/notes/tomb", alice+"/notes/foreign", alice+"/notes/failing"
	editedID, goneID, tombID, foreignID, failingID := create(edited, alice), create(gone, alice), create(tomb, alice), create(foreign, alice), create(failing, alice)

	// The same document makes no revision; a newer one does.
	e.fetcher.serve(t, edited, note(edited, alice, nil))
	if err := e.statuses.Refetch(ctx, editedID); err != nil || e.post(t, edited).Revision.Seq != 1 {
		t.Errorf("Refetch of an unchanged post = %v, seq %d; want no new revision", err, e.post(t, edited).Revision.Seq)
	}
	e.fetcher.serve(t, edited, note(edited, alice, map[string]any{
		"content": "<p>edited</p>", "updated": published.Add(time.Hour).Format(time.RFC3339),
	}))
	if err := e.statuses.Refetch(ctx, editedID); err != nil {
		t.Fatalf("Refetch(edited) = %v", err)
	}
	if p := e.post(t, edited); p.Revision.Seq != 2 || p.Revision.Bodies[0].ContentHTML != "<p>edited</p>" {
		t.Errorf("refetched post = %+v, want its second revision", p.Revision)
	}

	// 410 Gone and a Tombstone delete the post.
	e.fetcher.errs[gone] = fmt.Errorf("%w: %s", appfederation.ErrFetchGone, gone)
	e.fetcher.serve(t, tomb, map[string]any{"id": tomb, "type": "Tombstone"})
	for _, uri := range []string{gone, tomb} {
		id := map[string]string{gone: goneID, tomb: tombID}[uri]
		if err := e.statuses.Refetch(ctx, id); err != nil {
			t.Errorf("Refetch(%s) = %v", uri, err)
		}
		if deleted, err := e.content.Tombstoned(ctx, uri); err != nil || !deleted {
			t.Errorf("Tombstoned(%s) = %v, %v; want the refetch to delete it", uri, deleted, err)
		}
		if err := e.statuses.Refetch(ctx, id); err != nil {
			t.Errorf("Refetch of the deleted %s = %v, want nothing to do", uri, err)
		}
	}

	// Another author and a failed fetch leave the post as it is.
	e.fetcher.serve(t, foreign, note(foreign, bob, map[string]any{
		"content": "<p>taken</p>", "updated": published.Add(time.Hour).Format(time.RFC3339),
	}))
	if err := e.statuses.Refetch(ctx, foreignID); !errors.Is(err, appfederation.ErrNotTaken) || e.post(t, foreign).Revision.Seq != 1 {
		t.Errorf("Refetch of a post attributed to another account = %v, seq %d; want ErrNotTaken and no revision", err, e.post(t, foreign).Revision.Seq)
	}
	e.fetcher.errs[failing] = fmt.Errorf("%w: %s", appfederation.ErrFetchFailed, failing)
	if err := e.statuses.Refetch(ctx, failingID); !errors.Is(err, appfederation.ErrFetchFailed) || !e.stored(t, failing) {
		t.Errorf("Refetch of a post that cannot be fetched = %v; want ErrFetchFailed and the post kept", err)
	}

	// The requests: audited, and walking the posts of an account or a domain.
	var (
		mu      sync.Mutex
		audited []appmoderation.AuditEntry
	)
	e.projectors.Register(appmoderation.EventAuditRecorded, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
		mu.Lock()
		defer mu.Unlock()
		audited = append(audited, ev.(appmoderation.AuditRecorded).Entry)
		return nil
	})
	jobs := &remoteJobs{seen: map[string]bool{}}
	handlers := &queue.JobHandlerRegistry{}
	refetches := appfederation.NewRefetches(appfederation.RefetchesDeps{
		UnitOfWork: e.h.UnitOfWork, Statuses: e.content, Accounts: e.remote,
		Outbox: queue.AfterCommit(e.projectors, jobs, nil), Jobs: jobs, Clock: e.clock, IDs: e.ids, Host: contractHost,
	})
	refetches.RegisterJobs(handlers, e.statuses)
	walk := func() map[string]bool {
		t.Helper()
		h, _, _ := handlers.Lookup(appfederation.JobRefetchStatuses)
		for {
			pending := jobs.take(appfederation.JobRefetchStatuses)
			if len(pending) == 0 {
				break
			}
			for _, j := range pending {
				if res := h(ctx, queue.Job{Type: j.Type, Payload: j.Payload, Attempt: 1}); !res.IsDone() {
					t.Fatalf("%s %s: %v", j.Type, j.Payload, res.Err())
				}
			}
		}
		out := map[string]bool{}
		for _, j := range jobs.take(appfederation.JobRefetchStatus) {
			out[string(j.Payload)] = true
		}
		return out
	}
	payload := func(id string) string { return `{"status_id":"` + id + `"}` }

	if err := refetches.RefetchStatus(ctx, editedID, ""); err != nil {
		t.Fatalf("RefetchStatus = %v", err)
	}
	if got := walk(); len(got) != 1 || !got[payload(editedID)] {
		t.Errorf("RefetchStatus enqueued %v, want the refetch of %s", got, editedID)
	}
	for name, err := range map[string]error{
		"RefetchStatus(deleted)": refetches.RefetchStatus(ctx, goneID, ""),
		"RefetchStatus(unknown)": refetches.RefetchStatus(ctx, e.ids.NewID(), ""),
		"RefetchAccount(local)":  refetches.RefetchAccount(ctx, e.user(t, "local"), ""),
		"RefetchAccount(none)":   refetches.RefetchAccount(ctx, e.ids.NewID(), ""),
		"RefetchDomain(local)":   second(refetches.RefetchDomain(ctx, contractHost, "")),
		"RefetchDomain(invalid)": second(refetches.RefetchDomain(ctx, "not a host", "")),
	} {
		if !errors.Is(err, appfederation.ErrNotFound) && !errors.Is(err, appfederation.ErrNotRemote) && !errors.Is(err, appfederation.ErrInvalidHost) {
			t.Errorf("%s = %v, want a refusal", name, err)
		}
	}
	bobID := create(bob+"/notes/1", bob)
	aliceID, err := e.remote.FindByURI(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if err := refetches.RefetchAccount(ctx, aliceID.Account.ID, ""); err != nil {
		t.Fatalf("RefetchAccount = %v", err)
	}
	want := map[string]bool{payload(editedID): true, payload(tombID): false, payload(foreignID): true, payload(failingID): true}
	got := walk()
	for p, in := range want {
		if got[p] != in {
			t.Errorf("RefetchAccount enqueued %v; %s in it = %v, want %v", got, p, got[p], in)
		}
	}
	if got[payload(bobID)] || len(got) != 3 {
		t.Errorf("RefetchAccount enqueued %v, want the 3 live posts of alice", got)
	}
	host, err := refetches.RefetchDomain(ctx, "Remote.Example", "")
	if err != nil || host != "remote.example" {
		t.Fatalf("RefetchDomain = %q, %v", host, err)
	}
	if got := walk(); len(got) != 4 || !got[payload(bobID)] || !got[payload(editedID)] {
		t.Errorf("RefetchDomain enqueued %v, want the 4 live posts of alice and bob", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(audited) != 3 {
		t.Fatalf("audit log = %+v, want the 3 requests", audited)
	}
	for i, w := range []struct {
		action appmoderation.AuditAction
		target string
	}{
		{appmoderation.AuditStatusRefetchQueued, editedID},
		{appmoderation.AuditAccountStatusesRefetchQueued, aliceID.Account.ID},
		{appmoderation.AuditDomainStatusesRefetchQueued, "remote.example"},
	} {
		if a := audited[i]; a.Action != w.action || a.TargetID != w.target || a.ActorKind != "cli" || a.Validate() != nil {
			t.Errorf("audit entry %d = %+v, want %s on %s by the CLI", i, a, w.action, w.target)
		}
	}
}
