// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// insertRemoteRevisionParts writes the poll and the remote subtype of a revision.
func insertRemoteRevisionParts(ctx context.Context, q *sqlitegen.Queries, rev content.Revision) error {
	if p := rev.Poll; p != nil {
		err := q.InsertPoll(ctx, sqlitegen.InsertPollParams{
			ID: p.ID, RevisionID: rev.ID, Multiple: boolToInt(p.Multiple), HideTotals: boolToInt(p.HideTotals), ClosesAt: toNullMillis(p.ClosesAt),
		})
		if err != nil {
			return wrap(err, "insert poll")
		}
		for _, o := range p.Options {
			err := q.InsertPollOption(ctx, sqlitegen.InsertPollOptionParams{PollID: p.ID, Position: int64(o.Position), Title: o.Title})
			if err != nil {
				return wrap(err, "insert poll option")
			}
		}
	}
	if r := rev.Remote; r != nil {
		err := q.InsertRemoteStatusRevision(ctx, sqlitegen.InsertRemoteStatusRevisionParams{
			RevisionID: rev.ID, ActivityUri: toNullString(r.ActivityURI), HtmlPolicyVersion: int64(r.HTMLPolicyVersion),
		})
		if err != nil {
			return wrap(err, "insert remote status revision")
		}
	}
	return nil
}

// findRemoteRevisionParts reads the poll and the remote subtype of rev.
func findRemoteRevisionParts(ctx context.Context, q *sqlitegen.Queries, rev *content.Revision) error {
	poll, err := q.GetPollByRevision(ctx, rev.ID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return wrap(err, "get poll")
	default:
		p := &content.Poll{ID: poll.ID, Multiple: poll.Multiple != 0, HideTotals: poll.HideTotals != 0, ClosesAt: fromNullMillis(poll.ClosesAt)}
		options, err := q.ListPollOptions(ctx, poll.ID)
		if err != nil {
			return wrap(err, "list poll options")
		}
		for _, o := range options {
			p.Options = append(p.Options, content.PollOption{Position: int(o.Position), Title: o.Title})
		}
		rev.Poll = p
	}
	remote, err := q.GetRemoteStatusRevision(ctx, rev.ID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return wrap(err, "get remote status revision")
	default:
		rev.Remote = &content.RemoteRevisionSource{ActivityURI: remote.ActivityUri.String, HTMLPolicyVersion: int(remote.HtmlPolicyVersion)}
	}
	return nil
}

// LocalStatusInConversation implements content.Repository.
func (r *StatusRepository) LocalStatusInConversation(ctx context.Context, conversationURI string) (bool, error) {
	if conversationURI == "" {
		return false, nil
	}
	ok, err := r.db.reader().ExistsLocalStatusInConversation(ctx, toNullString(conversationURI))
	return ok, wrap(err, "find local status in conversation")
}

// EnsureCustomEmoji implements content.Repository.
func (r *StatusRepository) EnsureCustomEmoji(ctx context.Context, tx uow.Tx, e content.CustomEmoji) (content.CustomEmoji, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.CustomEmoji{}, err
	}
	params := sqlitegen.InsertCustomEmojiIfAbsentParams{
		ID: e.ID, Shortcode: e.Shortcode, Domain: e.Domain, Uri: toNullString(e.URI), CreatedAt: toMillis(e.CreatedAt),
	}
	for range 2 {
		if err := q.InsertCustomEmojiIfAbsent(ctx, params); err != nil {
			return content.CustomEmoji{}, wrap(err, "insert custom emoji")
		}
		row, err := q.GetCustomEmojiByShortcode(ctx, sqlitegen.GetCustomEmojiByShortcodeParams{Shortcode: e.Shortcode, Domain: e.Domain})
		if err == nil {
			return customEmojiFrom(row), nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return content.CustomEmoji{}, wrap(err, "get custom emoji")
		}
		// Another emoji has the URI: store this one without it.
		params.Uri = sql.NullString{}
	}
	return content.CustomEmoji{}, wrap(sql.ErrNoRows, "ensure custom emoji")
}

// CurrentCustomEmojiImageURL implements content.Repository.
func (r *StatusRepository) CurrentCustomEmojiImageURL(ctx context.Context, tx uow.Tx, emojiID string) (string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return "", err
	}
	remoteURL, err := q.GetLatestCustomEmojiImageRemoteURL(ctx, emojiID)
	if err != nil {
		return "", notFound(err, appcontent.ErrNotFound, "image of emoji "+emojiID)
	}
	return remoteURL, nil
}

// InsertCustomEmojiImage implements content.Repository.
func (r *StatusRepository) InsertCustomEmojiImage(ctx context.Context, tx uow.Tx, img content.CustomEmojiImage) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertCustomEmojiImage(ctx, sqlitegen.InsertCustomEmojiImageParams{
		ID: img.ID, EmojiID: img.EmojiID, MediaID: img.MediaID, RevisedAt: toMillis(img.RevisedAt),
	})
	return wrap(err, "insert custom emoji image")
}

// LatestPollObservation implements content.Repository.
func (r *StatusRepository) LatestPollObservation(ctx context.Context, tx uow.Tx, pollID string) (content.PollTallyObservation, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return content.PollTallyObservation{}, err
	}
	row, err := q.GetLatestPollTallyObservation(ctx, pollID)
	if err != nil {
		return content.PollTallyObservation{}, notFound(err, appcontent.ErrNotFound, "observation of poll "+pollID)
	}
	o := content.PollTallyObservation{
		ID: row.ID, PollID: row.PollID, VotersCount: fromNullInt64(row.VotersCount), ObservedAt: fromMillis(row.ObservedAt),
	}
	counts, err := q.ListPollTallyObservationCounts(ctx, row.ID)
	if err != nil {
		return content.PollTallyObservation{}, wrap(err, "list poll observation counts")
	}
	for _, c := range counts {
		o.Votes = append(o.Votes, c.Votes)
	}
	return o, nil
}

// InsertPollObservation implements content.Repository.
func (r *StatusRepository) InsertPollObservation(ctx context.Context, tx uow.Tx, o content.PollTallyObservation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertPollTallyObservation(ctx, sqlitegen.InsertPollTallyObservationParams{
		ID: o.ID, PollID: o.PollID, VotersCount: toNullInt64(o.VotersCount), ObservedAt: toMillis(o.ObservedAt),
	})
	if err != nil {
		return wrap(err, "insert poll observation")
	}
	for i, v := range o.Votes {
		err := q.InsertPollTallyObservationCount(ctx, sqlitegen.InsertPollTallyObservationCountParams{
			ObservationID: o.ID, Position: int64(i), Votes: v,
		})
		if err != nil {
			return wrap(err, "insert poll observation count")
		}
	}
	return nil
}

// ReplacePollTallies implements content.Repository.
func (r *StatusRepository) ReplacePollTallies(ctx context.Context, tx uow.Tx, pollID string, tallies []content.PollTally) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeletePollTallies(ctx, pollID); err != nil {
		return wrap(err, "delete poll tallies")
	}
	for _, t := range tallies {
		err := q.InsertPollTally(ctx, sqlitegen.InsertPollTallyParams{
			PollID: pollID, Position: int64(t.Position), Votes: t.Votes, VotersCount: toNullInt64(t.VotersCount),
		})
		if err != nil {
			return wrap(err, "insert poll tally")
		}
	}
	return nil
}

// FindCustomEmoji implements content.Repository.
func (r *StatusRepository) FindCustomEmoji(ctx context.Context, id string) (content.CustomEmoji, error) {
	row, err := r.db.reader().GetCustomEmoji(ctx, id)
	if err != nil {
		return content.CustomEmoji{}, notFound(err, appcontent.ErrNotFound, "custom emoji "+id)
	}
	return customEmojiFrom(row), nil
}

// FindCustomEmojiByShortcode implements content.Repository.
func (r *StatusRepository) FindCustomEmojiByShortcode(ctx context.Context, shortcode, domain string) (content.CustomEmoji, error) {
	row, err := r.db.reader().GetCustomEmojiByShortcode(ctx, sqlitegen.GetCustomEmojiByShortcodeParams{Shortcode: shortcode, Domain: domain})
	if err != nil {
		return content.CustomEmoji{}, notFound(err, appcontent.ErrNotFound, "custom emoji "+shortcode+"@"+domain)
	}
	return customEmojiFrom(row), nil
}

// FindCustomEmojiImage implements content.Repository.
func (r *StatusRepository) FindCustomEmojiImage(ctx context.Context, emojiID string) (content.CustomEmojiImage, error) {
	row, err := r.db.reader().GetLatestCustomEmojiImage(ctx, emojiID)
	if err != nil {
		return content.CustomEmojiImage{}, notFound(err, appcontent.ErrNotFound, "image of emoji "+emojiID)
	}
	return customEmojiImageFrom(row), nil
}

// FindPollTallies implements content.Repository.
func (r *StatusRepository) FindPollTallies(ctx context.Context, pollID string) ([]content.PollTally, error) {
	rows, err := r.db.reader().ListPollTallies(ctx, pollID)
	if err != nil {
		return nil, wrap(err, "list poll tallies")
	}
	out := make([]content.PollTally, len(rows))
	for i, row := range rows {
		out[i] = content.PollTally{PollID: row.PollID, Position: int(row.Position), Votes: row.Votes, VotersCount: fromNullInt64(row.VotersCount)}
	}
	return out, nil
}

func customEmojiFrom(row sqlitegen.CustomEmoji) content.CustomEmoji {
	return content.CustomEmoji{
		ID: row.ID, Shortcode: row.Shortcode, Domain: row.Domain, URI: row.Uri.String, CreatedByUserID: row.CreatedByUserID.String,
		CreatedAt: fromMillis(row.CreatedAt),
	}
}

func customEmojiImageFrom(row sqlitegen.CustomEmojiImage) content.CustomEmojiImage {
	return content.CustomEmojiImage{ID: row.ID, EmojiID: row.EmojiID, MediaID: row.MediaID, RevisedAt: fromMillis(row.RevisedAt)}
}

// InsertPollVote implements content.Repository.
func (r *StatusRepository) InsertPollVote(ctx context.Context, tx uow.Tx, v content.PollVote) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertPollVote(ctx, sqlitegen.InsertPollVoteParams{
		ID: v.ID, PollID: v.PollID, AccountID: v.AccountID, Position: int64(v.Position), Uri: toNullString(v.URI), VotedAt: toMillis(v.VotedAt),
	})
	if err != nil {
		return false, wrap(err, "insert poll vote")
	}
	return n > 0, nil
}

// ListPollVotes implements content.Repository.
func (r *StatusRepository) ListPollVotes(ctx context.Context, pollID, accountID string) ([]content.PollVote, error) {
	rows, err := r.db.reader().ListPollVotesOfAccount(ctx, sqlitegen.ListPollVotesOfAccountParams{PollID: pollID, AccountID: accountID})
	if err != nil {
		return nil, wrap(err, "list poll votes")
	}
	out := make([]content.PollVote, len(rows))
	for i, row := range rows {
		out[i] = content.PollVote{
			ID: row.ID, PollID: row.PollID, AccountID: row.AccountID, Position: int(row.Position), URI: row.Uri.String, VotedAt: fromMillis(row.VotedAt),
		}
	}
	return out, nil
}

// ListPollVotesTx implements content.Repository.
func (r *StatusRepository) ListPollVotesTx(ctx context.Context, tx uow.Tx, pollID, accountID string) ([]content.PollVote, error) {
	db, err := r.db.in(tx)
	if err != nil {
		return nil, err
	}
	return (&StatusRepository{db: db}).ListPollVotes(ctx, pollID, accountID)
}

// PollStatusID implements content.Repository.
func (r *StatusRepository) PollStatusID(ctx context.Context, pollID string) (string, error) {
	id, err := r.db.reader().GetPollStatus(ctx, pollID)
	if err != nil {
		return "", notFound(err, appcontent.ErrNotFound, "poll "+pollID)
	}
	return id, nil
}

// LockPoll implements content.Repository. It does nothing: write transactions begin with BEGIN
// IMMEDIATE and run one at a time.
func (r *StatusRepository) LockPoll(context.Context, uow.Tx, string) error { return nil }

// CountPollVotes implements content.Repository.
func (r *StatusRepository) CountPollVotes(ctx context.Context, tx uow.Tx, pollID string) (map[int]int64, int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.CountPollVotes(ctx, pollID)
	if err != nil {
		return nil, 0, wrap(err, "count poll votes")
	}
	votes := make(map[int]int64, len(rows))
	for _, row := range rows {
		votes[int(row.Position)] = row.Votes
	}
	voters, err := q.CountPollVoters(ctx, pollID)
	if err != nil {
		return nil, 0, wrap(err, "count poll voters")
	}
	return votes, voters, nil
}

// ListPollVoterIDs implements content.Repository.
func (r *StatusRepository) ListPollVoterIDs(ctx context.Context, pollID, after string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListPollVoterIDs(ctx, sqlitegen.ListPollVoterIDsParams{PollID: pollID, After: after, Lim: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list poll voters")
	}
	return ids, nil
}
