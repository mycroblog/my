// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

func TestFeedRoute(t *testing.T) {
	h := &Handler{o: Options{Host: "example.org", AccountDomain: "example.com"}}
	tests := []struct {
		path    string
		kind    feedKind
		name    string
		ok      bool
		comment string
	}{
		{"/@alice.atom", feedProfile, "alice", true, "a profile"},
		{"/@alice@example.com.atom", feedProfile, "alice", true, "with the account domain"},
		{"/@alice@example.org.atom", feedProfile, "alice", true, "with the host"},
		{"/@alice@remote.example.atom", "", "", false, "a remote account has no feed here"},
		{"/@alice/status/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f.atom", "", "", false, "a post has none"},
		{"/@al.ice.atom", "", "", false, "not a username"},
		{"/@alice", "", "", false, "the page itself"},
		{"/tags/cats.atom", feedTag, "cats", true, "a hashtag"},
		{"/tags/猫.atom", feedTag, "猫", true, "a hashtag in another script"},
		{"/tags/.atom", "", "", false, "no hashtag"},
		{"/tags/a/b.atom", "", "", false, "a nested path"},
		{"/home.atom", "", "", false, "another page"},
	}
	for _, tt := range tests {
		kind, name, ok := h.feedRoute(tt.path)
		if kind != tt.kind || name != tt.name || ok != tt.ok {
			t.Errorf("feedRoute(%q) (%s) = %q, %q, %v; want %q, %q, %v", tt.path, tt.comment, kind, name, ok, tt.kind, tt.name, tt.ok)
		}
	}
}

func TestTagPage(t *testing.T) {
	for path, want := range map[string]string{"/tags/Cats": "cats", "/tags/猫": "猫", "/tags/": "", "/tags/a/b": "", "/tags/not-a-tag": ""} {
		tag, ok := tagPage(path)
		if ok != (want != "") || tag.NameNormalized != want {
			t.Errorf("tagPage(%q) = %+v, %v; want %q", path, tag, ok, want)
		}
	}
}

func TestAtomTime(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	b, err := xml.Marshal(struct {
		XMLName xml.Name `xml:"t"`
		T       atomTime `xml:"updated"`
	}{T: atomTime(time.Date(2026, 1, 2, 9, 4, 5, 600, jst))})
	if err != nil || !strings.Contains(string(b), "<updated>2026-01-02T00:04:05Z</updated>") {
		t.Errorf("atomTime = %s, %v; want RFC 3339 in UTC to the second", b, err)
	}
}
