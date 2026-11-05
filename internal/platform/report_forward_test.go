// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// inboxRecorder is a kernel.HTTPClient that answers every POST with 202 and keeps it; every other
// request is a 404.
type inboxRecorder struct {
	mu    sync.Mutex
	posts []recordedPost
}

type recordedPost struct {
	url    string
	header http.Header
	body   []byte
}

func (r *inboxRecorder) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	status := http.StatusNotFound
	if req.Method == http.MethodPost {
		status = http.StatusAccepted
		r.mu.Lock()
		r.posts = append(r.posts, recordedPost{url: req.URL, header: req.Header.Clone(), body: slices.Clone(req.Body)})
		r.mu.Unlock()
	}
	return &kernel.HTTPResponse{StatusCode: status, Header: http.Header{}, URL: req.URL, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func (r *inboxRecorder) taken() []recordedPost {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.posts
	r.posts = nil
	return out
}

// TestReportForwarding files reports through the client API with the inline queue, whose jobs
// run before the response (FR-709, federation.md §11): a report of a remote account that asks
// to be forwarded reaches that account's inbox as a Flag signed by the instance actor, about the
// account and the reported status, with the comment and nothing of the reporter. A report that
// does not ask, and a report of a local account, send nothing.
func TestReportForwarding(t *testing.T) {
	t.Parallel()
	network := &inboxRecorder{}
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline"}, platform.AppOptions{HTTPClient: network}, "alice", "carol")
	ctx := t.Context()
	const bob = "https://remote.example/users/bob"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := c.app.Accounts.EnsureInstanceActor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var bobID, statusID string
	err = c.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		a, err := c.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: bob, Domain: "remote.example", Type: "Person", PreferredUsername: "bob",
			InboxURL: bob + "/inbox", SharedInboxURL: "https://remote.example/inbox",
			Keys:     []appaccount.RemoteKeyInput{{KeyID: bob + "#main-key", PublicKey: &key.PublicKey}},
			Document: []byte(`{"id":"` + bob + `"}`),
		})
		if err != nil {
			return err
		}
		bobID = a.ID
		now := time.Now().UTC()
		p, err := c.app.Content.IngestRemoteStatus(ctx, tx, appcontent.RemoteStatusInput{
			URI: bob + "/statuses/1", AuthorAccountID: bobID, AuthorDomain: "remote.example",
			Visibility: domcontent.VisibilityPublic, PublishedAt: now, RevisedAt: now,
			Bodies: []appcontent.RemoteBody{{Language: "en", HTML: "<p>buy now</p>"}},
		})
		statusID = p.Status.ID
		return err
	})
	if err != nil {
		t.Fatalf("remote actor and status: %v", err)
	}
	network.taken()

	file := func(target string, forward bool, statusIDs ...string) string {
		t.Helper()
		var r clientapi.Report
		res := c.do("alice", http.MethodPost, "/reports", map[string]any{
			"account_id": target, "status_ids": append([]string{}, statusIDs...), "category": "spam",
			"comment": "Spam, please look", "forward": forward,
		}, &r)
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("POST /reports = %d", res.StatusCode)
		}
		return r.Id
	}
	file(bobID, true, statusID)
	posts := network.taken()
	if len(posts) != 1 || posts[0].url != bob+"/inbox" {
		t.Fatalf("POSTs = %+v, want one to bob's own inbox", posts)
	}
	var flag map[string]any
	if err := json.Unmarshal(posts[0].body, &flag); err != nil {
		t.Fatal(err)
	}
	if flag["type"] != "Flag" || flag["actor"] != instance.URI || flag["content"] != "Spam, please look" {
		t.Errorf("Flag = %s, want the instance actor's with the comment", posts[0].body)
	}
	if got, _ := flag["object"].([]any); len(got) != 2 || got[0] != bob || got[1] != bob+"/statuses/1" {
		t.Errorf("Flag object = %v, want bob and his status", flag["object"])
	}
	for _, trace := range []string{c.ids["alice"], "alice", "@example.com", "127.0.0.1", "192.0.2."} {
		if strings.Contains(string(posts[0].body), trace) {
			t.Errorf("Flag %s carries %q of the reporter", posts[0].body, trace)
		}
	}
	if sig := posts[0].header.Get("Signature") + posts[0].header.Get("Signature-Input"); !strings.Contains(sig, instance.URI) {
		t.Errorf("signature = %q, want the instance actor's key", sig)
	}

	unsent := file(bobID, false, statusID)
	local := file(c.ids["carol"], true)
	if posts := network.taken(); len(posts) != 0 {
		t.Errorf("POSTs = %+v, want none for a report not forwarded or of a local account", posts)
	}

	// A moderator forwards the report that was not forwarded: the same Flag, once, and an entry
	// in the audit log. Forwarding it again sends nothing more.
	var detail clientapi.AdminReportDetail
	if res := c.do("alice", http.MethodGet, "/admin/reports/"+unsent, nil, &detail); res.StatusCode != http.StatusOK || detail.Forwarded || !detail.Forwardable {
		t.Fatalf("GET the report = %d, forwarded %v, forwardable %v", res.StatusCode, detail.Forwarded, detail.Forwardable)
	}
	for range 2 {
		detail = clientapi.AdminReportDetail{}
		if res := c.do("alice", http.MethodPost, "/admin/reports/"+unsent+"/forward", nil, &detail); res.StatusCode != http.StatusOK || !detail.Forwarded || detail.Forwardable {
			t.Errorf("forward = %d, forwarded %v, forwardable %v", res.StatusCode, detail.Forwarded, detail.Forwardable)
		}
	}
	posts = network.taken()
	if len(posts) != 1 || posts[0].url != bob+"/inbox" {
		t.Fatalf("POSTs after the forward = %+v, want one to bob's own inbox", posts)
	}
	flag = nil
	if err := json.Unmarshal(posts[0].body, &flag); err != nil {
		t.Fatal(err)
	}
	if got, _ := flag["object"].([]any); flag["type"] != "Flag" || flag["actor"] != instance.URI || len(got) != 2 || got[1] != bob+"/statuses/1" {
		t.Errorf("Flag of the forward = %s", posts[0].body)
	}
	audit, err := c.app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetReport, TargetID: unsent}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 1 || audit[0].Action != moderation.AuditReportForwarded || audit[0].ActorKind != moderation.AuditActorUser {
		t.Errorf("audit of the report = %+v, want one report_forwarded by a user", audit)
	}
	// A report of a local account cannot be forwarded.
	if res := c.do("alice", http.MethodPost, "/admin/reports/"+local+"/forward", nil, nil); res.StatusCode != http.StatusConflict {
		t.Errorf("forward of a report of a local account = %d, want 409", res.StatusCode)
	}
}
