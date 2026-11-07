// SPDX-License-Identifier: AGPL-3.0-only

package fake_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/voice/fake"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/pkg/testkit/voicetest"
)

func TestVoiceTransportContract(t *testing.T) {
	voicetest.VoiceTransportContract(t, func(*testing.T) voicetest.VoiceTransportHarness {
		clock := voicetest.VoiceTransportClock()
		tr := fake.New(clock)
		return voicetest.VoiceTransportHarness{
			Transport: tr, Clock: clock,
			Connect: func(_ *testing.T, room appvoice.RoomRef, s appvoice.Session) { tr.Connect(room, s) },
		}
	})
}
