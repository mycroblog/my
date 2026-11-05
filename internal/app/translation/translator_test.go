// SPDX-License-Identifier: AGPL-3.0-only

package translation

import "testing"

func TestMarkerForm(t *testing.T) {
	t.Parallel()
	if got := Marker(12); got != "⟦12⟧" {
		t.Errorf("Marker(12) = %q", got)
	}
	if got := ClosingMarker(12); got != "⟦/12⟧" {
		t.Errorf("ClosingMarker(12) = %q", got)
	}
}

func TestMarkersIntact(t *testing.T) {
	t.Parallel()
	const fragment = "Hello ⟦1⟧, ⟦2⟧read⟦/2⟧ ⟦3⟧"
	for _, tt := range []struct {
		name        string
		fragment    string
		translation string
		want        bool
	}{
		{"same place", fragment, "こんにちは ⟦1⟧、⟦2⟧読む⟦/2⟧ ⟦3⟧", true},
		{"another order", fragment, "⟦3⟧ ⟦2⟧読む⟦/2⟧ ⟦1⟧ こんにちは", true},
		{"no markers", "Hello", "こんにちは", true},
		{"nothing but a marker", "⟦1⟧", "⟦1⟧", true},
		{"look-alikes are text", "⟦a⟧ ⟦0⟧ ⟦ 1 ⟧ [1]", "[1] ⟦ 1 ⟧", true},
		{"lost", fragment, "こんにちは ⟦1⟧、⟦2⟧読む ⟦3⟧", false},
		{"twice", fragment, "⟦1⟧ ⟦1⟧ ⟦2⟧読む⟦/2⟧ ⟦3⟧", false},
		{"invented", fragment, "⟦1⟧ ⟦2⟧読む⟦/2⟧ ⟦3⟧ ⟦4⟧", false},
		{"invented without any in the fragment", "Hello", "⟦1⟧", false},
		{"opening turned into closing", "⟦1⟧", "⟦/1⟧", false},
		{"twice in the fragment", "⟦1⟧ ⟦1⟧", "⟦1⟧ ⟦1⟧", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := MarkersIntact(tt.fragment, tt.translation); got != tt.want {
				t.Errorf("MarkersIntact(%q, %q) = %t, want %t", tt.fragment, tt.translation, got, tt.want)
			}
		})
	}
}
