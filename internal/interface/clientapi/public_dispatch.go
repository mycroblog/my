// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"fmt"
	"sync"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/stream"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
)

// publicResultTTL is how long the dispatcher keeps what it decided and presented of an entry for
// the streams that handle the entry's signal after the first one.
const publicResultTTL = time.Minute

// publicDispatchTimeout bounds the shared work of one entry, which no request owns.
const publicDispatchTimeout = 30 * time.Second

// publicEntriesBatch are PublicEntries that decide an entry for many viewers at once, as
// *apptimeline.Service does.
type publicEntriesBatch interface {
	LocalEntryFor(ctx context.Context, viewers []apptimeline.StreamViewer, id string) (map[string]apptimeline.Entry, error)
	PublicEntryFor(ctx context.Context, viewers []apptimeline.StreamViewer, id string) (map[string]apptimeline.Entry, error)
}

// viewsBatch is a TimelineReader that presents statuses to many viewers at once, as
// *appcontent.Reader does.
type viewsBatch interface {
	ViewsFor(ctx context.Context, viewers []appcontent.Viewer, ids []string) (map[string][]appcontent.StatusView, error)
}

// publicDispatch is the per-process dispatcher of the streams of the public timelines (#2684):
// the first stream that handles the signal of a new entry decides it and presents its status for
// every stream of the timeline open at that moment, with a number of queries that does not grow
// with them, and the others take what it made for them. A stream opened after that decides alone.
type publicDispatch struct {
	mu      sync.Mutex
	streams map[stream.Timeline]map[*timelineStream]bool
	results map[publicKey]*publicResult
}

type publicKey struct {
	timeline stream.Timeline
	entryID  string
}

// publicResult is what the dispatcher made of an entry: the statuses of each stream that lists
// it, nil for one that does not, once ready is closed.
type publicResult struct {
	ready    chan struct{}
	statuses map[*timelineStream][]Status
	err      error
}

func newPublicDispatch() *publicDispatch {
	return &publicDispatch{streams: map[stream.Timeline]map[*timelineStream]bool{}, results: map[publicKey]*publicResult{}}
}

// add registers an open stream; remove forgets it.
func (d *publicDispatch) add(ts *timelineStream) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.streams[ts.timeline] == nil {
		d.streams[ts.timeline] = map[*timelineStream]bool{}
	}
	d.streams[ts.timeline][ts] = true
}

func (d *publicDispatch) remove(ts *timelineStream) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.streams[ts.timeline], ts)
}

// statuses returns the statuses that ts sends for the entry id, made for every stream of its
// timeline by the first of them to ask; ok is false when ts was not open when they were made.
func (d *publicDispatch) statuses(ctx context.Context, s *Server, ts *timelineStream, id string) (sts []Status, ok bool, err error) {
	key := publicKey{timeline: ts.timeline, entryID: id}
	d.mu.Lock()
	res, found := d.results[key]
	var streams []*timelineStream
	if !found {
		res = &publicResult{ready: make(chan struct{})}
		d.results[key] = res
		for st := range d.streams[ts.timeline] {
			streams = append(streams, st)
		}
	}
	d.mu.Unlock()
	if !found {
		work, cancel := context.WithTimeout(context.WithoutCancel(ctx), publicDispatchTimeout)
		res.statuses, res.err = s.publicStatuses(work, ts.timeline, id, streams)
		cancel()
		close(res.ready)
		time.AfterFunc(publicResultTTL, func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			if d.results[key] == res {
				delete(d.results, key)
			}
		})
	}
	select {
	case <-res.ready:
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
	if res.err != nil {
		return nil, false, res.err
	}
	sts, ok = res.statuses[ts]
	return sts, ok, nil
}

// publicStatuses decides the entry id for each of streams with one batch read, and presents its
// status to each viewer that its timeline lists it for with the shared parts read once. A stream
// whose account another stream opened with other settings (its scopes, its languages) is left
// out, to decide alone.
func (s *Server) publicStatuses(ctx context.Context, tl stream.Timeline, id string, streams []*timelineStream) (map[*timelineStream][]Status, error) {
	out := make(map[*timelineStream][]Status, len(streams))
	entries, okE := s.o.PublicEntries.(publicEntriesBatch)
	views, okV := s.o.TimelineReader.(viewsBatch)
	if !okE || !okV {
		return out, nil // each stream decides alone
	}
	settings := map[string]string{}
	var viewers []apptimeline.StreamViewer
	var kept []*timelineStream
	for _, st := range streams {
		v := apptimeline.StreamViewer{Viewer: localContentViewer(st.viewer), Languages: st.home.Languages}
		key := fmt.Sprint(v.Viewer.DirectDenied, v.Languages)
		switch k, seen := settings[v.Viewer.AccountID]; {
		case !seen:
			settings[v.Viewer.AccountID] = key
			viewers = append(viewers, v)
		case k != key:
			continue
		}
		kept = append(kept, st)
	}
	decide := entries.PublicEntryFor
	if tl == stream.TimelineLocal {
		decide = entries.LocalEntryFor
	}
	listed, err := decide(ctx, viewers, id)
	if err != nil {
		return nil, err
	}
	var entry apptimeline.Entry
	var shown []appcontent.Viewer
	for _, v := range viewers {
		if e, ok := listed[v.Viewer.AccountID]; ok {
			entry = e
			shown = append(shown, v.Viewer)
		}
	}
	for _, st := range kept {
		out[st] = nil
	}
	if len(shown) == 0 {
		return out, nil
	}
	items := entryItems([]apptimeline.Entry{entry})
	if items[0].BoostID != "" {
		return map[*timelineStream][]Status{}, nil // the public timelines list posts; a boost decides alone
	}
	byViewer, err := s.statusesFor(ctx, views, shown, items)
	if err != nil {
		return nil, err
	}
	for _, st := range kept {
		if _, ok := listed[st.viewer.AccountID]; ok {
			out[st] = byViewer[st.viewer.AccountID]
		}
	}
	return out, nil
}

// statusesFor is statuses of the posts of items for each of viewers, by viewer account, with
// the statuses presented to all of them through one ViewsFor and their accounts read once.
func (s *Server) statusesFor(ctx context.Context, views viewsBatch, viewers []appcontent.Viewer, items []statusItem) (map[string][]Status, error) {
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.StatusID
	}
	byViewer, err := views.ViewsFor(ctx, viewers, ids)
	if err != nil {
		return nil, err
	}
	var all []appcontent.StatusView
	for _, v := range viewers {
		all = append(all, byViewer[v.AccountID]...)
	}
	parts, err := s.statusParts(ctx, all)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]Status, len(viewers))
	for _, v := range viewers {
		byID := map[string]Status{}
		for _, view := range byViewer[v.AccountID] {
			byID[view.Status.ID] = s.newStatus(view, parts)
		}
		sts := make([]Status, 0, len(items))
		for _, it := range items {
			if st, ok := byID[it.StatusID]; ok {
				sts = append(sts, st)
			}
		}
		out[v.AccountID] = sts
	}
	return out, nil
}
