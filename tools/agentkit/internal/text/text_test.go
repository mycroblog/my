// SPDX-License-Identifier: AGPL-3.0-only

package text

import (
	"reflect"
	"testing"
)

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"4.4 content":  "44-content",
		"Decision（決定）": "decision決定",
		"users【リソース】":  "usersリソース",
		"6. リモートの Actor / オブジェクトの取得": "6-リモートの-actor--オブジェクトの取得",
		"`local` と連合":                 "local-と連合",
		"Current phase: M4 — Feature": "current-phase-m4--feature",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIDs(t *testing.T) {
	s := "FR-915 と NFR-COST-1、ADR-0070（#929）を見る。`custom_emojis` と `PermCreateCustomEmojis`。ADR-0070 again."
	if got := Requirements(s); !reflect.DeepEqual(got, []string{"FR-915", "NFR-COST-1"}) {
		t.Errorf("Requirements = %v", got)
	}
	if got := ADRs(s); !reflect.DeepEqual(got, []string{"ADR-0070"}) {
		t.Errorf("ADRs = %v", got)
	}
	if got := Issues(s); !reflect.DeepEqual(got, []string{"929"}) {
		t.Errorf("Issues = %v", got)
	}
	if got := Idents(s); !reflect.DeepEqual(got, []string{"custom_emojis", "PermCreateCustomEmojis"}) {
		t.Errorf("Idents = %v", got)
	}
}

func TestDependencies(t *testing.T) {
	body := "FR-915。ADR-0070（#929）に従う。**#929 がマージされるまで着手しない。**\nEpic: #925。\nBlocked by #930."
	if got := Dependencies(body); !reflect.DeepEqual(got, []string{"929", "930"}) {
		t.Errorf("Dependencies = %v", got)
	}
	body = "Depends on #2247. The rule is shared with the API (#2249) and the rebuild (#2250)."
	if got := Dependencies(body); !reflect.DeepEqual(got, []string{"2247"}) {
		t.Errorf("Dependencies = %v, want only the sentence that names the dependency", got)
	}
}

func TestTokens(t *testing.T) {
	if got := Tokens("abcdefgh"); got != 2 {
		t.Errorf("Tokens(ascii) = %d", got)
	}
	if got := Tokens("絵文字"); got != 3 {
		t.Errorf("Tokens(japanese) = %d", got)
	}
	if got := Ktok(1234); got != "1.2k" {
		t.Errorf("Ktok = %q", got)
	}
}
