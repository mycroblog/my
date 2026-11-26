// SPDX-License-Identifier: AGPL-3.0-only

package sfu_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/voice/sfu"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/voicetest"
)

// TestVoiceTransportContract runs the transport's contract on a registry with one node's lease
// and no node behind it: nothing connects, as with a media server that only a WebRTC client
// reaches.
func TestVoiceTransportContract(t *testing.T) {
	voicetest.VoiceTransportContract(t, func(t *testing.T) voicetest.VoiceTransportHarness {
		clock := voicetest.VoiceTransportClock()
		reg := sfu.NewRegistry(testkit.NewKeyValueStore(clock), clock)
		if err := reg.Register(t.Context(), sfu.NodeLease{ID: "node-1", URL: "wss://sfu.example.org/voice/sfu", Capacity: 100}); err != nil {
			t.Fatal(err)
		}
		return voicetest.VoiceTransportHarness{Transport: sfu.New(sfu.Options{Registry: reg, MaxParticipants: 10, Clock: clock}), Clock: clock}
	})
}
