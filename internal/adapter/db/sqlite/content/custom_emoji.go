// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"database/sql"
	"errors"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// CustomEmojiRepository implements content.CustomEmojiRepository on SQLite.
type CustomEmojiRepository struct {
	db *moduleDB
}

var _ appcontent.CustomEmojiRepository = (*CustomEmojiRepository)(nil)

var _ appcontent.EmojiPurgeRepository = (*CustomEmojiRepository)(nil)

// NewCustomEmojiRepository returns the content module's repository of the custom emoji that the
// administrators manage on d.
func NewCustomEmojiRepository(d *sqlite.DB) *CustomEmojiRepository {
	return &CustomEmojiRepository{db: &moduleDB{d}}
}

// InsertCustomEmoji implements content.CustomEmojiRepository.
func (r *CustomEmojiRepository) InsertCustomEmoji(ctx context.Context, tx uow.Tx, e content.CustomEmoji) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if e.Domain == "" {
		// A pending request to create a local emoji reserves its shortcode (ADR-0070 §2).
		reserved, err := q.CustomEmojiShortcodeReserved(ctx, e.Shortcode)
		if err != nil {
			return sqlite.Wrap(err, "custom emoji shortcode reserved")
		}
		if reserved != 0 {
			return appcontent.ErrShortcodeTaken
		}
	}
	err = q.InsertCustomEmoji(ctx, contentgen.InsertCustomEmojiParams{
		ID: e.ID, Shortcode: e.Shortcode, Domain: e.Domain, Uri: sqlite.ToNullString(e.URI), CreatedByUserID: sqlite.ToNullString(e.CreatedByUserID),
		CreatedAt: sqlite.ToMillis(e.CreatedAt),
	})
	if sqlite.IsUniqueViolation(err, "custom_emojis.shortcode, custom_emojis.domain") {
		return appcontent.ErrShortcodeTaken
	}
	return sqlite.Wrap(err, "insert custom emoji")
}

// InsertCustomEmojiImage implements content.CustomEmojiRepository.
func (r *CustomEmojiRepository) InsertCustomEmojiImage(ctx context.Context, tx uow.Tx, img content.CustomEmojiImage) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertCustomEmojiImage(ctx, contentgen.InsertCustomEmojiImageParams{
		ID: img.ID, EmojiID: img.EmojiID, MediaID: img.MediaID, RevisedAt: sqlite.ToMillis(img.RevisedAt),
	})
	return sqlite.Wrap(err, "insert custom emoji image")
}

// SetCustomEmojiSettings implements content.CustomEmojiRepository.
func (r *CustomEmojiRepository) SetCustomEmojiSettings(ctx context.Context, tx uow.Tx, s content.CustomEmojiSettings) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if s.IsDefault() {
		return sqlite.Wrap(q.DeleteCustomEmojiSettings(ctx, s.EmojiID), "delete custom emoji settings")
	}
	err = q.UpsertCustomEmojiSettings(ctx, contentgen.UpsertCustomEmojiSettingsParams{
		EmojiID: s.EmojiID, Enabled: sqlite.BoolToInt(s.Enabled), VisibleInPicker: sqlite.BoolToInt(s.VisibleInPicker), Category: sqlite.ToNullString(s.Category),
	})
	return sqlite.Wrap(err, "upsert custom emoji settings")
}

// SetCustomEmojiAliases implements content.CustomEmojiRepository.
func (r *CustomEmojiRepository) SetCustomEmojiAliases(ctx context.Context, tx uow.Tx, emojiID string, aliases []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteCustomEmojiAliases(ctx, emojiID); err != nil {
		return sqlite.Wrap(err, "delete custom emoji aliases")
	}
	for _, a := range aliases {
		if err := q.InsertCustomEmojiAlias(ctx, contentgen.InsertCustomEmojiAliasParams{EmojiID: emojiID, Alias: a}); err != nil {
			return sqlite.Wrap(err, "insert custom emoji alias")
		}
	}
	return nil
}

// DeleteCustomEmoji implements content.CustomEmojiRepository.
func (r *CustomEmojiRepository) DeleteCustomEmoji(ctx context.Context, tx uow.Tx, emojiID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, del := range []func(context.Context, string) error{
		q.DeleteCustomEmojiAliases, q.DeleteCustomEmojiSettings, q.DeleteCustomEmojiImages, q.DeleteCustomEmojiRow,
	} {
		if err := del(ctx, emojiID); err != nil {
			return sqlite.Wrap(err, "delete custom emoji")
		}
	}
	return nil
}

// FindCustomEmojiRecord implements content.CustomEmojiRepository.
func (r *CustomEmojiRepository) FindCustomEmojiRecord(ctx context.Context, id string) (content.CustomEmojiRecord, error) {
	q := r.db.reader()
	row, err := q.GetCustomEmoji(ctx, id)
	if err != nil {
		return content.CustomEmojiRecord{}, sqlite.NotFound(err, appcontent.ErrNotFound, "custom emoji "+id)
	}
	img, err := q.GetLatestCustomEmojiImage(ctx, id)
	if err != nil {
		return content.CustomEmojiRecord{}, sqlite.NotFound(err, appcontent.ErrNotFound, "custom emoji image "+id)
	}
	out := content.CustomEmojiRecord{CurrentCustomEmoji: content.CurrentCustomEmoji{
		Emoji: customEmojiFrom(row), Image: customEmojiImageFrom(img), Settings: content.DefaultCustomEmojiSettings(id),
	}}
	switch s, err := q.GetCustomEmojiSettings(ctx, id); {
	case err == nil:
		out.Settings = content.CustomEmojiSettings{EmojiID: id, Enabled: s.Enabled != 0, VisibleInPicker: s.VisibleInPicker != 0, Category: s.Category.String}
	case !errors.Is(err, sql.ErrNoRows):
		return content.CustomEmojiRecord{}, sqlite.Wrap(err, "get custom emoji settings")
	}
	aliases, err := r.aliases(ctx, []string{id})
	if err != nil {
		return content.CustomEmojiRecord{}, err
	}
	out.Aliases = aliases[id]
	return out, nil
}

// aliases returns the aliases of the emoji with an ID in ids, keyed by ID.
func (r *CustomEmojiRepository) aliases(ctx context.Context, ids []string) (map[string][]string, error) {
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListCustomEmojiAliases(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list custom emoji aliases")
	}
	out := map[string][]string{}
	for _, row := range rows {
		out[row.EmojiID] = append(out[row.EmojiID], row.Alias)
	}
	return out, nil
}

// emojiRecordRow is a row of ListAdminCustomEmojis or ListLocalCustomEmojis.
type emojiRecordRow struct {
	ID, Shortcode, Domain    string
	Uri                      sql.NullString //nolint:revive // the name of the generated rows, which convert to this type
	CreatedByUserID          sql.NullString
	CreatedAt                int64
	ImageID, ImageMediaID    string
	ImageRevisedAt           int64
	Enabled, VisibleInPicker int64
	Category                 string
}

func (r *CustomEmojiRepository) records(ctx context.Context, rows []emojiRecordRow) ([]content.CustomEmojiRecord, error) {
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	aliases, err := r.aliases(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]content.CustomEmojiRecord, len(rows))
	for i, row := range rows {
		out[i] = content.CustomEmojiRecord{
			CurrentCustomEmoji: content.CurrentCustomEmoji{
				Emoji: content.CustomEmoji{
					ID: row.ID, Shortcode: row.Shortcode, Domain: row.Domain, URI: row.Uri.String, CreatedByUserID: row.CreatedByUserID.String,
					CreatedAt: sqlite.FromMillis(row.CreatedAt),
				},
				Image: content.CustomEmojiImage{ID: row.ImageID, EmojiID: row.ID, MediaID: row.ImageMediaID, RevisedAt: sqlite.FromMillis(row.ImageRevisedAt)},
				Settings: content.CustomEmojiSettings{
					EmojiID: row.ID, Enabled: row.Enabled != 0, VisibleInPicker: row.VisibleInPicker != 0, Category: row.Category,
				},
			},
			Aliases: aliases[row.ID],
		}
	}
	return out, nil
}

// ListCustomEmojiRecords implements content.CustomEmojiRepository.
func (r *CustomEmojiRepository) ListCustomEmojiRecords(ctx context.Context, q appcontent.CustomEmojiQuery, limit int) ([]content.CustomEmojiRecord, error) {
	rows, err := r.db.reader().ListAdminCustomEmojis(ctx, contentgen.ListAdminCustomEmojisParams{
		Locality: int64(q.Locality), Domain: q.Domain, Shortcode: q.Shortcode, CreatedBy: q.CreatedBy, BeforeID: q.Before,
		RowLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list custom emoji")
	}
	out := make([]emojiRecordRow, len(rows))
	for i, row := range rows {
		out[i] = emojiRecordRow(row)
	}
	return r.records(ctx, out)
}

// ListLocalCustomEmojis implements content.CustomEmojiRepository.
func (r *CustomEmojiRepository) ListLocalCustomEmojis(ctx context.Context, shortcodes []string) ([]content.CustomEmojiRecord, error) {
	params := contentgen.ListLocalCustomEmojisParams{AllShortcodes: sqlite.BoolToInt(shortcodes == nil), Shortcodes: "[]"}
	if shortcodes != nil {
		list, err := sqlite.JSONList(shortcodes)
		if err != nil {
			return nil, err
		}
		params.Shortcodes = list
	}
	rows, err := r.db.reader().ListLocalCustomEmojis(ctx, params)
	if err != nil {
		return nil, sqlite.Wrap(err, "list local custom emoji")
	}
	out := make([]emojiRecordRow, len(rows))
	for i, row := range rows {
		out[i] = emojiRecordRow(row)
	}
	return r.records(ctx, out)
}

// ListUnreferencedRemoteEmojis implements content.EmojiPurgeRepository.
func (r *CustomEmojiRepository) ListUnreferencedRemoteEmojis(ctx context.Context, after string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListUnreferencedRemoteEmojis(ctx, contentgen.ListUnreferencedRemoteEmojisParams{After: after, RowLimit: int64(limit)})
	return ids, sqlite.Wrap(err, "list unreferenced remote emoji")
}

// CustomEmojiImageMediaIDs implements content.EmojiPurgeRepository.
func (r *CustomEmojiRepository) CustomEmojiImageMediaIDs(ctx context.Context, emojiID string) ([]string, error) {
	ids, err := r.db.reader().ListCustomEmojiImageMediaIDs(ctx, emojiID)
	return ids, sqlite.Wrap(err, "list custom emoji image media")
}

// PurgeRemoteEmoji implements content.EmojiPurgeRepository.
func (r *CustomEmojiRepository) PurgeRemoteEmoji(ctx context.Context, tx uow.Tx, emojiID string, mediaIDs []string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteUnreferencedRemoteEmoji(ctx, emojiID)
	if err != nil || n == 0 {
		return false, sqlite.Wrap(err, "delete remote emoji")
	}
	for _, del := range []func(context.Context, string) error{
		q.DeleteCustomEmojiAliases, q.DeleteCustomEmojiSettings, q.DeleteCustomEmojiImages,
	} {
		if err := del(ctx, emojiID); err != nil {
			return false, sqlite.Wrap(err, "delete remote emoji")
		}
	}
	for _, id := range mediaIDs {
		n, err := q.DeleteUnreferencedRemoteMedia(ctx, id)
		if err != nil {
			return false, sqlite.Wrap(err, "delete remote emoji image")
		}
		if n > 0 {
			if err := q.DeleteRemoteMediaFile(ctx, id); err != nil {
				return false, sqlite.Wrap(err, "delete remote emoji image")
			}
		}
	}
	return true, nil
}

// ExistingCustomEmojiIDs implements content.CustomEmojiRepository.
func (r *CustomEmojiRepository) ExistingCustomEmojiIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	out, err := r.db.reader().ListExistingCustomEmojiIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list existing custom emoji")
	}
	return out, nil
}
