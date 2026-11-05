// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/app/uow"
)

// TranslationRepository implements translation.Repository.
type TranslationRepository struct{ db *DB }

var _ translation.Repository = (*TranslationRepository)(nil)

// TranslationRepository returns the repository of the translation module.
func (d *DB) TranslationRepository() *TranslationRepository { return &TranslationRepository{db: d} }

// InsertTranslation implements translation.Repository.
func (r *TranslationRepository) InsertTranslation(ctx context.Context, tx uow.Tx, t translation.Stored) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertStatusTranslation(ctx, sqlitegen.InsertStatusTranslationParams{
		ID: t.ID, StatusID: t.StatusID, RevisionID: t.RevisionID, TargetLanguage: t.TargetLanguage, Provider: t.Provider,
		SourceLanguage: t.SourceLanguage, ContentHtml: t.ContentHTML, SpoilerText: t.SpoilerText, CreatedAt: toMillis(t.CreatedAt),
	})
	if err != nil {
		return false, wrap(err, "insert status translation")
	}
	if n == 0 {
		return false, nil
	}
	for _, a := range t.Attachments {
		err := q.InsertStatusTranslationAttachment(ctx, sqlitegen.InsertStatusTranslationAttachmentParams{
			TranslationID: t.ID, Position: int64(a.Position), Description: a.Description,
		})
		if err != nil {
			return false, wrap(err, "insert status translation attachment")
		}
	}
	for _, o := range t.PollOptions {
		err := q.InsertStatusTranslationPollOption(ctx, sqlitegen.InsertStatusTranslationPollOptionParams{
			TranslationID: t.ID, Position: int64(o.Position), Title: o.Title,
		})
		if err != nil {
			return false, wrap(err, "insert status translation poll option")
		}
	}
	return true, nil
}

// FindTranslations implements translation.Repository.
func (r *TranslationRepository) FindTranslations(ctx context.Context, revisionID, targetLanguage string) ([]translation.Stored, error) {
	q := r.db.reader()
	rows, err := q.ListStatusTranslations(ctx, sqlitegen.ListStatusTranslationsParams{RevisionID: revisionID, TargetLanguage: targetLanguage})
	if err != nil {
		return nil, wrap(err, "list status translations")
	}
	out := make([]translation.Stored, 0, len(rows))
	for _, row := range rows {
		t := translation.Stored{
			ID: row.ID, StatusID: row.StatusID, RevisionID: row.RevisionID, TargetLanguage: row.TargetLanguage, Provider: row.Provider,
			SourceLanguage: row.SourceLanguage, ContentHTML: row.ContentHtml, SpoilerText: row.SpoilerText, CreatedAt: fromMillis(row.CreatedAt),
		}
		attachments, err := q.ListStatusTranslationAttachments(ctx, row.ID)
		if err != nil {
			return nil, wrap(err, "list status translation attachments")
		}
		for _, a := range attachments {
			t.Attachments = append(t.Attachments, translation.StoredAttachment{Position: int(a.Position), Description: a.Description})
		}
		options, err := q.ListStatusTranslationPollOptions(ctx, row.ID)
		if err != nil {
			return nil, wrap(err, "list status translation poll options")
		}
		for _, o := range options {
			t.PollOptions = append(t.PollOptions, translation.StoredPollOption{Position: int(o.Position), Title: o.Title})
		}
		out = append(out, t)
	}
	return out, nil
}

// DeleteStatusTranslations implements translation.Repository.
func (r *TranslationRepository) DeleteStatusTranslations(ctx context.Context, tx uow.Tx, statusID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := q.ListStatusTranslationIDs(ctx, statusID)
	if err != nil {
		return wrap(err, "list status translation ids")
	}
	return r.DeleteTranslations(ctx, tx, ids)
}

// ListExpiredTranslations implements translation.Repository.
func (r *TranslationRepository) ListExpiredTranslations(ctx context.Context, createdBefore time.Time, limit int) ([]string, error) {
	rows, err := r.db.reader().ListExpiredStatusTranslationIDs(ctx, sqlitegen.ListExpiredStatusTranslationIDsParams{
		CreatedBefore: toMillis(createdBefore), BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list expired status translations")
	}
	return rows, nil
}

// DeleteTranslations implements translation.Repository.
func (r *TranslationRepository) DeleteTranslations(ctx context.Context, tx uow.Tx, ids []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := q.DeleteStatusTranslationAttachments(ctx, id); err != nil {
			return wrap(err, "delete status translation attachments")
		}
		if err := q.DeleteStatusTranslationPollOptions(ctx, id); err != nil {
			return wrap(err, "delete status translation poll options")
		}
		if err := q.DeleteStatusTranslation(ctx, id); err != nil {
			return wrap(err, "delete status translation")
		}
	}
	return nil
}

// AddUsageWithin implements translation.Repository.
func (r *TranslationRepository) AddUsageWithin(ctx context.Context, tx uow.Tx, accountID, month string, characters int64, limits translation.Limits) (bool, error) {
	if characters > limits.Account || characters > limits.Server {
		return false, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	err = q.EnsureTranslationAccountUsage(ctx, sqlitegen.EnsureTranslationAccountUsageParams{AccountID: accountID, Month: month})
	if err != nil {
		return false, wrap(err, "ensure translation account usage")
	}
	n, err := q.AddTranslationAccountUsageWithin(ctx, sqlitegen.AddTranslationAccountUsageWithinParams{AccountID: accountID, Month: month, Characters: characters, MaxCharacters: limits.Account})
	if err != nil || n == 0 {
		return false, wrap(err, "add translation account usage")
	}
	if err := q.EnsureTranslationServerUsage(ctx, month); err != nil {
		return false, wrap(err, "ensure translation server usage")
	}
	n, err = q.AddTranslationServerUsageWithin(ctx, sqlitegen.AddTranslationServerUsageWithinParams{Month: month, Characters: characters, MaxCharacters: limits.Server})
	return n > 0, wrap(err, "add translation server usage")
}

// AddCacheLookup implements translation.Repository.
func (r *TranslationRepository) AddCacheLookup(ctx context.Context, tx uow.Tx, month string, hit bool) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if hit {
		return wrap(q.AddTranslationCacheHit(ctx, month), "add translation cache hit")
	}
	return wrap(q.AddTranslationCacheMiss(ctx, month), "add translation cache miss")
}

// FindUsage implements translation.Repository.
func (r *TranslationRepository) FindUsage(ctx context.Context, accountID, month string) (translation.Usage, error) {
	q := r.db.reader()
	account, err := q.GetTranslationAccountUsage(ctx, sqlitegen.GetTranslationAccountUsageParams{AccountID: accountID, Month: month})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return translation.Usage{}, wrap(err, "get translation account usage")
	}
	server, err := r.FindServerUsage(ctx, month)
	if err != nil {
		return translation.Usage{}, err
	}
	return translation.Usage{Account: account, Server: server.Characters}, nil
}

// FindServerUsage implements translation.Repository.
func (r *TranslationRepository) FindServerUsage(ctx context.Context, month string) (translation.ServerUsage, error) {
	row, err := r.db.reader().GetTranslationServerUsage(ctx, month)
	if errors.Is(err, sql.ErrNoRows) {
		return translation.ServerUsage{}, nil
	}
	if err != nil {
		return translation.ServerUsage{}, wrap(err, "get translation server usage")
	}
	return translation.ServerUsage{Characters: row.Characters, CacheHits: row.CacheHits, CacheMisses: row.CacheMisses}, nil
}

// DeleteAccountUsage implements translation.Repository.
func (r *TranslationRepository) DeleteAccountUsage(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteTranslationAccountUsage(ctx, accountID), "delete translation account usage")
}

// DeleteUsageBefore implements translation.Repository.
func (r *TranslationRepository) DeleteUsageBefore(ctx context.Context, tx uow.Tx, month string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteTranslationAccountUsageBefore(ctx, month); err != nil {
		return wrap(err, "delete translation account usage")
	}
	return wrap(q.DeleteTranslationServerUsageBefore(ctx, month), "delete translation server usage")
}
