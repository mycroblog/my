// SPDX-License-Identifier: AGPL-3.0-only

package translation

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/translationgen"
	"github.com/mycroblog/my/internal/app/translation"
	"github.com/mycroblog/my/internal/app/uow"
)

// Repository implements translation.Repository.
type Repository struct{ db *moduleDB }

var _ translation.Repository = (*Repository)(nil)

// NewRepository returns the repository of the translation module.
func NewRepository(d *postgres.DB) *Repository {
	return &Repository{db: &moduleDB{d}}
}

// InsertTranslation implements translation.Repository.
func (r *Repository) InsertTranslation(ctx context.Context, tx uow.Tx, t translation.Stored) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertStatusTranslation(ctx, translationgen.InsertStatusTranslationParams{
		ID: t.ID, StatusID: t.StatusID, RevisionID: t.RevisionID, TargetLanguage: t.TargetLanguage, Provider: t.Provider,
		SourceLanguage: t.SourceLanguage, ContentHtml: t.ContentHTML, SpoilerText: t.SpoilerText, CreatedAt: t.CreatedAt,
	})
	if err != nil {
		return false, postgres.Wrap(err, "insert status translation")
	}
	if n == 0 {
		return false, nil
	}
	for _, a := range t.Attachments {
		position, err := postgres.ToInt32(a.Position, "attachment position")
		if err != nil {
			return false, err
		}
		err = q.InsertStatusTranslationAttachment(ctx, translationgen.InsertStatusTranslationAttachmentParams{
			TranslationID: t.ID, Position: position, Description: a.Description,
		})
		if err != nil {
			return false, postgres.Wrap(err, "insert status translation attachment")
		}
	}
	for _, o := range t.PollOptions {
		position, err := postgres.ToInt32(o.Position, "poll option position")
		if err != nil {
			return false, err
		}
		err = q.InsertStatusTranslationPollOption(ctx, translationgen.InsertStatusTranslationPollOptionParams{
			TranslationID: t.ID, Position: position, Title: o.Title,
		})
		if err != nil {
			return false, postgres.Wrap(err, "insert status translation poll option")
		}
	}
	return true, nil
}

// FindTranslations implements translation.Repository.
func (r *Repository) FindTranslations(ctx context.Context, revisionID, targetLanguage string) ([]translation.Stored, error) {
	q := r.db.reader()
	rows, err := q.ListStatusTranslations(ctx, translationgen.ListStatusTranslationsParams{RevisionID: revisionID, TargetLanguage: targetLanguage})
	if err != nil {
		return nil, postgres.Wrap(err, "list status translations")
	}
	out := make([]translation.Stored, 0, len(rows))
	for _, row := range rows {
		t := translation.Stored{
			ID: row.ID, StatusID: row.StatusID, RevisionID: row.RevisionID, TargetLanguage: row.TargetLanguage, Provider: row.Provider,
			SourceLanguage: row.SourceLanguage, ContentHTML: row.ContentHtml, SpoilerText: row.SpoilerText, CreatedAt: postgres.UTC(row.CreatedAt),
		}
		attachments, err := q.ListStatusTranslationAttachments(ctx, row.ID)
		if err != nil {
			return nil, postgres.Wrap(err, "list status translation attachments")
		}
		for _, a := range attachments {
			t.Attachments = append(t.Attachments, translation.StoredAttachment{Position: int(a.Position), Description: a.Description})
		}
		options, err := q.ListStatusTranslationPollOptions(ctx, row.ID)
		if err != nil {
			return nil, postgres.Wrap(err, "list status translation poll options")
		}
		for _, o := range options {
			t.PollOptions = append(t.PollOptions, translation.StoredPollOption{Position: int(o.Position), Title: o.Title})
		}
		out = append(out, t)
	}
	return out, nil
}

// DeleteStatusTranslations implements translation.Repository.
func (r *Repository) DeleteStatusTranslations(ctx context.Context, tx uow.Tx, statusID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := q.ListStatusTranslationIDs(ctx, statusID)
	if err != nil {
		return postgres.Wrap(err, "list status translation ids")
	}
	return r.DeleteTranslations(ctx, tx, ids)
}

// ListExpiredTranslations implements translation.Repository.
func (r *Repository) ListExpiredTranslations(ctx context.Context, createdBefore time.Time, limit int) ([]string, error) {
	n, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListExpiredStatusTranslationIDs(ctx, translationgen.ListExpiredStatusTranslationIDsParams{
		CreatedBefore: createdBefore, BatchLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list expired status translations")
	}
	return rows, nil
}

// DeleteTranslations implements translation.Repository.
func (r *Repository) DeleteTranslations(ctx context.Context, tx uow.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	list := ids
	for _, step := range []struct {
		what string
		fn   func(context.Context, []string) error
	}{
		{"delete status translation attachments", q.DeleteStatusTranslationAttachmentsOf},
		{"delete status translation poll options", q.DeleteStatusTranslationPollOptionsOf},
		{"delete status translations", q.DeleteStatusTranslationsByIDs},
	} {
		if err := step.fn(ctx, list); err != nil {
			return postgres.Wrap(err, step.what)
		}
	}
	return nil
}

// DeleteStatusTranslationsOf implements translation.Repository.
func (r *Repository) DeleteStatusTranslationsOf(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	if len(statusIDs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	list := statusIDs
	ids, err := q.ListStatusTranslationIDsOf(ctx, list)
	if err != nil {
		return postgres.Wrap(err, "list status translation ids")
	}
	return r.DeleteTranslations(ctx, tx, ids)
}

// AddUsageWithin implements translation.Repository.
func (r *Repository) AddUsageWithin(ctx context.Context, tx uow.Tx, accountID, month string, characters int64, limits translation.Limits) (bool, error) {
	if (accountID != "" && characters > limits.Account) || characters > limits.Server {
		return false, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	// Without an account, only the server's row counts (a moderator's translation of a report).
	if accountID != "" {
		err = q.EnsureTranslationAccountUsage(ctx, translationgen.EnsureTranslationAccountUsageParams{AccountID: accountID, Month: month})
		if err != nil {
			return false, postgres.Wrap(err, "ensure translation account usage")
		}
		n, err := q.AddTranslationAccountUsageWithin(ctx, translationgen.AddTranslationAccountUsageWithinParams{AccountID: accountID, Month: month, Characters: characters, MaxCharacters: limits.Account})
		if err != nil || n == 0 {
			return false, postgres.Wrap(err, "add translation account usage")
		}
	}
	if err := q.EnsureTranslationServerUsage(ctx, month); err != nil {
		return false, postgres.Wrap(err, "ensure translation server usage")
	}
	n, err := q.AddTranslationServerUsageWithin(ctx, translationgen.AddTranslationServerUsageWithinParams{Month: month, Characters: characters, MaxCharacters: limits.Server})
	return n > 0, postgres.Wrap(err, "add translation server usage")
}

// AddCacheLookups implements translation.Repository.
func (r *Repository) AddCacheLookups(ctx context.Context, tx uow.Tx, month string, hits, misses int64) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.AddTranslationCacheLookups(ctx, translationgen.AddTranslationCacheLookupsParams{Month: month, Hits: hits, Misses: misses})
	return postgres.Wrap(err, "add translation cache lookups")
}

// FindUsage implements translation.Repository.
func (r *Repository) FindUsage(ctx context.Context, accountID, month string) (translation.Usage, error) {
	q := r.db.reader()
	account, err := q.GetTranslationAccountUsage(ctx, translationgen.GetTranslationAccountUsageParams{AccountID: accountID, Month: month})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return translation.Usage{}, postgres.Wrap(err, "get translation account usage")
	}
	server, err := r.FindServerUsage(ctx, month)
	if err != nil {
		return translation.Usage{}, err
	}
	return translation.Usage{Account: account, Server: server.Characters}, nil
}

// FindServerUsage implements translation.Repository.
func (r *Repository) FindServerUsage(ctx context.Context, month string) (translation.ServerUsage, error) {
	row, err := r.db.reader().GetTranslationServerUsage(ctx, month)
	if errors.Is(err, pgx.ErrNoRows) {
		return translation.ServerUsage{}, nil
	}
	if err != nil {
		return translation.ServerUsage{}, postgres.Wrap(err, "get translation server usage")
	}
	return translation.ServerUsage{Characters: row.Characters, CacheHits: row.CacheHits, CacheMisses: row.CacheMisses}, nil
}

// DeleteAccountUsage implements translation.Repository.
func (r *Repository) DeleteAccountUsage(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteTranslationAccountUsage(ctx, accountID), "delete translation account usage")
}

// DeleteUsageBefore implements translation.Repository.
func (r *Repository) DeleteUsageBefore(ctx context.Context, tx uow.Tx, month string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteTranslationAccountUsageBefore(ctx, month); err != nil {
		return postgres.Wrap(err, "delete translation account usage")
	}
	return postgres.Wrap(q.DeleteTranslationServerUsageBefore(ctx, month), "delete translation server usage")
}
