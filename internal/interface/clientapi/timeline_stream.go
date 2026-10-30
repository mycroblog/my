// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"net/http"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/stream"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/instance"
)

// streamKindTimeline is the my.stream.kind of the streams of the local and federated timelines.
const streamKindTimeline = "timeline"

// timelineStreamPattern is the http.ServeMux pattern of the streams of the public timelines.
const timelineStreamPattern = "GET " + BasePath + "/streaming/timelines/{timeline}"

// PublicEntries tell whether the local and the federated timelines list a new entry.
// *apptimeline.Service implements them.
type PublicEntries interface {
	LocalEntry(ctx context.Context, viewer appcontent.Viewer, id string, q apptimeline.Query) (apptimeline.Entry, bool, error)
	PublicEntry(ctx context.Context, viewer appcontent.Viewer, id string, q apptimeline.Query) (apptimeline.Entry, bool, error)
}

// registerTimelineStream registers GET /streaming/timelines/{timeline} (operation
// streamTimeline), which the strict server does not generate (ADR-0029), behind the same guard
// as the generated operations.
func (s *Server) registerTimelineStream(r Router) {
	r.Handle(timelineStreamPattern, s.guard(timelineStreamPattern, http.HandlerFunc(s.serveTimelineStream)))
}

// publicTimelineStream is the timeline of a stream path and the instance setting that opens it.
type publicTimelineStream struct {
	timeline stream.Timeline
	access   func(instance.Settings) instance.TimelineAccess
}

var publicTimelineStreams = map[string]publicTimelineStream{
	"local":     {stream.TimelineLocal, instance.Settings.LocalTimelineAccess},
	"federated": {stream.TimelineFederated, instance.Settings.FederatedTimelineAccess},
}

// serveTimelineStream serves GET /api/v1/streaming/timelines/{timeline}: a Server-Sent Events
// stream of the new posts of the local or the federated timeline (ADR-0127), event update with
// a Status, for logged-in users only. It sends a post when the REST timeline (GET
// /timelines/local or /timelines/public) lists it for the viewer at that moment, which
// LocalEntry and PublicEntry decide with the same policy, and nothing is replayed. It is a 404
// for another timeline and while the instance setting of the timeline is off; every heartbeat
// checks the setting again, as it checks the access token. It shares the limit of open streams
// and the answers of a broker that is not ready with GET /streaming.
func (s *Server) serveTimelineStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v, ok := ViewerFrom(ctx)
	if !ok {
		s.responseError(w, r, errNoViewer)
		return
	}
	tl, ok := publicTimelineStreams[r.PathValue("timeline")]
	if !ok {
		s.writeProblem(w, r, http.StatusNotFound, ProblemTypeBlank, s.localizer(r).ServerErrorsNotFound(), "")
		return
	}
	if denied, err := s.timelineDenied(ctx, tl.access); err != nil {
		s.responseError(w, r, err)
		return
	} else if denied != nil {
		s.writeProblemOf(w, r, *denied)
		return
	}
	if s.o.TimelineStreams == nil || s.o.PublicEntries == nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, ProblemTypeBlank, s.localizer(r).ServerErrorsStreamingUnavailable(), "")
		return
	}
	sub, err := s.o.TimelineStreams.SubscribeTimeline(ctx, v.AccountID, tl.timeline)
	if s.subscribeRefused(w, r, err) {
		return
	}
	defer sub.Close()

	q, err := s.timelineQuery(ctx, &v, nil, nil, true)
	if err != nil {
		s.responseError(w, r, err)
		return
	}
	ts := &timelineStream{
		streamState: streamState{s: s, w: w, rc: http.NewResponseController(w), viewer: v, home: q, sentEntries: newRecentIDs(streamRecentEntries)},
		timeline:    tl.timeline,
	}
	alive := func(ctx context.Context) bool {
		denied, err := s.timelineDenied(ctx, tl.access)
		return err != nil || denied == nil // a failed read keeps the stream until the next heartbeat
	}
	s.runStream(w, r, &ts.streamState, sub, streamKindTimeline, ts.handle, alive)
}

// timelineStream is one open stream of a public timeline; its streamState's home holds the
// viewer's timeline languages.
type timelineStream struct {
	streamState
	timeline stream.Timeline
}

// handle sends the post of a public timeline entry when the viewer's timeline lists it and the
// stream has not sent it recently: a cross-process broker may deliver its signal twice.
func (ts *timelineStream) handle(ctx context.Context, e stream.Event) error {
	if e.Kind != stream.KindPublicEntry || ts.sentEntries.has(e.EntryID) {
		return nil
	}
	viewer := localContentViewer(ts.viewer)
	lookup := ts.s.o.PublicEntries.PublicEntry
	if ts.timeline == stream.TimelineLocal {
		lookup = ts.s.o.PublicEntries.LocalEntry
	}
	entry, ok, err := lookup(ctx, viewer, e.EntryID, ts.home)
	if err != nil || !ok {
		return err
	}
	statuses, err := ts.s.statuses(ctx, viewer, entryItems([]apptimeline.Entry{entry}))
	if err != nil {
		return err
	}
	for _, status := range statuses {
		if err := ts.event("update", status); err != nil {
			return err
		}
	}
	ts.sentEntries.add(e.EntryID)
	return nil
}
