// SPDX-License-Identifier: AGPL-3.0-only

package fake_test

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/pushservice/fake"
	"github.com/mycroblog/my/internal/adapter/webpush"
	"github.com/mycroblog/my/internal/app/kernel"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/pkg/testkit"
)

// start serves a fake push service over TLS on a loopback port and returns it with its origin
// and an HTTP client that trusts its certificate.
func start(t *testing.T) (*fake.Service, string, *http.Client) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "https://" + ln.Addr().String()
	svc, err := fake.New(fake.Options{Origin: origin})
	if err != nil {
		t.Fatal(err)
	}
	srv := &httptest.Server{Listener: ln, Config: &http.Server{Handler: svc.Handler(), ReadHeaderTimeout: 5 * time.Second}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return svc, origin, srv.Client()
}

// httpClient is the kernel.HTTPClient of the sender: plain net/http to the test server.
type httpClient struct{ c *http.Client }

func (h httpClient) Do(ctx context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	r, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	r.Header = req.Header.Clone()
	resp, err := h.c.Do(r)
	if err != nil {
		return nil, err
	}
	return &kernel.HTTPResponse{StatusCode: resp.StatusCode, Header: resp.Header, URL: req.URL, Body: resp.Body}, nil
}

func newSender(t *testing.T, client *http.Client) (*webpush.Sender, *ecdsa.PrivateKey) {
	t.Helper()
	priv, _, err := webpush.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	key, err := webpush.ParsePrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	s, err := webpush.New(webpush.Options{PrivateKey: key, Subject: "mailto:admin@social.example", HTTP: httpClient{client}, Clock: testkit.NewClock(time.Now())})
	if err != nil {
		t.Fatal(err)
	}
	return s, key
}

func subscriptionID(t *testing.T, sub fake.Subscription) string {
	t.Helper()
	_, id, ok := strings.Cut(sub.Endpoint, "/push/")
	if !ok {
		t.Fatalf("endpoint %s has no /push/", sub.Endpoint)
	}
	return id
}

func domainSubscription(sub fake.Subscription) notification.PushSubscription {
	return notification.PushSubscription{Endpoint: sub.Endpoint, KeyP256DH: sub.Keys.P256DH, KeyAuth: sub.Keys.Auth}
}

func TestSendThroughWebpushAdapter(t *testing.T) {
	svc, origin, client := start(t)
	sender, _ := newSender(t, client)

	// Subscribe through the HTTP API, as the test harness does for the browser.
	resp, err := client.Post(origin+"/subscriptions", "application/json", strings.NewReader(`{"applicationServerKey":"`+sender.ServerKey()+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	var sub fake.Subscription
	if err := json.NewDecoder(resp.Body).Decode(&sub); err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /subscriptions = %d, %v", resp.StatusCode, err)
	}
	_ = resp.Body.Close()
	if !strings.HasPrefix(sub.Endpoint, origin+"/push/") {
		t.Errorf("endpoint = %s, want under %s/push/", sub.Endpoint, origin)
	}

	payload := `{"type":"mention","title":"こんにちは"}`
	res, err := sender.Send(t.Context(), domainSubscription(sub), appnotification.PushMessage{Payload: []byte(payload), TTL: time.Hour, Topic: "reply"})
	if err != nil || res.Outcome != appnotification.PushDelivered || res.StatusCode != http.StatusCreated {
		t.Fatalf("Send = %+v, %v; want delivered with 201", res, err)
	}

	resp, err = client.Get(origin + "/subscriptions/" + subscriptionID(t, sub) + "/messages")
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Messages []fake.Message }
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if len(got.Messages) != 1 {
		t.Fatalf("messages = %+v, want one", got.Messages)
	}
	m := got.Messages[0]
	if m.Status != http.StatusCreated || m.Error != "" || m.Plaintext != payload || m.TTL != 3600 || m.Urgency != "normal" || m.Topic != "reply" || m.Subject != "mailto:admin@social.example" {
		t.Errorf("message = %+v", m)
	}

	// An expired subscription answers 410, which the sender reads as gone.
	resp, err = client.Post(origin+"/subscriptions/"+subscriptionID(t, sub)+"/expire", "", nil)
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expire = %v, %v", resp, err)
	}
	_ = resp.Body.Close()
	res, err = sender.Send(t.Context(), domainSubscription(sub), appnotification.PushMessage{Payload: []byte(payload), TTL: time.Hour})
	if err != nil || res.Outcome != appnotification.PushGone || res.StatusCode != http.StatusGone {
		t.Errorf("Send after expiry = %+v, %v; want gone with 410", res, err)
	}
	msgs, err := svc.Messages(subscriptionID(t, sub))
	if err != nil || len(msgs) != 2 || msgs[1].Status != http.StatusGone {
		t.Errorf("Messages = %+v, %v", msgs, err)
	}
}

// request is a push message the test builds by hand, to break one part of it at a time.
type request struct {
	key      *ecdsa.PrivateKey // signs the token
	k        string            // the k parameter; empty means the public key of key
	aud      string
	exp      time.Time
	sub      string
	auth     string // the Authorization field; empty means the vapid one built from the above
	ttl      string
	encoding string
	body     []byte
}

func signVAPID(t *testing.T, r request) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims := map[string]any{"aud": r.aud, "exp": r.exp.Unix()}
	if r.sub != "" {
		claims["sub"] = r.sub
	}
	c, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	input := header + "." + base64.RawURLEncoding.EncodeToString(c)
	digest := sha256.Sum256([]byte(input))
	rr, ss, err := ecdsa.Sign(rand.Reader, r.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	rr.FillBytes(sig[:32])
	ss.FillBytes(sig[32:])
	k := r.k
	if k == "" {
		pub, err := r.key.PublicKey.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		k = base64.RawURLEncoding.EncodeToString(pub)
	}
	return "vapid t=" + input + "." + base64.RawURLEncoding.EncodeToString(sig) + ", k=" + k
}

func encrypt(t *testing.T, sub fake.Subscription, plaintext string) []byte {
	t.Helper()
	uaPublic, err := ecdh.P256().NewPublicKey(mustB64(t, sub.Keys.P256DH))
	if err != nil {
		t.Fatal(err)
	}
	as, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	body, err := webpush.Encrypt([]byte(plaintext), uaPublic, mustB64(t, sub.Keys.Auth), as, salt)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func mustB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPushChecks(t *testing.T) {
	svc, origin, client := start(t)
	_, key := newSender(t, client)
	_, other := newSender(t, client)
	pub, err := key.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	sub, err := svc.Subscribe(base64.RawURLEncoding.EncodeToString(pub))
	if err != nil {
		t.Fatal(err)
	}
	otherPub, err := other.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	valid := func() request {
		return request{key: key, aud: origin, exp: now.Add(time.Hour), sub: "mailto:admin@social.example", ttl: "60", encoding: "aes128gcm", body: encrypt(t, sub, "hello")}
	}
	for _, tt := range []struct {
		name   string
		change func(*request)
		status int
		want   string
	}{
		{"a valid message", func(*request) {}, http.StatusCreated, ""},
		{"no Authorization", func(r *request) { r.auth = "-" }, http.StatusUnauthorized, "no vapid authorization"},
		{"a bad VAPID signature", func(r *request) { r.key, r.k = other, base64.RawURLEncoding.EncodeToString(pub) }, http.StatusForbidden, "does not verify"},
		{"a wrong k", func(r *request) { r.key = other }, http.StatusForbidden, "not the applicationServerKey"},
		{"a wrong aud", func(r *request) { r.aud = "https://push.example.net" }, http.StatusForbidden, "aud is"},
		{"an expired exp", func(r *request) { r.exp = now.Add(-time.Minute) }, http.StatusForbidden, "expired"},
		{"exp more than 24 hours ahead", func(r *request) { r.exp = now.Add(25 * time.Hour) }, http.StatusForbidden, "more than 24 hours"},
		{"no sub", func(r *request) { r.sub = "" }, http.StatusForbidden, "sub is missing"},
		{"no TTL", func(r *request) { r.ttl = "" }, http.StatusBadRequest, "TTL"},
		{"another Content-Encoding", func(r *request) { r.encoding = "aesgcm" }, http.StatusBadRequest, "Content-Encoding"},
		{"a body for another subscription", func(r *request) {
			other, err := svc.Subscribe(base64.RawURLEncoding.EncodeToString(otherPub))
			if err != nil {
				t.Fatal(err)
			}
			r.body = encrypt(t, other, "hello")
		}, http.StatusBadRequest, "does not decrypt"},
		{"a body over the limit", func(r *request) { r.body = make([]byte, fake.MaxBodyBytes+1) }, http.StatusRequestEntityTooLarge, "larger than"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := valid()
			tt.change(&r)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, sub.Endpoint, bytes.NewReader(r.body))
			if err != nil {
				t.Fatal(err)
			}
			switch r.auth {
			case "":
				req.Header.Set("Authorization", signVAPID(t, r))
			case "-":
			default:
				req.Header.Set("Authorization", r.auth)
			}
			if r.ttl != "" {
				req.Header.Set("TTL", r.ttl)
			}
			req.Header.Set("Content-Encoding", r.encoding)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != tt.status {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.status)
			}
			if tt.status == http.StatusCreated && !strings.HasPrefix(resp.Header.Get("Location"), sub.Endpoint+"/messages/") {
				t.Errorf("Location = %q", resp.Header.Get("Location"))
			}
			msgs, err := svc.Messages(subscriptionID(t, sub))
			if err != nil {
				t.Fatal(err)
			}
			last := msgs[len(msgs)-1]
			if last.Status != tt.status || !strings.Contains(last.Error, tt.want) || (tt.want == "") != (last.Error == "") {
				t.Errorf("recorded %+v, want status %d and an error containing %q", last, tt.status, tt.want)
			}
			if tt.status == http.StatusCreated && last.Plaintext != "hello" {
				t.Errorf("plaintext = %q", last.Plaintext)
			}
		})
	}
}

func TestUnknownSubscriptions(t *testing.T) {
	svc, origin, client := start(t)
	resp, err := client.Post(origin+"/push/nosuch", "application/octet-stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("push to an unknown subscription = %d, want 404", resp.StatusCode)
	}
	for _, path := range []string{"/subscriptions/nosuch/messages"} {
		resp, err := client.Get(origin + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
	}
	if err := svc.Expire("nosuch"); err == nil {
		t.Error("Expire of an unknown subscription succeeded")
	}
	for _, body := range []string{`{}`, `{"applicationServerKey":"not base64!"}`, `{"applicationServerKey":"` + base64.RawURLEncoding.EncodeToString(make([]byte, 65)) + `"}`, `[`} {
		resp, err := client.Post(origin+"/subscriptions", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("POST /subscriptions %s = %d, want 400", body, resp.StatusCode)
		}
	}
	if _, err := fake.New(fake.Options{}); err == nil {
		t.Error("New without an origin succeeded")
	}
	if _, err := fake.New(fake.Options{Origin: origin + "/"}); err == nil {
		t.Error("New with a trailing slash succeeded")
	}
}
