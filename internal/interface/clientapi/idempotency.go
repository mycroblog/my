// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/content"
)

// The Idempotency-Key of the client API (draft-ietf-httpapi-idempotency-key-header, ADR-0028
// §18): every write takes one, except the few whose answer carries a secret
// (x-no-idempotency-key in the spec), and a repeat gets the first answer.

// Problem types of the Idempotency-Key.
const (
	// ProblemTypeIdempotencyKeyReused is the 422 of a key that the operation got with another
	// request within IdempotencyWindow.
	ProblemTypeIdempotencyKeyReused = "/api/v1/problems/idempotency-key-reused"
	// ProblemTypeIdempotencyKeyInUse is the 409 of a repeat while the first request with its key
	// is still being carried out.
	ProblemTypeIdempotencyKeyInUse = "/api/v1/problems/idempotency-key-in-use"
)

// IdempotencyKeyHeader is the header that makes a write safe to send again.
const IdempotencyKeyHeader = "Idempotency-Key"

// IdempotencyWindow is how long the answer to a request with an Idempotency-Key is kept: a
// repeat within it gets that answer, and the key is free again after it.
const IdempotencyWindow = 24 * time.Hour

// idempotencyPendingTTL bounds how long a request in progress holds its key: longer than the
// HTTP server lets any request take, so that a request whose process ended stops refusing its
// repeats with ProblemTypeIdempotencyKeyInUse.
const idempotencyPendingTTL = 5 * time.Minute

// maxIdempotentBody is the largest answer that is kept. A larger one is not: its repeat is
// carried out again. No answer of a write comes near it.
const maxIdempotentBody = 1 << 20

// idempotencyNamespace is the KeyValueStore namespace of the kept answers.
const idempotencyNamespace = "idempotency"

// idempotentAnswer is what the store keeps under a key: a request in progress, or the
// fingerprint of the request and its answer.
type idempotentAnswer struct {
	Pending     bool        `json:"pending,omitempty"`
	Fingerprint string      `json:"fingerprint,omitempty"`
	Status      int         `json:"status,omitempty"`
	Header      http.Header `json:"header,omitempty"`
	Body        []byte      `json:"body,omitempty"`
}

var pendingAnswer = []byte(`{"pending":true}`)

// serveIdempotent serves the request of an operation that the guard let through. For an
// operation that takes an Idempotency-Key and a request that has one, the first request with
// the key is carried out and its answer kept for IdempotencyWindow when it is below 400: a
// refusal (any 4xx, a 429 too) or a 5xx frees the key, so that the request corrected, or allowed
// again, is carried out with it; a repeat of the same request gets that answer, the same key with another request
// (another path, query, Content-Type or body) a 422, and a repeat while the first is in
// progress a 409. Keys are the user's own, an anonymous client's those of its address, and
// each operation's own.
func (s *Server) serveIdempotent(w http.ResponseWriter, r *http.Request, pattern string, a operationAccess, next http.Handler) {
	values := r.Header.Values(IdempotencyKeyHeader)
	if !a.Idempotent || s.o.Idempotency == nil || len(values) == 0 || (len(values) == 1 && values[0] == "") {
		next.ServeHTTP(w, r)
		return
	}
	if len(values) > 1 || content.ValidateIdempotencyKey(values[0]) != nil {
		ctx := withLocale(r)
		s.writeProblemDocument(w, s.fieldProblem(ctx, IdempotencyKeyHeader, s.Localizer(ctx).ServerErrorsInvalidValue()))
		return
	}
	kv, ctx := s.o.Idempotency, r.Context()
	secret, err := s.idempotencyKey.load(ctx, s.o.IdempotencySecret)
	if err != nil {
		s.responseError(w, r, err)
		return
	}
	key := s.idempotencyStoreKey(r, a.Operation, values[0])
	// A kept answer may expire between the swap and the read: then the swap is tried once more.
	for range 2 {
		first, err := kv.CompareAndSwap(ctx, idempotencyNamespace, key, nil, pendingAnswer, idempotencyPendingTTL)
		if err != nil {
			s.responseError(w, r, err)
			return
		}
		if first {
			s.serveFirst(w, r, pattern, key, secret, next)
			return
		}
		raw, err := kv.Get(ctx, idempotencyNamespace, key)
		if errors.Is(err, kernel.ErrKeyNotFound) {
			continue
		}
		if err != nil {
			s.responseError(w, r, err)
			return
		}
		s.serveRepeat(w, r, pattern, secret, raw)
		return
	}
	s.writeProblem(w, r, http.StatusConflict, ProblemTypeIdempotencyKeyInUse, s.localizer(r).ServerErrorsIdempotencyKeyInUse(), "")
}

// serveFirst carries out the first request with the key and keeps its answer, or frees the key
// when the answer is not kept. The handler runs on a context that the client's going away does
// not cancel (it keeps the request's values and deadline), so that a write it commits is not
// turned into a 5xx that frees the key and carries the write out again on the retry. A key is
// not freed either once the client went away: what was done is not known, so its repeats get
// the 409 until idempotencyPendingTTL.
func (s *Server) serveFirst(w http.ResponseWriter, r *http.Request, pattern, key string, secret []byte, next http.Handler) {
	kv, ctx := s.o.Idempotency, context.WithoutCancel(r.Context())
	free := func() {
		if r.Context().Err() != nil {
			return
		}
		if err := kv.Delete(ctx, idempotencyNamespace, key); err != nil {
			s.o.Logger.WarnContext(ctx, "freeing an idempotency key failed", slog.Any("error", err))
		}
	}
	h := requestHash(r, secret)
	if s.maxBody(pattern) <= maxRequestBody {
		// A small body is read before the handler, so that its fingerprint never depends on what
		// the handler reads: once the answer is written, the rest of a body may be unreadable.
		b, err := io.ReadAll(r.Body)
		if err != nil {
			free()
			s.bodyError(w, r, pattern, err)
			return
		}
		_, _ = h.Write(b)
		r.Body = io.NopCloser(bytes.NewReader(b))
	} else {
		// An upload or an import is hashed as the handler reads it, which reads all of it.
		body := r.Body
		r.Body = struct {
			io.Reader
			io.Closer
		}{io.TeeReader(body, h), body}
	}
	rec := &answerRecorder{ResponseWriter: w, before: w.Header().Clone()}
	done := false
	defer func() {
		if !done {
			// A panic of the handler frees the key, so that a repeat is carried out.
			free()
		}
	}()
	handlerCtx := ctx
	if deadline, ok := r.Context().Deadline(); ok {
		var cancel context.CancelFunc
		handlerCtx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
	next.ServeHTTP(rec, r.WithContext(handlerCtx))
	done = true
	// What the handler left unread is part of the request too.
	_, drainErr := io.Copy(io.Discard, r.Body)
	status := rec.statusCode()
	if drainErr != nil || rec.overflow || status >= http.StatusBadRequest {
		free()
		return
	}
	answer, err := json.Marshal(idempotentAnswer{Fingerprint: hex.EncodeToString(h.Sum(nil)), Status: status, Header: rec.header, Body: rec.body.Bytes()})
	if err == nil {
		err = kv.Set(ctx, idempotencyNamespace, key, answer, IdempotencyWindow)
	}
	if err != nil {
		// The key stays pending until idempotencyPendingTTL: its repeats get a 409 until then
		// instead of being carried out twice.
		s.o.Logger.WarnContext(ctx, "keeping the answer to an idempotency key failed", slog.Any("error", err))
	}
}

// serveRepeat answers a repeat of a key from what the store keeps for it.
func (s *Server) serveRepeat(w http.ResponseWriter, r *http.Request, pattern string, secret, raw []byte) {
	var kept idempotentAnswer
	if err := json.Unmarshal(raw, &kept); err != nil {
		s.responseError(w, r, err)
		return
	}
	l := s.localizer(r)
	if kept.Pending {
		s.writeProblem(w, r, http.StatusConflict, ProblemTypeIdempotencyKeyInUse, l.ServerErrorsIdempotencyKeyInUse(), "")
		return
	}
	h := requestHash(r, secret)
	if _, err := io.Copy(h, r.Body); err != nil {
		s.bodyError(w, r, pattern, err)
		return
	}
	if hex.EncodeToString(h.Sum(nil)) != kept.Fingerprint {
		s.writeProblem(w, r, http.StatusUnprocessableEntity, ProblemTypeIdempotencyKeyReused, l.ServerErrorsIdempotencyKeyReused(), "")
		return
	}
	header := w.Header()
	for k, v := range kept.Header {
		header[k] = v
	}
	varyAcceptLanguage(header)
	w.WriteHeader(kept.Status)
	_, _ = w.Write(kept.Body)
}

// bodyError answers a request whose body could not be read: over the operation's limit, or cut
// off.
func (s *Server) bodyError(w http.ResponseWriter, r *http.Request, pattern string, err error) {
	if mbe := (*http.MaxBytesError)(nil); errors.As(err, &mbe) {
		s.writeTooLarge(w, r, pattern)
		return
	}
	s.writeProblem(w, r, http.StatusBadRequest, ProblemTypeBlank, s.localizer(r).ServerErrorsBadRequest(), "")
}

// idempotencyStoreKey is the store's key of an Idempotency-Key: of the user, or of an
// anonymous client's address, and of the operation.
func (s *Server) idempotencyStoreKey(r *http.Request, operation, key string) string {
	scope := "ip:" + s.clientKey(r)
	if v, ok := ViewerFrom(r.Context()); ok {
		scope = "account:" + v.AccountID
	}
	sum := sha256.Sum256([]byte(scope + "\x00" + operation + "\x00" + key))
	return hex.EncodeToString(sum[:])
}

// requestHash returns the HMAC-SHA256 under secret of the fingerprint of r, holding all of it
// but the body, which the caller writes to it. It is keyed so that what the store keeps tells
// nothing of a body, which may hold a password.
func requestHash(r *http.Request, secret []byte) hash.Hash {
	h := hmac.New(sha256.New, secret)
	for _, part := range []string{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Get("Content-Type")} {
		_, _ = io.WriteString(h, part)
		_, _ = h.Write([]byte{0})
	}
	return h
}

// answerRecorder passes an answer through and keeps a copy of it: its status, the headers that
// the handler set (Vary aside, which the guard and the replay set themselves), and up to
// maxIdempotentBody of its body.
type answerRecorder struct {
	http.ResponseWriter
	before   http.Header
	status   int
	header   http.Header
	body     bytes.Buffer
	overflow bool
}

func (a *answerRecorder) WriteHeader(status int) {
	if a.status == 0 {
		a.status = status
		a.header = http.Header{}
		for k, v := range a.Header() {
			if k != "Vary" && !slices.Equal(a.before[k], v) {
				a.header[k] = slices.Clone(v)
			}
		}
	}
	a.ResponseWriter.WriteHeader(status)
}

func (a *answerRecorder) Write(b []byte) (int, error) {
	if a.status == 0 {
		a.WriteHeader(http.StatusOK)
	}
	if a.body.Len()+len(b) > maxIdempotentBody {
		a.overflow = true
	} else if !a.overflow {
		a.body.Write(b)
	}
	return a.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the connection's writer.
func (a *answerRecorder) Unwrap() http.ResponseWriter { return a.ResponseWriter }

// statusCode is the status of the answer; 200 for a handler that wrote nothing.
func (a *answerRecorder) statusCode() int {
	if a.status == 0 {
		return http.StatusOK
	}
	return a.status
}

// idempotencySecret loads the key of the fingerprints from Options.IdempotencySecret once, and
// again after a failure; without one it is a random key of the process.
type idempotencySecret struct {
	mu  sync.Mutex
	key []byte
}

func (k *idempotencySecret) load(ctx context.Context, source func(context.Context) ([]byte, error)) ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.key != nil {
		return k.key, nil
	}
	if source == nil {
		key := make([]byte, sha256.Size)
		_, _ = rand.Read(key) // crypto/rand.Read never returns an error
		k.key = key
		return key, nil
	}
	key, err := source(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: load the idempotency key: %w", err)
	}
	if len(key) < sha256.Size {
		return nil, errors.New("clientapi: the idempotency key must be at least 32 bytes")
	}
	k.key = key
	return key, nil
}
