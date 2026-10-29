// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"errors"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// ErrNotFound reports that no row matches a lookup.
var ErrNotFound = errors.New("instance: not found")

// Repository is the persistence port of the instance module's settings, texts, rules, theme, brand images,
// announcements, service closure, and secrets (data-model §4.9). The database adapters implement it and
// pass testkit.InstanceRepositoryContract.
//
// Methods that take a uow.Tx run inside that transaction; the others read committed data
// outside any transaction. Lookups that find nothing return an error wrapping ErrNotFound.
type Repository interface {
	InsertSettingRevision(ctx context.Context, tx uow.Tx, r instance.SettingRevision) error
	// CurrentSetting returns the instance_settings row of key.
	CurrentSetting(ctx context.Context, tx uow.Tx, key instance.SettingKey) (instance.CurrentSetting, error)
	UpsertCurrentSetting(ctx context.Context, tx uow.Tx, c instance.CurrentSetting) error
	DeleteAllCurrentSettings(ctx context.Context, tx uow.Tx) error
	// ListSettingRevisions returns every setting revision, oldest first, for rebuilding.
	ListSettingRevisions(ctx context.Context, tx uow.Tx) ([]instance.SettingRevision, error)

	InsertTextRevision(ctx context.Context, tx uow.Tx, r instance.TextRevision) error
	// CurrentText returns the instance_texts row of key.
	CurrentText(ctx context.Context, tx uow.Tx, key instance.TextKey) (instance.CurrentText, error)
	UpsertCurrentText(ctx context.Context, tx uow.Tx, c instance.CurrentText) error
	DeleteAllCurrentTexts(ctx context.Context, tx uow.Tx) error
	// ListTextRevisionHeads returns the key and ID of every text revision, oldest first, for
	// rebuilding.
	ListTextRevisionHeads(ctx context.Context, tx uow.Tx) ([]instance.CurrentText, error)

	// ListCurrentSettings returns the current value of every key that has been set, by key.
	ListCurrentSettings(ctx context.Context) ([]instance.CurrentSetting, error)
	// ListSettingHistory returns the revisions of key, newest first, at most limit of them.
	ListSettingHistory(ctx context.Context, key instance.SettingKey, limit int) ([]instance.SettingRevision, error)
	// IsSettingValueUsed reports whether any revision of key, current or not, has value.
	IsSettingValueUsed(ctx context.Context, key instance.SettingKey, value string) (bool, error)
	// FindCurrentText returns the current text of key, which may be cleared (empty).
	FindCurrentText(ctx context.Context, key instance.TextKey) (instance.Text, error)
	// ListAllCurrentTexts returns the current text of every key that has one, including cleared
	// (empty) texts, by key.
	ListAllCurrentTexts(ctx context.Context) ([]instance.Text, error)
	// ListTextHistory returns the revisions of key, newest first, at most limit of them.
	ListTextHistory(ctx context.Context, key instance.TextKey, limit int) ([]instance.TextRevision, error)

	RuleRepository
	ThemeRepository
	BrandRepository
	AnnouncementRepository
	ClosureRepository
	SecretRepository
}
