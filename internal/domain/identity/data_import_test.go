// SPDX-License-Identifier: AGPL-3.0-only

package identity_test

import (
	"encoding/csv"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/domain/identity"
)

// The files below are as Mastodon 4 exports them from Settings > Import and export.
const (
	mastodonFollowing = "Account address,Show boosts,Notify on new posts,Languages\n" +
		"alice@example.com,true,false,\n" +
		"bob@social.example,false,true,\"en,ja\"\n"
	mastodonBlocks    = "spammer@spam.example\ntroll@example.net\n"
	mastodonMutes     = "Account address,Hide notifications\nnoisy@example.com,true\nchatty@example.org,false\n"
	mastodonBookmarks = "https://example.com/users/alice/statuses/1\nhttps://social.example/@bob/2\n"
	mastodonDomains   = "spam.example\nexample.net\n"
)

func TestParseImportCSVReadsMastodonFiles(t *testing.T) {
	cases := []struct {
		kind identity.ImportKind
		text string
		want []identity.ImportEntry
	}{
		{identity.ImportFollowing, mastodonFollowing, []identity.ImportEntry{
			{Line: 2, Target: "alice@example.com", ShowBoosts: true, HideNotifications: true},
			{Line: 3, Target: "bob@social.example", Notify: true, HideNotifications: true},
		}},
		{identity.ImportBlocks, mastodonBlocks, []identity.ImportEntry{
			{Line: 1, Target: "spammer@spam.example", ShowBoosts: true, HideNotifications: true},
			{Line: 2, Target: "troll@example.net", ShowBoosts: true, HideNotifications: true},
		}},
		{identity.ImportMutes, mastodonMutes, []identity.ImportEntry{
			{Line: 2, Target: "noisy@example.com", ShowBoosts: true, HideNotifications: true},
			{Line: 3, Target: "chatty@example.org", ShowBoosts: true},
		}},
		{identity.ImportBookmarks, mastodonBookmarks, []identity.ImportEntry{
			{Line: 1, Target: "https://example.com/users/alice/statuses/1", ShowBoosts: true, HideNotifications: true},
			{Line: 2, Target: "https://social.example/@bob/2", ShowBoosts: true, HideNotifications: true},
		}},
		{identity.ImportDomainMutes, mastodonDomains, []identity.ImportEntry{
			{Line: 1, Target: "spam.example", ShowBoosts: true, HideNotifications: true},
			{Line: 2, Target: "example.net", ShowBoosts: true, HideNotifications: true},
		}},
		// A domains file with the header that Mastodon's import reads.
		{identity.ImportDomainMutes, "#domain\nspam.example\n", []identity.ImportEntry{
			{Line: 2, Target: "spam.example", ShowBoosts: true, HideNotifications: true},
		}},
		// The older following export had one address per line and no header.
		{identity.ImportFollowing, "@carol@example.com\r\n\r\ndave\n", []identity.ImportEntry{
			{Line: 1, Target: "carol@example.com", ShowBoosts: true, HideNotifications: true},
			{Line: 3, Target: "dave", ShowBoosts: true, HideNotifications: true},
		}},
		// A blocks file with the header of the newer exports, and a byte order mark.
		{identity.ImportBlocks, "\uFEFFAccount address\nerin@example.com\n", []identity.ImportEntry{
			{Line: 2, Target: "erin@example.com", ShowBoosts: true, HideNotifications: true},
		}},
	}
	for i, c := range cases {
		t.Run(fmt.Sprintf("%d-%s", i, c.kind), func(t *testing.T) {
			got, skipped, err := identity.ParseImportCSV(c.kind, c.text)
			if err != nil || len(skipped) != 0 {
				t.Fatalf("ParseImportCSV = %v, %v", skipped, err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("entries = %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestParseImportCSVSkipsInvalidAndDuplicateLines(t *testing.T) {
	text := "alice@example.com\nnot an address\nALICE@EXAMPLE.COM\nbob@\n@alice@example.com\n"
	got, skipped, err := identity.ParseImportCSV(identity.ImportBlocks, text)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Target != "alice@example.com" {
		t.Errorf("entries = %+v, want alice only", got)
	}
	want := []identity.ImportSkipped{
		{Line: 2, Value: "not an address", Reason: identity.ImportSkipInvalid},
		{Line: 3, Value: "ALICE@EXAMPLE.COM", Reason: identity.ImportSkipDuplicate},
		{Line: 4, Value: "bob@", Reason: identity.ImportSkipInvalid},
		{Line: 5, Value: "alice@example.com", Reason: identity.ImportSkipDuplicate},
	}
	if !reflect.DeepEqual(skipped, want) {
		t.Errorf("skipped = %+v\nwant %+v", skipped, want)
	}
	if _, skipped, _ := identity.ParseImportCSV(identity.ImportBookmarks, "ftp://example.com/1\nhttps://example.com/1\n"); len(skipped) != 1 {
		t.Errorf("bookmarks skipped = %+v, want the ftp URL", skipped)
	}
	domains, skipped, err := identity.ParseImportCSV(identity.ImportDomainMutes,
		"spam.example\nSPAM.example.\nhttps://spam.example/\nuser@spam.example\nspam.example:443\nbad..example\n")
	if err != nil {
		t.Fatal(err)
	}
	wantSkipped := []identity.ImportSkipped{
		{Line: 2, Value: "SPAM.example.", Reason: identity.ImportSkipDuplicate},
		{Line: 3, Value: "https://spam.example/", Reason: identity.ImportSkipInvalid},
		{Line: 4, Value: "user@spam.example", Reason: identity.ImportSkipInvalid},
		{Line: 5, Value: "spam.example:443", Reason: identity.ImportSkipInvalid},
		{Line: 6, Value: "bad..example", Reason: identity.ImportSkipInvalid},
	}
	if len(domains) != 1 || !reflect.DeepEqual(skipped, wantSkipped) {
		t.Errorf("domains = %+v, skipped = %+v\nwant spam.example and %+v", domains, skipped, wantSkipped)
	}
}

// TestParseImportCSVReadsFollowedHashtags checks the file of followed hashtags: one per line,
// with or without its #, each once whatever its case; a # alone is a blank line.
func TestParseImportCSVReadsFollowedHashtags(t *testing.T) {
	tags, skipped, err := identity.ParseImportCSV(identity.ImportFollowedTags, "cats\n#Dogs\nCATS\nnot a tag\n#\nfoo.bar\n\n")
	if err != nil {
		t.Fatal(err)
	}
	wantTags := []identity.ImportEntry{{Line: 1, Target: "cats"}, {Line: 2, Target: "Dogs"}}
	wantSkipped := []identity.ImportSkipped{
		{Line: 3, Value: "CATS", Reason: identity.ImportSkipDuplicate},
		{Line: 4, Value: "not a tag", Reason: identity.ImportSkipInvalid},
		{Line: 6, Value: "foo.bar", Reason: identity.ImportSkipInvalid},
	}
	for i := range wantTags {
		wantTags[i].ShowBoosts, wantTags[i].HideNotifications = true, true
	}
	if !reflect.DeepEqual(tags, wantTags) || !reflect.DeepEqual(skipped, wantSkipped) {
		t.Errorf("tags = %+v, skipped = %+v\nwant %+v and %+v", tags, skipped, wantTags, wantSkipped)
	}
	if got := identity.ImportFollowedTags.FileName(); got != "followed_hashtags.csv" || !identity.ImportFollowedTags.Valid() {
		t.Errorf("FileName = %q, Valid = %v", got, identity.ImportFollowedTags.Valid())
	}
}

func TestParseImportCSVLimitsTheEntries(t *testing.T) {
	var b strings.Builder
	for i := range identity.MaxImportEntries + 1 {
		fmt.Fprintf(&b, "user%d@example.com\n", i)
	}
	if _, _, err := identity.ParseImportCSV(identity.ImportBlocks, b.String()); !errors.Is(err, identity.ErrTooManyImportEntries) {
		t.Errorf("ParseImportCSV(too many) = %v, want ErrTooManyImportEntries", err)
	}
	var parseErr *csv.ParseError
	if _, _, err := identity.ParseImportCSV(identity.ImportBlocks, "\"alice@example.com\n"); !errors.As(err, &parseErr) {
		t.Errorf("ParseImportCSV(unterminated quote) = %v, want a csv.ParseError", err)
	}
}

// TestExportCSVRoundTrips checks that an exported file is the one Mastodon writes and reads back
// as the same entries.
func TestExportCSVRoundTrips(t *testing.T) {
	cases := []struct {
		kind identity.ImportKind
		rows []identity.ExportRow
		want string
	}{
		{identity.ImportFollowing, []identity.ExportRow{
			{Target: "alice@example.com", ShowBoosts: true}, {Target: "bob@social.example", Notify: true},
		}, "Account address,Show boosts,Notify on new posts,Languages\nalice@example.com,true,false,\nbob@social.example,false,true,\n"},
		{identity.ImportBlocks, []identity.ExportRow{{Target: "spammer@spam.example"}, {Target: "troll@example.net"}}, mastodonBlocks},
		{identity.ImportMutes, []identity.ExportRow{
			{Target: "noisy@example.com", HideNotifications: true}, {Target: "chatty@example.org"},
		}, mastodonMutes},
		{identity.ImportBookmarks, []identity.ExportRow{
			{Target: "https://example.com/users/alice/statuses/1"}, {Target: "https://social.example/@bob/2"},
		}, mastodonBookmarks},
		{identity.ImportDomainMutes, []identity.ExportRow{{Target: "spam.example"}, {Target: "example.net"}}, mastodonDomains},
		{identity.ImportFollowedTags, []identity.ExportRow{{Target: "Cats"}, {Target: "日本語"}}, "Cats\n日本語\n"},
	}
	for _, c := range cases {
		got, err := identity.ExportCSV(c.kind, c.rows)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != c.want {
			t.Errorf("%s = %q, want %q", c.kind, got, c.want)
		}
		entries, skipped, err := identity.ParseImportCSV(c.kind, string(got))
		if err != nil || len(skipped) != 0 || len(entries) != len(c.rows) {
			t.Fatalf("%s read back = %+v, %+v, %v", c.kind, entries, skipped, err)
		}
		for i, e := range entries {
			r := c.rows[i]
			if e.Target != r.Target ||
				(c.kind == identity.ImportFollowing && (e.ShowBoosts != r.ShowBoosts || e.Notify != r.Notify)) ||
				(c.kind == identity.ImportMutes && e.HideNotifications != r.HideNotifications) {
				t.Errorf("%s entry %d = %+v, want %+v", c.kind, i, e, r)
			}
		}
	}
}

func TestAddressKey(t *testing.T) {
	for in, want := range map[string]string{
		"Alice@Example.COM": "alice@example.com", "@bob@example.com": "bob@example.com", "carol": "carol@",
	} {
		if got, ok := identity.AddressKey(in); !ok || got != want {
			t.Errorf("AddressKey(%q) = %q, %v, want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "@", "a@b@c", "a b@example.com", "a@", "https://example.com/@a"} {
		if got, ok := identity.AddressKey(in); ok {
			t.Errorf("AddressKey(%q) = %q, want not an address", in, got)
		}
	}
}
