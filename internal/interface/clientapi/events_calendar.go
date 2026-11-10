// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The iCalendar file of an event (ADR-0161 §15): one VEVENT of METHOD:PUBLISH, written by hand
// to RFC 5545. The times are in UTC, so that every calendar shows the same instants in its own
// time zone; the places are those of the View, which holds only the places that the viewer sees.

// calendarProductID is the PRODID of the files.
const calendarProductID = "-//mycroblog//my//EN"

// GetEventCalendar serves GET /api/v1/events/{id}/calendar.ics.
func (s *Server) GetEventCalendar(ctx context.Context, req GetEventCalendarRequestObject) (GetEventCalendarResponseObject, error) {
	events, v, p, err := s.events(ctx)
	if p != nil || err != nil {
		return respond[GetEventCalendardefaultApplicationProblemPlusJSONResponse](p, err)
	}
	view, err := events.Get(ctx, v.AccountID, req.Id)
	if err != nil {
		p, err := s.eventFail(ctx, err)
		return respond[GetEventCalendardefaultApplicationProblemPlusJSONResponse](p, err)
	}
	var b bytes.Buffer
	writeEventCalendar(&b, view, calendarSource{Host: s.o.Host, PageURL: s.origin() + "/events/" + view.ID, Now: time.Now()})
	cache := cacheNoStore
	disposition := `attachment; filename="` + view.ID + `.ics"`
	return GetEventCalendar200TextcalendarResponse{
		Body: &b, ContentLength: int64(b.Len()),
		Headers: GetEventCalendar200ResponseHeaders{CacheControl: &cache, ContentDisposition: &disposition},
	}, nil
}

// calendarSource is what the file of an event says besides the event: the host of its UID, the
// link to its page, and when the file is written (DTSTAMP).
type calendarSource struct {
	Host    string
	PageURL string
	Now     time.Time
}

// writeEventCalendar writes the iCalendar file of v to b.
func writeEventCalendar(b *bytes.Buffer, v appgathering.View, src calendarSource) {
	w := icalWriter{b: b}
	settings := v.Revision.Settings
	w.line("BEGIN", "VCALENDAR")
	w.line("VERSION", "2.0")
	w.line("PRODID", calendarProductID)
	w.line("CALSCALE", "GREGORIAN")
	w.line("METHOD", "PUBLISH")
	w.line("BEGIN", "VEVENT")
	w.line("UID", v.ID+"@"+src.Host)
	w.line("DTSTAMP", icalTime(src.Now))
	w.line("DTSTART", icalTime(v.Head.Schedule.StartsAt))
	w.line("DTEND", icalTime(v.Head.Schedule.EndsAt))
	if m := lastModified(v.Record); !m.IsZero() {
		w.line("LAST-MODIFIED", icalTime(m))
	}
	w.line("SEQUENCE", strconv.Itoa(v.Revisions))
	if v.Cancellation != nil || v.Head.State == gathering.StateCancelled {
		w.line("STATUS", "CANCELLED")
	} else {
		w.line("STATUS", "CONFIRMED")
	}
	w.line("SUMMARY", icalText(settings.Title))
	description := settings.Description
	if description != "" {
		description += "\n\n"
	}
	// Some calendars show no URL property, so the description ends with the link too.
	w.line("DESCRIPTION", icalText(description+src.PageURL))
	if loc := calendarLocation(v.Places); loc != "" {
		w.line("LOCATION", icalText(loc))
	}
	w.line("URL", src.PageURL)
	w.line("END", "VEVENT")
	w.line("END", "VCALENDAR")
}

// lastModified returns the latest change of the event's revision, date, and cancellation.
func lastModified(r appgathering.Record) time.Time {
	m := r.Revision.RevisedAt
	if r.Confirmation.ConfirmedAt.After(m) {
		m = r.Confirmation.ConfirmedAt
	}
	if r.Cancellation != nil && r.Cancellation.CancelledAt.After(m) {
		m = r.Cancellation.CancelledAt
	}
	return m
}

// calendarLocation returns the LOCATION of the places that the viewer sees: the physical place
// (its name and address) and the URL place (its name and URL), in their order. A voice chat
// room has no location of its own; the event's page leads to it.
func calendarLocation(places []gathering.Place) string {
	var parts []string
	for _, p := range places {
		var fields []string
		switch p.Kind {
		case gathering.PlacePhysical:
			fields = []string{p.Name, p.Address}
		case gathering.PlaceURL:
			fields = []string{p.Name, p.URL}
		default:
			continue
		}
		var nonEmpty []string
		for _, f := range fields {
			if f = strings.TrimSpace(f); f != "" {
				nonEmpty = append(nonEmpty, f)
			}
		}
		if len(nonEmpty) > 0 {
			parts = append(parts, strings.Join(nonEmpty, ", "))
		}
	}
	return strings.Join(parts, " / ")
}

// icalTime writes t as a DATE-TIME in UTC (RFC 5545 §3.3.5, form 2).
func icalTime(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// icalText escapes s as a TEXT value (RFC 5545 §3.3.11): backslashes, semicolons, and commas
// are escaped, line breaks become \n, and the other control characters are dropped.
func icalText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	s = strings.ReplaceAll(s, "\r\n", "\n")
	for _, r := range s {
		switch {
		case r == '\\' || r == ';' || r == ',':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n' || r == '\r':
			b.WriteString(`\n`)
		case r < 0x20 || r == 0x7f:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// icalWriter writes content lines (RFC 5545 §3.1): each ends with CRLF and is folded so that no
// line is longer than 75 octets, never inside a UTF-8 sequence.
type icalWriter struct{ b *bytes.Buffer }

// icalLineOctets is the longest line, without its CRLF.
const icalLineOctets = 75

// line writes the content line name:value; value is escaped already.
func (w icalWriter) line(name, value string) {
	s := name + ":" + value
	limit := icalLineOctets
	for len(s) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		w.b.WriteString(s[:cut])
		w.b.WriteString("\r\n ")
		s = s[cut:]
		// A continuation line starts with the space, which counts toward its octets.
		limit = icalLineOctets - 1
	}
	w.b.WriteString(s)
	w.b.WriteString("\r\n")
}
