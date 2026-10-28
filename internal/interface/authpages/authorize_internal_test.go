// SPDX-License-Identifier: AGPL-3.0-only

package authpages

import (
	"fmt"
	"testing"

	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
)

// TestScopeDescriptions checks that the consent screen words every scope the server knows,
// in every locale, instead of showing the scope token.
func TestScopeDescriptions(t *testing.T) {
	messages, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"en", "ja"} {
		l := messages.Localizer(locale)
		for _, s := range identity.KnownScopes() {
			if got := scopeDescription(l, s); got == s || got == "" {
				t.Errorf("%s: scope %s has no description (%q)", locale, s, got)
			}
		}
	}
}

// TestGroupScopes checks that the consent screen puts the requested scopes under the heading
// of their weight, the heaviest first, and leaves out a weight with no scope.
func TestGroupScopes(t *testing.T) {
	messages, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	l := messages.Localizer("en")
	groups := groupScopes(l, []string{
		identity.ScopeReadStatuses, identity.ScopeWriteStatuses, identity.ScopeReadDirect,
		identity.ScopeWriteFavourites, identity.ScopeAdminRead, identity.ScopeFollow,
	})
	var got [][]string
	for _, g := range groups {
		row := []string{g.Risk}
		for _, s := range g.Scopes {
			row = append(row, s.Text)
		}
		got = append(got, row)
	}
	want := [][]string{
		{riskCaution, scopeDescription(l, identity.ScopeWriteStatuses), scopeDescription(l, identity.ScopeReadDirect), scopeDescription(l, identity.ScopeAdminRead)},
		{riskWrite, scopeDescription(l, identity.ScopeWriteFavourites), scopeDescription(l, identity.ScopeFollow)},
		{riskRead, scopeDescription(l, identity.ScopeReadStatuses)},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("groups = %v, want %v", got, want)
	}
	if groups[0].Title != "Permissions that need your attention" || len(groups[0].Scopes[0].Cautions) != 1 {
		t.Errorf("first group = %+v, want the cautions under their heading", groups[0])
	}
	if only := groupScopes(l, []string{identity.ScopeReadStatuses}); len(only) != 1 || only[0].Risk != riskRead {
		t.Errorf("read only = %+v, want one group", only)
	}
}
