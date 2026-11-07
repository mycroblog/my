// SPDX-License-Identifier: AGPL-3.0-only

// Package voice_test runs the voice room flows (testkit.VoiceRoomsFlows) once, on SQLite
// (ADR-0153 §1).
package voice_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestVoiceRoomsFlows(t *testing.T) {
	t.Parallel()
	testkit.VoiceRoomsFlows(t, func(t *testing.T, opts uow.Options) testkit.VoiceHarness {
		db := flowtest.SQLite(t, opts)
		return testkit.VoiceHarness{UnitOfWork: db, Voice: db.VoiceRepository()}
	})
}
