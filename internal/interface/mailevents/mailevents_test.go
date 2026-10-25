// SPDX-License-Identifier: AGPL-3.0-only

package mailevents

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/pkg/testkit"
)

const (
	testTopic   = "arn:aws:sns:us-west-2:123456789012:MyTopic"
	testCertURL = "https://sns.us-west-2.amazonaws.com/SimpleNotificationService-0000000000000000000000.pem"
)

// signer signs SNS messages as SNS does, with the key of the certificate at testCertURL.
type signer struct {
	key     *rsa.PrivateKey
	certPEM []byte
}

func newSigner(t *testing.T) *signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "sns.amazonaws.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &signer{key: key, certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

// sign fills in the signature fields of m with SignatureVersion 2.
func (s *signer) sign(t *testing.T, m *snsMessage) {
	t.Helper()
	m.SignatureVersion = "2"
	if m.SigningCertURL == "" {
		m.SigningCertURL = testCertURL
	}
	str, ok := m.stringToSign()
	if !ok {
		t.Fatalf("no string to sign of type %q", m.Type)
	}
	digest := sha256.Sum256([]byte(str))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	m.Signature = base64.StdEncoding.EncodeToString(sig)
}

// fakeHTTP answers GETs from a map of URL to body and records the URLs it was asked for.
type fakeHTTP struct {
	mu     sync.Mutex
	bodies map[string][]byte
	got    []string
}

func (f *fakeHTTP) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, req.URL)
	body, ok := f.bodies[req.URL]
	status := http.StatusOK
	if !ok {
		status = http.StatusNotFound
	}
	return &kernel.HTTPResponse{StatusCode: status, URL: req.URL, Body: io.NopCloser(bytes.NewReader(body))}, nil
}

func (f *fakeHTTP) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.got...)
}

// memRecorder records failures in memory with a unique feedback key per row, as the identity
// module does, for the addresses it knows.
type memRecorder struct {
	known map[string]bool
	rows  map[string]appidentity.AddressFailure
	err   error
}

func newRecorder(addresses ...string) *memRecorder {
	r := &memRecorder{known: map[string]bool{}, rows: map[string]appidentity.AddressFailure{}}
	for _, a := range addresses {
		r.known[identity.NormalizeEmail(a)] = true
	}
	return r
}

func (r *memRecorder) RecordAddressFailure(_ context.Context, in appidentity.AddressFailure) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	for _, a := range in.Addresses {
		if r.known[identity.NormalizeEmail(a)] {
			if _, ok := r.rows[in.FeedbackKey]; !ok {
				r.rows[in.FeedbackKey] = in
			}
			return true, nil
		}
	}
	return false, nil
}

type env struct {
	signer *signer
	http   *fakeHTTP
	rec    *memRecorder
	kv     *testkit.KeyValueStore
	mux    *http.ServeMux
}

func newEnv(t *testing.T, rec *memRecorder) *env {
	t.Helper()
	s := newSigner(t)
	clock := testkit.NewClock(time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC))
	e := &env{signer: s, http: &fakeHTTP{bodies: map[string][]byte{testCertURL: s.certPEM}}, rec: rec, kv: testkit.NewKeyValueStore(clock)}
	topic, err := ParseTopicARN(testTopic)
	if err != nil {
		t.Fatal(err)
	}
	e.mux = http.NewServeMux()
	New(Options{
		Topic: topic, Recorder: rec, HTTPClient: e.http, KV: e.kv, Clock: clock,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Register(e.mux)
	return e
}

func (e *env) post(t *testing.T, body []byte) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, Path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "text/plain; charset=UTF-8")
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec.Code
}

func (e *env) postMessage(t *testing.T, m *snsMessage) int {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return e.post(t, b)
}

// notification returns an unsigned SNS notification of the topic carrying sesMessage.
func notification(sesMessage string) *snsMessage {
	return &snsMessage{
		Type: typeNotification, MessageID: "22b80b92-fdea-4c2c-8f9d-bdfb0c7bf324", TopicArn: testTopic,
		Message: sesMessage, Timestamp: "2026-10-25T00:00:00.000Z",
	}
}

// The example notifications of the SES documentation
// (https://docs.aws.amazon.com/ses/latest/dg/notification-examples.html and
// https://docs.aws.amazon.com/ses/latest/dg/event-publishing-retrieving-sns-examples.html).
const (
	exampleBounce = `{
  "notificationType":"Bounce",
  "bounce":{
    "bounceType":"Permanent",
    "reportingMTA":"dns; email.example.com",
    "bouncedRecipients":[
      {"emailAddress":"jane@example.com","status":"5.1.1","action":"failed","diagnosticCode":"smtp; 550 5.1.1 <jane@example.com>... User"}
    ],
    "bounceSubType":"General",
    "timestamp":"2016-01-27T14:59:38.237Z",
    "feedbackId":"00000138111222aa-33322211-cccc-cccc-cccc-ddddaaaa068a-000000",
    "remoteMtaIp":"127.0.2.0"
  },
  "mail":{
    "timestamp":"2016-01-27T14:59:38.237Z",
    "source":"john@example.com",
    "sourceArn":"arn:aws:ses:us-east-1:888888888888:identity/example.com",
    "sourceIp":"127.0.3.0",
    "sendingAccountId":"123456789012",
    "callerIdentity":"IAM_user_or_role_name",
    "messageId":"00000138111222aa-33322211-cccc-cccc-cccc-ddddaaaa0680-000000",
    "destination":["jane@example.com","mary@example.com","richard@example.com"]
  }
}`
	exampleComplaint = `{
  "notificationType":"Complaint",
  "complaint":{
    "userAgent":"AnyCompany Feedback Loop (V0.01)",
    "complainedRecipients":[{"emailAddress":"richard@example.com"}],
    "complaintFeedbackType":"abuse",
    "arrivalDate":"2016-01-27T14:59:38.237Z",
    "timestamp":"2016-01-27T14:59:38.237Z",
    "feedbackId":"000001378603177f-18c07c78-fa81-4a58-9dd1-fedc3cb8f49a-000000"
  },
  "mail":{
    "timestamp":"2016-01-27T14:59:38.237Z",
    "messageId":"000001378603177f-7a5433e7-8edb-42ae-af10-f0181f34d6ee-000000",
    "source":"john@example.com",
    "destination":["jane@example.com","mary@example.com","richard@example.com"]
  }
}`
	exampleSuppressed = `{
  "eventType":"Bounce",
  "bounce":{
    "feedbackId":"0100017f8ab5f2f6-1ec5f4e5-0c2f-4f66-9b07-0c8e4d6f0c3c-000000",
    "bounceType":"Permanent",
    "bounceSubType":"OnAccountSuppressionList",
    "bouncedRecipients":[
      {"emailAddress":"recipient@example.com","action":"failed","status":"5.1.1","diagnosticCode":"Amazon SES did not send the message to this address because it is on the suppression list for your account."}
    ],
    "timestamp":"2017-08-05T00:41:02.669Z",
    "reportingMTA":"dns; amazonses.com"
  },
  "mail":{
    "timestamp":"2017-08-05T00:41:02.669Z",
    "source":"Sender Name <sender@example.com>",
    "messageId":"EXAMPLE7c191be45-e9aedb9a-02f9-4d12-a87d-dd0099a07f8a-000000",
    "destination":["recipient@example.com"]
  }
}`
	exampleTransient = `{
  "notificationType":"Bounce",
  "bounce":{
    "bounceType":"Transient","bounceSubType":"MailboxFull",
    "bouncedRecipients":[{"emailAddress":"jane@example.com","status":"4.2.2"}],
    "timestamp":"2016-01-27T14:59:38.237Z","feedbackId":"transient-1"
  }
}`
	exampleUndetermined = `{
  "eventType":"Bounce",
  "bounce":{
    "bounceType":"Undetermined","bounceSubType":"Undetermined",
    "bouncedRecipients":[{"emailAddress":"jane@example.com"}],
    "timestamp":"2016-01-27T14:59:38.237Z","feedbackId":"undetermined-1"
  }
}`
	exampleDelivery = `{
  "notificationType":"Delivery",
  "delivery":{"timestamp":"2016-01-27T14:59:38.237Z","recipients":["jane@example.com"]}
}`
)

func TestRecordsTheExampleNotifications(t *testing.T) {
	for _, tc := range []struct {
		name, message, key, status string
		kind                       identity.EmailFailureKind
		at                         time.Time
	}{
		{"bounce", exampleBounce, "00000138111222aa-33322211-cccc-cccc-cccc-ddddaaaa068a-000000", "5.1.1", identity.EmailHardBounce,
			time.Date(2016, 1, 27, 14, 59, 38, 237e6, time.UTC)},
		{"complaint", exampleComplaint, "000001378603177f-18c07c78-fa81-4a58-9dd1-fedc3cb8f49a-000000", "", identity.EmailComplaint,
			time.Date(2016, 1, 27, 14, 59, 38, 237e6, time.UTC)},
		{"OnAccountSuppressionList", exampleSuppressed, "0100017f8ab5f2f6-1ec5f4e5-0c2f-4f66-9b07-0c8e4d6f0c3c-000000", "5.1.1", identity.EmailHardBounce,
			time.Date(2017, 8, 5, 0, 41, 2, 669e6, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, newRecorder("Jane@example.com", "richard@example.com", "recipient@example.com"))
			m := notification(tc.message)
			e.signer.sign(t, m)
			if code := e.postMessage(t, m); code != http.StatusOK {
				t.Fatalf("status %d, want 200", code)
			}
			// SNS delivers the same notification again: still one row.
			if code := e.postMessage(t, m); code != http.StatusOK {
				t.Fatalf("status of the second delivery %d, want 200", code)
			}
			if len(e.rec.rows) != 1 {
				t.Fatalf("recorded %d rows, want 1: %+v", len(e.rec.rows), e.rec.rows)
			}
			got, ok := e.rec.rows[tc.key]
			if !ok || got.Kind != tc.kind || got.Source != identity.EmailFailureSES || got.EnhancedStatus != tc.status || !got.OccurredAt.Equal(tc.at) {
				t.Errorf("recorded %+v, want %s %s with status %q at %s", e.rec.rows, tc.key, tc.kind, tc.status, tc.at)
			}
			// The certificate came once, then from the cache.
			if reqs := e.http.requests(); len(reqs) != 1 || reqs[0] != testCertURL {
				t.Errorf("requests %v, want one of the certificate", reqs)
			}
		})
	}
}

func TestIgnoresOtherNotifications(t *testing.T) {
	for name, message := range map[string]string{
		"transient": exampleTransient, "undetermined": exampleUndetermined, "delivery": exampleDelivery,
		"not-spam": strings.Replace(exampleComplaint, `"abuse"`, `"not-spam"`, 1), "not SES": "hello",
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, newRecorder("jane@example.com", "richard@example.com"))
			m := notification(message)
			e.signer.sign(t, m)
			if code := e.postMessage(t, m); code != http.StatusOK {
				t.Fatalf("status %d, want 200", code)
			}
			if len(e.rec.rows) != 0 {
				t.Errorf("recorded %+v, want nothing", e.rec.rows)
			}
		})
	}
}

func TestUnknownAddressRecordsNothing(t *testing.T) {
	e := newEnv(t, newRecorder("someone@example.com"))
	m := notification(exampleBounce)
	e.signer.sign(t, m)
	if code := e.postMessage(t, m); code != http.StatusOK || len(e.rec.rows) != 0 {
		t.Errorf("status %d, rows %+v; want 200 and nothing", code, e.rec.rows)
	}
}

func TestPunycodeDomainMatchesTheUnicodeAddress(t *testing.T) {
	e := newEnv(t, newRecorder("jane@例え.jp"))
	m := notification(strings.ReplaceAll(exampleBounce, "jane@example.com", "jane@xn--r8jz45g.jp"))
	e.signer.sign(t, m)
	if code := e.postMessage(t, m); code != http.StatusOK || len(e.rec.rows) != 1 {
		t.Errorf("status %d, rows %+v; want 200 and one", code, e.rec.rows)
	}
}

func TestRejectsWhatIsNotASignedMessageOfTheTopic(t *testing.T) {
	other := newSigner(t)
	for _, tc := range []struct {
		name  string
		build func(t *testing.T, e *env) *snsMessage
	}{
		{"wrong signature", func(t *testing.T, _ *env) *snsMessage {
			m := notification(exampleBounce)
			other.sign(t, m)
			return m
		}},
		{"tampered message", func(t *testing.T, e *env) *snsMessage {
			m := notification(exampleComplaint)
			e.signer.sign(t, m)
			m.Message = exampleBounce
			return m
		}},
		{"added subject", func(t *testing.T, e *env) *snsMessage {
			m := notification(exampleBounce)
			e.signer.sign(t, m)
			subject := "x"
			m.Subject = &subject
			return m
		}},
		{"other topic", func(t *testing.T, e *env) *snsMessage {
			m := notification(exampleBounce)
			m.TopicArn = "arn:aws:sns:us-west-2:123456789012:OtherTopic"
			e.signer.sign(t, m)
			return m
		}},
		{"SignatureVersion 1", func(t *testing.T, e *env) *snsMessage {
			m := notification(exampleBounce)
			e.signer.sign(t, m)
			m.SignatureVersion = "1"
			return m
		}},
		{"certificate on another host", func(t *testing.T, e *env) *snsMessage {
			m := notification(exampleBounce)
			m.SigningCertURL = "https://sns.us-west-2.amazonaws.com.evil.example/cert.pem"
			e.http.bodies[m.SigningCertURL] = e.signer.certPEM
			e.signer.sign(t, m)
			return m
		}},
		{"certificate of another region", func(t *testing.T, e *env) *snsMessage {
			m := notification(exampleBounce)
			m.SigningCertURL = "https://sns.us-east-1.amazonaws.com/SimpleNotificationService-0.pem"
			e.http.bodies[m.SigningCertURL] = e.signer.certPEM
			e.signer.sign(t, m)
			return m
		}},
		{"certificate over http", func(t *testing.T, e *env) *snsMessage {
			m := notification(exampleBounce)
			m.SigningCertURL = "http://sns.us-west-2.amazonaws.com/SimpleNotificationService-0.pem"
			e.http.bodies[m.SigningCertURL] = e.signer.certPEM
			e.signer.sign(t, m)
			return m
		}},
		{"unknown type", func(t *testing.T, e *env) *snsMessage {
			m := notification(exampleBounce)
			e.signer.sign(t, m)
			m.Type = "Other"
			return m
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, newRecorder("jane@example.com", "richard@example.com"))
			m := tc.build(t, e)
			if code := e.postMessage(t, m); code != http.StatusBadRequest {
				t.Errorf("status %d, want 400", code)
			}
			if len(e.rec.rows) != 0 {
				t.Errorf("recorded %+v, want nothing", e.rec.rows)
			}
			for _, u := range e.http.requests() {
				if u != testCertURL {
					t.Errorf("fetched %s, want only the certificate of the topic's host", u)
				}
			}
		})
	}
	t.Run("not JSON", func(t *testing.T) {
		e := newEnv(t, newRecorder())
		if code := e.post(t, []byte("not json")); code != http.StatusBadRequest {
			t.Errorf("status %d, want 400", code)
		}
	})
}

func TestBodyLimit(t *testing.T) {
	e := newEnv(t, newRecorder("jane@example.com"))
	if code := e.post(t, bytes.Repeat([]byte("x"), MaxBodyBytes+1)); code != http.StatusRequestEntityTooLarge {
		t.Errorf("a body over the limit: status %d, want 413", code)
	}
	if reqs := e.http.requests(); len(reqs) != 0 {
		t.Errorf("a body over the limit fetched %v", reqs)
	}
}

func TestSubscriptionConfirmation(t *testing.T) {
	const subscribe = "https://sns.us-west-2.amazonaws.com/?Action=ConfirmSubscription&TopicArn=arn:aws:sns:us-west-2:123456789012:MyTopic&Token=2336412f37"
	confirmation := func() *snsMessage {
		return &snsMessage{ //nolint:gosec // G101: the Token of the AWS documentation's example, not a credential
			Type: typeSubscriptionConfirmation, MessageID: "165545c9-2a5c-472c-8df2-7ff2be2b3b1b", Token: "2336412f37",
			TopicArn: testTopic, Message: "You have chosen to subscribe to the topic " + testTopic + ".",
			SubscribeURL: subscribe, Timestamp: "2026-10-25T00:00:00.000Z",
		}
	}
	t.Run("confirms", func(t *testing.T) {
		e := newEnv(t, newRecorder())
		e.http.bodies[subscribe] = []byte("<ConfirmSubscriptionResponse/>")
		m := confirmation()
		e.signer.sign(t, m)
		if code := e.postMessage(t, m); code != http.StatusOK {
			t.Fatalf("status %d, want 200", code)
		}
		if reqs := e.http.requests(); len(reqs) != 2 || reqs[1] != subscribe {
			t.Errorf("requests %v, want the certificate and the SubscribeURL", reqs)
		}
	})
	t.Run("SNS does not confirm", func(t *testing.T) {
		e := newEnv(t, newRecorder())
		m := confirmation()
		e.signer.sign(t, m)
		if code := e.postMessage(t, m); code != http.StatusServiceUnavailable {
			t.Errorf("status %d, want 503", code)
		}
	})
	t.Run("SubscribeURL on another host", func(t *testing.T) {
		e := newEnv(t, newRecorder())
		m := confirmation()
		m.SubscribeURL = "https://evil.example/?Action=ConfirmSubscription"
		e.http.bodies[m.SubscribeURL] = nil
		e.signer.sign(t, m)
		if code := e.postMessage(t, m); code != http.StatusBadRequest {
			t.Errorf("status %d, want 400", code)
		}
		if reqs := e.http.requests(); len(reqs) != 1 {
			t.Errorf("requests %v, want the certificate only", reqs)
		}
	})
}

func TestUnavailable(t *testing.T) {
	t.Run("certificate not fetched", func(t *testing.T) {
		e := newEnv(t, newRecorder("jane@example.com"))
		delete(e.http.bodies, testCertURL)
		m := notification(exampleBounce)
		e.signer.sign(t, m)
		if code := e.postMessage(t, m); code != http.StatusServiceUnavailable || len(e.rec.rows) != 0 {
			t.Errorf("status %d, rows %+v; want 503 and nothing", code, e.rec.rows)
		}
	})
	t.Run("not recorded", func(t *testing.T) {
		rec := newRecorder("jane@example.com")
		rec.err = errors.New("database is down")
		e := newEnv(t, rec)
		m := notification(exampleBounce)
		e.signer.sign(t, m)
		if code := e.postMessage(t, m); code != http.StatusServiceUnavailable {
			t.Errorf("status %d, want 503", code)
		}
	})
}

func TestParseTopicARN(t *testing.T) {
	for _, ok := range []string{testTopic, "arn:aws-us-gov:sns:us-gov-west-1:123456789012:my_topic-1"} {
		if _, err := ParseTopicARN(ok); err != nil {
			t.Errorf("ParseTopicARN(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "arn:aws:sqs:us-west-2:123456789012:q", "arn:aws:sns::123456789012:t", "arn:aws:sns:us-west-2:1234:t", "arn:aws:sns:us-west-2:123456789012:"} {
		if _, err := ParseTopicARN(bad); err == nil {
			t.Errorf("ParseTopicARN(%q) accepted", bad)
		}
	}
}
