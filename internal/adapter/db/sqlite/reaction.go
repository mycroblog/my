// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file holds the emoji reactions of content.InteractionRepository (ADR-0052).

// InsertReaction implements content.InteractionRepository.
func (r *InteractionRepository) InsertReaction(ctx context.Context, tx uow.Tx, re content.Reaction) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertReaction(ctx, sqlitegen.InsertReactionParams{
		ID: re.ID, AccountID: re.AccountID, StatusID: re.StatusID, EmojiKey: re.Emoji.Key(),
		CustomEmojiID: toNullString(re.Emoji.CustomEmojiID), Uri: re.URI, CreatedAt: toMillis(re.CreatedAt),
	})
	if isUniqueViolation(err, "reactions.uri") {
		return false, fmt.Errorf("insert reaction %s: %w", re.URI, appcontent.ErrReactionURIUsed)
	}
	if err != nil {
		return false, wrap(err, "insert reaction")
	}
	return n == 1, nil
}

// DeleteReaction implements content.InteractionRepository.
func (r *InteractionRepository) DeleteReaction(ctx context.Context, tx uow.Tx, id string) (content.Reaction, bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.Reaction{}, false, err
	}
	rows, err := q.DeleteReaction(ctx, id)
	if err != nil {
		return content.Reaction{}, false, wrap(err, "delete reaction")
	}
	if len(rows) == 0 {
		return content.Reaction{}, false, nil
	}
	return reactionFrom(rows[0]), true, nil
}

// DeleteAccountStatusReactions implements content.InteractionRepository.
func (r *InteractionRepository) DeleteAccountStatusReactions(ctx context.Context, tx uow.Tx, accountID, statusID, exceptKey string) ([]content.Reaction, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.DeleteAccountStatusReactions(ctx, sqlitegen.DeleteAccountStatusReactionsParams{AccountID: accountID, StatusID: statusID, ExceptKey: exceptKey})
	if err != nil {
		return nil, wrap(err, "delete reactions of an account")
	}
	return reactionsFrom(rows), nil
}

// AddReactions implements content.InteractionRepository.
func (r *InteractionRepository) AddReactions(ctx context.Context, tx uow.Tx, statusID string, emoji content.ReactionEmoji, delta int64) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	key := emoji.Key()
	switch delta {
	case 1:
		err = q.IncrementReactionCount(ctx, sqlitegen.IncrementReactionCountParams{
			EmojiKey: key, CustomEmojiID: toNullString(emoji.CustomEmojiID), StatusID: statusID,
		})
		return wrap(err, "increment reaction count")
	case -1:
		if err := q.DeleteSingleReactionCount(ctx, sqlitegen.DeleteSingleReactionCountParams{StatusID: statusID, EmojiKey: key}); err != nil {
			return wrap(err, "delete reaction count")
		}
		err = q.DecrementReactionCount(ctx, sqlitegen.DecrementReactionCountParams{StatusID: statusID, EmojiKey: key})
		return wrap(err, "decrement reaction count")
	}
	return fmt.Errorf("sqlite: add reactions: delta %d is not 1 or -1", delta)
}

// DeleteStatusReactionCounts implements content.InteractionRepository.
func (r *InteractionRepository) DeleteStatusReactionCounts(ctx context.Context, tx uow.Tx, statusID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteStatusReactionCounts(ctx, statusID), "delete reaction counts")
}

// RebuildReactionCounts implements content.InteractionRepository.
func (r *InteractionRepository) RebuildReactionCounts(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteAllReactionCounts(ctx); err != nil {
		return wrap(err, "empty status_reaction_counts")
	}
	return wrap(q.RebuildReactionCounts(ctx), "rebuild status_reaction_counts")
}

// FindReaction implements content.InteractionRepository.
func (r *InteractionRepository) FindReaction(ctx context.Context, accountID, statusID, emojiKey string) (content.Reaction, error) {
	row, err := r.db.reader().GetReaction(ctx, sqlitegen.GetReactionParams{AccountID: accountID, StatusID: statusID, EmojiKey: emojiKey})
	if err != nil {
		return content.Reaction{}, notFound(err, appcontent.ErrNotFound, "reaction of "+accountID+" on "+statusID)
	}
	return reactionFrom(row), nil
}

// FindReactionByURI implements content.InteractionRepository.
func (r *InteractionRepository) FindReactionByURI(ctx context.Context, uri string) (content.Reaction, error) {
	row, err := r.db.reader().GetReactionByURI(ctx, uri)
	if err != nil {
		return content.Reaction{}, notFound(err, appcontent.ErrNotFound, "reaction "+uri)
	}
	return reactionFrom(row), nil
}

// ListAccountStatusReactions implements content.InteractionRepository.
func (r *InteractionRepository) ListAccountStatusReactions(ctx context.Context, accountID, statusID string) ([]content.Reaction, error) {
	rows, err := r.db.reader().ListAccountStatusReactions(ctx, sqlitegen.ListAccountStatusReactionsParams{AccountID: accountID, StatusID: statusID})
	if err != nil {
		return nil, wrap(err, "list reactions of an account")
	}
	return reactionsFrom(rows), nil
}

// ListStatusReactions implements content.InteractionRepository.
func (r *InteractionRepository) ListStatusReactions(ctx context.Context, statusID, emojiKey, before string, limit int) ([]content.Reaction, error) {
	rows, err := r.db.reader().ListStatusReactions(ctx, sqlitegen.ListStatusReactionsParams{
		StatusID: statusID, EmojiKey: emojiKey, Before: before, PageLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list reactions to a status")
	}
	return reactionsFrom(rows), nil
}

// ListReactionCounts implements content.InteractionRepository.
func (r *InteractionRepository) ListReactionCounts(ctx context.Context, statusID string, limit int) ([]content.ReactionCount, error) {
	rows, err := r.db.reader().ListReactionCounts(ctx, sqlitegen.ListReactionCountsParams{StatusID: statusID, PageLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list reaction counts")
	}
	return reactionCountsFrom(rows), nil
}

// ListAllReactionCounts implements content.InteractionRepository.
func (r *InteractionRepository) ListAllReactionCounts(ctx context.Context) ([]content.ReactionCount, error) {
	rows, err := r.db.reader().ListAllReactionCounts(ctx)
	if err != nil {
		return nil, wrap(err, "list reaction counts")
	}
	return reactionCountsFrom(rows), nil
}

func reactionEmojiFrom(key string, customEmojiID string) content.ReactionEmoji {
	if customEmojiID != "" {
		return content.ReactionEmoji{CustomEmojiID: customEmojiID}
	}
	return content.ReactionEmoji{Unicode: key}
}

func reactionFrom(row sqlitegen.Reaction) content.Reaction {
	return content.Reaction{
		ID: row.ID, AccountID: row.AccountID, StatusID: row.StatusID, Emoji: reactionEmojiFrom(row.EmojiKey, row.CustomEmojiID.String),
		URI: row.Uri, CreatedAt: fromMillis(row.CreatedAt),
	}
}

func reactionsFrom(rows []sqlitegen.Reaction) []content.Reaction {
	out := make([]content.Reaction, len(rows))
	for i, row := range rows {
		out[i] = reactionFrom(row)
	}
	return out
}

func reactionCountsFrom(rows []sqlitegen.StatusReactionCount) []content.ReactionCount {
	out := make([]content.ReactionCount, len(rows))
	for i, row := range rows {
		out[i] = content.ReactionCount{StatusID: row.StatusID, Emoji: reactionEmojiFrom(row.EmojiKey, row.CustomEmojiID.String), Count: row.Count}
	}
	return out
}
