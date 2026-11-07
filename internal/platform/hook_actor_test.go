// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	appaccount "github.com/mycroblog/my/internal/app/account"
	apphook "github.com/mycroblog/my/internal/app/hook"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/hook"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform"
)

// actorDoc is the document of a remote actor of the fake network, with its profile and its key.
type actorDoc struct {
	id, name, summary, sharedInbox string
	locked                         bool
	key                            *rsa.PrivateKey
}

func (d actorDoc) json(t *testing.T) map[string]any {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&d.key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{
		"@context": []string{"https://www.w3.org/ns/activitystreams", "https://w3id.org/security/v1"},
		"id":       d.id, "type": "Person", "preferredUsername": "bob", "name": d.name, "summary": d.summary,
		"inbox": d.id + "/inbox", "followers": d.id + "/followers", "following": d.id + "/following",
		"manuallyApprovesFollowers": d.locked,
		"icon":                      map[string]any{"type": "Image", "url": d.id + "/avatar.png"},
		"attachment":                []any{map[string]any{"type": "PropertyValue", "name": "site", "value": "<a href=\"https://spam.example\">spam.example</a>"}},
		"publicKey": map[string]any{
			"id": d.id + "#main-key", "owner": d.id,
			"publicKeyPem": string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})),
		},
	}
	if d.sharedInbox != "" {
		doc["endpoints"] = map[string]any{"sharedInbox": d.sharedInbox}
	}
	return doc
}

// TestInboundActorHook runs the inbound.actor hook on the assembled server: the first fetch of an
// actor, a fetch again, and an Update at the inbox, against a real plugin over a Unix socket.
func TestInboundActorHook(t *testing.T) {
	t.Parallel()
	plugin := new(followPlugin)
	plugin.verdict.Store("VERDICT_DROP")
	mux := http.NewServeMux()
	mux.Handle("POST /my.plugin.hook.v1.HookService/Check", plugin)
	network := &relayNet{docs: map[string][]byte{}}
	c := newStatusesClientWith(t, map[string]string{
		"MY_QUEUE": "inline", "MY_PLUGIN_INBOUND_ACTOR_ADDRESS": fakeplugin.ServeUnix(t, mux),
	}, platform.AppOptions{HTTPClient: network}, "alice")
	ctx := t.Context()

	set := func(key instance.SettingKey, value string) {
		t.Helper()
		if _, err := c.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: key, Value: value}); err != nil {
			t.Fatal(err)
		}
	}
	newActor := func(name string) actorDoc {
		t.Helper()
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		d := actorDoc{id: "https://remote.example/users/" + name, name: "Bob", summary: "<p>hello</p>", key: key}
		network.serve(t, d.id, d.json(t))
		return d
	}
	refresh := func(d actorDoc) appaccount.Actor {
		t.Helper()
		network.serve(t, d.id, d.json(t))
		a, err := c.app.RemoteActors.Refresh(ctx, d.id)
		if err != nil {
			t.Fatalf("Refresh(%s) = %v", d.id, err)
		}
		return a
	}
	stored := func(d actorDoc) appaccount.Actor {
		t.Helper()
		a, err := c.app.Accounts.FindByURI(ctx, d.id)
		if err != nil || a.Profile == nil {
			t.Fatalf("FindByURI(%s) = %+v, %v", d.id, a, err)
		}
		return a
	}
	// update sends an Update of the actor that embeds signer's document d, signed with the key the
	// server stored.
	update := func(signer, d actorDoc, id string) {
		t.Helper()
		code := inboxCode(t, c, remoteActor{id: signer.id, key: signer.key}, map[string]any{
			"id": id, "type": "Update", "actor": d.id, "object": d.json(t),
		})
		if code != http.StatusAccepted {
			t.Fatalf("Update %s = %d", id, code)
		}
	}
	decisionOf := func(id string, outcome hook.Outcome) hook.Record {
		t.Helper()
		records, err := c.app.Hooks.List(ctx, apphook.ListQuery{Hook: hook.InboundActor, Outcome: outcome, Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range records {
			if len(r.Decision.SubjectKey) > len(id) && r.Decision.SubjectKey[:len(id)+1] == id+"#" {
				return r
			}
		}
		t.Fatalf("no %s decision on %s in %+v", outcome, id, records)
		return hook.Record{}
	}
	reportsOn := func(accountID string) []moderation.ReportRecord {
		t.Helper()
		page, err := c.app.Moderation.Reports(ctx, appmoderation.ReportQuery{}, 100)
		if err != nil {
			t.Fatal(err)
		}
		var out []moderation.ReportRecord
		for _, r := range page {
			if r.Report.TargetAccountID == accountID {
				out = append(out, r)
			}
		}
		return out
	}

	// observe is the default: the first fetch asks, records the verdict, and stores the actor.
	bob := newActor("bob")
	a := refresh(bob)
	if a.Profile == nil || a.Profile.DisplayName != "Bob" {
		t.Fatalf("the actor stored in observe = %+v", a.Profile)
	}
	d := decisionOf(bob.id, hook.Drop).Decision
	if d.Mode != hook.ModeObserve || d.AccountID != a.Account.ID {
		t.Errorf("the decision in observe = %+v, account %s", d, a.Account.ID)
	}
	body := plugin.last.Load().(map[string]any)
	if body["hook"] != "inbound.actor" || body["subjectKey"] != d.SubjectKey {
		t.Errorf("the request = %v", body)
	}
	sent, _ := body["inboundActor"].(map[string]any)
	if sent["actorUri"] != bob.id || sent["domain"] != "remote.example" || sent["username"] != "bob" || sent["newActor"] != true ||
		sent["displayName"] != "Bob" || sent["noteHtml"] != "<p>hello</p>" || sent["avatarUrl"] != bob.id+"/avatar.png" {
		t.Errorf("the profile sent = %v", sent)
	}
	if fields, _ := sent["fields"].([]any); len(fields) != 1 {
		t.Errorf("the fields sent = %v", sent["fields"])
	}
	if got := len(reportsOn(a.Account.ID)); got != 0 {
		t.Errorf("a drop in observe filed %d reports", got)
	}

	// A fetch again of the same profile asks nothing, even with the keys or the endpoints changed.
	calls := plugin.calls.Load()
	bob.sharedInbox = "https://remote.example/inbox"
	refresh(bob)
	if got := plugin.calls.Load(); got != calls {
		t.Errorf("a fetch of an unchanged profile called the plugin %d times", got-calls)
	}
	if box, err := c.app.Accounts.RemoteInbox(ctx, a.Account.ID); err != nil || box.SharedInboxURL != bob.sharedInbox {
		t.Errorf("the shared inbox after a fetch again = %+v, %v", box, err)
	}

	// enforce: a drop of an Update of a known actor keeps its profile and stores the rest.
	set(instance.SettingHookInboundActorMode, "enforce")
	spam := bob
	spam.name, spam.summary, spam.locked = "Buy pills", "<p>cheap pills at spam.example</p>", true
	spam.sharedInbox = "https://remote.example/shared"
	update(bob, spam, bob.id+"#update-1")
	if p := stored(bob).Profile; p.DisplayName != "Bob" || p.NoteHTML != "<p>hello</p>" || !p.Locked {
		t.Errorf("the profile after a drop = %q %q locked %v, want the stored one, locked", p.DisplayName, p.NoteHTML, p.Locked)
	}
	if box, err := c.app.Accounts.RemoteInbox(ctx, a.Account.ID); err != nil || box.SharedInboxURL != spam.sharedInbox {
		t.Errorf("the shared inbox after a drop = %+v, %v", box, err)
	}
	dropped := decisionOf(bob.id, hook.Drop)
	if dropped.Decision.Mode != hook.ModeEnforce || dropped.Decision.SubjectKey == d.SubjectKey {
		t.Errorf("the decision of the drop = %+v", dropped.Decision)
	}

	// The same profile fetched again reads the decision: no call, and the profile stays.
	calls = plugin.calls.Load()
	refresh(spam)
	if got := plugin.calls.Load(); got != calls {
		t.Errorf("a fetch of a dropped profile called the plugin %d times", got-calls)
	}
	if p := stored(bob).Profile; p.DisplayName != "Bob" {
		t.Errorf("the profile after a fetch of a dropped one = %q", p.DisplayName)
	}

	// Cancelling the drop fetches the actor again, which stores its profile without a call.
	if res := c.do("alice", http.MethodPost, "/admin/hook_decisions/"+dropped.Decision.ID+"/override", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("override = %d", res.StatusCode)
	}
	if got := plugin.calls.Load(); got != calls {
		t.Errorf("the override called the plugin %d times", got-calls)
	}
	if p := stored(bob).Profile; p.DisplayName != "Buy pills" {
		t.Errorf("the profile after the override = %q", p.DisplayName)
	}

	// A flag stores the profile and files one report on the account per decision.
	plugin.verdict.Store("VERDICT_FLAG")
	flagged := spam
	flagged.name = "Flagged"
	refresh(flagged)
	if p := stored(bob).Profile; p.DisplayName != "Flagged" {
		t.Errorf("the profile after a flag = %q", p.DisplayName)
	}
	flag := decisionOf(bob.id, hook.Flag).Decision
	inst, err := c.app.Accounts.EnsureInstanceActor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reports := reportsOn(a.Account.ID)
	if len(reports) != 1 {
		t.Fatalf("reports after a flag = %+v, want one", reports)
	}
	if r := reports[0].Report; r.ReporterAccountID != inst.ID || r.URI != "" || r.Forwarded || r.Category != moderation.CategorySpam ||
		r.HookOrigin == nil || r.HookOrigin.DecisionID != flag.ID || r.HookOrigin.Hook != "inbound.actor" {
		t.Errorf("the report of a flag = %+v, origin %+v", r, r.HookOrigin)
	}
	refresh(flagged)
	if got := len(reportsOn(a.Account.ID)); got != 1 {
		t.Errorf("reports after a fetch again = %d, want 1", got)
	}

	// A drop of an actor fetched for the first time stores it and is taken for a flag.
	plugin.verdict.Store("VERDICT_DROP")
	carol := newActor("carol")
	ca := refresh(carol)
	if ca.Profile == nil || ca.Profile.DisplayName != "Bob" {
		t.Errorf("a new actor dropped = %+v", ca.Profile)
	}
	first := decisionOf(carol.id, hook.Drop).Decision
	if first.AccountID != ca.Account.ID || first.Mode != hook.ModeEnforce {
		t.Errorf("the decision on a new actor = %+v, account %s", first, ca.Account.ID)
	}
	if r := reportsOn(ca.Account.ID); len(r) != 1 || r[0].Report.HookOrigin == nil || r[0].Report.HookOrigin.DecisionID != first.ID {
		t.Errorf("reports on a dropped new actor = %+v", r)
	}

	// A failure passes by default: the Update is stored and the job ends, holding nothing back.
	plugin.verdict.Store("")
	passed := flagged
	passed.name = "Passed"
	update(bob, passed, bob.id+"#update-2")
	if p := stored(bob).Profile; p.DisplayName != "Passed" {
		t.Errorf("the profile after a failure under pass = %q", p.DisplayName)
	}
	if r := decisionOf(bob.id, hook.FailedError); r.Decision.Mode != hook.ModeEnforce {
		t.Errorf("the failure = %+v", r.Decision)
	}

	// Under retry the job fails with ErrRetryLater and stores no profile; a later run asks again.
	set(instance.SettingHookInboundActorOnFailure, "retry")
	retried := passed
	retried.name = "Retried"
	update(bob, retried, bob.id+"#update-3")
	_, err = c.app.InboxProcessor.Process(ctx, federation.IRI(bob.id+"#update-3"), federation.TrustSigned)
	if !errors.Is(err, apphook.ErrRetryLater) {
		t.Errorf("Process under retry = %v, want ErrRetryLater", err)
	}
	if p := stored(bob).Profile; p.DisplayName != "Passed" {
		t.Errorf("the profile after a failure under retry = %q", p.DisplayName)
	}
	plugin.verdict.Store("VERDICT_ALLOW")
	if _, err := c.app.InboxProcessor.Process(ctx, federation.IRI(bob.id+"#update-3"), federation.TrustSigned); err != nil {
		t.Errorf("Process after the plugin came back: %v", err)
	}
	if p := stored(bob).Profile; p.DisplayName != "Retried" {
		t.Errorf("the profile after the plugin came back = %q", p.DisplayName)
	}
	decisionOf(bob.id, hook.Allow)
}
