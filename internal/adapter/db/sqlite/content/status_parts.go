// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	"github.com/mycroblog/my/internal/domain/content"
)

// FindDeletions implements content.Repository.
func (r *StatusRepository) FindDeletions(ctx context.Context, statusIDs []string) (map[string]content.Deletion, error) {
	out := map[string]content.Deletion{}
	if len(statusIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListStatusDeletionsOf(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status deletions")
	}
	for _, row := range rows {
		out[row.StatusID] = deletionFrom(row)
	}
	return out, nil
}

// FindTags implements content.Repository.
func (r *StatusRepository) FindTags(ctx context.Context, ids []string) (map[string]content.Tag, error) {
	out := map[string]content.Tag{}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListTagsByIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list tags")
	}
	for _, row := range rows {
		out[row.ID] = tagFrom(row)
	}
	return out, nil
}

// FindCountersOf implements content.Repository.
func (r *StatusRepository) FindCountersOf(ctx context.Context, statusIDs []string) (map[string]content.Counters, error) {
	out := map[string]content.Counters{}
	if len(statusIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListStatusCountersOf(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list status counters")
	}
	for _, row := range rows {
		out[row.StatusID] = content.Counters(row)
	}
	return out, nil
}

// MentionedIn implements content.InteractionRepository.
func (r *InteractionRepository) MentionedIn(ctx context.Context, statusIDs []string, accountID string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(statusIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListStatusesMentioning(ctx, contentgen.ListStatusesMentioningParams{AccountID: accountID, Ids: list})
	if err != nil {
		return nil, sqlite.Wrap(err, "list statuses mentioning "+accountID)
	}
	for _, row := range rows {
		out[row] = true
	}
	return out, nil
}

// FindCustomEmojis implements content.Repository.
func (r *StatusRepository) FindCustomEmojis(ctx context.Context, ids []string) (map[string]content.CustomEmoji, error) {
	out := map[string]content.CustomEmoji{}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListCustomEmojisByIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list custom emoji")
	}
	for _, row := range rows {
		out[row.ID] = customEmojiFrom(row)
	}
	return out, nil
}
