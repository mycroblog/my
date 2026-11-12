// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/domain/content"
)

func TestParseVisibility(t *testing.T) {
	for _, v := range content.Visibilities() {
		got, err := content.ParseVisibility(string(v))
		if err != nil || got != v {
			t.Errorf("ParseVisibility(%q) = %q, %v", v, got, err)
		}
	}
	for _, bad := range []string{"", "Public", "followers", "limited"} {
		if _, err := content.ParseVisibility(bad); !errors.Is(err, content.ErrInvalidVisibility) {
			t.Errorf("ParseVisibility(%q) = %v, want ErrInvalidVisibility", bad, err)
		}
	}
}

// Effective visibility = min(original, ceiling) (ADR-0058 §11).
func TestEffectiveVisibility(t *testing.T) {
	silenced := content.AuthorRestrictions{VisibilityCeilingPrivate: true}
	cases := []struct {
		original               content.Visibility
		unrestricted, silenced content.Visibility
	}{
		{content.VisibilityPublic, content.VisibilityPublic, content.VisibilityPrivate},
		{content.VisibilityLocal, content.VisibilityLocal, content.VisibilityPrivate},
		{content.VisibilityUnlisted, content.VisibilityUnlisted, content.VisibilityPrivate},
		{content.VisibilityPrivate, content.VisibilityPrivate, content.VisibilityPrivate},
		{content.VisibilityDirect, content.VisibilityDirect, content.VisibilityDirect},
	}
	for _, c := range cases {
		if got := content.EffectiveVisibility(c.original, content.AuthorRestrictions{}); got != c.unrestricted {
			t.Errorf("EffectiveVisibility(%s, none) = %s, want %s", c.original, got, c.unrestricted)
		}
		if got := content.EffectiveVisibility(c.original, silenced); got != c.silenced {
			t.Errorf("EffectiveVisibility(%s, silenced) = %s, want %s", c.original, got, c.silenced)
		}
	}
	if got := content.EffectiveVisibility("bogus", silenced); got.Valid() {
		t.Errorf("EffectiveVisibility(bogus) = %s, want an invalid value", got)
	}
	if got := content.Narrower(content.VisibilityPublic, "bogus"); got.Valid() {
		t.Errorf("Narrower(public, bogus) = %s, want an invalid value", got)
	}
}

func TestPostingVisibilities(t *testing.T) {
	if got := content.PostingVisibilities(content.AuthorRestrictions{}); !slices.Equal(got, content.Visibilities()) {
		t.Errorf("PostingVisibilities(none) = %v, want every visibility", got)
	}
	silenced := content.AuthorRestrictions{VisibilityCeilingPrivate: true}
	want := []content.Visibility{content.VisibilityPrivate, content.VisibilityDirect}
	if got := content.PostingVisibilities(silenced); !slices.Equal(got, want) {
		t.Errorf("PostingVisibilities(silenced) = %v, want %v", got, want)
	}
}
