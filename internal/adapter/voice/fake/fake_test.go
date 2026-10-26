// SPDX-License-Identifier: AGPL-3.0-only

package fake_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/voice/fake"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestVoiceTransportContract(t *testing.T) {
	testkit.VoiceTransportContract(t, func(*testing.T) testkit.VoiceTransportHarness {
		clock := testkit.VoiceTransportClock()
		tr := fake.New(clock)
		return testkit.VoiceTransportHarness{
			Transport: tr, Clock: clock,
			Connect: func(_ *testing.T, room appvoice.RoomRef, s appvoice.Session) { tr.Connect(room, s) },
		}
	})
}
