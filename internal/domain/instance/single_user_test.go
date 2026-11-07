// SPDX-License-Identifier: AGPL-3.0-only

package instance_test

import (
	"testing"

	"github.com/mycroblog/my/internal/domain/instance"
)

func TestSingleUserHoldsTheEffectiveValues(t *testing.T) {
	stored := []instance.CurrentSetting{
		{Key: instance.SettingRegistrationMode, RevisionID: testID(1), Value: string(instance.RegistrationOpen)},
		{Key: instance.SettingLocalTimelineVisibility, RevisionID: testID(2), Value: string(instance.AudienceEveryone)},
		{Key: instance.SettingFederatedTimelineVisibility, RevisionID: testID(3), Value: string(instance.AudienceSignedIn)},
	}
	off := instance.NewSettings(stored)
	if off.SingleUser() {
		t.Fatal("SingleUser() by default = true, want false")
	}
	if got := off.EffectiveRegistrationMode(instance.ClosureNone); got != instance.RegistrationOpen {
		t.Errorf("EffectiveRegistrationMode() = %s, want open", got)
	}
	if got := off.EffectiveRegistrationMode(instance.ClosureAnnounced); got != instance.RegistrationClosed {
		t.Errorf("EffectiveRegistrationMode(announced) = %s, want closed", got)
	}
	if off.LocalTimelineVisibility() != instance.AudienceEveryone || off.FederatedTimelineVisibility() != instance.AudienceSignedIn {
		t.Errorf("timelines = %s, %s, want the stored values", off.LocalTimelineVisibility(), off.FederatedTimelineVisibility())
	}

	on := instance.NewSettings(append(stored, instance.CurrentSetting{Key: instance.SettingSingleUser, RevisionID: testID(4), Value: "true"}))
	if !on.SingleUser() {
		t.Fatal("SingleUser() = false, want true")
	}
	if got := on.EffectiveRegistrationMode(instance.ClosureNone); got != instance.RegistrationClosed {
		t.Errorf("EffectiveRegistrationMode() on a one-person server = %s, want closed", got)
	}
	if on.LocalTimelineVisibility() != instance.AudienceOff || on.FederatedTimelineVisibility() != instance.AudienceOff {
		t.Errorf("timelines on a one-person server = %s, %s, want off", on.LocalTimelineVisibility(), on.FederatedTimelineVisibility())
	}
	// The stored values stay, and apply again when the mode is turned off.
	if v, _ := on.Value(instance.SettingRegistrationMode); v != string(instance.RegistrationOpen) {
		t.Errorf("Value(registration_mode) = %q, want the stored open", v)
	}
	if v, _ := on.Value(instance.SettingLocalTimelineVisibility); v != string(instance.AudienceEveryone) {
		t.Errorf("Value(local_timeline_visibility) = %q, want the stored everyone", v)
	}
}
