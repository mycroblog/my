// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/content"
)

// everything is a saved policy that uses every exception.
func everything() content.AutoDeletePolicy {
	return content.AutoDeletePolicy{
		AccountID: "a", Enabled: true, MinAgeDays: 400,
		KeepPinned: true, KeepBookmarked: true, KeepFavourited: true, KeepMedia: true, KeepPolls: true,
		KeepVisibilities: []content.Visibility{content.VisibilityDirect}, KeepMinBoosts: 5, KeepMinFavourites: 7,
	}
}

func TestEffectiveAutoDeletePolicy(t *testing.T) {
	defaultLimit := content.AutoDeleteLimit{MaxAgeDays: 90, AllowPinned: true, KeptPins: content.MaxPins, GraceDays: 30}
	disabled := content.DefaultAutoDeletePolicy("a")
	short := everything()
	short.MinAgeDays = 10
	for _, tc := range []struct {
		name   string
		saved  content.AutoDeletePolicy
		limit  content.AutoDeleteLimit
		exempt bool
		want   func(p *content.AutoDeletePolicy)
		bound  bool
	}{
		{name: "no maximum", saved: disabled, limit: content.AutoDeleteLimit{}, want: func(*content.AutoDeletePolicy) {}},
		{name: "exempt", saved: disabled, limit: defaultLimit, exempt: true, want: func(*content.AutoDeletePolicy) {}},
		{
			name: "disabled policy is enabled at the maximum", saved: disabled, limit: defaultLimit, bound: true,
			want: func(p *content.AutoDeletePolicy) {
				p.Enabled, p.MinAgeDays, p.KeepBookmarked, p.KeepFavourited, p.KeepVisibilities = true, 90, false, false, nil
			},
		},
		{
			name: "longer policy is cut to the maximum, only pins kept", saved: everything(), limit: defaultLimit, bound: true,
			want: func(p *content.AutoDeletePolicy) {
				p.MinAgeDays = 90
				p.KeepBookmarked, p.KeepFavourited, p.KeepMedia, p.KeepPolls = false, false, false, false
				p.KeepVisibilities, p.KeepMinBoosts, p.KeepMinFavourites = nil, 0, 0
			},
		},
		{
			name: "shorter policy keeps its days", saved: short, bound: true,
			limit: content.AutoDeleteLimit{MaxAgeDays: 90, AllowBookmarked: true, AllowFavourited: true, AllowMedia: true, AllowPolls: true, AllowVisibilities: true, AllowThresholds: true, KeptPins: 2},
			want:  func(p *content.AutoDeletePolicy) { p.KeepPinned = false },
		},
		{
			name: "fewer kept pins cap the pins", saved: short, bound: false,
			limit: content.AutoDeleteLimit{MaxAgeDays: 90, AllowPinned: true, KeptPins: 2, AllowBookmarked: true, AllowFavourited: true, AllowMedia: true, AllowPolls: true, AllowVisibilities: true, AllowThresholds: true},
			want:  func(p *content.AutoDeletePolicy) { p.MaxKeptPins = 2 },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.saved
			tc.want(&want)
			got := content.EffectiveAutoDeletePolicy(tc.saved, tc.limit, tc.exempt)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("effective = %+v\nwant        %+v", got, want)
			}
			if b := content.AutoDeleteBound(tc.saved, got, 0); b != tc.bound {
				t.Errorf("bound = %v, want %v", b, tc.bound)
			}
		})
	}
	// The cap on the pins binds an account only when it has more pins than the cap.
	limit := content.AutoDeleteLimit{MaxAgeDays: 90, AllowPinned: true, KeptPins: 2}
	p := content.AutoDeletePolicy{AccountID: "a", Enabled: true, MinAgeDays: 30, KeepPinned: true}
	eff := content.EffectiveAutoDeletePolicy(p, limit, false)
	if content.AutoDeleteBound(p, eff, 2) || !content.AutoDeleteBound(p, eff, 3) {
		t.Errorf("bound with 2 and 3 pins under a cap of 2 = %v, %v; want false, true", content.AutoDeleteBound(p, eff, 2), content.AutoDeleteBound(p, eff, 3))
	}
}

func TestAutoDeleteLimitCheck(t *testing.T) {
	limit := content.AutoDeleteLimit{MaxAgeDays: 90, AllowPinned: true, KeptPins: content.MaxPins}
	ok := content.AutoDeletePolicy{Enabled: true, MinAgeDays: 90, KeepPinned: true}
	if err := limit.Check(ok); err != nil {
		t.Errorf("Check(%+v) = %v, want nil", ok, err)
	}
	if err := (content.AutoDeleteLimit{}).Check(content.DefaultAutoDeletePolicy("a")); err != nil {
		t.Errorf("Check without a maximum = %v", err)
	}
	for property, edit := range map[string]func(*content.AutoDeletePolicy){
		"enabled":             func(p *content.AutoDeletePolicy) { p.Enabled = false },
		"min_age_days":        func(p *content.AutoDeletePolicy) { p.MinAgeDays = 91 },
		"keep_bookmarked":     func(p *content.AutoDeletePolicy) { p.KeepBookmarked = true },
		"keep_favourited":     func(p *content.AutoDeletePolicy) { p.KeepFavourited = true },
		"keep_media":          func(p *content.AutoDeletePolicy) { p.KeepMedia = true },
		"keep_polls":          func(p *content.AutoDeletePolicy) { p.KeepPolls = true },
		"keep_visibilities":   func(p *content.AutoDeletePolicy) { p.KeepVisibilities = []content.Visibility{content.VisibilityDirect} },
		"keep_min_boosts":     func(p *content.AutoDeletePolicy) { p.KeepMinBoosts = 1 },
		"keep_min_favourites": func(p *content.AutoDeletePolicy) { p.KeepMinFavourites = 1 },
	} {
		p := ok
		edit(&p)
		var invalid *content.AutoDeletePolicyError
		err := limit.Check(p)
		if !errors.As(err, &invalid) || invalid.Property != property || !errors.Is(err, content.ErrAutoDeleteOverLimit) {
			t.Errorf("Check with %s = %v, want an over-limit error naming it", property, err)
		}
	}
	noPins := limit
	noPins.AllowPinned = false
	if err := noPins.Check(ok); err == nil {
		t.Error("Check of kept pins without AllowPinned = nil")
	}
}

func TestAutoDeleteLimitNotice(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	limit := content.AutoDeleteLimit{MaxAgeDays: 90, GraceDays: 30}
	n := content.NewAutoDeleteLimitNotice("n", "a", limit, now)
	if !n.EnforceFrom.Equal(now.AddDate(0, 0, 30)) || n.MaxAgeDays != 90 {
		t.Errorf("notice = %+v", n)
	}
	if n.Enforced(now.AddDate(0, 0, 29)) || !n.Enforced(n.EnforceFrom) {
		t.Error("Enforced before and at EnforceFrom")
	}
	if n.LoweredBy(limit) || n.LoweredBy(content.AutoDeleteLimit{MaxAgeDays: 120}) || !n.LoweredBy(content.AutoDeleteLimit{MaxAgeDays: 60}) {
		t.Error("LoweredBy the same, a higher, and a lower maximum")
	}
	if immediate := content.NewAutoDeleteLimitNotice("n", "a", content.AutoDeleteLimit{MaxAgeDays: 90}, now); !immediate.Enforced(now) {
		t.Error("a notice without grace is enforced at once")
	}
}
