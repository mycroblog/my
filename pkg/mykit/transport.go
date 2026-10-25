// SPDX-License-Identifier: MIT

package mykit

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// transport is the HttpRequestDoer of the generated client. For each request it adds the headers, the access
// token, and the Idempotency-Key; after a 401 it refreshes the token once and sends the request again; after a
// 429 or 503 it waits and retries the requests that are safe to repeat; and it turns an answer with a status of
// 400 or more into a *Problem and a failure to get an answer into a *NetworkError.
type transport struct {
	client         *http.Client
	tokens         TokenSource
	acceptLanguage string
	userAgent      string
	retry          RetryPolicy
	// basePath is the path of the client API, which the operations' paths are relative to.
	basePath string
	// sleep waits for d or until ctx ends; tests replace it.
	sleep func(ctx context.Context, d time.Duration) error
}

// Do sends req as the description of transport says.
func (t *transport) Do(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if req.Header.Get("Accept-Language") == "" && t.acceptLanguage != "" {
		req.Header.Set("Accept-Language", t.acceptLanguage)
	}
	req.Header.Set("User-Agent", t.userAgent)
	if req.Header.Get(idempotencyKeyHeader) == "" && acceptsIdempotencyKey(req.Method, req.URL.Path, t.basePath) {
		// One key for every attempt, so that the server applies the request once.
		req.Header.Set(idempotencyKeyHeader, uuid.NewString())
	}
	// A request is sent again only when its body can be read again.
	replayable := req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
	retryable := replayable && (req.Method == http.MethodGet || req.Method == http.MethodHead || req.Header.Get(idempotencyKeyHeader) != "")

	refreshed := false
	retries := 0
	for attempt := 0; ; attempt++ {
		r := req
		if attempt > 0 {
			var err error
			if r, err = rewind(req); err != nil {
				return nil, err
			}
		}
		var tok *Token
		if t.tokens != nil {
			var err error
			if tok, err = t.tokens.Token(ctx); err != nil {
				return nil, err
			}
			r.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		}
		resp, err := t.client.Do(r)
		if err != nil {
			return nil, &NetworkError{Method: req.Method, URL: req.URL.Redacted(), Err: err}
		}
		switch {
		case resp.StatusCode == http.StatusUnauthorized && tok != nil && !refreshed && replayable:
			refresher, ok := t.tokens.(Refresher)
			if !ok {
				return nil, problemFrom(resp)
			}
			discard(resp)
			refreshed = true
			if _, err := refresher.Refresh(ctx, tok); err != nil {
				return nil, err
			}
			continue
		case (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable) &&
			retryable && retries < t.retry.maxRetries():
			wait, ok := retryDelay(resp.Header.Get("Retry-After"), retries, t.retry.maxWait(), time.Now())
			if !ok {
				return nil, problemFrom(resp)
			}
			discard(resp)
			retries++
			if err := t.sleep(ctx, wait); err != nil {
				return nil, err
			}
			continue
		case resp.StatusCode >= http.StatusBadRequest:
			return nil, problemFrom(resp)
		}
		return resp, nil
	}
}

// rewind returns a copy of req to send again, with its body from the start.
func rewind(req *http.Request) (*http.Request, error) {
	r := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		r.Body = body
	}
	return r, nil
}

// discard reads a little of the body of a response that is not returned, so that its connection can be
// reused, and closes it.
func discard(resp *http.Response) {
	_, _ = io.CopyN(io.Discard, resp.Body, 64<<10)
	_ = resp.Body.Close()
}

// retryDelay is how long to wait before retry number n+1 (n from 0): what Retry-After asks for (seconds or an
// HTTP date), or without it an exponential backoff from one second with jitter. It reports false when the wait
// is longer than limit.
func retryDelay(retryAfter string, n int, limit time.Duration, now time.Time) (time.Duration, bool) {
	if retryAfter != "" {
		var d time.Duration
		if secs, err := strconv.Atoi(retryAfter); err == nil {
			d = time.Duration(max(secs, 0)) * time.Second
		} else if at, err := http.ParseTime(retryAfter); err == nil {
			d = max(at.Sub(now), 0)
		} else {
			d = time.Second
		}
		return d, d <= limit
	}
	d := time.Second << min(n, 10)
	d += rand.N(d / 2) //nolint:gosec // jitter, not a secret
	return min(d, limit), true
}

// sleepContext waits for d, or returns the error of ctx when it ends first.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NetworkError is the failure to get an answer from the server: the connection, TLS, a timeout, or the end of
// the request's context. errors.Is sees through it to the cause, such as context.DeadlineExceeded.
type NetworkError struct {
	Method string
	URL    string
	Err    error
}

func (e *NetworkError) Error() string {
	return "mykit: " + e.Method + " " + e.URL + ": " + e.Err.Error()
}

func (e *NetworkError) Unwrap() error { return e.Err }

// IsNetworkError reports whether err is, or wraps, a *NetworkError.
func IsNetworkError(err error) bool {
	var ne *NetworkError
	return errors.As(err, &ne)
}
