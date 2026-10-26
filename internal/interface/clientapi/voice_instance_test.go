// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestInstanceVoice checks that GET /instance says whether the voice chat rooms are on and who
// may open them, and says permitted while they are off whatever the setting (ADR-0100 §2).
func TestInstanceVoice(t *testing.T) {
	everyone := &fakeInstance{settings: []instance.CurrentSetting{
		{Key: instance.SettingVoiceRoomStarters, RevisionID: testkit.NamedID("revision-1"), Value: string(instance.VoiceRoomStartersEveryone)},
	}}
	for _, tc := range []struct {
		name     string
		voice    clientapi.VoiceRooms
		inst     *fakeInstance
		enabled  bool
		starters clientapi.InstanceVoiceRoomStarters
	}{
		{"off", nil, everyone, false, clientapi.InstanceVoiceRoomStartersPermitted},
		{"on, the default", &fakeVoiceRooms{}, &fakeInstance{}, true, clientapi.InstanceVoiceRoomStartersPermitted},
		{"on, everyone", &fakeVoiceRooms{}, everyone, true, clientapi.InstanceVoiceRoomStartersEveryone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newAuthEnv(t, func(o *clientapi.Options) {
				o.Instance = tc.inst
				if tc.voice != nil {
					o.Voice = tc.voice
				}
			})
			res := e.send(t, http.MethodGet, "/instance", "", "")
			if res.StatusCode != http.StatusOK {
				t.Fatalf("GET /instance = %d", res.StatusCode)
			}
			got := decode[clientapi.Instance](t, res)
			if got.VoiceEnabled != tc.enabled || got.VoiceRoomStarters != tc.starters {
				t.Errorf("voice_enabled %v, voice_room_starters %q; want %v, %q", got.VoiceEnabled, got.VoiceRoomStarters, tc.enabled, tc.starters)
			}
		})
	}
}
