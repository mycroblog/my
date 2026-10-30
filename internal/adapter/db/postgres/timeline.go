// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// TimelineRepository implements timeline.Repository on PostgreSQL.
type TimelineRepository struct {
	db *DB
}

var _ apptimeline.Repository = (*TimelineRepository)(nil)

// TimelineRepository returns the timeline module's repository on d.
func (d *DB) TimelineRepository() *TimelineRepository { return &TimelineRepository{db: d} }

// InsertEntry implements timeline.Repository.
func (r *TimelineRepository) InsertEntry(ctx context.Context, tx uow.Tx, e apptimeline.Entry) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	seq, err := toInt32(e.RevisionSeq, "revision seq")
	if err != nil {
		return false, err
	}
	n, err := q.InsertFeedEntry(ctx, postgresgen.InsertFeedEntryParams{
		ID: e.ID, Kind: string(e.Kind), StatusID: e.StatusID,
		AuthorAccountID: e.AuthorAccountID, AuthorDomain: e.AuthorDomain,
		OriginalAuthorAccountID: e.OriginalAuthorAccountID, OriginalAuthorDomain: e.OriginalAuthorDomain,
		Visibility: string(e.Visibility), IsLocal: e.IsLocal, Language: e.Language, RevisionSeq: seq,
		IsReply: e.IsReply,
	})
	if err != nil {
		return false, wrap(err, "insert feed entry")
	}
	return n == 1, nil
}

// Entry implements timeline.Repository.
func (r *TimelineRepository) Entry(ctx context.Context, tx uow.Tx, id string) (apptimeline.Entry, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return apptimeline.Entry{}, err
	}
	row, err := q.GetFeedEntry(ctx, id)
	if err != nil {
		return apptimeline.Entry{}, notFound(err, apptimeline.ErrNotFound, "feed entry "+id)
	}
	return entryFrom(row), nil
}

// UpdateRevision implements timeline.Repository.
func (r *TimelineRepository) UpdateRevision(ctx context.Context, tx uow.Tx, statusID, language string, seq int) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := toInt32(seq, "revision seq")
	if err != nil {
		return err
	}
	_, err = q.UpdateFeedEntriesRevision(ctx, postgresgen.UpdateFeedEntriesRevisionParams{
		Language: language, RevisionSeq: n, StatusID: statusID,
	})
	return wrap(err, "update the revision of feed entries")
}

// DeleteEntry implements timeline.Repository.
func (r *TimelineRepository) DeleteEntry(ctx context.Context, tx uow.Tx, id string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := deleteEntryChildren(ctx, q, id); err != nil {
		return err
	}
	return wrap(q.DeleteFeedEntry(ctx, id), "delete feed entry")
}

// DeleteStatusEntries implements timeline.Repository. Only the post entry (ID = status ID) has
// direct recipients and tags.
func (r *TimelineRepository) DeleteStatusEntries(ctx context.Context, tx uow.Tx, statusID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := deleteEntryChildren(ctx, q, statusID); err != nil {
		return err
	}
	return wrap(q.DeleteFeedEntriesOfStatus(ctx, statusID), "delete feed entries of a status")
}

func deleteEntryChildren(ctx context.Context, q *postgresgen.Queries, entryID string) error {
	if err := q.DeleteFeedDirectRecipients(ctx, entryID); err != nil {
		return wrap(err, "delete feed direct recipients")
	}
	used, err := q.ListCountedFeedTagIDs(ctx, entryID)
	if err != nil {
		return wrap(err, "list the counted tags of a feed entry")
	}
	if err := q.DeleteFeedTagEntries(ctx, entryID); err != nil {
		return wrap(err, "delete feed tag entries")
	}
	deltas := make(map[string]int64, len(used))
	for _, id := range used {
		deltas[id]--
	}
	return applyTagUsage(ctx, q, deltas)
}

// applyTagUsage adds deltas[tag] posts to the usage of each tag, in tag order, after
// feed_tag_entries has changed, and reads its last_used_at again (tag_usage, ADR-0092 §1). A
// tag's usage that drops to zero is deleted.
func applyTagUsage(ctx context.Context, q *postgresgen.Queries, deltas map[string]int64) error {
	for _, id := range slices.Sorted(maps.Keys(deltas)) {
		d := deltas[id]
		var err error
		switch {
		case d > 0:
			err = q.AddTagUsage(ctx, postgresgen.AddTagUsageParams{TagID: id, N: d})
		case d < 0:
			if err = q.DeleteDrainedTagUsage(ctx, postgresgen.DeleteDrainedTagUsageParams{TagID: id, N: -d}); err == nil {
				err = q.AdjustTagUsage(ctx, postgresgen.AdjustTagUsageParams{TagID: id, Delta: d})
			}
		default:
			err = q.AdjustTagUsage(ctx, postgresgen.AdjustTagUsageParams{TagID: id})
		}
		if err != nil {
			return wrap(err, "update tag usage")
		}
	}
	return nil
}

// ReplaceDirectRecipients implements timeline.Repository.
func (r *TimelineRepository) ReplaceDirectRecipients(ctx context.Context, tx uow.Tx, entryID string, accountIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteFeedDirectRecipients(ctx, entryID); err != nil {
		return wrap(err, "delete feed direct recipients")
	}
	for _, id := range accountIDs {
		if err := q.InsertFeedDirectRecipient(ctx, postgresgen.InsertFeedDirectRecipientParams{EntryID: entryID, RecipientAccountID: id}); err != nil {
			return wrap(err, "insert feed direct recipient")
		}
	}
	return nil
}

// ReplaceTags implements timeline.Repository.
func (r *TimelineRepository) ReplaceTags(ctx context.Context, tx uow.Tx, e apptimeline.Entry, tagIDs []string, revisedAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	used, err := q.ListCountedFeedTagIDs(ctx, e.ID)
	if err != nil {
		return wrap(err, "list the counted tags of a feed entry")
	}
	if err := q.DeleteFeedTagEntries(ctx, e.ID); err != nil {
		return wrap(err, "delete feed tag entries")
	}
	deltas := map[string]int64{}
	for _, id := range used {
		deltas[id]--
	}
	counts := apptimeline.CountsTagUsage(e.Visibility)
	added := map[string]bool{}
	for _, id := range tagIDs {
		err := q.InsertFeedTagEntry(ctx, postgresgen.InsertFeedTagEntryParams{
			TagID: id, EntryID: e.ID, AuthorAccountID: e.AuthorAccountID, Visibility: string(e.Visibility), IsLocal: e.IsLocal,
			RevisedAt: utc(revisedAt),
		})
		if err != nil {
			return wrap(err, "insert feed tag entry")
		}
		if counts && !added[id] { // a tag listed twice counts once
			added[id] = true
			deltas[id]++
		}
	}
	return applyTagUsage(ctx, q, deltas)
}

// UpsertFollowEdge implements timeline.Repository.
func (r *TimelineRepository) UpsertFollowEdge(ctx context.Context, tx uow.Tx, e apptimeline.FollowEdge) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertFeedFollowEdge(ctx, postgresgen.UpsertFeedFollowEdgeParams{
		FollowerAccountID: e.FollowerAccountID, FolloweeAccountID: e.FolloweeAccountID, ShowBoosts: e.ShowBoosts,
	}), "upsert feed follow edge")
}

// DeleteFollowEdge implements timeline.Repository.
func (r *TimelineRepository) DeleteFollowEdge(ctx context.Context, tx uow.Tx, followerAccountID, followeeAccountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteFeedFollowEdge(ctx, postgresgen.DeleteFeedFollowEdgeParams{
		FollowerAccountID: followerAccountID, FolloweeAccountID: followeeAccountID,
	}), "delete feed follow edge")
}

// UpsertExclusion implements timeline.Repository.
func (r *TimelineRepository) UpsertExclusion(ctx context.Context, tx uow.Tx, x apptimeline.Exclusion) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertFeedExclusion(ctx, postgresgen.UpsertFeedExclusionParams{
		ViewerAccountID: x.ViewerAccountID, TargetAccountID: x.TargetAccountID, Reason: string(x.Reason), ExpiresAt: (x.ExpiresAt),
	}), "upsert feed exclusion")
}

// DeleteExclusion implements timeline.Repository.
func (r *TimelineRepository) DeleteExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, targetAccountID string, reason timeline.ExclusionReason) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteFeedExclusion(ctx, postgresgen.DeleteFeedExclusionParams{
		ViewerAccountID: viewerAccountID, TargetAccountID: targetAccountID, Reason: string(reason),
	}), "delete feed exclusion")
}

// UpsertAuthorRestriction implements timeline.Repository.
func (r *TimelineRepository) UpsertAuthorRestriction(ctx context.Context, tx uow.Tx, ar apptimeline.AuthorRestriction) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertFeedAuthorRestriction(ctx, postgresgen.UpsertFeedAuthorRestrictionParams{
		AccountID: ar.AccountID, Restriction: string(ar.Restriction), EffectiveUntil: (ar.EffectiveUntil),
	}), "upsert feed author restriction")
}

// DeleteAuthorRestriction implements timeline.Repository.
func (r *TimelineRepository) DeleteAuthorRestriction(ctx context.Context, tx uow.Tx, accountID string, restriction timeline.Restriction) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteFeedAuthorRestriction(ctx, postgresgen.DeleteFeedAuthorRestrictionParams{
		AccountID: accountID, Restriction: string(restriction),
	}), "delete feed author restriction")
}

// UpsertDomainRestriction implements timeline.Repository.
func (r *TimelineRepository) UpsertDomainRestriction(ctx context.Context, tx uow.Tx, dr apptimeline.DomainRestriction) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertFeedDomainRestriction(ctx, postgresgen.UpsertFeedDomainRestrictionParams{
		Domain: dr.Domain, Restriction: string(dr.Restriction), EffectiveUntil: (dr.EffectiveUntil),
	}), "upsert feed domain restriction")
}

// DeleteDomainRestriction implements timeline.Repository.
func (r *TimelineRepository) DeleteDomainRestriction(ctx context.Context, tx uow.Tx, domain string, restriction timeline.Restriction) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteFeedDomainRestriction(ctx, postgresgen.DeleteFeedDomainRestrictionParams{
		Domain: domain, Restriction: string(restriction),
	}), "delete feed domain restriction")
}

// InsertDomainMute implements timeline.Repository.
func (r *TimelineRepository) InsertDomainMute(ctx context.Context, tx uow.Tx, m apptimeline.DomainMute) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertFeedDomainMute(ctx, postgresgen.InsertFeedDomainMuteParams{
		ViewerAccountID: m.ViewerAccountID, Domain: m.Domain,
	}), "insert feed domain mute")
}

// DeleteDomainMute implements timeline.Repository.
func (r *TimelineRepository) DeleteDomainMute(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteFeedDomainMute(ctx, postgresgen.DeleteFeedDomainMuteParams{
		ViewerAccountID: viewerAccountID, Domain: domain,
	}), "delete feed domain mute")
}

// DeleteProjection implements timeline.Repository.
func (r *TimelineRepository) DeleteProjection(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, del := range []func(context.Context) error{
		q.DeleteAllFeedEntries, q.DeleteAllFeedDirectRecipients, q.DeleteAllFeedTagEntries, q.DeleteAllTagUsage,
		q.DeleteAllFeedFollowEdges, q.DeleteAllFeedExclusions, q.DeleteAllFeedDomainMutes,
	} {
		if err := del(ctx); err != nil {
			return wrap(err, "empty the feed projections")
		}
	}
	return nil
}

// ListHome implements timeline.Repository.
func (r *TimelineRepository) ListHome(ctx context.Context, hq apptimeline.HomeQuery) ([]apptimeline.Entry, error) {
	all, langs := languages(hq.Languages)
	limit, err := toInt32(hq.Limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListHomeEntries(ctx, postgresgen.ListHomeEntriesParams{
		Viewer: hq.ViewerAccountID, Before: toNullString(hq.Before), WindowFloor: toNullString(hq.Floor), Now: hq.Now,
		AllLanguages: all, Languages: langs, PageLimit: limit,
	})
	return entriesFrom(rows, err, "list home entries")
}

// ListLocal implements timeline.Repository.
func (r *TimelineRepository) ListLocal(ctx context.Context, lq apptimeline.LocalQuery) ([]apptimeline.Entry, error) {
	all, langs := languages(lq.Languages)
	limit, err := toInt32(lq.Limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListLocalEntries(ctx, postgresgen.ListLocalEntriesParams{
		IncludeLocal: lq.IncludeLocal, Before: toNullString(lq.Before), WindowFloor: toNullString(lq.Floor), Now: lq.Now, Viewer: toNullString(lq.ViewerAccountID),
		AllLanguages: all, Languages: langs, PageLimit: limit,
	})
	return entriesFrom(rows, err, "list local entries")
}

// ListPublic implements timeline.Repository.
func (r *TimelineRepository) ListPublic(ctx context.Context, pq apptimeline.PublicQuery) ([]apptimeline.Entry, error) {
	all, langs := languages(pq.Languages)
	limit, err := toInt32(pq.Limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListPublicEntries(ctx, postgresgen.ListPublicEntriesParams{
		Before: toNullString(pq.Before), WindowFloor: toNullString(pq.Floor), Now: pq.Now, Viewer: toNullString(pq.ViewerAccountID),
		AllLanguages: all, Languages: langs, PageLimit: limit,
	})
	return entriesFrom(rows, err, "list public entries")
}

// ListTag implements timeline.Repository.
func (r *TimelineRepository) ListTag(ctx context.Context, tq apptimeline.TagQuery) ([]apptimeline.Entry, error) {
	all, langs := languages(tq.Languages)
	limit, err := toInt32(tq.Limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListTagEntries(ctx, postgresgen.ListTagEntriesParams{
		TagID: tq.TagID, IncludeLocal: tq.IncludeLocal, Before: toNullString(tq.Before), Now: tq.Now,
		Viewer: toNullString(tq.ViewerAccountID), AllLanguages: all, Languages: langs, PageLimit: limit,
	})
	return entriesFrom(rows, err, "list tag entries")
}

// ListProfile implements timeline.Repository.
func (r *TimelineRepository) ListProfile(ctx context.Context, pq apptimeline.ProfileQuery) ([]apptimeline.Entry, error) {
	all, langs := languages(pq.Languages)
	limit, err := toInt32(pq.Limit, "page limit")
	if err != nil {
		return nil, err
	}
	has := func(v appcontent.Visibility) bool { return slices.Contains(pq.Visibilities, v) }
	rows, err := r.db.reader().ListProfileEntries(ctx, postgresgen.ListProfileEntriesParams{
		Author: pq.AuthorAccountID, Before: toNullString(pq.Before),
		IncludePublic: has(appcontent.VisibilityPublic), IncludeLocal: has(appcontent.VisibilityLocal),
		IncludeUnlisted: has(appcontent.VisibilityUnlisted), IncludePrivate: has(appcontent.VisibilityPrivate),
		IncludeDirect: pq.IncludeDirect, Viewer: toNullString(pq.ViewerAccountID), Now: pq.Now,
		ExcludeReplies: pq.ExcludeReplies, ExcludeBoosts: pq.ExcludeBoosts,
		AllLanguages: all, Languages: langs, PageLimit: limit,
	})
	return entriesFrom(rows, err, "list profile entries")
}

// Facts implements timeline.Repository.
func (r *TimelineRepository) Facts(ctx context.Context, fq apptimeline.FactsQuery) (apptimeline.Facts, error) {
	q := r.db.reader()
	f := apptimeline.Facts{
		Followed: map[string]bool{}, Exclusions: map[string][]timeline.ExclusionReason{},
		AccountRestrictions: map[string][]timeline.Restriction{}, DomainRestrictions: map[string][]timeline.Restriction{},
		Direct: map[string]bool{},
	}
	accounts := textArray(fq.AccountIDs)
	now := fq.Now
	if fq.ViewerAccountID != "" {
		followed, err := q.ListFeedFollowedAmong(ctx, postgresgen.ListFeedFollowedAmongParams{Viewer: fq.ViewerAccountID, AccountIds: accounts})
		if err != nil {
			return f, wrap(err, "list followed accounts")
		}
		for _, id := range followed {
			f.Followed[id] = true
		}
		exclusions, err := q.ListFeedExclusionsAmong(ctx, postgresgen.ListFeedExclusionsAmongParams{
			Viewer: fq.ViewerAccountID, Now: now, AccountIds: accounts,
		})
		if err != nil {
			return f, wrap(err, "list exclusions")
		}
		for _, x := range exclusions {
			f.Exclusions[x.TargetAccountID] = append(f.Exclusions[x.TargetAccountID], timeline.ExclusionReason(x.Reason))
		}
		entries := textArray(fq.EntryIDs)
		direct, err := q.ListFeedDirectEntriesAmong(ctx, postgresgen.ListFeedDirectEntriesAmongParams{Viewer: fq.ViewerAccountID, EntryIds: entries})
		if err != nil {
			return f, wrap(err, "list direct entries")
		}
		for _, id := range direct {
			f.Direct[id] = true
		}
	}
	restrictions, err := q.ListFeedAuthorRestrictionsAmong(ctx, postgresgen.ListFeedAuthorRestrictionsAmongParams{Now: now, AccountIds: accounts})
	if err != nil {
		return f, wrap(err, "list author restrictions")
	}
	for _, ar := range restrictions {
		f.AccountRestrictions[ar.AccountID] = append(f.AccountRestrictions[ar.AccountID], timeline.Restriction(ar.Restriction))
	}
	domains := textArray(fq.Domains)
	domainRestrictions, err := q.ListFeedDomainRestrictionsAmong(ctx, postgresgen.ListFeedDomainRestrictionsAmongParams{Now: now, Domains: domains})
	if err != nil {
		return f, wrap(err, "list domain restrictions")
	}
	for _, dr := range domainRestrictions {
		f.DomainRestrictions[dr.Domain] = append(f.DomainRestrictions[dr.Domain], timeline.Restriction(dr.Restriction))
	}
	return f, nil
}

// Snapshot implements timeline.Repository.
func (r *TimelineRepository) Snapshot(ctx context.Context) (apptimeline.Projection, error) {
	q := r.db.reader()
	var p apptimeline.Projection
	entries, err := q.ListAllFeedEntries(ctx)
	if p.Entries, err = entriesFrom(entries, err, "list feed entries"); err != nil {
		return p, err
	}
	recipients, err := q.ListAllFeedDirectRecipients(ctx)
	if err != nil {
		return p, wrap(err, "list feed direct recipients")
	}
	for _, row := range recipients {
		p.DirectRecipients = append(p.DirectRecipients, apptimeline.DirectRecipient(row))
	}
	tags, err := q.ListAllFeedTagEntries(ctx)
	if err != nil {
		return p, wrap(err, "list feed tag entries")
	}
	for _, row := range tags {
		p.TagEntries = append(p.TagEntries, apptimeline.TagEntry{
			TagID: row.TagID, EntryID: row.EntryID, AuthorAccountID: row.AuthorAccountID,
			Visibility: appcontent.Visibility(row.Visibility), IsLocal: row.IsLocal, RevisedAt: utc(row.RevisedAt),
		})
	}
	usage, err := q.ListAllTagUsage(ctx)
	if err != nil {
		return p, wrap(err, "list tag usage")
	}
	p.TagUsage = tagUsageFrom(usage)
	edges, err := q.ListAllFeedFollowEdges(ctx)
	if err != nil {
		return p, wrap(err, "list feed follow edges")
	}
	for _, row := range edges {
		p.FollowEdges = append(p.FollowEdges, apptimeline.FollowEdge{
			FollowerAccountID: row.FollowerAccountID, FolloweeAccountID: row.FolloweeAccountID, ShowBoosts: row.ShowBoosts,
		})
	}
	exclusions, err := q.ListAllFeedExclusions(ctx)
	if err != nil {
		return p, wrap(err, "list feed exclusions")
	}
	for _, row := range exclusions {
		p.Exclusions = append(p.Exclusions, apptimeline.Exclusion{
			ViewerAccountID: row.ViewerAccountID, TargetAccountID: row.TargetAccountID,
			Reason: timeline.ExclusionReason(row.Reason), ExpiresAt: utcPtr(row.ExpiresAt),
		})
	}
	domainMutes, err := q.ListAllFeedDomainMutes(ctx)
	if err != nil {
		return p, wrap(err, "list feed domain mutes")
	}
	for _, row := range domainMutes {
		p.DomainMutes = append(p.DomainMutes, apptimeline.DomainMute{ViewerAccountID: row.ViewerAccountID, Domain: row.Domain})
	}
	return p, nil
}

func entryFrom(row postgresgen.FeedEntry) apptimeline.Entry {
	return apptimeline.Entry{
		ID: row.ID, Kind: timeline.Kind(row.Kind), StatusID: row.StatusID,
		AuthorAccountID: row.AuthorAccountID, AuthorDomain: row.AuthorDomain,
		OriginalAuthorAccountID: row.OriginalAuthorAccountID, OriginalAuthorDomain: row.OriginalAuthorDomain,
		Visibility: appcontent.Visibility(row.Visibility), IsLocal: row.IsLocal,
		Language: row.Language, RevisionSeq: int(row.RevisionSeq), IsReply: row.IsReply,
	}
}

func entriesFrom(rows []postgresgen.FeedEntry, err error, what string) ([]apptimeline.Entry, error) {
	if err != nil {
		return nil, wrap(err, what)
	}
	out := make([]apptimeline.Entry, len(rows))
	for i, row := range rows {
		out[i] = entryFrom(row)
	}
	return out, nil
}

// languages returns the arguments of a language filter: all is true when langs keeps every
// language.
func languages(langs []string) (all bool, list []string) {
	return len(langs) == 0, textArray(langs)
}

// textArray returns s as a text[] argument, never NULL.
func textArray(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// TagUsageOf implements timeline.Repository.
func (r *TimelineRepository) TagUsageOf(ctx context.Context, tagIDs []string) ([]apptimeline.TagUsage, error) {
	if len(tagIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.reader().ListTagUsageOf(ctx, tagIDs)
	if err != nil {
		return nil, wrap(err, "list tag usage")
	}
	return tagUsageFrom(rows), nil
}

func tagUsageFrom(rows []postgresgen.TagUsage) []apptimeline.TagUsage {
	var out []apptimeline.TagUsage
	for _, row := range rows {
		out = append(out, apptimeline.TagUsage{TagID: row.TagID, StatusesCount: row.StatusesCount, LastUsedAt: utc(row.LastUsedAt)})
	}
	return out
}
