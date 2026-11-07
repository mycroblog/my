// SPDX-License-Identifier: AGPL-3.0-only

// Package content_test runs the content flows of pkg/testkit once, on SQLite (ADR-0153 §1).
package content_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/contenttest"
)

func TestInteractionFlows(t *testing.T) {
	t.Parallel()
	contenttest.InteractionFlows(t, func(t *testing.T, opts uow.Options) contenttest.InteractionHarness {
		db := flowtest.SQLite(t, opts)
		return contenttest.InteractionHarness{UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository()}
	})
}

func TestAutoDeleteFlows(t *testing.T) {
	t.Parallel()
	contenttest.AutoDeleteFlows(t, func(t *testing.T, opts uow.Options) contenttest.AutoDeleteHarness {
		db := flowtest.SQLite(t, opts)
		return contenttest.AutoDeleteHarness{
			InteractionHarness: contenttest.InteractionHarness{UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository()},
			AutoDeletes:        db.StatusRepository(),
		}
	})
}

func TestMediaFlows(t *testing.T) {
	t.Parallel()
	contenttest.MediaFlows(t, func(t *testing.T, opts uow.Options) contenttest.MediaHarness {
		db := flowtest.SQLite(t, opts)
		return contenttest.MediaHarness{UnitOfWork: db, Media: db.MediaRepository(), Statuses: db.StatusRepository()}
	})
}

func TestCustomEmojiFlows(t *testing.T) {
	t.Parallel()
	contenttest.CustomEmojiFlows(t, func(t *testing.T, opts uow.Options) contenttest.CustomEmojiHarness {
		db := flowtest.SQLite(t, opts)
		return contenttest.CustomEmojiHarness{
			UnitOfWork: db, Media: db.MediaRepository(), Statuses: db.StatusRepository(), Emojis: db.CustomEmojiRepository(),
		}
	})
}
