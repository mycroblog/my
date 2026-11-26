// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"encoding/json"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
)

// InsertReplayed implements timeline.Repository, one statement for each kind of row, each a
// JSON array of the rows.
func (r *Repository) InsertReplayed(ctx context.Context, tx uow.Tx, entries []apptimeline.Entry, recipients []apptimeline.DirectRecipient,
	tags []apptimeline.TagEntry, usage map[string]int64,
) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		type row struct {
			ID                      string `json:"id"`
			Kind                    string `json:"kind"`
			StatusID                string `json:"status_id"`
			AuthorAccountID         string `json:"author_account_id"`
			AuthorDomain            string `json:"author_domain"`
			OriginalAuthorAccountID string `json:"original_author_account_id"`
			OriginalAuthorDomain    string `json:"original_author_domain"`
			Visibility              string `json:"visibility"`
			IsLocal                 int64  `json:"is_local"`
			Language                string `json:"language"`
			RevisionSeq             int64  `json:"revision_seq"`
			IsReply                 int64  `json:"is_reply"`
		}
		rows := make([]row, len(entries))
		for i, e := range entries {
			rows[i] = row{
				ID: e.ID, Kind: string(e.Kind), StatusID: e.StatusID, AuthorAccountID: e.AuthorAccountID, AuthorDomain: e.AuthorDomain,
				OriginalAuthorAccountID: e.OriginalAuthorAccountID, OriginalAuthorDomain: e.OriginalAuthorDomain,
				Visibility: string(e.Visibility), IsLocal: sqlite.BoolToInt(e.IsLocal), Language: e.Language,
				RevisionSeq: int64(e.RevisionSeq), IsReply: sqlite.BoolToInt(e.IsReply),
			}
		}
		list, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		if err := q.InsertManyFeedEntries(ctx, string(list)); err != nil {
			return sqlite.Wrap(err, "insert feed entries")
		}
	}
	if len(recipients) > 0 {
		type row struct {
			EntryID            string `json:"entry_id"`
			RecipientAccountID string `json:"recipient_account_id"`
		}
		rows := make([]row, len(recipients))
		for i, d := range recipients {
			rows[i] = row{EntryID: d.EntryID, RecipientAccountID: d.RecipientAccountID}
		}
		list, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		if err := q.InsertManyFeedDirectRecipients(ctx, string(list)); err != nil {
			return sqlite.Wrap(err, "insert feed direct recipients")
		}
	}
	if len(tags) > 0 {
		type row struct {
			TagID           string `json:"tag_id"`
			EntryID         string `json:"entry_id"`
			AuthorAccountID string `json:"author_account_id"`
			Visibility      string `json:"visibility"`
			IsLocal         int64  `json:"is_local"`
			RevisedAt       int64  `json:"revised_at"`
		}
		rows := make([]row, len(tags))
		for i, t := range tags {
			rows[i] = row{
				TagID: t.TagID, EntryID: t.EntryID, AuthorAccountID: t.AuthorAccountID, Visibility: string(t.Visibility),
				IsLocal: sqlite.BoolToInt(t.IsLocal), RevisedAt: sqlite.ToMillis(t.RevisedAt),
			}
		}
		list, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		if err := q.InsertManyFeedTagEntries(ctx, string(list)); err != nil {
			return sqlite.Wrap(err, "insert feed tag entries")
		}
	}
	return applyTagUsage(ctx, q, usage)
}
