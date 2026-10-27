// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/webhook"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// webhookReceiver is an endpoint on loopback that answers with the next queued status code
// (204 when none) and keeps what it receives.
type webhookReceiver struct {
	mu       sync.Mutex
	statuses []int
	got      []*http.Request
	bodies   [][]byte
}

func (r *webhookReceiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got, r.bodies = append(r.got, req), append(r.bodies, body)
	status := http.StatusNoContent
	if len(r.statuses) > 0 {
		status, r.statuses = r.statuses[0], r.statuses[1:]
	}
	w.WriteHeader(status)
}

func (r *webhookReceiver) take() ([]*http.Request, [][]byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	got, bodies := r.got, r.bodies
	r.got, r.bodies = nil, nil
	return got, bodies
}

func (r *webhookReceiver) answer(statuses ...int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses = statuses
}

// TestWebhooks sets up a webhook through the admin API on a receiver on loopback, which
// webhook_allowed_destinations lets the webhooks alone reach, and checks through the composition
// root on SQLite with the inline queue that a public local post, and not an unlisted one,
// reaches it signed with Standard Webhooks v1 (ADR-0060), that the test message and a
// redelivery keep their webhook-id, that a failure is listed with the delivery, that a 410 Gone
// disables the webhook as the system with its audit log entry and the admin_webhook_disabled
// notification, and that the operations need manage_webhooks.
func TestWebhooks(t *testing.T) {
	t.Parallel()
	recv := &webhookReceiver{}
	srv := httptest.NewServer(recv)
	t.Cleanup(srv.Close)
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline",
		"MY_WEBHOOK_ALLOWED_DESTINATIONS": "127.0.0.1",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h, spec, ctx := app.Handler(), apitest.Load(t), t.Context()
	admin, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
		Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatal(err)
	}
	adminToken := issueAccessToken(t, app, admin.User.ID, "read write admin")
	bob, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "bob", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	bobToken := issueAccessToken(t, app, bob.User.ID, "read write admin")

	api := func(token, method, path string, body any, out any) int {
		t.Helper()
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, clientapi.BasePath+path, r)
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res := spec.Serve(t, h, req)
		if out != nil {
			if err := json.NewDecoder(res.Body).Decode(out); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
		}
		return res.StatusCode
	}

	// Without manage_webhooks, nothing.
	if code := api(bobToken, http.MethodGet, "/admin/webhooks", nil, nil); code != http.StatusForbidden {
		t.Errorf("GET /admin/webhooks without manage_webhooks = %d, want 403", code)
	}
	var problem clientapi.WebhookProblem
	if code := api(adminToken, http.MethodPost, "/admin/webhooks", map[string]any{
		"url": "ftp://hooks.example/", "events": []string{"status.created"},
	}, &problem); code != http.StatusUnprocessableEntity || problem.Reason == nil || *problem.Reason != clientapi.WebhookProblemReasonUrl {
		t.Errorf("POST an ftp URL = %d %+v", code, problem)
	}
	var created clientapi.AdminWebhookCreated
	if code := api(adminToken, http.MethodPost, "/admin/webhooks", map[string]any{
		"url": srv.URL + "/hook?token=s3cret", "description": "Moderation chat", "events": []string{"status.created", "report.created"},
	}, &created); code != http.StatusOK {
		t.Fatalf("POST /admin/webhooks = %d", code)
	}
	hook := created.Webhook
	secret, err := webhook.ParseSecret(created.Secret)
	if err != nil || hook.State != clientapi.AdminWebhookStateEnabled || len(hook.Events) != 2 {
		t.Fatalf("created = %+v, %v", created, err)
	}
	verify := func(req *http.Request, body []byte, wantType string) string {
		t.Helper()
		id, ts := req.Header.Get(webhook.HeaderID), req.Header.Get(webhook.HeaderTimestamp)
		sec, err := strconv.ParseInt(ts, 10, 64)
		if err != nil || !webhook.Verify(secret, req.Header.Get(webhook.HeaderSignature), id, time.Unix(sec, 0), body) {
			t.Errorf("the signature of %s does not verify: %v", body, req.Header)
		}
		var msg struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(body, &msg); err != nil || msg.Type != wantType {
			t.Errorf("body %s, want type %s", body, wantType)
		}
		return id
	}

	// A public local post is sent; an unlisted one is not.
	if _, err := app.Content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: admin.AccountID, TextInput: appcontent.TextInput{Text: "hidden"}, Visibility: domcontent.VisibilityUnlisted,
	}); err != nil {
		t.Fatal(err)
	}
	post, err := app.Content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: admin.AccountID, TextInput: appcontent.TextInput{Text: "hello webhooks"}, Visibility: domcontent.VisibilityPublic,
	})
	if err != nil {
		t.Fatal(err)
	}
	reqs, bodies := recv.take()
	if len(reqs) != 1 || reqs[0].URL.Path != "/hook" || reqs[0].URL.Query().Get("token") != "s3cret" {
		t.Fatalf("received %d requests: %v", len(reqs), reqs)
	}
	verify(reqs[0], bodies[0], "status.created")
	if !bytes.Contains(bodies[0], []byte(post.Status.ID)) || !bytes.Contains(bodies[0], []byte("hello webhooks")) {
		t.Errorf("status.created body = %s", bodies[0])
	}

	// The test message fails, is listed as failing, and a redelivery sends it again.
	recv.answer(http.StatusInternalServerError)
	var delivery clientapi.AdminWebhookDelivery
	if code := api(adminToken, http.MethodPost, "/admin/webhooks/"+hook.Id+"/test", nil, &delivery); code != http.StatusOK ||
		delivery.EventType != clientapi.WebhookMessageTypeWebhookTest {
		t.Fatalf("POST test = %d %+v", code, delivery)
	}
	reqs, bodies = recv.take()
	if len(reqs) != 1 {
		t.Fatalf("test requests = %d", len(reqs))
	}
	testID := verify(reqs[0], bodies[0], "webhook.test")
	var page clientapi.AdminWebhookDeliveryPage
	if code := api(adminToken, http.MethodGet, "/admin/webhooks/"+hook.Id+"/deliveries", nil, &page); code != http.StatusOK || len(page.Items) != 2 ||
		page.Items[0].Status != clientapi.AdminWebhookDeliveryStatusFailing || page.Items[0].LastAttempt == nil ||
		page.Items[0].LastAttempt.StatusCode == nil || *page.Items[0].LastAttempt.StatusCode != http.StatusInternalServerError ||
		page.Items[1].Status != clientapi.AdminWebhookDeliveryStatusSucceeded {
		t.Fatalf("GET deliveries = %d %+v", code, page)
	}
	var got clientapi.AdminWebhook
	if code := api(adminToken, http.MethodGet, "/admin/webhooks/"+hook.Id, nil, &got); code != http.StatusOK || got.Failure == nil || got.Failure.ConsecutiveFailures != 1 {
		t.Errorf("GET the failing webhook = %d %+v", code, got)
	}
	if code := api(adminToken, http.MethodPost, "/admin/webhooks/"+hook.Id+"/deliveries/"+delivery.Id+"/redeliver", nil, nil); code != http.StatusOK {
		t.Fatalf("POST redeliver = %d", code)
	}
	reqs, bodies = recv.take()
	if len(reqs) != 1 || verify(reqs[0], bodies[0], "webhook.test") != testID {
		t.Errorf("redelivery = %d requests, want the same webhook-id %s", len(reqs), testID)
	}

	// 410 Gone disables the webhook as the system, and tells the administrators.
	recv.answer(http.StatusGone)
	if code := api(adminToken, http.MethodPost, "/admin/webhooks/"+hook.Id+"/test", nil, nil); code != http.StatusOK {
		t.Fatalf("POST test = %d", code)
	}
	recv.take()
	if code := api(adminToken, http.MethodGet, "/admin/webhooks/"+hook.Id, nil, &got); code != http.StatusOK ||
		got.State != clientapi.AdminWebhookStateDisabled || got.StateReason != clientapi.AdminWebhookStateReasonGone {
		t.Fatalf("GET the gone webhook = %d %+v", code, got)
	}
	var notes clientapi.NotificationPage
	if code := api(adminToken, http.MethodGet, "/notifications", nil, &notes); code != http.StatusOK || len(notes.Items) == 0 ||
		notes.Items[0].Type != clientapi.NotificationTypeAdminWebhookDisabled {
		t.Errorf("GET /notifications = %d %+v, want admin_webhook_disabled", code, notes)
	}
	var audit clientapi.AdminAuditEntryPage
	if code := api(adminToken, http.MethodGet, "/admin/audit_log?target_kind=webhook&target_id="+hook.Id, nil, &audit); code != http.StatusOK || len(audit.Items) != 2 ||
		audit.Items[0].Action != clientapi.AdminAuditActionWebhookDisabled || audit.Items[0].ActorKind != clientapi.AdminAuditActorKindSystem ||
		audit.Items[1].Action != clientapi.AdminAuditActionWebhookCreated || audit.Items[1].TargetLabel != "Moderation chat (127.0.0.1)" {
		t.Errorf("GET the audit log = %d %+v", code, audit)
	}
	// A disabled webhook receives nothing.
	if _, err := app.Content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: admin.AccountID, TextInput: appcontent.TextInput{Text: "again"}, Visibility: domcontent.VisibilityPublic,
	}); err != nil {
		t.Fatal(err)
	}
	if reqs, _ := recv.take(); len(reqs) != 0 {
		t.Errorf("a disabled webhook received %d requests", len(reqs))
	}
	var secretResp clientapi.AdminWebhookSecret
	if code := api(adminToken, http.MethodPost, "/admin/webhooks/"+hook.Id+"/rotate_secret", nil, &secretResp); code != http.StatusOK || secretResp.Secret == created.Secret {
		t.Errorf("POST rotate_secret = %d %+v", code, secretResp)
	}
	if code := api(adminToken, http.MethodDelete, "/admin/webhooks/"+hook.Id, nil, nil); code != http.StatusNoContent {
		t.Errorf("DELETE = %d", code)
	}
	var list clientapi.AdminWebhookList
	if code := api(adminToken, http.MethodGet, "/admin/webhooks", nil, &list); code != http.StatusOK || len(list.Items) != 0 {
		t.Errorf("GET /admin/webhooks after the deletion = %d %+v", code, list)
	}
}

// TestWebhookData checks through the composition root on SQLite with the inline queue that the
// data of the webhook messages is what the admin API answers for the same object right after
// the event (ADR-0060): a report as it is filed and resolved, an action on an account as it is
// issued and lifted, an appeal with its action, an action on a domain, and a local account
// whose profile changes, a registration that awaits a moderator, and a custom emoji as it is
// created and deleted.
func TestWebhookData(t *testing.T) {
	t.Parallel()
	recv := &webhookReceiver{}
	srv := httptest.NewServer(recv)
	t.Cleanup(srv.Close)
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline",
		"MY_WEBHOOK_ALLOWED_DESTINATIONS": "127.0.0.1",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h, spec, ctx := app.Handler(), apitest.Load(t), t.Context()
	admin, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
		Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatal(err)
	}
	adminToken := issueAccessToken(t, app, admin.User.ID, "read write admin")
	bob, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "bob", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	bobToken := issueAccessToken(t, app, bob.User.ID, "read write")

	api := func(token, method, path string, body any, out any) {
		t.Helper()
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, clientapi.BasePath+path, r)
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res := spec.Serve(t, h, req)
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
			t.Fatalf("%s %s = %d", method, path, res.StatusCode)
		}
		if out != nil {
			if err := json.NewDecoder(res.Body).Decode(out); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
		}
	}
	api(adminToken, http.MethodPost, "/admin/webhooks", map[string]any{
		"url": srv.URL + "/hook", "events": []string{
			"report.created", "report.resolved", "moderation_action.created", "moderation_action.lifted",
			"appeal.created", "domain_moderation_action.created", "account.updated", "registration.pending",
			"custom_emoji.created", "custom_emoji.deleted", "error_group.created",
		},
	}, nil)
	// message returns the data of the one message received since the last call, of type want.
	message := func(want string) map[string]any {
		t.Helper()
		_, bodies := recv.take()
		if len(bodies) != 1 {
			t.Fatalf("received %d messages, want one %s", len(bodies), want)
		}
		var msg struct {
			Type string         `json:"type"`
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(bodies[0], &msg); err != nil || msg.Type != want {
			t.Fatalf("body %s, want type %s: %v", bodies[0], want, err)
		}
		return msg.Data
	}
	// same checks that the data of a message is the admin API's representation.
	same := func(name string, data, want map[string]any) {
		t.Helper()
		if !reflect.DeepEqual(data, want) {
			got, _ := json.Marshal(data)
			exp, _ := json.Marshal(want)
			t.Errorf("the data of %s = %s, want the admin API's %s", name, got, exp)
		}
	}

	// A report, filed and resolved, is AdminReport, with the accounts it embeds.
	var report struct {
		ID string `json:"id"`
	}
	api(bobToken, http.MethodPost, "/reports", map[string]any{"account_id": admin.AccountID, "category": "spam", "comment": "rude"}, &report)
	data := message("report.created")
	var reports struct {
		Items []map[string]any `json:"items"`
	}
	api(adminToken, http.MethodGet, "/admin/reports", nil, &reports)
	if len(reports.Items) != 1 {
		t.Fatalf("GET /admin/reports = %+v", reports)
	}
	same("report.created", data, reports.Items[0])
	if acct, ok := data["reporter_account"].(map[string]any); !ok || acct["username"] != "bob" {
		t.Errorf("report.created reporter_account = %v", data["reporter_account"])
	}
	api(adminToken, http.MethodPost, "/admin/reports/"+report.ID+"/resolve", map[string]any{"outcome": "dismissed"}, nil)
	data = message("report.resolved")
	api(adminToken, http.MethodGet, "/admin/reports", nil, &reports)
	same("report.resolved", data, reports.Items[0])
	if data["state"] != "resolved" || data["outcome"] != "dismissed" {
		t.Errorf("report.resolved = %v", data)
	}

	// An action on an account, issued and lifted, is AdminAccountAction with its moderator.
	var action map[string]any
	api(adminToken, http.MethodPost, "/admin/accounts/"+bob.AccountID+"/actions", map[string]any{"kind": "warn", "public_reason": "be kind"}, &action)
	same("moderation_action.created", message("moderation_action.created"), action)
	if action["issued_by_account_id"] != admin.AccountID {
		t.Errorf("the action = %v, want issued by %s", action, admin.AccountID)
	}
	actionID, _ := action["id"].(string)

	// An appeal is AdminAppeal, with its action.
	var appealed struct {
		Appeal struct {
			ID string `json:"id"`
		} `json:"appeal"`
	}
	api(bobToken, http.MethodPost, "/moderation/actions/"+actionID+"/appeal", map[string]any{"text": "I was kind"}, &appealed)
	data = message("appeal.created")
	var appeal map[string]any
	api(adminToken, http.MethodGet, "/admin/appeals/"+appealed.Appeal.ID, nil, &appeal)
	same("appeal.created", data, appeal)

	api(adminToken, http.MethodPost, "/admin/account_actions/"+actionID+"/lift", map[string]any{"reason": "appealed"}, &action)
	same("moderation_action.lifted", message("moderation_action.lifted"), action)

	// An action on a domain is AdminDomainAction.
	var domainAction map[string]any
	api(adminToken, http.MethodPost, "/admin/domain_actions", map[string]any{"domain": "spam.example", "kind": "suspend", "private_note": "spam"}, &domainAction)
	same("domain_moderation_action.created", message("domain_moderation_action.created"), domainAction)

	// A change of a local profile is AdminAccount, with the new profile.
	api(bobToken, http.MethodPatch, "/accounts/update_credentials", map[string]any{"display_name": "Bobby"}, nil)
	data = message("account.updated")
	var accounts struct {
		Items []map[string]any `json:"items"`
	}
	api(adminToken, http.MethodGet, "/admin/accounts?username=bob", nil, &accounts)
	if len(accounts.Items) != 1 {
		t.Fatalf("GET /admin/accounts?username=bob = %+v", accounts)
	}
	same("account.updated", data, accounts.Items[0])
	if acct, ok := data["account"].(map[string]any); !ok || acct["display_name"] != "Bobby" {
		t.Errorf("account.updated account = %v", data["account"])
	}

	// A registration that awaits a moderator is AdminRegistration, with the email address.
	if _, err := app.Instance.SetSetting(ctx, appinstance.SetSettingInput{
		Key: instance.SettingRegistrationMode, Value: string(instance.RegistrationApproval),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Identity.SignUp(ctx, identity.SignUpInput{
		Username: "dave", Email: "dave@example.com", Password: "correct horse battery staple", Reason: "I like ducks.",
	}); err != nil {
		t.Fatal(err)
	}
	data = message("registration.pending")
	var registrations struct {
		Items []map[string]any `json:"items"`
	}
	api(adminToken, http.MethodGet, "/admin/registrations", nil, &registrations)
	if len(registrations.Items) != 1 {
		t.Fatalf("GET /admin/registrations = %+v", registrations)
	}
	same("registration.pending", data, registrations.Items[0])
	if data["email"] != "dave@example.com" {
		t.Errorf("registration.pending = %v", data)
	}

	// A custom emoji is AdminCustomEmoji with the operation, and its deletion presents it as it was.
	m, err := app.Media.Upload(ctx, appcontent.UploadInput{
		OwnerAccountID: admin.AccountID, Content: bytes.NewReader(testkit.FixturePNG(32, 32, true)), Purpose: domcontent.PurposeEmoji,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res := app.Media.Process(ctx, m.File.ID, domcontent.PurposeEmoji); !res.IsDone() {
		t.Fatalf("Process = %+v", res)
	}
	var emoji map[string]any
	api(adminToken, http.MethodPost, "/admin/custom_emojis", map[string]any{"shortcode": "blobcat", "media_id": m.File.ID, "aliases": []string{"neko"}}, &emoji)
	data = message("custom_emoji.created")
	if data["operation"] != "custom_emoji_created" || data["url"] == nil {
		t.Errorf("custom_emoji.created = %v", data)
	}
	delete(data, "operation")
	same("custom_emoji.created", data, emoji)
	emojiID, _ := emoji["id"].(string)
	api(adminToken, http.MethodDelete, "/admin/custom_emojis/"+emojiID, nil, nil)
	data = message("custom_emoji.deleted")
	if data["operation"] != "custom_emoji_deleted" {
		t.Errorf("custom_emoji.deleted = %v", data)
	}
	delete(data, "operation")
	same("custom_emoji.deleted", data, emoji)

	// A new error group is ErrorGroup, presented before its first occurrence is counted; a web
	// report without a session sends nothing.
	reportError := func(fp string, unauthenticated bool) {
		t.Helper()
		err := app.Diagnostics.RecordFirst(ctx, kernel.ErrorEvent{
			Time: time.Now(), Source: kernel.ErrorSourceWeb, Kind: kernel.ErrorKindError, Type: "TypeError",
			Message: "x is undefined", Where: "/home", Fingerprint: fp, Version: "1.0.0", Unauthenticated: unauthenticated,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	reportError("fp-anonymous", true)
	if _, bodies := recv.take(); len(bodies) != 0 {
		t.Errorf("a report without a session sent %d messages", len(bodies))
	}
	reportError("fp-webhook", false)
	data = message("error_group.created")
	var groups struct {
		Items []map[string]any `json:"items"`
	}
	api(adminToken, http.MethodGet, "/admin/error_groups?source=web", nil, &groups)
	var group map[string]any
	for _, g := range groups.Items {
		if g["fingerprint"] == "fp-webhook" {
			group = g
		}
	}
	if group == nil || data["count_24h"] != float64(0) || group["count_24h"] != float64(1) {
		t.Fatalf("error_group.created = %v, the group %v", data, group)
	}
	for _, k := range []string{"count_24h", "count_7d", "last_seen_at", "last_version"} {
		delete(data, k)
		delete(group, k)
	}
	same("error_group.created", data, group)
}
