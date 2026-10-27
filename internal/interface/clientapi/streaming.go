// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/stream"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/instance"
)

// Defaults of the streaming endpoint.
const (
	// DefaultStreamHeartbeat is how often an idle stream sends a comment line, which keeps
	// proxies from closing it and lets the server notice a client that has gone.
	DefaultStreamHeartbeat = 30 * time.Second
	// DefaultStreamWriteTimeout bounds each write to a stream, so that a client that stops
	// reading releases its stream.
	DefaultStreamWriteTimeout = 10 * time.Second
	// streamRetryAfter is the Retry-After, in seconds, of a stream refused because the broker
	// is unavailable or shutting down.
	streamRetryAfter = 5
	// streamRecentEntries is the number of home timeline entries a stream remembers having
	// sent, so that a signal that a cross-process broker delivers twice sends its entry once.
	streamRecentEntries = 512
)

// ProblemTypeTooManyStreams is the problem type of a stream refused because the user already
// holds as many open streams as the server allows.
const ProblemTypeTooManyStreams = "/api/v1/problems/too-many-streams"

// StreamConnections is the my.stream.connections metric (ADR-0055 §3): the open streams of
// this process, by the kind of stream: user, the streaming endpoint of the viewer's home
// timeline and notifications, and voice, the room streams of the voice chat (voice_stream.go).
var StreamConnections = kernel.Instrument{
	Name: "my.stream.connections", Unit: "{connection}", Description: "Open streams (Server-Sent Events) of this process, by kind.",
	LabelKeys: []string{labelStreamKind},
}

const (
	labelStreamKind = "my.stream.kind"
	streamKindUser  = "user"
)

// streamingPattern is the http.ServeMux pattern of the streaming endpoint.
const streamingPattern = "GET " + BasePath + "/streaming"

// HomeEntries tells whether a home timeline lists a new entry. *apptimeline.Service implements it.
type HomeEntries interface {
	HomeEntry(ctx context.Context, viewer appcontent.Viewer, id string, q apptimeline.Query) (apptimeline.Entry, bool, error)
}

// registerStreaming registers GET /streaming (operation streamEvents), which the strict server
// does not generate (ADR-0029), behind the same guard as the generated operations.
func (s *Server) registerStreaming(r Router) {
	r.Handle(streamingPattern, s.guard(streamingPattern, http.HandlerFunc(s.serveStreaming)))
}

// serveStreaming serves GET /api/v1/streaming: a Server-Sent Events stream of the viewer's new
// home timeline entries (event update, data a Status), notifications (event notification,
// data a Notification), and announcements (event announcement, data an Announcement, and event
// announcement_removed, data its ID). The stream subscribes to the signals of stream.Subscriber and reads
// each change back through the use cases, as the viewer would see it in the home timeline or
// the notification list. It ends when the client goes, the subscription ends (the broker shuts
// down, the client fell behind, or the broker may have missed signals), a write times out, or
// the access token is no longer valid, which every heartbeat checks again. A broker that is
// unavailable or shutting down makes it a 503 with Retry-After.
func (s *Server) serveStreaming(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v, ok := ViewerFrom(ctx)
	if !ok {
		s.responseError(w, r, errNoViewer)
		return
	}
	if s.o.Streams == nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, ProblemTypeBlank, s.localizer(r).ServerErrorsStreamingUnavailable(), "")
		return
	}
	sub, err := s.o.Streams.Subscribe(ctx, v.AccountID)
	switch {
	case errors.Is(err, stream.ErrTooManySubscriptions):
		l := s.localizer(r)
		s.writeProblem(w, r, http.StatusTooManyRequests, ProblemTypeTooManyStreams, l.ServerErrorsRateLimited(), l.ServerErrorsStreamingTooMany())
		return
	case errors.Is(err, stream.ErrClosed), errors.Is(err, stream.ErrUnavailable):
		if errors.Is(err, stream.ErrUnavailable) {
			s.o.Logger.WarnContext(ctx, "streaming broker unavailable", slog.Any("error", err))
		}
		w.Header().Set("Retry-After", strconv.Itoa(streamRetryAfter))
		s.writeProblem(w, r, http.StatusServiceUnavailable, ProblemTypeBlank, s.localizer(r).ServerErrorsStreamingUnavailable(), "")
		return
	case ctx.Err() != nil:
		return // the client went while the broker got ready
	case err != nil:
		s.responseError(w, r, err)
		return
	}
	defer sub.Close()

	home, err := s.timelineQuery(ctx, &v, nil, nil, true)
	if err != nil {
		s.responseError(w, r, err)
		return
	}
	st := &streamState{
		s: s, w: w, rc: http.NewResponseController(w), viewer: v, home: home,
		sentEntries: newRecentIDs(streamRecentEntries),
		// Notifications from the moment the stream opens; earlier ones are in the list.
		lastNotification: kernel.UUIDv7Floor(time.Now()),
	}
	token, _, _ := bearerToken(r.Header)

	// The stream outlives the server's read timeout, which would otherwise end it.
	if err := st.rc.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		s.responseError(w, r, err)
		return
	}
	kind := kernel.NewLabels(labelStreamKind, streamKindUser)
	s.streams.Add(ctx, 1, kind)
	defer s.streams.Add(context.WithoutCancel(ctx), -1, kind)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", cacheNoStore)
	h.Set("X-Accel-Buffering", "no") // nginx: do not buffer the stream
	w.WriteHeader(http.StatusOK)
	if st.write(": connected\n\n") != nil {
		return
	}

	heartbeat := s.o.StreamHeartbeat
	if heartbeat <= 0 {
		heartbeat = DefaultStreamHeartbeat
	}
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.Done():
			return
		case <-ticker.C:
			if _, err := s.o.Authenticator.AuthenticateBearer(ctx, token); err != nil {
				return // revoked, expired, or the account became unusable
			}
			if s.streamClosed(ctx) {
				return // the server was closed (ADR-0088 §2)
			}
			if st.write(": heartbeat\n\n") != nil {
				return
			}
		case e := <-sub.Events():
			if err := st.handle(ctx, e); err != nil {
				if !errors.Is(err, errStreamWrite) && ctx.Err() == nil {
					s.o.Logger.ErrorContext(ctx, "streaming", slog.String("account_id", v.AccountID), slog.Any("error", err))
				}
				return
			}
		}
	}
}

// streamClosed reports whether the service closure stops the open streams: guard refuses new
// ones, and every heartbeat ends an open one once the server is closed. A failed read keeps the
// stream; the next heartbeat reads again.
func (s *Server) streamClosed(ctx context.Context) bool {
	if s.o.Closures == nil {
		return false
	}
	c, err := s.o.Closures.Closure(ctx)
	return err == nil && !instance.ClosureAllows(c.CurrentState(), instance.ClosureStream)
}

// errStreamWrite reports a write to the client that failed: the client went or stopped reading.
var errStreamWrite = errors.New("clientapi: write to the stream")

// streamState is one open stream.
type streamState struct {
	s      *Server
	w      http.ResponseWriter
	rc     *http.ResponseController
	viewer Viewer
	// home selects the viewer's timeline languages.
	home apptimeline.Query
	// lastNotification is the ID of the last notification sent, or the floor of the time the
	// stream opened.
	lastNotification string
	// sentEntries holds the IDs of the home timeline entries recently sent.
	sentEntries *recentIDs
}

// handle sends what signal e means for the viewer.
func (st *streamState) handle(ctx context.Context, e stream.Event) error {
	switch e.Kind {
	case stream.KindHomeEntry:
		return st.homeEntry(ctx, e.EntryID)
	case stream.KindNotifications:
		return st.notifications(ctx)
	case stream.KindAnnouncement:
		return st.announcement(ctx, e.AnnouncementID)
	}
	return nil
}

// announcementRemoved is the data of an announcement_removed event.
type announcementRemoved struct {
	ID string `json:"id"`
}

// announcement sends the announcement id as the viewer sees it (event announcement), or, when
// the users no longer see it, its removal (event announcement_removed).
func (st *streamState) announcement(ctx context.Context, id string) error {
	if st.s.o.Announcements == nil {
		return nil
	}
	a, ok, err := st.s.o.Announcements.ShownAnnouncement(ctx, st.viewer.UserID, id)
	if err != nil {
		return fmt.Errorf("announcement %s: %w", id, err)
	}
	if !ok {
		return st.event("announcement_removed", announcementRemoved{ID: id})
	}
	return st.event("announcement", newAnnouncement(a))
}

// homeEntry sends the entry id when the viewer's home timeline lists it and the stream has not
// sent it recently: a cross-process broker may deliver its signal twice.
func (st *streamState) homeEntry(ctx context.Context, id string) error {
	if st.s.o.HomeEntries == nil || st.sentEntries.has(id) {
		return nil
	}
	viewer := localContentViewer(st.viewer)
	entry, ok, err := st.s.o.HomeEntries.HomeEntry(ctx, viewer, id, st.home)
	if err != nil || !ok {
		return err
	}
	statuses, err := st.s.statuses(ctx, viewer, entryItems([]apptimeline.Entry{entry}))
	if err != nil {
		return err
	}
	for _, status := range statuses {
		if err := st.event("update", status); err != nil {
			return err
		}
	}
	st.sentEntries.add(id)
	return nil
}

// recentIDs remembers the last IDs added to it, up to its capacity.
type recentIDs struct {
	ids  []string
	next int
	set  map[string]struct{}
}

func newRecentIDs(capacity int) *recentIDs {
	return &recentIDs{ids: make([]string, 0, capacity), set: make(map[string]struct{}, capacity)}
}

func (r *recentIDs) has(id string) bool {
	_, ok := r.set[id]
	return ok
}

// add remembers id, forgetting the oldest one when it is full.
func (r *recentIDs) add(id string) {
	if r.has(id) {
		return
	}
	if len(r.ids) < cap(r.ids) {
		r.ids = append(r.ids, id)
	} else {
		delete(r.set, r.ids[r.next])
		r.ids[r.next] = id
		r.next = (r.next + 1) % len(r.ids)
	}
	r.set[id] = struct{}{}
}

// notifications sends the viewer's notifications after the last one sent, oldest first, a
// page at a time until it has caught up, so that a burst larger than a page loses nothing.
func (st *streamState) notifications(ctx context.Context) error {
	for {
		ns, err := st.s.o.Notifications.List(ctx, st.viewer.AccountID, appnotification.Query{
			After: st.lastNotification, Limit: appnotification.MaxPageLimit,
		})
		if err != nil {
			return fmt.Errorf("list the notifications of %s: %w", st.viewer.AccountID, err)
		}
		items, err := st.s.newNotifications(ctx, st.viewer, ns)
		if err != nil {
			return err
		}
		for i, item := range items {
			if err := st.event("notification", item); err != nil {
				return err
			}
			st.lastNotification = ns[i].ID
		}
		if len(ns) < appnotification.MaxPageLimit {
			return nil
		}
	}
}

// event writes one event with data as its JSON.
func (st *streamState) event(name string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return st.write("event: " + name + "\ndata: " + string(b) + "\n\n")
}

// write writes and flushes text, within the stream write timeout. A flushed write clears the
// deadline again: HTTP/2 resets a stream whose deadline passes even while it is idle, so a
// deadline left in place would end every idle stream before its next heartbeat.
func (st *streamState) write(text string) error {
	timeout := st.s.o.StreamWriteTimeout
	if timeout <= 0 {
		timeout = DefaultStreamWriteTimeout
	}
	if err := st.rc.SetWriteDeadline(time.Now().Add(timeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return errors.Join(errStreamWrite, err)
	}
	if _, err := st.w.Write([]byte(text)); err != nil {
		return errors.Join(errStreamWrite, err)
	}
	if err := st.rc.Flush(); err != nil {
		return errors.Join(errStreamWrite, err)
	}
	if err := st.rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return errors.Join(errStreamWrite, err)
	}
	return nil
}
