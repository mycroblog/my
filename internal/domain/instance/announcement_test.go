// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestAnnouncementLabel(t *testing.T) {
	for body, want := range map[string]string{
		"# Maintenance\n\nOn Sunday.": "Maintenance",
		"\n\n  - first item\n":        "first item",
		"> quoted":                    "quoted",
		strings.Repeat("a", 100):      strings.Repeat("a", 79) + "…",
		" \n ":                        "",
	} {
		if got := AnnouncementLabel("", body); got != want {
			t.Errorf("AnnouncementLabel(%q) = %q, want %q", body, got, want)
		}
	}
}

func TestAnnouncementLabelTitle(t *testing.T) {
	if got := AnnouncementLabel("Maintenance on Sunday", "# Other"); got != "Maintenance on Sunday" {
		t.Errorf("AnnouncementLabel = %q, want the title", got)
	}
}

func TestValidateAnnouncementTitle(t *testing.T) {
	runes := func(s string) int { return utf8.RuneCountInString(s) }
	for title, want := range map[string]bool{
		"Maintenance":                 false,
		"":                            true,
		" Maintenance":                true,
		"Maintenance\nOn Sunday":      true,
		"Tab\there":                   true,
		strings.Repeat("a", 100):      false,
		strings.Repeat("a", 101):      true,
		strings.Repeat("\u00e9", 100): false,
	} {
		err := ValidateAnnouncement(title, "Body", time.Time{}, time.Time{}, runes)
		if got := errors.Is(err, ErrAnnouncementTitle); got != want {
			t.Errorf("ValidateAnnouncement(%q) = %v, want the title error %v", title, err, want)
		}
	}
	// The limit counts grapheme clusters: one cluster of many code points is one.
	one := func(string) int { return 1 }
	if err := ValidateAnnouncement(strings.Repeat("e\u0301", 200), "Body", time.Time{}, time.Time{}, one); err != nil {
		t.Errorf("200 clusters counted as one: %v", err)
	}
	if err := ValidateAnnouncement(strings.Repeat("e\u0301", 501), "Body", time.Time{}, time.Time{}, one); !errors.Is(err, ErrAnnouncementTitle) {
		t.Errorf("over the code point bound: %v, want the title error", err)
	}
}

func TestAnnouncementHasShownByEndedBy(t *testing.T) {
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	a := Announcement{State: AnnouncementPublished, PublishedAt: at, Revision: AnnouncementRevision{StartsAt: at.Add(time.Hour), EndsAt: at.Add(2 * time.Hour)}}
	if a.HasShownBy(at) || !a.HasShownBy(at.Add(time.Hour)) || !a.HasShownBy(at.Add(3*time.Hour)) {
		t.Error("HasShownBy: want false before the start of the window, true from it on, ended or not")
	}
	if a.EndedBy(at.Add(time.Hour)) || !a.EndedBy(at.Add(2*time.Hour)) {
		t.Error("EndedBy: want true from the end of the window")
	}
	a.State = AnnouncementWithdrawn
	if a.HasShownBy(at.Add(time.Hour)) {
		t.Error("a withdrawn announcement has shown")
	}
	if (Announcement{Revision: AnnouncementRevision{}}).EndedBy(at) {
		t.Error("an announcement without an end has ended")
	}
}

func TestAnnouncementHeadApply(t *testing.T) {
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	h := AnnouncementHead{AnnouncementID: testID(1)}.Apply(testID(1), nil)
	if h.State != AnnouncementDraft || h.RevisionID != testID(1) || !h.PublishedAt.IsZero() {
		t.Fatalf("after a revision: %+v, want a draft", h)
	}
	h = h.Apply("", &AnnouncementStateChange{State: AnnouncementPublished, ChangedAt: at})
	h = h.Apply(testID(0), nil) // an older revision changes nothing
	if h.State != AnnouncementPublished || h.RevisionID != testID(1) || !h.PublishedAt.Equal(at) {
		t.Fatalf("after publishing: %+v", h)
	}
	h = h.Apply("", &AnnouncementStateChange{State: AnnouncementWithdrawn, ChangedAt: at.Add(time.Hour)})
	h = h.Apply("", &AnnouncementStateChange{State: AnnouncementPublished, ChangedAt: at.Add(2 * time.Hour)})
	if h.State != AnnouncementWithdrawn || !h.PublishedAt.Equal(at) {
		t.Errorf("after withdrawing: %+v, want withdrawn for good, published at %v", h, at)
	}
	// A draft withdrawn before any publication still has a time, as announcement_heads requires.
	d := AnnouncementHead{}.Apply(testID(1), &AnnouncementStateChange{State: AnnouncementWithdrawn, ChangedAt: at})
	if d.State != AnnouncementWithdrawn || !d.PublishedAt.Equal(at) {
		t.Errorf("withdrawn draft: %+v", d)
	}
}

func TestAnnouncementShowsAt(t *testing.T) {
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	a := Announcement{State: AnnouncementPublished, PublishedAt: at, Revision: AnnouncementRevision{StartsAt: at.Add(time.Hour), EndsAt: at.Add(2 * time.Hour)}}
	for offset, want := range map[time.Duration]bool{0: false, time.Hour: true, 90 * time.Minute: true, 2 * time.Hour: false} {
		if got := a.ShowsAt(at.Add(offset)); got != want {
			t.Errorf("ShowsAt(+%v) = %v, want %v", offset, got, want)
		}
	}
	if !a.ShowsFrom().Equal(at.Add(time.Hour)) {
		t.Errorf("ShowsFrom = %v, want the start of the window", a.ShowsFrom())
	}
	a.State = AnnouncementDraft
	if a.ShowsAt(at.Add(time.Hour)) {
		t.Error("a draft shows")
	}
}
