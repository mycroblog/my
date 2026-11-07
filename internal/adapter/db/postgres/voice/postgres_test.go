// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/voicetest"
)

func TestVoiceRepositoryContract(t *testing.T) {
	voicetest.VoiceRepositoryContract(t, func(t *testing.T, opts uow.Options) voicetest.VoiceHarness {
		db := newTestDB(t, opts)
		return voicetest.VoiceHarness{UnitOfWork: db, Voice: NewRepository(db)}
	})
}

func TestVoiceRoomsContract(t *testing.T) {
	voicetest.VoiceRoomsContract(t, func(t *testing.T, opts uow.Options) voicetest.VoiceHarness {
		db := newTestDB(t, opts)
		return voicetest.VoiceHarness{UnitOfWork: db, Voice: NewRepository(db)}
	})
}
