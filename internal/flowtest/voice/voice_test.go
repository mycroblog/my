// SPDX-License-Identifier: AGPL-3.0-only

// Package voice_test runs the voice room flows (voicetest.VoiceRoomsFlows) once, on SQLite
// (ADR-0153 §1).
package voice_test

import (
	"testing"

	sqlitevoice "github.com/mycroblog/my/internal/adapter/db/sqlite/voice"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/voicetest"
)

func TestVoiceRoomsFlows(t *testing.T) {
	t.Parallel()
	voicetest.VoiceRoomsFlows(t, func(t *testing.T, opts uow.Options) voicetest.VoiceHarness {
		db := flowtest.SQLite(t, opts)
		return voicetest.VoiceHarness{UnitOfWork: db, Voice: sqlitevoice.NewRepository(db)}
	})
}
