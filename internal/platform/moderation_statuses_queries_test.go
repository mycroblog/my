// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// moderationStatusesQueries counts the round trips of a local report of n statuses of bob by
// alice, of an action that marks them sensitive, and of one that deletes them.
func moderationStatusesQueries(t *testing.T, n int) (report, mark, del int) {
	t.Helper()
	tracer := testkit.NewTracer()
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, "alice", "bob")
	ctx := t.Context()
	var ids []string
	for i := range n {
		ids = append(ids, c.post("bob", map[string]any{"text": fmt.Sprintf("post %d", i), "visibility": "public"}).Id)
	}
	was := ops(tracer)
	if _, err := c.app.LocalReports.File(ctx, appmoderation.LocalReportInput{
		ReporterAccountID: c.ids["alice"], TargetAccountID: c.ids["bob"], StatusIDs: ids, Category: moderation.CategorySpam,
	}); err != nil {
		t.Fatalf("File: %v", err)
	}
	report, names := opsSince(tracer, was)
	t.Logf("a report of %d statuses: %d queries: %v", n, report, names)
	issue := func(kind moderation.ActionKind) int {
		t.Helper()
		was := ops(tracer)
		if _, err := c.app.Moderation.IssueAccountAction(ctx, appmoderation.AccountActionInput{
			TargetAccountID: c.ids["bob"], Kind: kind, ReasonCategory: moderation.CategorySpam, StatusIDs: ids,
		}); err != nil {
			t.Fatalf("IssueAccountAction(%s): %v", kind, err)
		}
		count, names := opsSince(tracer, was)
		t.Logf("%s on %d statuses: %d queries: %v", kind, n, count, names)
		return count
	}
	mark = issue(moderation.ActionMarkStatusesSensitive)
	fanOuts := len(mustJobs(t, c, appfederation.JobFanOut))
	del = issue(moderation.ActionDeleteStatuses)
	if got := len(mustJobs(t, c, appfederation.JobFanOut)) - fanOuts; got != n {
		t.Errorf("the deletion enqueued %d fan-outs, want the Delete of each of the %d statuses", got, n)
	}
	if got, err := c.app.Content.PostCount(ctx, c.ids["bob"]); err != nil || got != 0 {
		t.Errorf("bob's posts after the deletion = %d, %v; want none", got, err)
	}
	for _, id := range ids {
		if _, err := c.app.Statuses.View(ctx, appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: c.ids["alice"]}, id); err == nil {
			t.Errorf("status %s after the action is still visible", id)
		}
	}
	return report, mark, del
}

// TestModerationStatusesQueries checks that the round trips of a report and of an action on
// statuses do not grow with the statuses they name: they are read, checked, recorded, and their
// deletions projected together.
func TestModerationStatusesQueries(t *testing.T) {
	t.Parallel()
	smallReport, smallMark, smallDel := moderationStatusesQueries(t, 2)
	largeReport, largeMark, largeDel := moderationStatusesQueries(t, 5)
	if largeReport > smallReport {
		t.Errorf("a report makes %d queries for 5 statuses, more than the %d for 2", largeReport, smallReport)
	}
	if largeMark > smallMark {
		t.Errorf("an action makes %d queries for 5 statuses, more than the %d for 2", largeMark, smallMark)
	}
	if largeDel > smallDel {
		t.Errorf("a deletion makes %d queries for 5 statuses, more than the %d for 2", largeDel, smallDel)
	}
}

// flagQueries counts the round trips of an inbound Flag that names bob and n of his statuses,
// and returns them with the statuses of the report it filed.
func flagQueries(t *testing.T, n int) (int, int) {
	t.Helper()
	tracer := testkit.NewTracer()
	network := &relayNet{docs: map[string][]byte{}}
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline"}, platform.AppOptions{Tracer: tracer, HTTPClient: network}, "bob")
	reporter := newRemoteActor(t, network, "https://remote.example/users/carol", "Person", "https://remote.example/users/carol/inbox")
	objects := []any{"https://example.test/ap/actors/" + c.ids["bob"]}
	for i := range n {
		st := c.post("bob", map[string]any{"text": fmt.Sprintf("post %d", i), "visibility": "public"})
		objects = append(objects, "https://example.test/ap/objects/"+st.Id)
	}
	// The reporter is known before the Flag.
	postInbox(t, c, reporter, map[string]any{"id": reporter.id + "#hello", "type": "Delete", "actor": reporter.id, "object": reporter.id + "/missing"})
	was := ops(tracer)
	postInbox(t, c, reporter, map[string]any{"id": reporter.id + "#flag", "type": "Flag", "actor": reporter.id, "object": objects, "content": "spam"})
	count, names := opsSince(tracer, was)
	t.Logf("a Flag of %d statuses: %d queries: %v", n, count, names)
	reports, err := c.app.Moderation.Reports(t.Context(), appmoderation.ReportQuery{}, 10)
	if err != nil || len(reports) != 1 {
		t.Fatalf("ListReports = %d, %v; want the Flag's report", len(reports), err)
	}
	return count, len(reports[0].Report.StatusIDs)
}

// TestFlagQueries checks that the round trips of an inbound Flag do not grow with the objects it
// names: the local accounts and statuses among them are read together.
func TestFlagQueries(t *testing.T) {
	t.Parallel()
	small, smallStatuses := flagQueries(t, 2)
	large, largeStatuses := flagQueries(t, 6)
	if smallStatuses != 2 || largeStatuses != 6 {
		t.Errorf("the Flags' reports have %d and %d statuses, want 2 and 6", smallStatuses, largeStatuses)
	}
	if large > small {
		t.Errorf("a Flag makes %d queries for 6 statuses, more than the %d for 2", large, small)
	}
}
