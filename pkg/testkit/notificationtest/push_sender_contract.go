// SPDX-License-Identifier: AGPL-3.0-only

package notificationtest

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/pkg/testkit"
)

// PushService is the fake push service that PushSenderContract gives the sender under test.
type PushService struct {
	// HTTP is the sender's HTTP client: the push service in memory, which records each
	// request and answers it as the case sets. It never reaches the network.
	HTTP kernel.HTTPClient
	// Clock is the sender's clock.
	Clock *testkit.Clock
}

// PushSenderFactory returns the sender under test on s.
type PushSenderFactory func(t *testing.T, s PushService) appnotification.PushSender

// pushServiceEndpoint is the endpoint of the contract's subscriptions.
const pushServiceEndpoint = "https://push.example.net/wpush/v2/abc123"

// pushMaxPlaintext is the largest payload that every push service accepts: one aes128gcm
// record of 4096 bytes with no padding (RFC 8291 §4).
const pushMaxPlaintext = 3993

// PushSenderContract verifies an appnotification.PushSender against a fake push service and
// a fake browser, without the network. It checks that the server key is an uncompressed
// P-256 point in unpadded base64url; that Send posts one request to the subscription's
// endpoint with the Web Push purpose, the aes128gcm coding of RFC 8291 that the browser
// decrypts to the payload, a fresh salt and ephemeral key per message, the TTL and the Topic
// of RFC 8030, and a VAPID authorization of RFC 8292 (an ES256 JWT for the endpoint's origin
// that the server key verifies, valid for at most 24 hours, with a contact); that it accepts
// payloads up to 3993 bytes and refuses larger ones and invalid subscription keys with an
// error and without a request; and that it classifies the answers: 2xx delivered, 404 and
// 410 gone, 429 and 5xx retried after their Retry-After (seconds or an HTTP date), another
// 4xx rejected, a refused destination gone, and a transport failure retried, never an error.
func PushSenderContract(t *testing.T, newSender PushSenderFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *pushSenderEnv)
	}{
		{"ServerKeyIsAP256Point", pushServerKeyIsAPoint},
		{"PostsAnEncryptedMessage", pushPostsAnEncryptedMessage},
		{"SignsWithVAPID", pushSignsWithVAPID},
		{"UsesFreshKeysPerMessage", pushUsesFreshKeys},
		{"OmitsAnEmptyTopic", pushOmitsAnEmptyTopic},
		{"SendsTheLargestPayload", pushSendsTheLargestPayload},
		{"RefusesOversizedPayloads", pushRefusesOversizedPayloads},
		{"RefusesInvalidKeys", pushRefusesInvalidKeys},
		{"ClassifiesAnswers", pushClassifiesAnswers},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, newPushSenderEnv(t, newSender))
		})
	}
}

// pushSenderEnv is one case's sender, fake push service, and fake browser.
type pushSenderEnv struct {
	t       *testing.T
	clock   *testkit.Clock
	service *fakePushService
	sender  appnotification.PushSender
	browser *ecdh.PrivateKey
	auth    []byte
}

func newPushSenderEnv(t *testing.T, newSender PushSenderFactory) *pushSenderEnv {
	t.Helper()
	browser, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}
	e := &pushSenderEnv{
		t: t, clock: testkit.NewClock(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)),
		service: &fakePushService{}, browser: browser, auth: auth,
	}
	e.service.answer(http.StatusCreated, nil)
	e.sender = newSender(t, PushService{HTTP: e.service, Clock: e.clock})
	return e
}

// subscription is the fake browser's subscription.
func (e *pushSenderEnv) subscription() notification.PushSubscription {
	return notification.PushSubscription{
		ID: testkit.NamedID("sub"), AccountID: testkit.NamedID("acct"), TokenFamilyID: testkit.NamedID("fam"), Endpoint: pushServiceEndpoint,
		KeyP256DH: base64.RawURLEncoding.EncodeToString(e.browser.PublicKey().Bytes()),
		KeyAuth:   base64.RawURLEncoding.EncodeToString(e.auth),
		Alerts:    []notification.Type{notification.TypeMention},
	}
}

// send sends m to the fake browser and returns the result and the one request it made.
func (e *pushSenderEnv) send(m appnotification.PushMessage) (appnotification.PushResult, *kernel.HTTPRequest) {
	e.t.Helper()
	before := len(e.service.requests())
	res, err := e.sender.Send(e.t.Context(), e.subscription(), m)
	if err != nil {
		e.t.Fatalf("Send = %v", err)
	}
	reqs := e.service.requests()
	if len(reqs) != before+1 {
		e.t.Fatalf("Send made %d requests, want 1", len(reqs)-before)
	}
	return res, reqs[len(reqs)-1]
}

// decrypt is the browser's side of RFC 8291 §3.4 and RFC 8188: it returns the plaintext of a
// message body, its salt, and the sender's ephemeral public key.
func (e *pushSenderEnv) decrypt(body []byte) (plaintext, salt, ephemeral []byte) {
	e.t.Helper()
	if len(body) < 21 {
		e.t.Fatalf("a body of %d bytes has no aes128gcm header", len(body))
	}
	salt, rs, idLen := body[:16], binary.BigEndian.Uint32(body[16:20]), int(body[20])
	if idLen != 65 || len(body) < 21+idLen {
		e.t.Fatalf("the key ID is %d bytes, want an uncompressed P-256 point of 65", idLen)
	}
	if rs < 18 || len(body) > int(rs)+21+idLen {
		e.t.Fatalf("a body of %d bytes in records of %d is not one record", len(body), rs)
	}
	ephemeral = body[21 : 21+idLen]
	asPublic, err := ecdh.P256().NewPublicKey(ephemeral)
	if err != nil {
		e.t.Fatalf("the key ID is not a P-256 point: %v", err)
	}
	secret, err := e.browser.ECDH(asPublic)
	if err != nil {
		e.t.Fatal(err)
	}
	info := append(append([]byte("WebPush: info\x00"), e.browser.PublicKey().Bytes()...), ephemeral...)
	ikm, err := hkdf.Key(sha256.New, secret, e.auth, string(info), 32)
	if err != nil {
		e.t.Fatal(err)
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		e.t.Fatal(err)
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		e.t.Fatal(err)
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		e.t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		e.t.Fatal(err)
	}
	plain, err := gcm.Open(nil, nonce, body[21+idLen:], nil)
	if err != nil {
		e.t.Fatalf("the browser cannot decrypt the message: %v", err)
	}
	// The last record ends with the delimiter 0x02, then optional zero padding (RFC 8188 §2).
	plain = bytes.TrimRight(plain, "\x00")
	if len(plain) == 0 || plain[len(plain)-1] != 0x02 {
		e.t.Fatal("the record does not end with the last record's delimiter")
	}
	return plain[:len(plain)-1], salt, ephemeral
}

// fakePushService is a push service in memory, behind kernel.HTTPClient.
type fakePushService struct {
	mu    sync.Mutex
	reqs  []*kernel.HTTPRequest
	reply func(*kernel.HTTPRequest) (*kernel.HTTPResponse, error)
}

func (s *fakePushService) Do(_ context.Context, r *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *r
	c.Header = r.Header.Clone()
	c.Body = append([]byte(nil), r.Body...)
	s.reqs = append(s.reqs, &c)
	return s.reply(r)
}

func (s *fakePushService) requests() []*kernel.HTTPRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*kernel.HTTPRequest(nil), s.reqs...)
}

func (s *fakePushService) setReply(reply func(*kernel.HTTPRequest) (*kernel.HTTPResponse, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reply = reply
}

// answer makes the service answer every request with code and header.
func (s *fakePushService) answer(code int, header http.Header) {
	s.setReply(func(r *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
		h := http.Header{}
		for k, v := range header {
			h[k] = v
		}
		return &kernel.HTTPResponse{StatusCode: code, Header: h, URL: r.URL, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
}

func (s *fakePushService) fail(err error) {
	s.setReply(func(*kernel.HTTPRequest) (*kernel.HTTPResponse, error) { return nil, err })
}

func pushServerKeyIsAPoint(t *testing.T, e *pushSenderEnv) {
	key := e.sender.ServerKey()
	b, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil || strings.HasSuffix(key, "=") {
		t.Fatalf("ServerKey = %q, not unpadded base64url: %v", key, err)
	}
	if _, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), b); err != nil {
		t.Fatalf("ServerKey is not an uncompressed P-256 point: %v", err)
	}
	if again := e.sender.ServerKey(); again != key {
		t.Errorf("ServerKey changed from %q to %q", key, again)
	}
}

func pushPostsAnEncryptedMessage(t *testing.T, e *pushSenderEnv) {
	payload := []byte(`{"title":"こんにちは","body":"hi"}`)
	res, r := e.send(appnotification.PushMessage{Payload: payload, TTL: 48 * time.Hour, Topic: "abc"})
	if res.Outcome != appnotification.PushDelivered || res.StatusCode != http.StatusCreated {
		t.Errorf("Send = %+v, want delivered with 201", res)
	}
	if r.Method != http.MethodPost || r.URL != pushServiceEndpoint || r.Purpose != kernel.HTTPPurposeWebPush {
		t.Errorf("request %s %s for purpose %v, want POST %s for HTTPPurposeWebPush", r.Method, r.URL, r.Purpose, pushServiceEndpoint)
	}
	for k, want := range map[string]string{"Content-Encoding": "aes128gcm", "TTL": "172800", "Topic": "abc"} {
		if got := r.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if got, _, _ := e.decrypt(r.Body); !bytes.Equal(got, payload) {
		t.Errorf("the browser decrypts %q, want %q", got, payload)
	}
}

func pushSignsWithVAPID(t *testing.T, e *pushSenderEnv) {
	_, r := e.send(appnotification.PushMessage{Payload: []byte("{}"), TTL: time.Hour})
	auth := r.Header.Get("Authorization")
	params, ok := strings.CutPrefix(auth, "vapid ")
	if !ok {
		t.Fatalf("Authorization = %q, want the vapid scheme", auth)
	}
	var token, key string
	for p := range strings.SplitSeq(params, ",") {
		name, value, _ := strings.Cut(strings.TrimSpace(p), "=")
		switch name {
		case "t":
			token = value
		case "k":
			key = value
		}
	}
	if key != e.sender.ServerKey() {
		t.Errorf("Authorization k = %q, want the server key %q", key, e.sender.ServerKey())
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("Authorization t = %q, not a JWS", token)
	}
	decode := func(s string) []byte {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			t.Fatalf("a JWT part %q is not base64url: %v", s, err)
		}
		return b
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(decode(parts[0]), &header); err != nil || header.Alg != "ES256" {
		t.Errorf("JWT header %s, %v; want alg ES256", decode(parts[0]), err)
	}
	var claims struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(decode(parts[1]), &claims); err != nil {
		t.Fatalf("JWT claims %s: %v", decode(parts[1]), err)
	}
	now := e.clock.Now()
	if claims.Aud != "https://push.example.net" {
		t.Errorf("aud = %q, want the endpoint's origin", claims.Aud)
	}
	if exp := time.Unix(claims.Exp, 0); !exp.After(now) || exp.After(now.Add(24*time.Hour)) {
		t.Errorf("exp = %v, want after %v and at most 24 hours later", exp, now)
	}
	if !strings.HasPrefix(claims.Sub, "https:") && !strings.HasPrefix(claims.Sub, "mailto:") {
		t.Errorf("sub = %q, want an https URL or a mailto: URI", claims.Sub)
	}
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), decode(e.sender.ServerKey()))
	if err != nil {
		t.Fatal(err)
	}
	sig := decode(parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(sig) != 64 || !ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Error("the server key does not verify the VAPID signature")
	}
}

func pushUsesFreshKeys(t *testing.T, e *pushSenderEnv) {
	m := appnotification.PushMessage{Payload: []byte(`{"n":1}`), TTL: time.Hour}
	_, first := e.send(m)
	_, second := e.send(m)
	p1, salt1, key1 := e.decrypt(first.Body)
	p2, salt2, key2 := e.decrypt(second.Body)
	if !bytes.Equal(p1, m.Payload) || !bytes.Equal(p2, m.Payload) {
		t.Errorf("the browser decrypts %q and %q, want %q", p1, p2, m.Payload)
	}
	if bytes.Equal(salt1, salt2) || bytes.Equal(key1, key2) {
		t.Error("two messages share a salt or an ephemeral key (RFC 8291 §3.4)")
	}
}

func pushOmitsAnEmptyTopic(t *testing.T, e *pushSenderEnv) {
	_, r := e.send(appnotification.PushMessage{Payload: []byte("{}"), TTL: 90 * time.Second})
	if _, ok := r.Header["Topic"]; ok {
		t.Errorf("Topic = %q, want none", r.Header.Get("Topic"))
	}
	if got := r.Header.Get("TTL"); got != "90" {
		t.Errorf("TTL = %q, want 90", got)
	}
}

func pushSendsTheLargestPayload(t *testing.T, e *pushSenderEnv) {
	payload := bytes.Repeat([]byte("a"), pushMaxPlaintext)
	res, r := e.send(appnotification.PushMessage{Payload: payload, TTL: time.Hour})
	if res.Outcome != appnotification.PushDelivered {
		t.Errorf("Send = %+v, want delivered", res)
	}
	if len(r.Body) > 4096+86 {
		t.Errorf("a body of %d bytes: the record is larger than 4096 bytes", len(r.Body))
	}
	if got, _, _ := e.decrypt(r.Body); !bytes.Equal(got, payload) {
		t.Errorf("the browser decrypts %d bytes, want %d", len(got), len(payload))
	}
}

func pushRefusesOversizedPayloads(t *testing.T, e *pushSenderEnv) {
	m := appnotification.PushMessage{Payload: bytes.Repeat([]byte("a"), pushMaxPlaintext+1), TTL: time.Hour}
	if res, err := e.sender.Send(t.Context(), e.subscription(), m); err == nil {
		t.Errorf("Send of %d bytes = %+v, want an error", len(m.Payload), res)
	}
	if n := len(e.service.requests()); n != 0 {
		t.Errorf("Send made %d requests, want none", n)
	}
}

func pushRefusesInvalidKeys(t *testing.T, e *pushSenderEnv) {
	valid := e.subscription()
	b64 := base64.RawURLEncoding.EncodeToString
	for _, tc := range []struct {
		name         string
		p256dh, auth string
	}{
		{"p256dh not base64url", "not base64!", valid.KeyAuth},
		{"p256dh not a point", b64(make([]byte, 65)), valid.KeyAuth},
		{"p256dh compressed", b64(append([]byte{0x02}, e.browser.PublicKey().Bytes()[1:33]...)), valid.KeyAuth},
		{"auth of 15 bytes", valid.KeyP256DH, b64(make([]byte, 15))},
		{"auth missing", valid.KeyP256DH, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := valid
			s.KeyP256DH, s.KeyAuth = tc.p256dh, tc.auth
			before := len(e.service.requests())
			res, err := e.sender.Send(t.Context(), s, appnotification.PushMessage{Payload: []byte("{}"), TTL: time.Hour})
			if !errors.Is(err, notification.ErrInvalidPushKey) {
				t.Errorf("Send = %+v, %v; want ErrInvalidPushKey", res, err)
			}
			if n := len(e.service.requests()) - before; n != 0 {
				t.Errorf("Send made %d requests, want none", n)
			}
		})
	}
}

func pushClassifiesAnswers(t *testing.T, e *pushSenderEnv) {
	now := e.clock.Now()
	for _, tc := range []struct {
		name   string
		code   int
		header http.Header
		err    error
		want   appnotification.PushResult
	}{
		{name: "created", code: http.StatusCreated, want: appnotification.PushResult{Outcome: appnotification.PushDelivered, StatusCode: 201}},
		{name: "accepted", code: http.StatusAccepted, want: appnotification.PushResult{Outcome: appnotification.PushDelivered, StatusCode: 202}},
		{name: "not found", code: http.StatusNotFound, want: appnotification.PushResult{Outcome: appnotification.PushGone, StatusCode: 404}},
		{name: "gone", code: http.StatusGone, want: appnotification.PushResult{Outcome: appnotification.PushGone, StatusCode: 410}},
		{
			name: "too many requests in seconds", code: http.StatusTooManyRequests, header: http.Header{"Retry-After": {"120"}},
			want: appnotification.PushResult{Outcome: appnotification.PushRetry, StatusCode: 429, RetryAfter: 2 * time.Minute},
		},
		{
			name: "unavailable until a date", code: http.StatusServiceUnavailable,
			header: http.Header{"Retry-After": {now.Add(time.Hour).Format(http.TimeFormat)}},
			want:   appnotification.PushResult{Outcome: appnotification.PushRetry, StatusCode: 503, RetryAfter: time.Hour},
		},
		{name: "server error", code: http.StatusInternalServerError, want: appnotification.PushResult{Outcome: appnotification.PushRetry, StatusCode: 500}},
		{name: "bad request", code: http.StatusBadRequest, want: appnotification.PushResult{Outcome: appnotification.PushRejected, StatusCode: 400}},
		{name: "payload too large", code: http.StatusRequestEntityTooLarge, want: appnotification.PushResult{Outcome: appnotification.PushRejected, StatusCode: 413}},
		{name: "forbidden destination", err: fmt.Errorf("dial: %w", kernel.ErrForbiddenDestination), want: appnotification.PushResult{Outcome: appnotification.PushGone}},
		{name: "invalid request", err: fmt.Errorf("url: %w", kernel.ErrInvalidHTTPRequest), want: appnotification.PushResult{Outcome: appnotification.PushGone}},
		{name: "network failure", err: errors.New("connection reset"), want: appnotification.PushResult{Outcome: appnotification.PushRetry}},
		{name: "timeout", err: context.DeadlineExceeded, want: appnotification.PushResult{Outcome: appnotification.PushRetry}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err != nil {
				e.service.fail(tc.err)
			} else {
				e.service.answer(tc.code, tc.header)
			}
			if got, _ := e.send(appnotification.PushMessage{Payload: []byte("{}"), TTL: time.Hour}); got != tc.want {
				t.Errorf("Send = %+v, want %+v", got, tc.want)
			}
		})
	}
}
