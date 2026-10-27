// SPDX-License-Identifier: MIT

package mykit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"mime"
	"net/http"
	"strings"
	"time"
)

// DefaultHeartbeatTimeout is how long a stream may stay silent before the client drops it and connects again:
// two of the heartbeats that the server sends every 30 seconds.
const DefaultHeartbeatTimeout = 60 * time.Second

const (
	// streamBaseDelay is the wait after a stream that ended, before connecting again.
	streamBaseDelay = time.Second
	// streamMaxDelay is the longest wait between failed connections.
	streamMaxDelay = time.Minute
	// maxEventBytes bounds a line of the stream, and the data of one event (its data lines, each with its LF).
	maxEventBytes = 4 << 20
)

// StreamEvent is an event of GET /streaming: one of *UpdateEvent, *NotificationEvent, *AnnouncementEvent,
// *AnnouncementRemovedEvent, and *UnknownEvent. Branch on it with a type switch.
type StreamEvent interface {
	// EventName is the `event` field of the event, such as "update".
	EventName() string
}

// UpdateEvent is a new post or boost in the home timeline, as it would appear there.
type UpdateEvent struct{ Status Status }

// NotificationEvent is a new notification.
type NotificationEvent struct{ Notification Notification }

// AnnouncementEvent is an announcement that was published or changed.
type AnnouncementEvent struct{ Announcement Announcement }

// AnnouncementRemovedEvent is an announcement that no longer shows.
type AnnouncementRemovedEvent struct{ ID string }

// UnknownEvent is an event that this version of the library does not know, which a newer server may send. Name
// is its `event` field ("message" when it has none) and Data its data, unparsed.
type UnknownEvent struct {
	Name string
	Data string
}

// EventName is "update".
func (*UpdateEvent) EventName() string { return "update" }

// EventName is "notification".
func (*NotificationEvent) EventName() string { return "notification" }

// EventName is "announcement".
func (*AnnouncementEvent) EventName() string { return "announcement" }

// EventName is "announcement_removed".
func (*AnnouncementRemovedEvent) EventName() string { return "announcement_removed" }

// EventName is the event's own name.
func (e *UnknownEvent) EventName() string { return e.Name }

// StreamOptions configures API.Stream.
type StreamOptions struct {
	// OnOpen is called whenever a stream opens: reconnected is false for the first one and true for every later
	// one. The server replays nothing, so after a reconnection read the home timeline and the notifications again
	// for what happened while the stream was closed.
	OnOpen func(reconnected bool)
	// OnEvent receives the events in the order they arrive. The stream waits for it to return.
	OnEvent func(StreamEvent)
	// OnError, when set, receives every failure that the client recovers from by connecting again (a network
	// error, a 429, a 5xx, a silent stream) and every event whose data it cannot read, which it skips.
	OnError func(error)
	// HeartbeatTimeout is how long the server may send nothing, heartbeats included, before the client drops
	// the stream and connects again. Zero means DefaultHeartbeatTimeout.
	HeartbeatTimeout time.Duration
}

// ErrHeartbeatTimeout is the failure, passed to StreamOptions.OnError, of a stream on which nothing arrived for
// the heartbeat timeout.
var ErrHeartbeatTimeout = errors.New("mykit: no heartbeat from the stream")

// Stream reads GET /streaming, the Server-Sent Events of the user's new home entries, notifications, and
// announcements, until ctx ends. It sends the access token of the TokenSource and, after a 401, refreshes it once
// per connection. It never sends Last-Event-ID: the server does not resume a stream. Whenever the stream ends or
// fails, it connects again: after a 429 (too many streams) or a 503 it waits for Retry-After, and otherwise
// for an exponential backoff with jitter. A stream that is silent for the heartbeat timeout counts as failed.
//
// Stream returns ctx's error when ctx ends, and returns the error without connecting again when the server
// refuses the request for good: a *Problem of a 4xx other than 408 and 429, such as a 401 that a refresh did not
// cure or a 403 for a missing scope (read:statuses and read:notifications), or a failure of the TokenSource
// other than a network one, such as an *OAuthError of a refresh token that was revoked. The HTTPClient of the Config must not have a Timeout,
// which would end every stream.
func (a *API) Stream(ctx context.Context, opts StreamOptions) error {
	heartbeat := opts.HeartbeatTimeout
	if heartbeat <= 0 {
		heartbeat = DefaultHeartbeatTimeout
	}
	opened := false
	failures := 0
	for {
		wait, err := a.streamOnce(ctx, &opts, heartbeat, &opened, &failures)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if !recoverable(err) {
				return err
			}
			if opts.OnError != nil {
				opts.OnError(err)
			}
		}
		if wait < 0 {
			wait = streamDelay(failures)
		}
		if err := a.transport.sleep(ctx, wait); err != nil {
			return err
		}
	}
}

// streamOnce opens one stream and reads it to its end. It returns the wait that the server asked for before the
// next connection, or a negative one to wait for the backoff of *failures; and the failure, nil when the stream
// ended normally.
func (a *API) streamOnce(ctx context.Context, opts *StreamOptions, heartbeat time.Duration, opened *bool, failures *int) (time.Duration, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	// The watchdog covers the connection too: a server that never answers is as silent as one that stopped.
	watchdog := time.AfterFunc(heartbeat, func() { cancel(ErrHeartbeatTimeout) })
	defer watchdog.Stop()

	resp, err := a.openStream(ctx)
	if err != nil {
		*failures++
		var p *streamRefusal
		if errors.As(err, &p) {
			return p.wait, p.problem
		}
		return -1, causeOf(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if opts.OnOpen != nil {
		watchdog.Stop()
		opts.OnOpen(*opened)
	}
	*opened = true
	*failures = 0
	watchdog.Reset(heartbeat)

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), maxEventBytes)
	sc.Split(scanLines)
	var parser eventParser
	for sc.Scan() {
		ev, ok, err := parser.line(sc.Text())
		if err != nil {
			*failures++
			return -1, causeOf(ctx, &NetworkError{Method: http.MethodGet, URL: resp.Request.URL.Redacted(), Err: err})
		}
		if !ok {
			watchdog.Reset(heartbeat)
			continue
		}
		watchdog.Stop()
		a.dispatch(opts, ev)
		watchdog.Reset(heartbeat)
	}
	if err := sc.Err(); err != nil {
		*failures++
		return -1, causeOf(ctx, &NetworkError{Method: http.MethodGet, URL: resp.Request.URL.Redacted(), Err: err})
	}
	// The server ended the stream (its shutdown, a token that stopped being valid, a client that fell behind).
	return -1, nil
}

// recoverable reports whether connecting again may cure err: a failure of the network or of the server, a
// silent stream, or a 408 or 429. A refusal of the request or of the token is for good.
func recoverable(err error) bool {
	var p *Problem
	if errors.As(err, &p) {
		return p.Status >= 500 || p.Status == http.StatusRequestTimeout || p.Status == http.StatusTooManyRequests
	}
	var oe *OAuthError
	if errors.As(err, &oe) {
		return oe.Status >= 500 || oe.Status == http.StatusTooManyRequests
	}
	return errors.Is(err, ErrHeartbeatTimeout) || errors.Is(err, errNotEventStream) || IsNetworkError(err)
}

// errNotEventStream is the failure of a 2xx answer to GET /streaming that is no event stream, such as a page
// of a proxy.
var errNotEventStream = errors.New("not an event stream")

// causeOf is the heartbeat timeout when the watchdog ended ctx, and err otherwise.
func causeOf(ctx context.Context, err error) error {
	if cause := context.Cause(ctx); errors.Is(cause, ErrHeartbeatTimeout) {
		return ErrHeartbeatTimeout
	}
	return err
}

// streamRefusal is an error answer to GET /streaming, with the wait that its Retry-After asks for (negative
// without one).
type streamRefusal struct {
	problem *Problem
	wait    time.Duration
}

func (r *streamRefusal) Error() string { return r.problem.Error() }

// openStream sends GET /streaming with the headers of the client and the access token, refreshing it once after
// a 401, and returns the answer when it is an event stream.
func (a *API) openStream(ctx context.Context) (*http.Response, error) {
	t := a.transport
	u := a.origin.String() + APIBasePath + "/streaming"
	refreshed := false
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")
		req.Header.Set("User-Agent", t.userAgent)
		if t.acceptLanguage != "" {
			req.Header.Set("Accept-Language", t.acceptLanguage)
		}
		var tok *Token
		if t.tokens != nil {
			if tok, err = t.tokens.Token(ctx); err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		}
		resp, err := t.client.Do(req)
		if err != nil {
			return nil, &NetworkError{Method: req.Method, URL: req.URL.Redacted(), Err: err}
		}
		if resp.StatusCode == http.StatusUnauthorized && tok != nil && !refreshed {
			if refresher, ok := t.tokens.(Refresher); ok {
				discard(resp)
				refreshed = true
				if _, err := refresher.Refresh(ctx, tok); err != nil {
					return nil, err
				}
				continue
			}
		}
		if resp.StatusCode >= http.StatusBadRequest {
			wait := time.Duration(-1)
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
				if d, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
					wait = d
				}
			}
			return nil, &streamRefusal{problem: problemFrom(resp), wait: wait}
		}
		if mt, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type")); err != nil || mt != "text/event-stream" {
			discard(resp)
			return nil, fmt.Errorf("mykit: GET %s answered %q: %w", req.URL.Redacted(), resp.Header.Get("Content-Type"), errNotEventStream)
		}
		return resp, nil
	}
}

// dispatch hands one event to OnEvent, typed by its name.
func (a *API) dispatch(opts *StreamOptions, ev rawEvent) {
	typed, err := parseStreamEvent(ev)
	if err != nil {
		if opts.OnError != nil {
			opts.OnError(err)
		}
		return
	}
	if opts.OnEvent != nil {
		opts.OnEvent(typed)
	}
}

// parseStreamEvent types an event by its name; an unknown name becomes an *UnknownEvent.
func parseStreamEvent(ev rawEvent) (StreamEvent, error) {
	var typed StreamEvent
	var target any
	switch ev.name {
	case "update":
		e := &UpdateEvent{}
		typed, target = e, &e.Status
	case "notification":
		e := &NotificationEvent{}
		typed, target = e, &e.Notification
	case "announcement":
		e := &AnnouncementEvent{}
		typed, target = e, &e.Announcement
	case "announcement_removed":
		e := &AnnouncementRemovedEvent{}
		var removed struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(ev.data), &removed); err != nil || removed.ID == "" {
			return nil, fmt.Errorf("mykit: the data of a stream event %q has no id", ev.name)
		}
		e.ID = removed.ID
		return e, nil
	default:
		return &UnknownEvent{Name: ev.name, Data: ev.data}, nil
	}
	if err := json.Unmarshal([]byte(ev.data), target); err != nil {
		return nil, fmt.Errorf("mykit: the data of a stream event %q: %w", ev.name, err)
	}
	return typed, nil
}

// streamDelay is the wait before the next connection: one second after a stream that ended normally, doubling
// with each failure in a row up to a minute, and spread over its upper half at random so that the clients a
// server restart disconnected do not all come back at once.
func streamDelay(failures int) time.Duration {
	d := min(streamBaseDelay<<min(failures, 10), streamMaxDelay)
	return d/2 + rand.N(d/2+1) //nolint:gosec // jitter, not a secret
}

// rawEvent is one dispatched event of the stream: its name ("message" when it has none) and its data lines
// joined by LF.
type rawEvent struct {
	name string
	data string
}

// eventParser interprets the lines of a text/event-stream as the HTML Standard says ("Server-sent events",
// §9.2.6). The server sends no `id` and no `retry`, so those fields are ignored like any unknown one.
type eventParser struct {
	started bool
	name    string
	data    bytes.Buffer
	hasData bool
}

// errEventTooLarge is the failure of a stream whose event has more than maxEventBytes of data.
var errEventTooLarge = errors.New("a stream event is larger than 4 MiB")

// line processes one line and reports the event that a blank line dispatched. It fails with errEventTooLarge
// when the data of the event grows beyond maxEventBytes.
func (p *eventParser) line(line string) (rawEvent, bool, error) {
	if !p.started {
		p.started = true
		// A leading byte order mark is not part of the stream.
		line = strings.TrimPrefix(line, "\uFEFF")
	}
	if line == "" {
		ev, ok := p.dispatch()
		return ev, ok, nil
	}
	if line[0] == ':' {
		// A comment, such as ": heartbeat".
		return rawEvent{}, false, nil
	}
	field, value := line, ""
	if i := strings.IndexByte(line, ':'); i >= 0 {
		field, value = line[:i], line[i+1:]
		if value != "" && value[0] == ' ' {
			value = value[1:]
		}
	}
	switch field {
	case "event":
		p.name = value
	case "data":
		if p.data.Len()+len(value)+1 > maxEventBytes {
			return rawEvent{}, false, errEventTooLarge
		}
		p.data.WriteString(value)
		p.data.WriteByte('\n')
		p.hasData = true
	}
	return rawEvent{}, false, nil
}

func (p *eventParser) dispatch() (rawEvent, bool) {
	name := p.name
	if name == "" {
		name = "message"
	}
	data := p.data.String()
	hasData := p.hasData
	p.name = ""
	p.data.Reset()
	p.hasData = false
	// An event without data is not dispatched.
	if !hasData {
		return rawEvent{}, false
	}
	return rawEvent{name: name, data: data[:len(data)-1]}, true
}

// scanLines splits the stream at CRLF, LF, or CR, the line ends of an event stream.
func scanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		if data[i] == '\n' {
			return i + 1, data[:i], nil
		}
		// A CR: the line ends here, and a LF right after it belongs to the same line end.
		if i+1 < len(data) {
			if data[i+1] == '\n' {
				return i + 2, data[:i], nil
			}
			return i + 1, data[:i], nil
		}
		if atEOF {
			return i + 1, data[:i], nil
		}
		// The CR is the last byte read: wait for the next one.
		return 0, nil, nil
	}
	if atEOF {
		// A line without its end is no complete line; the parser drops the event it belongs to.
		return len(data), nil, nil
	}
	return 0, nil, nil
}
