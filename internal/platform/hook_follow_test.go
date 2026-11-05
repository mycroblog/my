// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	apphook "github.com/mycroblog/my/internal/app/hook"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/hook"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// followPlugin answers the verdict a test sets, or 503 when it is empty. It counts its calls and
// keeps the last request.
type followPlugin struct {
	verdict atomic.Value
	calls   atomic.Int64
	last    atomic.Value
}

func (p *followPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.calls.Add(1)
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	p.last.Store(body)
	verdict := p.verdict.Load().(string)
	if verdict == "" {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"verdict": verdict, "reason": "REASON_SPAM"})
}

// TestInboundFollowHook runs the inbound.follow hook on the assembled server, from a signed Follow
// at the inbox to a real plugin over a Unix socket.
func TestInboundFollowHook(t *testing.T) {
	t.Parallel()
	plugin := new(followPlugin)
	plugin.verdict.Store("VERDICT_DROP")
	mux := http.NewServeMux()
	mux.Handle("POST /my.plugin.hook.v1.HookService/Check", plugin)
	network := &relayNet{docs: map[string][]byte{}}
	c := newStatusesClientWith(t, map[string]string{
		"MY_QUEUE": "inline", "MY_PLUGIN_INBOUND_FOLLOW_ADDRESS": fakeplugin.ServeUnix(t, mux),
	}, platform.AppOptions{HTTPClient: network}, "alice")
	ctx := t.Context()
	alice := "https://example.test/ap/actors/" + c.ids["alice"]

	set := func(key instance.SettingKey, value string) {
		t.Helper()
		if _, err := c.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: key, Value: value}); err != nil {
			t.Fatal(err)
		}
	}
	actor := func(name string) (remoteActor, string) {
		t.Helper()
		id := "https://remote.example/users/" + name
		return newRemoteActor(t, network, id, "Person", id+"/inbox"), id
	}
	follow := func(signer remoteActor, id string) {
		t.Helper()
		code := inboxCode(t, c, signer, map[string]any{"id": id, "type": "Follow", "actor": signer.id, "object": alice})
		if code != http.StatusAccepted {
			t.Fatalf("Follow %s = %d", id, code)
		}
	}
	following := func(uri string) bool {
		t.Helper()
		a, err := c.app.Accounts.FindByURI(ctx, uri)
		if err != nil {
			t.Fatal(err)
		}
		r, err := c.app.Graph.Relationship(ctx, a.Account.ID, c.ids["alice"])
		if err != nil {
			t.Fatal(err)
		}
		return r.ActivelyFollowing()
	}
	notifications := func() int {
		t.Helper()
		var page clientapi.NotificationPage
		if res := c.do("alice", http.MethodGet, "/notifications", nil, &page); res.StatusCode != http.StatusOK {
			t.Fatalf("GET /notifications = %d", res.StatusCode)
		}
		return len(page.Items)
	}
	decisionOf := func(subject string) hook.Record {
		t.Helper()
		records, err := c.app.Hooks.List(ctx, apphook.ListQuery{Hook: hook.InboundFollow, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range records {
			if r.Decision.SubjectKey == subject {
				return r
			}
		}
		t.Fatalf("no decision on %s in %+v", subject, records)
		return hook.Record{}
	}

	// observe is the default: the verdict is recorded, and the follow goes on.
	observer, observerURI := actor("observed")
	follow(observer, observerURI+"/follow")
	if !following(observerURI) {
		t.Error("a drop in observe stopped the follow")
	}
	if d := decisionOf(observerURI + "/follow").Decision; d.Mode != hook.ModeObserve || d.Outcome != hook.Drop || d.AccountID == "" {
		t.Errorf("the decision in observe = %+v", d)
	}
	body := plugin.last.Load().(map[string]any)
	if body["hook"] != "inbound.follow" || body["subjectKey"] != observerURI+"/follow" {
		t.Errorf("the request = %v", body)
	}
	if parties, _ := body["follow"].(map[string]any); parties["followerUri"] != observerURI || parties["followeeUri"] != alice {
		t.Errorf("the parties = %v", body["follow"])
	}

	// An Undo of a follow never asks.
	calls := plugin.calls.Load()
	code := inboxCode(t, c, observer, map[string]any{
		"id": observerURI + "/undo", "type": "Undo", "actor": observerURI,
		"object": map[string]any{"id": observerURI + "/follow", "type": "Follow", "actor": observerURI, "object": alice},
	})
	if code != http.StatusAccepted || following(observerURI) {
		t.Errorf("Undo = %d, following %v", code, following(observerURI))
	}
	if got := plugin.calls.Load(); got != calls {
		t.Errorf("the Undo called the plugin %d times", got-calls)
	}

	// enforce: a drop creates no follow and no notification, and leaves nothing to send.
	set(instance.SettingHookInboundFollowMode, "enforce")
	before := notifications()
	network.taken()
	dropped, droppedURI := actor("dropped")
	droppedFollow := droppedURI + "/follow"
	follow(dropped, droppedFollow)
	if following(droppedURI) {
		t.Error("a drop in enforce let the follow through")
	}
	if got := notifications(); got != before {
		t.Errorf("notifications after a drop = %d, want %d", got, before)
	}
	if got := network.taken()[droppedURI+"/inbox"]; len(got) != 0 {
		t.Errorf("sent to the follower of a drop = %v", got)
	}

	// A redelivery is a duplicate, and running the job again reads the decision: no new call.
	calls = plugin.calls.Load()
	follow(dropped, droppedFollow)
	if _, err := c.app.InboxProcessor.Process(ctx, federation.IRI(droppedFollow), federation.TrustSigned); err != nil {
		t.Errorf("Process again: %v", err)
	}
	if got := plugin.calls.Load(); got != calls || following(droppedURI) {
		t.Errorf("after the redelivery: %d more calls, following %v", got-calls, following(droppedURI))
	}

	// Cancelling the drop runs the Follow again as an allow, with no call.
	id := decisionOf(droppedFollow).Decision.ID
	if res := c.do("alice", http.MethodPost, "/admin/hook_decisions/"+id+"/override", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("override = %d", res.StatusCode)
	}
	if got := plugin.calls.Load(); got != calls || !following(droppedURI) {
		t.Errorf("after the override: %d more calls, following %v", got-calls, following(droppedURI))
	}
	if got := notifications(); got != before+1 {
		t.Errorf("notifications after the override = %d, want %d", got, before+1)
	}

	// A flag lets the follow through.
	plugin.verdict.Store("VERDICT_FLAG")
	flagged, flaggedURI := actor("flagged")
	follow(flagged, flaggedURI+"/follow")
	if !following(flaggedURI) {
		t.Error("a flag stopped the follow")
	}
	if d := decisionOf(flaggedURI + "/follow").Decision; d.Mode != hook.ModeEnforce || d.Outcome != hook.Flag {
		t.Errorf("the decision of a flag = %+v", d)
	}

	// A failure passes by default and is recorded as what it is.
	plugin.verdict.Store("")
	passed, passedURI := actor("passed")
	follow(passed, passedURI+"/follow")
	if !following(passedURI) {
		t.Error("a failed call stopped the follow under pass")
	}
	if d := decisionOf(passedURI + "/follow").Decision; d.Outcome != hook.FailedError {
		t.Errorf("the decision of a failed call = %+v", d)
	}

	// Under retry the job fails with ErrRetryLater and records nothing; a later run asks again.
	set(instance.SettingHookInboundFollowOnFailure, "retry")
	retried, retriedURI := actor("retried")
	follow(retried, retriedURI+"/follow")
	_, err := c.app.InboxProcessor.Process(ctx, federation.IRI(retriedURI+"/follow"), federation.TrustSigned)
	if !errors.Is(err, apphook.ErrRetryLater) {
		t.Errorf("Process under retry = %v, want ErrRetryLater", err)
	}
	if following(retriedURI) {
		t.Error("a failed call let the follow through under retry")
	}
	plugin.verdict.Store("VERDICT_ALLOW")
	if _, err := c.app.InboxProcessor.Process(ctx, federation.IRI(retriedURI+"/follow"), federation.TrustSigned); err != nil {
		t.Errorf("Process after the plugin came back: %v", err)
	}
	if !following(retriedURI) {
		t.Error("the follow did not go through after the plugin came back")
	}

	// A Follow of the instance actor goes to the relays and does not ask.
	calls = plugin.calls.Load()
	inst, err := c.app.Accounts.EnsureInstanceActor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	code = inboxCode(t, c, retried, map[string]any{"id": retriedURI + "/instance", "type": "Follow", "actor": retriedURI, "object": inst.URI})
	if code != http.StatusAccepted || plugin.calls.Load() != calls {
		t.Errorf("a Follow of the instance actor = %d, %d more calls", code, plugin.calls.Load()-calls)
	}
}
