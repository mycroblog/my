// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// member registers name as a local user without a role, who signs in to the client API.
func (e *eventBoardEnv) member(name string) {
	e.t.Helper()
	r, err := e.app.Identity.RegisterLocalUser(e.t.Context(), identity.RegisterInput{Username: name, Password: "correct horse battery staple", DisplayName: name})
	if err != nil {
		e.t.Fatalf("RegisterLocalUser(%s): %v", name, err)
	}
	e.ids[name], e.userIDs[name] = r.AccountID, r.User.ID
	e.tokens[name] = issueTokens(e.t, e.app, r.User.ID, "").AccessToken
}

// TestEventBoardPostsAreModeratedAsAnyPost checks ADR-0161 §14 and §17 through the API: a reader
// reports a post of a board, and a user who does not read the board cannot cite it; the report is
// never forwarded; a moderator sees the post in the report and deletes it with delete_statuses,
// which resolves the report, is in the audit log, and announces the deletion as that of a board
// post only, so that no Delete and no job of federation goes out; a suspension of the writer
// hides the writer's other posts on the board.
func TestEventBoardPostsAreModeratedAsAnyPost(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice", "mona", "sam")
	e.member("wren")
	ctx := t.Context()
	events := e.recordEvents()
	ev := e.createEvent("readers", nil)
	e.answer("mona", ev.Id, gathering.AnswerGoing)
	e.answer("wren", ev.Id, gathering.AnswerGoing)
	post := e.mustPostOnBoard("wren", ev.Id, map[string]any{"text": "boardspamneedle buy now"})
	kept := e.mustPostOnBoard("wren", ev.Id, map[string]any{"text": "see you all"})
	e.drain()

	// A user who does not read the board cannot cite its post, as for a post they may not see.
	if res := e.send("sam", http.MethodPost, "/reports", map[string]any{
		"account_id": e.ids["wren"], "status_ids": []string{post.Id}, "category": "spam",
	}, nil, nil); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("a report citing a board post by a user who does not read the board = %s, want 422", e.outcome(res))
	}
	// A reader reports it; the report is not forwarded, even when asked.
	var report clientapi.Report
	if res := e.send("mona", http.MethodPost, "/reports", map[string]any{
		"account_id": e.ids["wren"], "status_ids": []string{post.Id}, "category": "spam", "comment": "spam on the board", "forward": true,
	}, nil, &report); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /reports citing a board post = %s, want 201", e.outcome(res))
	}
	if report.Forwarded || !slices.Equal(report.StatusIds, []string{post.Id}) || report.TargetAccount.Id != e.ids["wren"] {
		t.Errorf("the report = %+v, want wren's board post, not forwarded", report)
	}

	// The moderator sees the post in the report, and cannot forward it.
	var detail clientapi.AdminReportDetail
	if res := e.send("alice", http.MethodGet, "/admin/reports/"+report.Id, nil, nil, &detail); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/reports/%s = %s", report.Id, e.outcome(res))
	}
	if detail.Forwarded || detail.Forwardable || !slices.Equal(detail.StatusIds, []string{post.Id}) || len(detail.Statuses) != 1 ||
		detail.Statuses[0].Status.Id != post.Id || !strings.Contains(detail.Statuses[0].Status.Content, "boardspamneedle") {
		t.Errorf("the report as the moderator sees it = %+v, want the board post, neither forwarded nor forwardable", detail)
	}
	if res := e.send("alice", http.MethodPost, "/admin/reports/"+report.Id+"/forward", nil, nil, nil); res.StatusCode < 400 {
		t.Errorf("forward the report of a board post = %d, want a refusal", res.StatusCode)
	}

	// delete_statuses deletes the post for everyone, its writer included.
	seen := map[string]bool{}
	for _, j := range e.pendingJobs() {
		seen[j.ID] = true
	}
	var action clientapi.AdminAccountAction
	if res := e.send("alice", http.MethodPost, "/admin/accounts/"+e.ids["wren"]+"/actions", map[string]any{
		"kind": "delete_statuses", "status_ids": []string{post.Id}, "report_id": report.Id,
	}, nil, &action); res.StatusCode != http.StatusCreated {
		t.Fatalf("delete_statuses of the board post = %s, want 201", e.outcome(res))
	}
	for _, j := range e.pendingJobs() {
		if seen[j.ID] {
			continue
		}
		if j.Type == federation.JobFanOut || j.Type == federation.JobDeliver ||
			(j.Type != appcontent.JobFetchLinkCard && (strings.Contains(string(j.Payload), post.Id) || strings.Contains(j.IdempotencyKey, post.Id)) &&
				!strings.HasPrefix(j.Type, "notification.")) {
			t.Errorf("the deletion of the board post enqueues the job %s %s %s", j.Type, j.IdempotencyKey, j.Payload)
		}
	}
	e.drain()
	for _, user := range []string{"alice", "mona", "wren"} {
		if res := e.send(user, http.MethodGet, "/statuses/"+post.Id, nil, nil, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET the deleted board post as %s = %d, want 404", user, res.StatusCode)
		}
	}
	if got := e.boardIDs("mona", ev.Id); !slices.Equal(got, []string{kept.Id}) {
		t.Errorf("the board after the deletion = %v, want %v", got, []string{kept.Id})
	}
	// The deletion is that of a board post: no event of an ordinary status names it.
	for _, typ := range []string{appcontent.EventStatusDeleted, appcontent.EventStatusesDeleted} {
		if slices.Contains(events.named(typ), post.Id) {
			t.Errorf("%s names the board post that a moderator deleted", typ)
		}
	}
	if !slices.Contains(events.named(appcontent.EventBoardStatusesDeleted), post.Id) {
		t.Errorf("%s names %v, without the board post that a moderator deleted", appcontent.EventBoardStatusesDeleted, events.named(appcontent.EventBoardStatusesDeleted))
	}

	// The report is resolved as actioned, and still shows the post to the moderators.
	detail = clientapi.AdminReportDetail{}
	if res := e.send("alice", http.MethodGet, "/admin/reports/"+report.Id, nil, nil, &detail); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/reports/%s = %s", report.Id, e.outcome(res))
	}
	if detail.State != clientapi.AdminReportStateResolved || detail.Outcome == nil || *detail.Outcome != clientapi.AdminReportOutcomeActioned ||
		len(detail.Statuses) != 1 || detail.Statuses[0].Status.Id != post.Id || detail.Statuses[0].DeletedAt == nil {
		t.Errorf("the report after the action = %+v, want it actioned with the deleted board post", detail)
	}
	// The audit log has the action.
	audit, err := e.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetAccount, TargetID: e.ids["wren"]}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(audit, func(a moderation.AuditEntry) bool {
		return a.Action == moderation.AuditAccountActionIssued && a.ActorKind == moderation.AuditActorUser && a.ActorUserID == e.userIDs["alice"] && a.SourceEventID == action.Id
	}) {
		t.Errorf("the audit log of wren = %+v, want the account_action_issued %s by alice", audit, action.Id)
	}

	// A suspension of the writer hides the writer's posts on the board from the other readers, as
	// it hides every other post.
	if res := e.send("alice", http.MethodPost, "/admin/accounts/"+e.ids["wren"]+"/actions", map[string]any{"kind": "suspend"}, nil, nil); res.StatusCode != http.StatusCreated {
		t.Fatalf("suspend the writer = %s, want 201", e.outcome(res))
	}
	e.drain()
	if res := e.send("mona", http.MethodGet, "/statuses/"+kept.Id, nil, nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET a board post of the suspended writer as a reader = %d, want 404", res.StatusCode)
	}
	if got := e.boardIDs("mona", ev.Id); slices.Contains(got, kept.Id) {
		t.Errorf("the board as a reader after the suspension of the writer = %v, want it without %s", got, kept.Id)
	}
}
