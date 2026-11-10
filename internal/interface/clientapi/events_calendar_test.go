// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// calendarEvents serves one event whose view the test sets.
type calendarEvents struct {
	fakeEvents
	v appgathering.View
}

func (c *calendarEvents) Get(context.Context, string, string) (appgathering.View, error) {
	return c.v, c.err
}

// getCalendar fetches the file of the event and returns its unfolded content lines, after
// checking that every line ends with CRLF, is at most 75 octets, and is valid UTF-8.
func getCalendar(t *testing.T, e *authEnv) (*http.Response, []string) {
	t.Helper()
	res := e.send(t, http.MethodGet, "/events/"+eventID+"/calendar.ics", "read-events", "")
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("calendar = %d: %s", res.StatusCode, body)
	}
	raw := string(body)
	if !strings.HasSuffix(raw, "\r\n") {
		t.Fatalf("the file does not end with CRLF: %q", raw)
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSuffix(raw, "\r\n"), "\r\n") {
		if strings.Contains(l, "\n") || len(l) > 75 || !utf8.ValidString(l) {
			t.Errorf("bad content line %q (%d octets)", l, len(l))
		}
		if strings.HasPrefix(l, " ") && len(lines) > 0 {
			lines[len(lines)-1] += l[1:]
			continue
		}
		lines = append(lines, l)
	}
	return res, lines
}

// property returns the value of the first property name, false without one.
func property(lines []string, name string) (string, bool) {
	for _, l := range lines {
		if v, ok := strings.CutPrefix(l, name+":"); ok {
			return v, true
		}
	}
	return "", false
}

func TestEventCalendar(t *testing.T) {
	f := &fakeEvents{}
	e := newEventsEnv(t, f)

	res, lines := getCalendar(t, e)
	if ct := res.Header.Get("Content-Type"); ct != "text/calendar" {
		t.Errorf("Content-Type = %q", ct)
	}
	if res.Header.Get("Cache-Control") != "no-store" || res.Header.Get("Content-Disposition") != `attachment; filename="`+eventID+`.ics"` {
		t.Errorf("headers = %v", res.Header)
	}
	want := map[string]string{
		"VERSION": "2.0", "METHOD": "PUBLISH", "UID": eventID + "@social.example",
		"DTSTART": "20261201T120000Z", "DTEND": "20261201T140000Z", "SEQUENCE": "1", "STATUS": "CONFIRMED",
		"SUMMARY": "Picnic", "URL": "https://social.example/events/" + eventID,
	}
	for name, value := range want {
		if got, ok := property(lines, name); !ok || got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
	if lines[0] != "BEGIN:VCALENDAR" || lines[len(lines)-1] != "END:VCALENDAR" || !slices.Contains(lines, "BEGIN:VEVENT") {
		t.Errorf("lines = %q", lines)
	}
	if _, ok := property(lines, "DTSTAMP"); !ok {
		t.Error("no DTSTAMP")
	}
	// The physical place is hidden from the viewer, and the voice place has no location.
	if _, ok := property(lines, "LOCATION"); ok {
		t.Errorf("LOCATION of hidden places: %q", lines)
	}
	if all := strings.Join(lines, "\n"); strings.Contains(all, "Secret park") || strings.Contains(all, "Hidden") {
		t.Errorf("the file carries a hidden place: %s", all)
	}

	f.err = appgathering.ErrNotFound
	if res := e.send(t, http.MethodGet, "/events/"+eventID+"/calendar.ics", "read-events", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("unseen event = %d", res.StatusCode)
	}
}

func TestEventCalendarPlacesAndText(t *testing.T) {
	c := &calendarEvents{}
	c.v = c.view()
	c.v.Revision.Settings.Title = "Picnic, lunch; and \\ games"
	c.v.Revision.Settings.Description = strings.Repeat("公園でピクニック。", 20) + "\nBring food."
	c.v.Places = []gathering.Place{
		{Kind: gathering.PlacePhysical, Name: "Central park", Address: "1-2-3 Chiyoda, Tokyo"},
		{Kind: gathering.PlaceVoice, Speakers: gathering.SpeakersManual},
		{Kind: gathering.PlaceURL, URL: "https://meet.example/abc"},
	}
	c.v.HiddenPlaces = nil
	cancelledAt := eventStart.Add(-time.Hour)
	c.v.Cancellation = &appgathering.Cancellation{GatheringID: eventID, Reason: gathering.CancelReason("organizer"), CancelledAt: cancelledAt}
	c.v.Head.State = gathering.StateCancelled
	c.v.Revisions = 3
	e := newEventsEnv(t, c)

	_, lines := getCalendar(t, e)
	want := map[string]string{
		"SUMMARY":       `Picnic\, lunch\; and \\ games`,
		"DESCRIPTION":   strings.Repeat("公園でピクニック。", 20) + `\nBring food.\n\nhttps://social.example/events/` + eventID,
		"LOCATION":      `Central park\, 1-2-3 Chiyoda\, Tokyo / https://meet.example/abc`,
		"STATUS":        "CANCELLED",
		"SEQUENCE":      "3",
		"LAST-MODIFIED": "20261201T110000Z",
	}
	for name, value := range want {
		if got, ok := property(lines, name); !ok || got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
}
