// SPDX-License-Identifier: AGPL-3.0-only

// Package fake is a fake Web Push service (RFC 8030) for the end-to-end tests and the local
// demonstration: it stands in for a browser vendor's push service, which the test browsers cannot
// reach. Only the test builds (build tag e2e_fakes) link it, as the command my dev push-service.
//
// It issues subscriptions for an application server key, receives push messages as a push
// service does (it verifies the VAPID authorization of RFC 8292 and decrypts the aes128gcm body of
// RFC 8291 with the keys it issued), expires subscriptions, and lets the tests read what each
// subscription received. Its state lives in memory only. The receiving side is written apart from
// internal/adapter/webpush, the sending side, so that the two do not share a mistake.
//
// HTTP API (JSON):
//
//	POST /subscriptions                 {"applicationServerKey": "<base64url>"} -> 201 PushSubscriptionJSON
//	POST /push/{id}                     a push message (RFC 8030 §5) -> 201, 400, 401, 403, 404, 410, 413
//	POST /subscriptions/{id}/expire     later pushes to the subscription answer 410 -> 204
//	GET  /subscriptions/{id}/messages   {"messages": [Message...]} -> 200
package fake

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MaxBodyBytes is the largest message body the fake accepts; a push service must accept 4096
// bytes (RFC 8030 §7.2, RFC 8291 §4) and may refuse more with 413.
const MaxBodyBytes = 4096

// ErrNotFound reports an unknown subscription.
var ErrNotFound = errors.New("no such subscription")

// Options configure a Service.
type Options struct {
	// Origin is the push service's origin as the subscriptions' endpoints and the aud claim of
	// VAPID tokens name it, such as https://localhost:8930 (required).
	Origin string
	// Now returns the current time; nil means time.Now.
	Now func() time.Time
	// Random is the source of the keys and identifiers; nil means crypto/rand.
	Random io.Reader
}

// Service is the fake push service. It is safe for concurrent use.
type Service struct {
	o    Options
	mu   sync.Mutex
	subs map[string]*subscription
}

type subscription struct {
	serverKey []byte
	key       *ecdh.PrivateKey
	auth      []byte
	expired   bool
	messages  []Message
}

// Subscription is an issued subscription in the shape of the browser's PushSubscriptionJSON.
type Subscription struct {
	Endpoint       string           `json:"endpoint"`
	ExpirationTime *int64           `json:"expirationTime"`
	Keys           SubscriptionKeys `json:"keys"`
}

// SubscriptionKeys are the keys of a Subscription, in unpadded base64url.
type SubscriptionKeys struct {
	P256DH string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// Message is a push message the fake received, accepted or not.
type Message struct {
	// ReceivedAt is when it arrived.
	ReceivedAt time.Time `json:"received_at"`
	// Status is the status code the fake answered.
	Status int `json:"status"`
	// Error is why the fake refused it; empty when it was accepted.
	Error string `json:"error,omitempty"`
	// Plaintext is the decrypted payload of an accepted message.
	Plaintext string `json:"plaintext,omitempty"`
	// TTL, Urgency, and Topic are the request's header fields (RFC 8030 §5.2-§5.4).
	TTL     int    `json:"ttl"`
	Urgency string `json:"urgency,omitempty"`
	Topic   string `json:"topic,omitempty"`
	// Subject is the sub claim of the verified VAPID token.
	Subject string `json:"subject,omitempty"`
}

// New returns a Service with o.
func New(o Options) (*Service, error) {
	if o.Origin == "" || strings.HasSuffix(o.Origin, "/") {
		return nil, errors.New("fake push service: the origin is required, without a trailing slash")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Random == nil {
		o.Random = rand.Reader
	}
	return &Service{o: o, subs: map[string]*subscription{}}, nil
}

// Subscribe issues a subscription for the application server key applicationServerKey, an
// uncompressed P-256 point in base64url, as PushManager.subscribe does in a browser.
func (s *Service) Subscribe(applicationServerKey string) (Subscription, error) {
	serverKey, err := decodeB64(applicationServerKey)
	if err != nil {
		return Subscription{}, fmt.Errorf("applicationServerKey is not base64url: %w", err)
	}
	if _, err := ecdh.P256().NewPublicKey(serverKey); err != nil {
		return Subscription{}, fmt.Errorf("applicationServerKey is not an uncompressed P-256 public key: %w", err)
	}
	key, err := ecdh.P256().GenerateKey(s.o.Random)
	if err != nil {
		return Subscription{}, err
	}
	auth := make([]byte, 16)
	idBytes := make([]byte, 16)
	if _, err := io.ReadFull(s.o.Random, auth); err != nil {
		return Subscription{}, err
	}
	if _, err := io.ReadFull(s.o.Random, idBytes); err != nil {
		return Subscription{}, err
	}
	id := base64.RawURLEncoding.EncodeToString(idBytes)
	s.mu.Lock()
	s.subs[id] = &subscription{serverKey: serverKey, key: key, auth: auth}
	s.mu.Unlock()
	return Subscription{
		Endpoint: s.o.Origin + "/push/" + id,
		Keys: SubscriptionKeys{
			P256DH: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
			Auth:   base64.RawURLEncoding.EncodeToString(auth),
		},
	}, nil
}

// Expire expires the subscription id: later pushes to it answer 410.
func (s *Service) Expire(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[id]
	if !ok {
		return ErrNotFound
	}
	sub.expired = true
	return nil
}

// Messages returns the push messages that the subscription id received, oldest first.
func (s *Service) Messages(id string) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]Message(nil), sub.messages...), nil
}

// Handler returns the HTTP API of the fake.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /subscriptions", s.serveSubscribe)
	mux.HandleFunc("POST /subscriptions/{id}/expire", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Expire(r.PathValue("id")); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /subscriptions/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		msgs, err := s.Messages(r.PathValue("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if msgs == nil {
			msgs = []Message{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
	})
	mux.HandleFunc("POST /push/{id}", s.servePush)
	return mux
}

func (s *Service) serveSubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ApplicationServerKey string `json:"applicationServerKey"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "the body must be JSON with applicationServerKey", http.StatusBadRequest)
		return
	}
	sub, err := s.Subscribe(req.ApplicationServerKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, sub)
}

// topicPattern is a Topic header field: at most 32 characters of the base64url alphabet
// (RFC 8030 §5.4).
var topicPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// servePush receives a push message (RFC 8030 §5). It records every request to a known
// subscription, accepted or not, with the result of its checks.
func (s *Service) servePush(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	sub, ok := s.subs[id]
	expired := ok && sub.expired
	s.mu.Unlock()
	switch {
	case !ok:
		http.Error(w, ErrNotFound.Error(), http.StatusNotFound)
		return
	case expired:
		s.record(sub, Message{Status: http.StatusGone, Error: "the subscription expired"})
		http.Error(w, "the subscription expired", http.StatusGone)
		return
	}

	m, plaintextErr := s.receive(r, sub)
	m.ReceivedAt = s.o.Now()
	if plaintextErr != nil {
		m.Error = plaintextErr.Error()
	}
	n := s.record(sub, m)
	if m.Status != http.StatusCreated {
		http.Error(w, m.Error, m.Status)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("%s/push/%s/messages/%d", s.o.Origin, id, n))
	w.WriteHeader(http.StatusCreated)
}

// receive checks and decrypts the push message of r to sub. It returns the message with the
// status code to answer, and the reason when that is not 201.
func (s *Service) receive(r *http.Request, sub *subscription) (Message, error) {
	m := Message{Urgency: r.Header.Get("Urgency"), Topic: r.Header.Get("Topic")}
	fail := func(status int, err error) (Message, error) {
		m.Status = status
		return m, err
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
	if err != nil {
		return fail(http.StatusBadRequest, fmt.Errorf("reading the body: %w", err))
	}
	if len(body) > MaxBodyBytes {
		return fail(http.StatusRequestEntityTooLarge, fmt.Errorf("the body is larger than %d bytes", MaxBodyBytes))
	}

	v, err := VerifyVAPID(r.Header.Get("Authorization"), s.o.Origin, s.o.Now())
	switch {
	case errors.Is(err, errNoVAPID):
		return fail(http.StatusUnauthorized, err)
	case err != nil:
		return fail(http.StatusForbidden, fmt.Errorf("vapid: %w", err))
	case string(v.Key) != string(sub.serverKey):
		return fail(http.StatusForbidden, errors.New("vapid: k is not the applicationServerKey of the subscription"))
	}
	m.Subject = v.Subject

	ttl, err := strconv.Atoi(r.Header.Get("TTL"))
	if err != nil || ttl < 0 {
		return fail(http.StatusBadRequest, errors.New("the TTL header field must be a non-negative integer"))
	}
	m.TTL = ttl
	switch m.Urgency {
	case "", "very-low", "low", "normal", "high":
	default:
		return fail(http.StatusBadRequest, fmt.Errorf("the Urgency %q is not very-low, low, normal, or high", m.Urgency))
	}
	if m.Topic != "" && !topicPattern.MatchString(m.Topic) {
		return fail(http.StatusBadRequest, errors.New("the Topic must be at most 32 characters of base64url"))
	}
	if ce := r.Header.Get("Content-Encoding"); !strings.EqualFold(ce, "aes128gcm") {
		return fail(http.StatusBadRequest, fmt.Errorf("the Content-Encoding is %q; want aes128gcm", ce))
	}
	plaintext, err := Decrypt(body, sub.key, sub.auth)
	if err != nil {
		return fail(http.StatusBadRequest, fmt.Errorf("decrypting: %w", err))
	}
	m.Plaintext = string(plaintext)
	m.Status = http.StatusCreated
	return m, nil
}

// record appends m to the messages of sub and returns its number, from 1.
func (s *Service) record(sub *subscription, m Message) int {
	if m.ReceivedAt.IsZero() {
		m.ReceivedAt = s.o.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sub.messages = append(sub.messages, m)
	return len(sub.messages)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
