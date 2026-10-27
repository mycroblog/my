// SPDX-License-Identifier: AGPL-3.0-only

// Package sentry sends the error reports of errors_exporter = sentry (ADR-0103 §4 (3)) to a
// Sentry-compatible intake: Sentry (hosted or self-hosted), GlitchTip, or the Sentry intake of
// Datadog Error Tracking. It is a small client of the envelope protocol (sentry_version=7)
// written on net/http, not sentry-go: the reporter already scrubs, fingerprints, limits, and
// queues the events, and the SDK would add the hostname, read SENTRY_* variables, and run git.
//
// A Client sends one event per SendError, never retries, and never follows a redirect. It follows the rate limits of the
// receiver: after a 429 (Retry-After, or 60 seconds when the response says nothing) and after
// any response whose X-Sentry-Rate-Limits covers the error category, SendError drops the events
// until the limit ends and returns an error whose Dropped method reports true.
//
// The event carries only what the ErrorEvent holds: no server_name, user, or request, and none
// of the host's names or addresses. The destination is the service that the operator
// configured, so the HTTP client is not the SSRF-guarded kernel.HTTPClient.
package sentry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Name is the value of errors_exporter that selects this adapter.
const Name = "sentry"

// protocolVersion is the version of the Sentry protocol that the client speaks.
const protocolVersion = "7"

// defaultRateLimit is how long the client stops after a 429 that names no time.
const defaultRateLimit = 60 * time.Second

// maxTagValue is the longest tag value that Sentry keeps.
const maxTagValue = 200

// modulePrefix starts the packages and files of this repository, whose frames are in_app.
const modulePrefix = "github.com/mycroblog/my/"

// Options configure a Client.
type Options struct {
	// DSN is the errors_sentry_dsn: https://<public key>[:<secret key>]@<host>[/<path>]/<project>.
	DSN string
	// HTTPClient sends the envelopes; it should have a timeout. Nil uses a client with a
	// 10-second timeout. The Client uses a copy that never follows a redirect.
	HTTPClient *http.Client
	// Version is the server version, sent in the sentry_client of the authentication header.
	Version string
	// Now returns the current time; nil uses time.Now.
	Now func() time.Time
	// EventID returns a new event ID of 32 hexadecimal digits; nil uses a random UUID.
	EventID func() string
}

// Client sends ErrorEvents as Sentry envelopes. It is safe for concurrent use.
type Client struct {
	endpoint string
	auth     string
	http     *http.Client
	now      func() time.Time
	eventID  func() string

	mu      sync.Mutex
	limited time.Time // no event is sent before this time
}

// New returns the Client of o.DSN, or an error when the DSN is not one.
func New(o Options) (*Client, error) {
	d, err := ParseDSN(o.DSN)
	if err != nil {
		return nil, err
	}
	c := &Client{endpoint: d.EnvelopeURL(), now: o.Now, eventID: o.EventID}
	if o.HTTPClient != nil {
		h := *o.HTTPClient
		c.http = &h
	} else {
		c.http = &http.Client{Timeout: 10 * time.Second}
	}
	// A redirect is never followed: it would carry the key of X-Sentry-Auth and the event to
	// wherever the intake points. The 3xx is the answer, a failure.
	c.http.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if c.now == nil {
		c.now = time.Now
	}
	if c.eventID == nil {
		c.eventID = randomEventID
	}
	version := o.Version
	if version == "" {
		version = "dev"
	}
	c.auth = "Sentry sentry_version=" + protocolVersion + ", sentry_client=my/" + version + ", sentry_key=" + d.PublicKey
	if d.SecretKey != "" {
		c.auth += ", sentry_secret=" + d.SecretKey
	}
	return c, nil
}

// DSN is a parsed Sentry DSN.
type DSN struct {
	Scheme, Host, Path, ProjectID string
	PublicKey, SecretKey          string
}

// ParseDSN parses dsn, which needs the http or https scheme, a public key, a host, and a
// project ID as the last segment of the path.
func ParseDSN(dsn string) (DSN, error) {
	u, err := url.Parse(strings.TrimSpace(dsn))
	if err != nil {
		return DSN{}, errors.New("sentry: the DSN is not a URL") // the URL holds the key; never print it
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return DSN{}, errors.New("sentry: the DSN needs the http or https scheme")
	}
	if u.User == nil || u.User.Username() == "" {
		return DSN{}, errors.New("sentry: the DSN has no public key")
	}
	if u.Host == "" {
		return DSN{}, errors.New("sentry: the DSN has no host")
	}
	path := strings.TrimSuffix(u.Path, "/")
	i := strings.LastIndexByte(path, '/')
	project := path[i+1:]
	if i < 0 || project == "" {
		return DSN{}, errors.New("sentry: the DSN has no project ID")
	}
	secret, _ := u.User.Password()
	return DSN{Scheme: u.Scheme, Host: u.Host, Path: path[:i], ProjectID: project, PublicKey: u.User.Username(), SecretKey: secret}, nil
}

// EnvelopeURL returns the URL of the project's envelope endpoint.
func (d DSN) EnvelopeURL() string {
	return d.Scheme + "://" + d.Host + d.Path + "/api/" + url.PathEscape(d.ProjectID) + "/envelope/"
}

// SendError sends ev as one envelope. It returns nil when the receiver took it, an error whose
// Dropped method reports true when the event was not sent or not taken because of a rate
// limit, and another error when the request or the receiver failed. It never retries.
func (c *Client) SendError(ctx context.Context, ev kernel.ErrorEvent) error {
	now := c.now()
	if c.limitedAt(now) {
		return errRateLimited
	}
	body, err := c.envelope(ev, now)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sentry: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-sentry-envelope")
	req.Header.Set("X-Sentry-Auth", c.auth)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sentry: send: %w", redactURL(err))
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()

	until := rateLimits(resp.Header.Get("X-Sentry-Rate-Limits"), now)
	if resp.StatusCode == http.StatusTooManyRequests && until.IsZero() {
		until = retryAfter(resp.Header.Get("Retry-After"), now)
	}
	c.limit(until)
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return errRateLimited
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return fmt.Errorf("sentry: the receiver answered %d", resp.StatusCode)
	}
	return nil
}

// limitedAt reports whether a rate limit stops sending at now.
func (c *Client) limitedAt(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return now.Before(c.limited)
}

// limit stops sending until until, unless a longer limit is already set.
func (c *Client) limit(until time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if until.After(c.limited) {
		c.limited = until
	}
}

// droppedError is the error of an event that a rate limit dropped.
type droppedError struct{}

func (droppedError) Error() string { return "sentry: rate limited by the receiver" }

// Dropped reports true: the event was dropped, not failed.
func (droppedError) Dropped() bool { return true }

var errRateLimited error = droppedError{}

// rateLimits returns the end of the longest limit of the X-Sentry-Rate-Limits value h that
// covers the error category, or the zero time. h is a comma-separated list of
// retry_after:categories:scope[:reason_code[:namespaces]], with the categories separated by
// semicolons and empty for every category.
func rateLimits(h string, now time.Time) time.Time {
	var until time.Time
	for limit := range strings.SplitSeq(h, ",") {
		parts := strings.Split(strings.TrimSpace(limit), ":")
		if len(parts) < 2 {
			continue
		}
		secs, err := strconv.ParseFloat(parts[0], 64)
		if err != nil || secs <= 0 {
			continue
		}
		if parts[1] != "" && !hasCategory(parts[1], "error") {
			continue
		}
		if t := now.Add(time.Duration(secs * float64(time.Second))); t.After(until) {
			until = t
		}
	}
	return until
}

func hasCategory(categories, want string) bool {
	for c := range strings.SplitSeq(categories, ";") {
		if c == want {
			return true
		}
	}
	return false
}

// retryAfter returns the end of the Retry-After value h (seconds or an HTTP date), or 60
// seconds from now when h is empty or not one.
func retryAfter(h string, now time.Time) time.Time {
	if secs, err := strconv.ParseFloat(strings.TrimSpace(h), 64); err == nil && secs > 0 {
		return now.Add(time.Duration(secs * float64(time.Second)))
	}
	if t, err := http.ParseTime(h); err == nil && t.After(now) {
		return t
	}
	return now.Add(defaultRateLimit)
}

// redactURL drops the URL of a *url.Error, so that an error never repeats the endpoint.
func redactURL(err error) error {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		return ue.Err
	}
	return err
}

func randomEventID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // variant 10
	return hex.EncodeToString(b[:])
}

// envelope returns the envelope of ev: its header, the header of the event item, and the event.
func (c *Client) envelope(ev kernel.ErrorEvent, now time.Time) ([]byte, error) {
	id := c.eventID()
	payload, err := json.Marshal(newEvent(id, ev))
	if err != nil {
		return nil, fmt.Errorf("sentry: event: %w", err)
	}
	var b bytes.Buffer
	header, _ := json.Marshal(envelopeHeader{EventID: id, SentAt: now.UTC().Format(time.RFC3339Nano)})
	item, _ := json.Marshal(itemHeader{Type: "event", Length: len(payload), ContentType: "application/json"})
	for _, line := range [][]byte{header, item, payload} {
		b.Write(line)
		b.WriteByte('\n')
	}
	return b.Bytes(), nil
}

type envelopeHeader struct {
	EventID string `json:"event_id"`
	SentAt  string `json:"sent_at"`
}

type itemHeader struct {
	Type        string `json:"type"`
	Length      int    `json:"length"`
	ContentType string `json:"content_type"`
}

// event is the Sentry event of an ErrorEvent (ADR-0103 §4 (3)): no server_name, user, or
// request.
type event struct {
	EventID     string            `json:"event_id"`
	Timestamp   string            `json:"timestamp"`
	Platform    string            `json:"platform"`
	Level       string            `json:"level"`
	Environment string            `json:"environment,omitempty"`
	Release     string            `json:"release,omitempty"`
	Fingerprint []string          `json:"fingerprint,omitempty"`
	Exception   exceptions        `json:"exception"`
	Contexts    *contexts         `json:"contexts,omitempty"`
	Tags        map[string]string `json:"tags"`
}

type exceptions struct {
	Values []exception `json:"values"`
}

type exception struct {
	Type       string      `json:"type"`
	Value      string      `json:"value"`
	Stacktrace *stacktrace `json:"stacktrace,omitempty"`
}

type stacktrace struct {
	Frames []frame `json:"frames"`
}

type frame struct {
	Function string `json:"function,omitempty"`
	Module   string `json:"module,omitempty"`
	Filename string `json:"filename,omitempty"`
	Lineno   int    `json:"lineno,omitempty"`
	InApp    bool   `json:"in_app"`
}

type contexts struct {
	Trace traceContext `json:"trace"`
}

type traceContext struct {
	Type    string `json:"type"`
	TraceID string `json:"trace_id"`
	SpanID  string `json:"span_id,omitempty"`
}

func newEvent(id string, ev kernel.ErrorEvent) event {
	e := event{
		EventID:     id,
		Timestamp:   ev.Time.UTC().Format(time.RFC3339Nano),
		Platform:    "go",
		Level:       "error",
		Environment: ev.Environment,
		Release:     ev.Version,
		Exception:   exceptions{Values: []exception{{Type: lastType(ev.Type), Value: ev.Message}}},
		Tags:        map[string]string{"service": "my"},
	}
	if ev.Fatal {
		e.Level = "fatal"
	}
	if ev.Fingerprint != "" {
		e.Fingerprint = []string{ev.Fingerprint}
		e.Tags["error.fingerprint"] = ev.Fingerprint
	}
	if frames := parseStack(ev.Stack); len(frames) > 0 {
		e.Exception.Values[0].Stacktrace = &stacktrace{Frames: frames}
	}
	if ev.TraceID != "" {
		e.Contexts = &contexts{Trace: traceContext{Type: "trace", TraceID: ev.TraceID, SpanID: ev.SpanID}}
	}
	if ev.Role != "" {
		e.Tags["my.role"] = tagValue(ev.Role)
	}
	if ev.Where != "" {
		e.Tags["my.error.where"] = tagValue(ev.Where)
	}
	return e
}

// lastType returns the innermost type of the chain t, "*fmt.wrapError > *pgconn.PgError".
func lastType(t string) string {
	if i := strings.LastIndex(t, " > "); i >= 0 {
		t = t[i+len(" > "):]
	}
	if t == "" {
		return "error"
	}
	return t
}

// tagValue fits v into a Sentry tag value: one line of at most 200 characters.
func tagValue(v string) string {
	v = strings.ReplaceAll(v, "\n", " ")
	if r := []rune(v); len(r) > maxTagValue {
		v = string(r[:maxTagValue])
	}
	return v
}

// parseStack returns the frames of a debug.Stack() stack, outermost first as Sentry orders
// them: each frame is a line with the function and its arguments, then a tab-indented line with
// the file, the line number, and the offset. The goroutine header and lines of another form are
// skipped; "created by" lines are the outermost frame.
func parseStack(stack string) []frame {
	lines := strings.Split(stack, "\n")
	var frames []frame
	for i := 0; i < len(lines); i++ {
		fn := strings.TrimSpace(lines[i])
		if fn == "" || strings.HasPrefix(lines[i], "\t") || strings.HasPrefix(fn, "goroutine ") || i+1 >= len(lines) ||
			!strings.HasPrefix(lines[i+1], "\t") {
			continue
		}
		file, line := fileLine(lines[i+1])
		i++
		fn = strings.TrimPrefix(fn, "created by ")
		if j := strings.Index(fn, " in goroutine "); j >= 0 {
			fn = fn[:j]
		}
		if strings.HasSuffix(fn, ")") {
			if j := strings.LastIndexByte(fn, '('); j > 0 {
				fn = fn[:j]
			}
		}
		module, function := splitFunction(fn)
		frames = append(frames, frame{
			Function: function, Module: module, Filename: file, Lineno: line,
			InApp: strings.HasPrefix(module+"/", modulePrefix) || strings.HasPrefix(file, modulePrefix),
		})
	}
	for i, j := 0, len(frames)-1; i < j; i, j = i+1, j-1 {
		frames[i], frames[j] = frames[j], frames[i]
	}
	return frames
}

// fileLine splits "\tpath/file.go:42 +0x32" into the file and the line number.
func fileLine(s string) (string, int) {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, " +0x"); i >= 0 {
		s = s[:i]
	}
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return s, 0
	}
	n, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return s, 0
	}
	return s[:i], n
}

// splitFunction splits a qualified Go function name, such as
// "github.com/mycroblog/my/internal/app/status.(*Service).Get", into its package path and the
// function within the package.
func splitFunction(name string) (string, string) {
	start := strings.LastIndexByte(name, '/') + 1
	if i := strings.IndexByte(name[start:], '.'); i >= 0 {
		return name[:start+i], name[start+i+1:]
	}
	return "", name
}
