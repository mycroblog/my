// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// pushService records the messages posted to it and answers them with status.
type pushService struct {
	mu     sync.Mutex
	status int
	reqs   []*kernel.HTTPRequest
}

func (p *pushService) Do(_ context.Context, r *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reqs = append(p.reqs, r)
	return &kernel.HTTPResponse{StatusCode: p.status, Header: http.Header{}, URL: r.URL, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func (p *pushService) sent() []*kernel.HTTPRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]*kernel.HTTPRequest(nil), p.reqs...)
}

// openPushMessage decrypts a message body for the browser's key (RFC 8291 §3.4).
func openPushMessage(t *testing.T, body []byte, ua *ecdh.PrivateKey, auth []byte) appnotification.PushPayload {
	t.Helper()
	salt, idLen := body[:16], int(body[20])
	as, err := ecdh.P256().NewPublicKey(body[21 : 21+idLen])
	if err != nil {
		t.Fatal(err)
	}
	secret, err := ua.ECDH(as)
	if err != nil {
		t.Fatal(err)
	}
	info := append(append([]byte("WebPush: info\x00"), ua.PublicKey().Bytes()...), as.Bytes()...)
	ikm, _ := hkdf.Key(sha256.New, secret, auth, string(info), 32)
	cek, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, body[21+idLen:], nil)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	var p appnotification.PushPayload
	if err := json.Unmarshal(plain[:len(plain)-1], &p); err != nil {
		t.Fatalf("payload %q: %v", plain, err)
	}
	return p
}

// TestWebPush drives Web Push end to end on the real database with the inline queue: the
// server key in the instance, registering the subscription of a grant with its alerts, the
// encrypted message of a mention in the recipient's locale, no message for a type the
// subscription does not take, and the deletion of a subscription that the push service
// reports gone.
func TestWebPush(t *testing.T) {
	t.Parallel()
	const vapidKey = "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline", "MY_VAPID_PRIVATE_KEY": vapidKey,
	})
	service := &pushService{status: http.StatusCreated}
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), HTTPClient: service})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h, spec, ctx := app.Handler(), apitest.Load(t), t.Context()

	alice, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
		Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if _, err := app.Identity.UpdatePreferences(ctx, alice.User.ID, identity.PreferencesUpdate{Locale: new("ja")}); err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	bob, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
		Username: "bob", Password: "correct horse battery staple", DisplayName: "Bob", Locale: "en",
	})
	if err != nil {
		t.Fatalf("RegisterLocalUser: %v", err)
	}
	token := issueAccessToken(t, app, alice.User.ID, "push")
	send := func(method, path, body string) *http.Response {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		return spec.Serve(t, h, req)
	}

	res := spec.Serve(t, h, httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil))
	var inst clientapi.Instance
	if err := json.NewDecoder(res.Body).Decode(&inst); err != nil {
		t.Fatal(err)
	}
	if inst.PushServerKey == nil || *inst.PushServerKey == "" {
		t.Fatal("the instance has no push_server_key")
	}

	if res := send(http.MethodGet, "/api/v1/push/subscription", ""); res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET before subscribing = %d, want 404", res.StatusCode)
	}
	ua, err := ecdh.P256().GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	auth := []byte("0123456789abcdef")
	subscribe := `{"endpoint":"https://push.example.net/send/1","keys":{"p256dh":"` +
		base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes()) + `","auth":"` + base64.RawURLEncoding.EncodeToString(auth) +
		`"},"alerts":["mention","mention"]}`
	if res := send(http.MethodPut, "/api/v1/push/subscription", strings.Replace(subscribe, "https://", "http://", 1)); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("PUT with an http endpoint = %d, want 422", res.StatusCode)
	}
	res = send(http.MethodPut, "/api/v1/push/subscription", subscribe)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT = %d", res.StatusCode)
	}
	var sub clientapi.PushSubscription
	if err := json.NewDecoder(res.Body).Decode(&sub); err != nil {
		t.Fatal(err)
	}
	if len(sub.Alerts) != 1 || sub.Alerts[0] != clientapi.NotificationTypeMention || sub.ServerKey != *inst.PushServerKey {
		t.Errorf("subscription = %+v", sub)
	}

	post := func(text string) string {
		t.Helper()
		p, err := app.Content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: bob.AccountID, TextInput: appcontent.TextInput{Text: text}, Visibility: domcontent.VisibilityPublic,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return p.Status.ID
	}
	statusID := post("@alice hello")
	reqs := service.sent()
	if len(reqs) != 1 {
		t.Fatalf("%d messages, want 1", len(reqs))
	}
	if reqs[0].URL != "https://push.example.net/send/1" || !strings.HasPrefix(reqs[0].Header.Get("Authorization"), "vapid t=") {
		t.Errorf("message to %s with Authorization %q", reqs[0].URL, reqs[0].Header.Get("Authorization"))
	}
	msg := openPushMessage(t, reqs[0].Body, ua, auth)
	if msg.Type != "mention" || msg.Path != "/@bob/status/"+statusID || msg.Lang != "ja" || msg.Title != "Bobさんがあなたをメンションしました" || msg.NotificationID == "" {
		t.Errorf("message = %+v", msg)
	}

	// A favourite is a type the subscription does not take.
	alicePost, err := app.Content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: alice.AccountID, TextInput: appcontent.TextInput{Text: "hi"}, Visibility: domcontent.VisibilityPublic,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Interactions.Favourite(ctx, bob.AccountID, alicePost.Status.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(service.sent()); n != 1 {
		t.Errorf("%d messages after a favourite, want 1", n)
	}

	// The push service reports the subscription gone: it is deleted.
	service.mu.Lock()
	service.status = http.StatusGone
	service.mu.Unlock()
	post("@alice again")
	if n := len(service.sent()); n != 2 {
		t.Errorf("%d messages, want 2", n)
	}
	if res := send(http.MethodGet, "/api/v1/push/subscription", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET after 410 = %d, want 404", res.StatusCode)
	}
	if res := send(http.MethodDelete, "/api/v1/push/subscription", ""); res.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE = %d, want 204", res.StatusCode)
	}
}

// TestWebPushOff checks that an instance without vapid_private_key offers no Web Push.
func TestWebPushOff(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h, spec := app.Handler(), apitest.Load(t)
	alice, err := app.Identity.CreateAdmin(t.Context(), identity.CreateAdminInput{
		Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatal(err)
	}
	res := spec.Serve(t, h, httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil))
	var inst clientapi.Instance
	if err := json.NewDecoder(res.Body).Decode(&inst); err != nil {
		t.Fatal(err)
	}
	if inst.PushServerKey != nil {
		t.Errorf("push_server_key = %q, want none", *inst.PushServerKey)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/push/subscription", strings.NewReader(
		`{"endpoint":"https://push.example.net/1","keys":{"p256dh":"x","auth":"y"},"alerts":[]}`))
	req.Header.Set("Authorization", "Bearer "+issueAccessToken(t, app, alice.User.ID, "push"))
	req.Header.Set("Content-Type", "application/json")
	if res := spec.Serve(t, h, req); res.StatusCode != http.StatusNotFound {
		t.Errorf("PUT = %d, want 404", res.StatusCode)
	}
}

// TestWebPushQueuedBeforeDeactivation runs, once the recipient has deactivated the account, a job
// that a mention queued before it: the message stays unsent (ADR-0089 §16).
func TestWebPushQueuedBeforeDeactivation(t *testing.T) {
	t.Parallel()
	const vapidKey = "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"
	service := &pushService{status: http.StatusCreated}
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_VAPID_PRIVATE_KEY": vapidKey,
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), HTTPClient: service})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h, spec, ctx := app.Handler(), apitest.Load(t), t.Context()
	const password = "correct horse battery staple"
	if _, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{Username: "root", Email: "root@example.com", Password: password}); err != nil {
		t.Fatal(err)
	}
	alice, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "alice", Password: password, Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "bob", Password: password, Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	token := issueAccessToken(t, app, alice.User.ID, "")
	send := func(method, path, body string) *http.Response {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		return spec.Serve(t, h, req)
	}
	ua, err := ecdh.P256().GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	subscribe := `{"endpoint":"https://push.example.net/send/1","keys":{"p256dh":"` +
		base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes()) + `","auth":"` + base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef")) +
		`"},"alerts":["mention"]}`
	if res := send(http.MethodPut, "/api/v1/push/subscription", subscribe); res.StatusCode != http.StatusOK {
		t.Fatalf("PUT = %d", res.StatusCode)
	}
	drain := func() {
		t.Helper()
		for {
			ran, err := app.Runner.RunOnce(ctx)
			if err != nil {
				t.Fatalf("RunOnce: %v", err)
			}
			if !ran {
				return
			}
		}
	}
	post := func(text string) {
		t.Helper()
		if _, err := app.Content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: bob.AccountID, TextInput: appcontent.TextInput{Text: text}, Visibility: domcontent.VisibilityPublic,
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// A mention before: the worker sends it, so the path works.
	post("@alice before")
	drain()
	if n := len(service.sent()); n != 1 {
		t.Fatalf("%d messages before the deactivation, want 1", n)
	}

	// A mention queued before the deactivation, which the worker runs after it.
	post("@alice just before")
	if res := send(http.MethodPost, "/api/v1/accounts/deactivation", `{"password":"`+password+`"}`); res.StatusCode != http.StatusOK {
		t.Fatalf("POST /accounts/deactivation = %d", res.StatusCode)
	}
	drain()
	if n := len(service.sent()); n != 1 {
		t.Errorf("%d messages after the deactivation, want the 1 from before", n)
	}
}
