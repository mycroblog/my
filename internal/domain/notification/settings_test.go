// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"errors"
	"testing"
)

// TestGroupsCoverEveryType checks that every notification type is in exactly one row of the
// settings.
func TestGroupsCoverEveryType(t *testing.T) {
	seen := map[Type]Group{}
	for _, g := range Groups {
		for _, typ := range g.Types {
			if prev, ok := seen[typ]; ok {
				t.Errorf("%s is in %s and %s", typ, prev, g.Group)
			}
			seen[typ] = g.Group
		}
	}
	for _, typ := range Types {
		if _, ok := seen[typ]; !ok {
			t.Errorf("%s is in no group", typ)
		}
		if _, ok := GroupOf(typ); !ok {
			t.Errorf("GroupOf(%s) found nothing", typ)
		}
	}
}

// TestGroupDefaults checks the defaults and the fixed channels of each section (ADR-0109 §2).
func TestGroupDefaults(t *testing.T) {
	none := NewChoices(nil)
	for _, g := range Groups {
		inApp, email := none.Enabled(g.Group, ChannelInApp), none.Enabled(g.Group, ChannelEmail)
		switch {
		case g.Email.Absent:
			if g.Section != SectionOrdinary || !inApp || email || g.InApp.Fixed || g.Unsubscribable() || g.DelayedEmail() {
				t.Errorf("%s: in app %v, email %v, %+v; want on, and no email", g.Group, inApp, email, g)
			}
		case g.Section == SectionOrdinary:
			if !inApp || email || g.InApp.Fixed || g.Email.Fixed || !g.DelayedEmail() {
				t.Errorf("%s: in app %v, email %v, %+v; want on, off, both chosen, emailed after the delay", g.Group, inApp, email, g)
			}
		case g.Section == SectionModerator:
			if _, ok := g.Permission(); !ok || !inApp || !email || g.InApp.Fixed || g.Email.Fixed || g.DelayedEmail() {
				t.Errorf("%s: in app %v, email %v, %+v; want on, on, both chosen, emailed at once", g.Group, inApp, email, g)
			}
		case g.Group == GroupModeration:
			if !inApp || email || !g.InApp.Fixed || g.Email.Fixed || !g.DelayedEmail() {
				t.Errorf("%s: in app %v, email %v, %+v; want fixed on, chosen off", g.Group, inApp, email, g)
			}
		default:
			if !inApp || !email || !g.InApp.Fixed || !g.Email.Fixed || g.Unsubscribable() {
				t.Errorf("%s: in app %v, email %v, %+v; want both fixed on", g.Group, inApp, email, g)
			}
		}
	}
}

// TestChoices checks that a choice overrides the default, never a fixed channel.
func TestChoices(t *testing.T) {
	c := NewChoices([]Setting{
		{Group: GroupMention, Channel: ChannelEmail, Enabled: true},
		{Group: GroupAdminReport, Channel: ChannelInApp, Enabled: false},
		{Group: GroupServiceClosure, Channel: ChannelEmail, Enabled: false},
	})
	if !c.TypeEnabled(TypeMention, ChannelEmail) || c.TypeEnabled(TypeAdminReport, ChannelInApp) || !c.TypeEnabled(TypeServiceClosure, ChannelEmail) {
		t.Errorf("choices = %v", c)
	}
	if !c.TypeEnabled(TypeVoiceRoomInvited, ChannelInApp) || c.TypeEnabled(TypeVoiceRoomInvited, ChannelEmail) {
		t.Error("voice_room_invited does not follow the defaults of its group")
	}
}

// TestSettingValidate checks the choices that cannot be made.
func TestSettingValidate(t *testing.T) {
	for _, tc := range []struct {
		s    Setting
		want error
	}{
		{Setting{Group: GroupMention, Channel: ChannelInApp}, nil},
		{Setting{Group: GroupModeration, Channel: ChannelEmail}, nil},
		{Setting{Group: GroupModeration, Channel: ChannelInApp}, ErrFixedSetting},
		{Setting{Group: GroupSupporter, Channel: ChannelEmail}, ErrFixedSetting},
		{Setting{Group: "unknown", Channel: ChannelEmail}, ErrInvalidSetting},
		{Setting{Group: GroupMention, Channel: "sound"}, ErrInvalidSetting},
		{Setting{Group: GroupStatus, Channel: ChannelInApp}, nil},
		{Setting{Group: GroupStatus, Channel: ChannelEmail}, ErrInvalidSetting},
	} {
		if err := tc.s.Validate(); !errors.Is(err, tc.want) {
			t.Errorf("Validate(%+v) = %v, want %v", tc.s, err, tc.want)
		}
	}
}
