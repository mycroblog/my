// SPDX-License-Identifier: MIT

package mybot

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mycroblog/my/pkg/mykit"
)

func TestPlainText(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"<p>hello</p>", "hello"},
		{`<p><span class="h-card"><a href="https://example.com/@bot" class="u-url mention">@<span>bot</span></a></span> ping</p>`, "@bot ping"},
		{"<p>one<br>two<br/>three</p><p>four</p>", "one\ntwo\nthree\n\nfour"},
		{`<p>see <a href="https://example.com/x"><span class="invisible">https://</span>example.com/x</a></p>`, "see https://example.com/x"},
		{"<p>a &amp; b &lt;c&gt; &#39;d&#39;</p>", "a & b <c> 'd'"},
		{"<p>1 < 2</p>", "1 < 2"},
	} {
		if got := PlainText(tc.in); got != tc.want {
			t.Errorf("PlainText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseCommand(t *testing.T) {
	me := &mykit.CredentialAccount{Username: "bot", Acct: "bot", Domain: "example.com"}
	for _, tc := range []struct {
		in, name, args string
		ok             bool
	}{
		{"@bot ping", "ping", "", true},
		{"@Bot DICE 2d6  please ", "dice", "2d6  please", true},
		{"@bot@example.com, help", "help", "", true},
		{"@bot @bot hi there", "hi", "there", true},
		{"@alice @bot ping", "@alice", "@bot ping", true},
		{"ping", "ping", "", true},
		{"@bot", "", "", false},
		{"   ", "", "", false},
	} {
		name, args, ok := parseCommand(tc.in, me)
		if name != tc.name || args != tc.args || ok != tc.ok {
			t.Errorf("parseCommand(%q) = %q, %q, %v; want %q, %q, %v", tc.in, name, args, ok, tc.name, tc.args, tc.ok)
		}
	}
}

func TestReplyVisibility(t *testing.T) {
	for in, want := range map[mykit.Visibility]mykit.Visibility{
		mykit.VisibilityPublic:   mykit.VisibilityUnlisted,
		mykit.VisibilityUnlisted: mykit.VisibilityUnlisted,
		mykit.VisibilityLocal:    mykit.VisibilityLocal,
		mykit.VisibilityPrivate:  mykit.VisibilityPrivate,
		mykit.VisibilityDirect:   mykit.VisibilityDirect,
	} {
		if got := ReplyVisibility(in); got != want {
			t.Errorf("ReplyVisibility(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestFileStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	now := time.Now()
	for _, err := range []error{
		s.MarkProcessed(ctx, "old", now.Add(-8*24*time.Hour)),
		s.MarkProcessed(ctx, "new", now),
		s.SetPosition(ctx, "new"),
		s.SetLastRun(ctx, "daily", now),
		s.SetRefreshToken(ctx, "rt"),
		s.ForgetProcessed(ctx, now.Add(-7*24*time.Hour)),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("the state file's mode is %v, want 0600", perm)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("the directory holds %d files, want the state file alone", len(entries))
	}

	r, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := r.IsProcessed(ctx, "new"); !ok {
		t.Error("new is not processed after reopening")
	}
	if ok, _ := r.IsProcessed(ctx, "old"); ok {
		t.Error("old was not forgotten")
	}
	if p, _ := r.Position(ctx); p != "new" {
		t.Errorf("position = %q", p)
	}
	if at, _ := r.LastRun(ctx, "daily"); !at.Equal(now) {
		t.Errorf("last run = %v, want %v", at, now)
	}
	if rt, _ := r.RefreshToken(ctx); rt != "rt" {
		t.Errorf("refresh token = %q", rt)
	}

	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(path); err == nil {
		t.Error("NewFileStore read a broken file")
	}
}

func TestTracker(t *testing.T) {
	tr := newTracker()
	for _, id := range []string{"a", "b", "c"} {
		tr.add(id)
	}
	if p := tr.complete("b"); p != "" {
		t.Errorf("b before a moved the position to %q", p)
	}
	if p := tr.complete("a"); p != "b" {
		t.Errorf("after a and b: %q, want b", p)
	}
	tr.hold()
	if p := tr.complete("c"); p != "" {
		t.Errorf("held: %q", p)
	}
	if p := tr.release(); p != "c" {
		t.Errorf("released: %q, want c", p)
	}
}
