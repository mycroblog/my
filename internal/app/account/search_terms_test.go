// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/domain/account"
)

func TestSearchTerms(t *testing.T) {
	local := account.Handle{AccountID: "a", Username: "Alice", UsernameNormalized: "alice", Domain: account.LocalDomain}
	remote := account.Handle{AccountID: "b", Username: "Bob", UsernameNormalized: "bob", Domain: "remote.example"}
	for _, c := range []struct {
		name        string
		h           account.Handle
		local       bool
		displayName string
		want        []string
	}{
		{"local without a display name", local, true, "", []string{"username:alice"}},
		{"remote", remote, false, "Bob", []string{"username:bob", "username:bob@remote.example", "display_name:bob"}},
		{"words", local, true, "Alice  Liddell", []string{"username:alice", "display_name:alice liddell", "display_name:alice", "display_name:liddell"}},
		{"NFKC", local, true, "ＡＬＩＣＥ", []string{"username:alice", "display_name:alice"}},
		{"shortcodes", local, true, ":blobcat: Alice :wave: L", []string{"username:alice", "display_name:alice l", "display_name:alice", "display_name:l"}},
		{"not shortcodes", local, true, "a:b: 12:30:00 :x:", []string{"username:alice", "display_name:a:b: 12:30:00 :x:", "display_name:a:b:", "display_name:12:30:00", "display_name::x:"}},
		{"CJK", local, true, "アリス・リデル", []string{"username:alice", "display_name:アリス・リデル"}},
		{"only shortcodes", local, true, ":blobcat:", []string{"username:alice"}},
		{"cut", local, true, strings.Repeat("あ", 150), []string{"username:alice", "display_name:" + strings.Repeat("あ", MaxSearchQueryLength)}},
	} {
		var got []string
		for _, term := range SearchTerms(c.h, c.local, true, c.displayName) {
			if term.AccountID != c.h.AccountID || term.IsLocal != c.local {
				t.Errorf("%s: term %+v", c.name, term)
			}
			if term.Kind == account.SearchTermInfix {
				continue
			}
			got = append(got, string(term.Kind)+":"+term.Term)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: SearchTerms = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSearchTermsInfixAndUndiscoverable(t *testing.T) {
	local := account.Handle{AccountID: "a", Username: "Ann", UsernameNormalized: "ann", Domain: account.LocalDomain}
	remote := account.Handle{AccountID: "b", Username: "Bo", UsernameNormalized: "bo", Domain: "remote.example"}
	for _, c := range []struct {
		name         string
		h            account.Handle
		local        bool
		discoverable bool
		displayName  string
		want         []string
	}{
		{"local", local, true, true, "Mo Li", []string{
			"username:ann", "display_name:mo li", "display_name:mo", "display_name:li",
			"infix:nn", "infix:n", "infix:o li", "infix:li", "infix:i",
		}},
		{"CJK", local, true, true, "アリス", []string{"username:ann", "display_name:アリス", "infix:nn", "infix:n", "infix:リス", "infix:ス"}},
		{"not discoverable", local, true, false, "Mo Li", []string{"exact_username:ann"}},
		{"remote not discoverable", remote, false, false, "Bo", []string{"username:bo", "username:bo@remote.example", "display_name:bo"}},
	} {
		var got []string
		for _, term := range SearchTerms(c.h, c.local, c.discoverable, c.displayName) {
			got = append(got, string(term.Kind)+":"+term.Term)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: SearchTerms = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestNormalizeSearchQuery(t *testing.T) {
	for q, want := range map[string]string{
		"@Alice":           "alice",
		"ＡＬＩ":              "ali",
		"alice \t lid":     "alice lid",
		"alfred@Remote.Ex": "alfred@remote.ex",
		"@" + strings.Repeat("a", MaxSearchQueryLength): strings.Repeat("a", MaxSearchQueryLength),
	} {
		if got, err := NormalizeSearchQuery(q); err != nil || got != want {
			t.Errorf("NormalizeSearchQuery(%q) = %q, %v; want %q", q, got, err, want)
		}
	}
	for _, q := range []string{"", "@", " ", strings.Repeat("a", MaxSearchQueryLength+1)} {
		if _, err := NormalizeSearchQuery(q); err == nil {
			t.Errorf("NormalizeSearchQuery(%q) = nil error", q)
		}
	}
}
