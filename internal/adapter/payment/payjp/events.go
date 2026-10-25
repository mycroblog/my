// SPDX-License-Identifier: AGPL-3.0-only

package payjp

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

// PAY.JP lists the events newest first by an offset, with since and until in seconds, both
// included. The adapter reads them oldest first in windows of time that hold at most a page
// each: a window with more is narrowed, an empty one widened (ADR-0099 §5, §6).
const (
	// maxWindowRequests bounds the requests of one call; the cursor carries on from there.
	maxWindowRequests = 40
	// firstWindow is the width of the first window after an empty one.
	firstWindow = time.Hour
	// eventLag keeps the walk this far behind the clock, for an event of the last second not to
	// be passed before PAY.JP lists it.
	eventLag = 5 * time.Second
)

// eventWindow is the result of one call of walkEvents.
type eventWindow struct {
	// Events are the events of the windows read, oldest first.
	Events []event
	// Next is the time from which the next call reads; past Until when every event through
	// Until was read.
	Next time.Time
	// Until is the end of the listing: the time of the call.
	Until time.Time
}

// walkEvents reads the events from the time from (included) toward now, oldest first, until it
// has a page's worth of them or reached now, within maxWindowRequests requests.
func (p *Provider) walkEvents(ctx context.Context, from time.Time) (eventWindow, error) {
	now := p.o.Now().Add(-eventLag).Truncate(time.Second)
	if from.IsZero() {
		from = time.Unix(0, 0).UTC()
	}
	out := eventWindow{Next: from, Until: now}
	width := now.Sub(from) + time.Second
	if width > 366*24*time.Hour {
		width = firstWindow
	}
	for requests := 0; requests < maxWindowRequests && !out.Next.After(now); {
		end := out.Next.Add(width - time.Second)
		if end.After(now) {
			end = now
		}
		if width <= time.Second {
			// A single second with more than a page: read it all by its offsets.
			evs, n, err := p.eventsOfSecond(ctx, out.Next)
			requests += n
			if err != nil {
				return eventWindow{}, err
			}
			out.Events = append(out.Events, evs...)
			out.Next = out.Next.Add(time.Second)
			width = firstWindow
			if len(out.Events) > 0 {
				return out, nil
			}
			continue
		}
		page, err := p.listEvents(ctx, out.Next, end, 0)
		requests++
		if err != nil {
			return eventWindow{}, err
		}
		if page.HasMore {
			width = max(width/8, time.Second)
			continue
		}
		slices.Reverse(page.Data)
		out.Events = append(out.Events, page.Data...)
		out.Next = end.Add(time.Second)
		if len(page.Data) > 0 {
			return out, nil
		}
		width *= 4
	}
	return out, nil
}

// eventsOfSecond reads every event of one second, oldest first, and the requests it took.
func (p *Provider) eventsOfSecond(ctx context.Context, at time.Time) ([]event, int, error) {
	var out []event
	n := 0
	for offset := 0; ; offset += supporters.PageSize {
		page, err := p.listEvents(ctx, at, at, offset)
		n++
		if err != nil {
			return nil, n, err
		}
		out = append(out, page.Data...)
		if !page.HasMore || len(page.Data) == 0 {
			slices.Reverse(out)
			return out, n, nil
		}
	}
}

func (p *Provider) listEvents(ctx context.Context, since, until time.Time, offset int) (list[event], error) {
	q := url.Values{
		"limit": {strconv.Itoa(supporters.PageSize)}, "offset": {strconv.Itoa(offset)},
		"since": {strconv.FormatInt(since.Unix(), 10)}, "until": {strconv.FormatInt(until.Unix(), 10)},
	}
	var page list[event]
	err := p.get(ctx, "list the events", "/events", q, &page)
	return page, err
}

// timeCursor reads the cursor of the event lists: the Unix time to read from; "" is from.
func timeCursor(cursor string, from time.Time) (time.Time, error) {
	if cursor == "" {
		if from.IsZero() {
			return time.Time{}, nil
		}
		return from.Truncate(time.Second), nil
	}
	n, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("payjp: %w: cursor %q", supporters.ErrPaymentInvalidRequest, cursor)
	}
	return time.Unix(n, 0).UTC(), nil
}

// ListChanges implements supporters.PaymentProvider: the notices of the subscriptions' events
// and of the failed charges, oldest first. PAY.JP does not document how long it keeps the
// events, so it never fails with ErrChangesWindow (ADR-0099 §6).
func (p *Provider) ListChanges(ctx context.Context, since time.Time, cursor string) (supporters.ChangePage, error) {
	from, err := timeCursor(cursor, since)
	if err != nil {
		return supporters.ChangePage{}, err
	}
	w, err := p.walkEvents(ctx, from)
	if err != nil {
		return supporters.ChangePage{}, err
	}
	out := supporters.ChangePage{Until: w.Until}
	for _, ev := range w.Events {
		if n := noticeOf(ev); n.Kind == supporters.NoticeSubscription {
			out.Notices = append(out.Notices, n)
		}
	}
	if !w.Next.After(w.Until) {
		out.Next = strconv.FormatInt(w.Next.Unix(), 10)
	}
	return out, nil
}
