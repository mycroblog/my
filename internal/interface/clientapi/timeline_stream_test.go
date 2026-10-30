// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/stream"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/timeline"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// publicEntryCall is one call of fakePublicEntries.
type publicEntryCall struct {
	timeline string
	viewer   appcontent.Viewer
	id       string
	query    apptimeline.Query
}

// fakePublicEntries lists the entries of listed[timeline] ("local", "public") and records its
// calls.
type fakePublicEntries struct {
	mu     sync.Mutex
	listed map[string][]string
	calls  []publicEntryCall
}

func (f *fakePublicEntries) entry(tl string, v appcontent.Viewer, id string, q apptimeline.Query) (apptimeline.Entry, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, publicEntryCall{timeline: tl, viewer: v, id: id, query: q})
	if !slices.Contains(f.listed[tl], id) {
		return apptimeline.Entry{}, false, nil
	}
	return apptimeline.Entry{ID: id, Kind: timeline.KindPost, StatusID: id, AuthorAccountID: aliceAccountID, OriginalAuthorAccountID: aliceAccountID}, true, nil
}

func (f *fakePublicEntries) LocalEntry(_ context.Context, v appcontent.Viewer, id string, q apptimeline.Query) (apptimeline.Entry, bool, error) {
	return f.entry("local", v, id, q)
}

func (f *fakePublicEntries) PublicEntry(_ context.Context, v appcontent.Viewer, id string, q apptimeline.Query) (apptimeline.Entry, bool, error) {
	return f.entry("public", v, id, q)
}

func (f *fakePublicEntries) called() []publicEntryCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// switchableInstance is an InstanceInfo whose settings a test may change while a stream reads
// them.
type switchableInstance struct {
	mu       sync.Mutex
	settings []instance.CurrentSetting
}

func (f *switchableInstance) set(settings ...instance.CurrentSetting) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.settings = settings
}

func (f *switchableInstance) Settings(context.Context) (instance.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return instance.NewSettings(f.settings), nil
}

func (f *switchableInstance) Text(context.Context, instance.TextKey) (instance.Text, bool, error) {
	return instance.Text{}, false, nil
}

// lockedViews serializes the calls of a fakeViews, which several streams read at once.
type lockedViews struct {
	mu sync.Mutex
	v  *fakeViews
}

func (l *lockedViews) Views(ctx context.Context, viewer appcontent.Viewer, ids []string) ([]appcontent.StatusView, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.v.Views(ctx, viewer, ids)
}

func (l *lockedViews) Boosts(ctx context.Context, ids []string) ([]content.Boost, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.v.Boosts(ctx, ids)
}

func (l *lockedViews) Tag(ctx context.Context, name string) (content.Tag, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.v.Tag(ctx, name)
}

func (l *lockedViews) Pinned(ctx context.Context, viewer appcontent.Viewer, accountID string) ([]appcontent.StatusView, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.v.Pinned(ctx, viewer, accountID)
}

// timelineStreamEnv serves the streams of the public timelines on hub, with the statuses of
// newTimelineEnv.
type timelineStreamEnv struct {
	*authEnv
	hub      *stream.Hub
	entries  *fakePublicEntries
	instance *switchableInstance
	srv      *httptest.Server
}

func newTimelineStreamEnv(t *testing.T, o stream.HubOptions, tweak func(*clientapi.Options)) *timelineStreamEnv {
	t.Helper()
	views := &lockedViews{v: newTimelineEnv(t).views}
	e := &timelineStreamEnv{
		hub:      stream.NewHub(o),
		entries:  &fakePublicEntries{listed: map[string][]string{}},
		instance: &switchableInstance{},
	}
	e.authEnv = newAuthEnv(t, func(o *clientapi.Options) {
		o.Streams, o.TimelineStreams, o.PublicEntries = e.hub, e.hub, e.entries
		o.TimelineReader, o.Instance = views, e.instance
		o.TimelineLanguages = func(context.Context, string) ([]string, error) { return []string{"ja"}, nil }
		o.StreamHeartbeat = 20 * time.Millisecond
		if tweak != nil {
			tweak(o)
		}
	})
	e.srv = httptest.NewServer(e.h)
	t.Cleanup(e.srv.Close)
	return e
}

// open opens the stream of path with token and returns its response and a reader of its lines;
// canceling ctx disconnects.
func (e *timelineStreamEnv) open(ctx context.Context, t *testing.T, path, token string) (*http.Request, *http.Response, *bufio.Reader) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.srv.URL+clientapi.BasePath+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", path, res.StatusCode)
	}
	r := bufio.NewReader(res.Body)
	if line := readLine(t, r); line != ": connected" {
		t.Fatalf("first line = %q, want the connected comment", line)
	}
	return req, res, r
}

// nextEvent returns the name and data of the next event of r, skipping heartbeats.
func nextEvent(t *testing.T, r *bufio.Reader) (string, string) {
	t.Helper()
	for {
		line := readLine(t, r)
		if strings.HasPrefix(line, ":") {
			continue
		}
		name, ok := strings.CutPrefix(line, "event: ")
		if !ok {
			t.Fatalf("line = %q, want an event", line)
		}
		data, ok := strings.CutPrefix(readLine(t, r), "data: ")
		if !ok {
			t.Fatal("an event without data")
		}
		return name, data
	}
}

func TestTimelineStreamDeliversTheTimelinesPosts(t *testing.T) {
	e := newTimelineStreamEnv(t, stream.HubOptions{}, nil)
	e.entries.listed = map[string][]string{"local": {postID}, "public": {postID}}
	req, res, local := e.open(t.Context(), t, "/streaming/timelines/local", "read")
	_, _, federated := e.open(t.Context(), t, "/streaming/timelines/federated", "read")
	e.spec.ValidateResponse(t, req, res.StatusCode, res.Header, nil)
	for k, want := range map[string]string{"Content-Type": "text/event-stream", "Cache-Control": "no-store"} {
		if got := res.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	// The signals of the other timeline, of the home timeline, and of entries the timeline does
	// not list send nothing; the next update of each stream is the post.
	e.hub.Publish(stream.Event{Kind: stream.KindHomeEntry, EntryID: postID})
	e.hub.Publish(stream.Event{Kind: stream.KindPublicEntry, EntryID: goneID, OnLocal: true, OnFederated: true})
	e.hub.Publish(stream.Event{Kind: stream.KindPublicEntry, EntryID: postID, OnLocal: true})
	e.hub.Publish(stream.Event{Kind: stream.KindPublicEntry, EntryID: postID, OnFederated: true})
	for name, r := range map[string]*bufio.Reader{"local": local, "federated": federated} {
		event, data := nextEvent(t, r)
		if event != "update" {
			t.Fatalf("%s event = %q, want update", name, event)
		}
		e.spec.ValidateSchema(t, "Status", []byte(data))
		var st clientapi.Status
		if err := json.Unmarshal([]byte(data), &st); err != nil {
			t.Fatal(err)
		}
		if st.Id != postID || st.Account.Username != "alice" {
			t.Errorf("%s update = %+v, want alice's post", name, st)
		}
	}
	// A signal delivered twice sends its post once.
	e.hub.Publish(stream.Event{Kind: stream.KindPublicEntry, EntryID: postID, OnLocal: true, OnFederated: true})
	viewer := appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: aliceAccountID}
	want := []publicEntryCall{
		{"local", viewer, goneID, apptimeline.Query{Languages: []string{"ja"}}},
		{"local", viewer, postID, apptimeline.Query{Languages: []string{"ja"}}},
		{"public", viewer, goneID, apptimeline.Query{Languages: []string{"ja"}}},
		{"public", viewer, postID, apptimeline.Query{Languages: []string{"ja"}}},
	}
	byTimelineAndID := func(a, b publicEntryCall) int { return strings.Compare(a.timeline+a.id, b.timeline+b.id) }
	slices.SortFunc(want, byTimelineAndID)
	deadline := time.Now().Add(time.Second)
	for {
		calls := e.entries.called()
		slices.SortFunc(calls, byTimelineAndID)
		if slices.EqualFunc(calls, want, func(a, b publicEntryCall) bool {
			return a.timeline == b.timeline && a.viewer == b.viewer && a.id == b.id && slices.Equal(a.query.Languages, b.query.Languages)
		}) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("calls = %+v, want %+v", calls, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if line := readLine(t, local); line != ": heartbeat" {
		t.Errorf("after the repeated signal, line = %q, want a heartbeat", line)
	}
}

func TestTimelineStreamAccess(t *testing.T) {
	e := newTimelineStreamEnv(t, stream.HubOptions{}, nil)
	// Anonymous requests are a 401, whatever the timeline settings allow visitors.
	for _, path := range []string{"/streaming/timelines/local", "/streaming/timelines/federated"} {
		res := e.get(t, path, nil)
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous %s = %d, want 401", path, res.StatusCode)
		}
		if p := decodeProblem(t, res); p.Type != clientapi.ProblemTypeUnauthenticated {
			t.Errorf("anonymous %s problem = %+v", path, p)
		}
	}
	// Another timeline is a 404.
	if res := e.get(t, "/streaming/timelines/home", map[string]string{"Authorization": "Bearer read"}); res.StatusCode != http.StatusNotFound {
		t.Errorf("home = %d, want 404", res.StatusCode)
	}
	// A timeline turned off is a 404; the other one opens.
	e.instance.set(instance.CurrentSetting{Key: instance.SettingLocalTimelineAccess, RevisionID: testkit.NamedID("revision-1"), Value: "off"})
	res := e.get(t, "/streaming/timelines/local", map[string]string{"Authorization": "Bearer read", "Accept-Language": "ja"})
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("local turned off = %d, want 404", res.StatusCode)
	}
	if p := decodeProblem(t, res); p.Type != clientapi.ProblemTypeBlank || p.Title != "指定されたリソースが見つかりません。" {
		t.Errorf("problem = %+v", p)
	}
	_, _, r := e.open(t.Context(), t, "/streaming/timelines/federated", "read")

	// Turning the open timeline off ends its stream at the next heartbeat.
	e.instance.set(instance.CurrentSetting{Key: instance.SettingFederatedTimelineAccess, RevisionID: testkit.NamedID("revision-2"), Value: "off"})
	waitEnd(t, r)
	waitReleased(t, e.hub)
	if got := len(e.entries.called()); got != 0 {
		t.Errorf("%d entries read, want none", got)
	}
}

func TestTimelineStreamCountsTowardTheStreamLimit(t *testing.T) {
	e := newTimelineStreamEnv(t, stream.HubOptions{MaxPerAccount: 2}, nil)
	_, _, _ = e.open(t.Context(), t, "/streaming", "read")
	_, _, _ = e.open(t.Context(), t, "/streaming/timelines/local", "read")
	for _, path := range []string{"/streaming/timelines/federated", "/streaming"} {
		res := e.get(t, path, map[string]string{"Authorization": "Bearer read"})
		if res.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("%s past the limit = %d, want 429", path, res.StatusCode)
		}
		if p := decodeProblem(t, res); p.Type != clientapi.ProblemTypeTooManyStreams || res.Header.Get("Content-Language") != "ja" {
			t.Errorf("problem = %+v (%s), want too-many-streams in alice's locale", p, res.Header.Get("Content-Language"))
		}
	}
	// Another account has its own streams.
	e.open(t.Context(), t, "/streaming/timelines/federated", "bob")
}

func TestTimelineStreamCountsInTheConnections(t *testing.T) {
	m := testkit.NewMeter()
	e := newTimelineStreamEnv(t, stream.HubOptions{}, func(o *clientapi.Options) { o.Meter = m })
	kind := map[string]string{"my.stream.kind": "timeline"}
	ctx, disconnect := context.WithCancel(t.Context())
	e.open(ctx, t, "/streaming/timelines/local", "read")
	if got := m.Sum("my.stream.connections", kind); got != 1 {
		t.Errorf("my.stream.connections = %v with a stream open, want 1", got)
	}
	disconnect()
	waitReleased(t, e.hub)
	if got := m.Sum("my.stream.connections", kind); got != 0 {
		t.Errorf("my.stream.connections = %v after the stream ended, want 0", got)
	}
}

func TestTimelineStreamBrokerUnavailable(t *testing.T) {
	broker := stream.NewMemoryBroker()
	broker.SetUnreachable(true)
	relay := stream.NewRelay(broker.Bus(), stream.RelayOptions{ReadyTimeout: 20 * time.Millisecond})
	t.Cleanup(func() { _ = relay.Close(context.Background()) })
	e := newTimelineStreamEnv(t, stream.HubOptions{}, func(o *clientapi.Options) { o.TimelineStreams = relay })
	res := e.get(t, "/streaming/timelines/local", map[string]string{"Authorization": "Bearer read"})
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", res.StatusCode)
	}
	if got := res.Header.Get("Retry-After"); got == "" || got == "0" {
		t.Errorf("Retry-After = %q, want seconds", got)
	}

	// Without a broker the stream is unavailable too.
	e = newTimelineStreamEnv(t, stream.HubOptions{}, func(o *clientapi.Options) { o.TimelineStreams = nil })
	if res := e.get(t, "/streaming/timelines/federated", map[string]string{"Authorization": "Bearer read"}); res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("without a broker = %d, want 503", res.StatusCode)
	}
}
