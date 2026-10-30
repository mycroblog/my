// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// TimelineRepository implements timeline.Repository on SQLite.
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
	n, err := q.InsertFeedEntry(ctx, sqlitegen.InsertFeedEntryParams{
		ID: e.ID, Kind: string(e.Kind), StatusID: e.StatusID,
		AuthorAccountID: e.AuthorAccountID, AuthorDomain: e.AuthorDomain,
		OriginalAuthorAccountID: e.OriginalAuthorAccountID, OriginalAuthorDomain: e.OriginalAuthorDomain,
		Visibility: string(e.Visibility), IsLocal: boolToInt(e.IsLocal), Language: e.Language, RevisionSeq: int64(e.RevisionSeq),
		IsReply: boolToInt(e.IsReply),
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
	_, err = q.UpdateFeedEntriesRevision(ctx, sqlitegen.UpdateFeedEntriesRevisionParams{
		Language: language, RevisionSeq: int64(seq), StatusID: statusID,
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

func deleteEntryChildren(ctx context.Context, q *sqlitegen.Queries, entryID string) error {
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
func applyTagUsage(ctx context.Context, q *sqlitegen.Queries, deltas map[string]int64) error {
	for _, id := range slices.Sorted(maps.Keys(deltas)) {
		d := deltas[id]
		var err error
		switch {
		case d > 0:
			err = q.AddTagUsage(ctx, sqlitegen.AddTagUsageParams{TagID: id, N: d})
		case d < 0:
			if err = q.DeleteDrainedTagUsage(ctx, sqlitegen.DeleteDrainedTagUsageParams{TagID: id, N: -d}); err == nil {
				err = q.AdjustTagUsage(ctx, sqlitegen.AdjustTagUsageParams{TagID: id, Delta: d})
			}
		default:
			err = q.AdjustTagUsage(ctx, sqlitegen.AdjustTagUsageParams{TagID: id})
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
		if err := q.InsertFeedDirectRecipient(ctx, sqlitegen.InsertFeedDirectRecipientParams{EntryID: entryID, RecipientAccountID: id}); err != nil {
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
		err := q.InsertFeedTagEntry(ctx, sqlitegen.InsertFeedTagEntryParams{
			TagID: id, EntryID: e.ID, AuthorAccountID: e.AuthorAccountID, Visibility: string(e.Visibility), IsLocal: boolToInt(e.IsLocal),
			RevisedAt: toMillis(revisedAt),
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
	return wrap(q.UpsertFeedFollowEdge(ctx, sqlitegen.UpsertFeedFollowEdgeParams{
		FollowerAccountID: e.FollowerAccountID, FolloweeAccountID: e.FolloweeAccountID, ShowBoosts: boolToInt(e.ShowBoosts),
	}), "upsert feed follow edge")
}

// DeleteFollowEdge implements timeline.Repository.
func (r *TimelineRepository) DeleteFollowEdge(ctx context.Context, tx uow.Tx, followerAccountID, followeeAccountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteFeedFollowEdge(ctx, sqlitegen.DeleteFeedFollowEdgeParams{
		FollowerAccountID: followerAccountID, FolloweeAccountID: followeeAccountID,
	}), "delete feed follow edge")
}

// UpsertExclusion implements timeline.Repository.
func (r *TimelineRepository) UpsertExclusion(ctx context.Context, tx uow.Tx, x apptimeline.Exclusion) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertFeedExclusion(ctx, sqlitegen.UpsertFeedExclusionParams{
		ViewerAccountID: x.ViewerAccountID, TargetAccountID: x.TargetAccountID, Reason: string(x.Reason), ExpiresAt: toNullMillis(x.ExpiresAt),
	}), "upsert feed exclusion")
}

// DeleteExclusion implements timeline.Repository.
func (r *TimelineRepository) DeleteExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, targetAccountID string, reason timeline.ExclusionReason) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteFeedExclusion(ctx, sqlitegen.DeleteFeedExclusionParams{
		ViewerAccountID: viewerAccountID, TargetAccountID: targetAccountID, Reason: string(reason),
	}), "delete feed exclusion")
}

// UpsertAuthorRestriction implements timeline.Repository.
func (r *TimelineRepository) UpsertAuthorRestriction(ctx context.Context, tx uow.Tx, ar apptimeline.AuthorRestriction) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertFeedAuthorRestriction(ctx, sqlitegen.UpsertFeedAuthorRestrictionParams{
		AccountID: ar.AccountID, Restriction: string(ar.Restriction), EffectiveUntil: toNullMillis(ar.EffectiveUntil),
	}), "upsert feed author restriction")
}

// DeleteAuthorRestriction implements timeline.Repository.
func (r *TimelineRepository) DeleteAuthorRestriction(ctx context.Context, tx uow.Tx, accountID string, restriction timeline.Restriction) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteFeedAuthorRestriction(ctx, sqlitegen.DeleteFeedAuthorRestrictionParams{
		AccountID: accountID, Restriction: string(restriction),
	}), "delete feed author restriction")
}

// UpsertDomainRestriction implements timeline.Repository.
func (r *TimelineRepository) UpsertDomainRestriction(ctx context.Context, tx uow.Tx, dr apptimeline.DomainRestriction) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertFeedDomainRestriction(ctx, sqlitegen.UpsertFeedDomainRestrictionParams{
		Domain: dr.Domain, Restriction: string(dr.Restriction), EffectiveUntil: toNullMillis(dr.EffectiveUntil),
	}), "upsert feed domain restriction")
}

// DeleteDomainRestriction implements timeline.Repository.
func (r *TimelineRepository) DeleteDomainRestriction(ctx context.Context, tx uow.Tx, domain string, restriction timeline.Restriction) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteFeedDomainRestriction(ctx, sqlitegen.DeleteFeedDomainRestrictionParams{
		Domain: domain, Restriction: string(restriction),
	}), "delete feed domain restriction")
}

// InsertDomainMute implements timeline.Repository.
func (r *TimelineRepository) InsertDomainMute(ctx context.Context, tx uow.Tx, m apptimeline.DomainMute) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertFeedDomainMute(ctx, sqlitegen.InsertFeedDomainMuteParams{
		ViewerAccountID: m.ViewerAccountID, Domain: m.Domain,
	}), "insert feed domain mute")
}

// DeleteDomainMute implements timeline.Repository.
func (r *TimelineRepository) DeleteDomainMute(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteFeedDomainMute(ctx, sqlitegen.DeleteFeedDomainMuteParams{
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
	all, langs, err := languages(hq.Languages)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListHomeEntries(ctx, sqlitegen.ListHomeEntriesParams{
		Viewer: hq.ViewerAccountID, Before: hq.Before, WindowFloor: hq.Floor, Now: toMillis(hq.Now),
		AllLanguages: all, Languages: langs, PageLimit: int64(hq.Limit),
	})
	return entriesFrom(rows, err, "list home entries")
}

// ListLocal implements timeline.Repository.
func (r *TimelineRepository) ListLocal(ctx context.Context, lq apptimeline.LocalQuery) ([]apptimeline.Entry, error) {
	all, langs, err := languages(lq.Languages)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListLocalEntries(ctx, sqlitegen.ListLocalEntriesParams{
		IncludeLocal: boolToInt(lq.IncludeLocal), Before: lq.Before, WindowFloor: lq.Floor, Now: toMillis(lq.Now), Viewer: lq.ViewerAccountID,
		AllLanguages: all, Languages: langs, PageLimit: int64(lq.Limit),
	})
	return entriesFrom(rows, err, "list local entries")
}

// ListPublic implements timeline.Repository.
func (r *TimelineRepository) ListPublic(ctx context.Context, pq apptimeline.PublicQuery) ([]apptimeline.Entry, error) {
	all, langs, err := languages(pq.Languages)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListPublicEntries(ctx, sqlitegen.ListPublicEntriesParams{
		Before: pq.Before, WindowFloor: pq.Floor, Now: toMillis(pq.Now), Viewer: pq.ViewerAccountID,
		AllLanguages: all, Languages: langs, PageLimit: int64(pq.Limit),
	})
	return entriesFrom(rows, err, "list public entries")
}

// ListTag implements timeline.Repository.
func (r *TimelineRepository) ListTag(ctx context.Context, tq apptimeline.TagQuery) ([]apptimeline.Entry, error) {
	all, langs, err := languages(tq.Languages)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListTagEntries(ctx, sqlitegen.ListTagEntriesParams{
		TagID: tq.TagID, IncludeLocal: boolToInt(tq.IncludeLocal), Before: tq.Before, Now: toMillis(tq.Now),
		Viewer: tq.ViewerAccountID, AllLanguages: all, Languages: langs, PageLimit: int64(tq.Limit),
	})
	return entriesFrom(rows, err, "list tag entries")
}

// ListProfile implements timeline.Repository.
func (r *TimelineRepository) ListProfile(ctx context.Context, pq apptimeline.ProfileQuery) ([]apptimeline.Entry, error) {
	all, langs, err := languages(pq.Languages)
	if err != nil {
		return nil, err
	}
	has := func(v appcontent.Visibility) int64 {
		for _, w := range pq.Visibilities {
			if w == v {
				return 1
			}
		}
		return 0
	}
	rows, err := r.db.reader().ListProfileEntries(ctx, sqlitegen.ListProfileEntriesParams{
		Author: pq.AuthorAccountID, Before: pq.Before,
		IncludePublic: has(appcontent.VisibilityPublic), IncludeLocal: has(appcontent.VisibilityLocal),
		IncludeUnlisted: has(appcontent.VisibilityUnlisted), IncludePrivate: has(appcontent.VisibilityPrivate),
		IncludeDirect: boolToInt(pq.IncludeDirect), Viewer: pq.ViewerAccountID, Now: toMillis(pq.Now),
		ExcludeReplies: boolToInt(pq.ExcludeReplies), ExcludeBoosts: boolToInt(pq.ExcludeBoosts),
		AllLanguages: all, Languages: langs, PageLimit: int64(pq.Limit),
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
	accounts, err := jsonList(fq.AccountIDs)
	if err != nil {
		return f, err
	}
	now := toMillis(fq.Now)
	if fq.ViewerAccountID != "" {
		followed, err := q.ListFeedFollowedAmong(ctx, sqlitegen.ListFeedFollowedAmongParams{Viewer: fq.ViewerAccountID, AccountIds: accounts})
		if err != nil {
			return f, wrap(err, "list followed accounts")
		}
		for _, id := range followed {
			f.Followed[id] = true
		}
		exclusions, err := q.ListFeedExclusionsAmong(ctx, sqlitegen.ListFeedExclusionsAmongParams{
			Viewer: fq.ViewerAccountID, Now: now, AccountIds: accounts,
		})
		if err != nil {
			return f, wrap(err, "list exclusions")
		}
		for _, x := range exclusions {
			f.Exclusions[x.TargetAccountID] = append(f.Exclusions[x.TargetAccountID], timeline.ExclusionReason(x.Reason))
		}
		entries, err := jsonList(fq.EntryIDs)
		if err != nil {
			return f, err
		}
		direct, err := q.ListFeedDirectEntriesAmong(ctx, sqlitegen.ListFeedDirectEntriesAmongParams{Viewer: fq.ViewerAccountID, EntryIds: entries})
		if err != nil {
			return f, wrap(err, "list direct entries")
		}
		for _, id := range direct {
			f.Direct[id] = true
		}
	}
	restrictions, err := q.ListFeedAuthorRestrictionsAmong(ctx, sqlitegen.ListFeedAuthorRestrictionsAmongParams{Now: now, AccountIds: accounts})
	if err != nil {
		return f, wrap(err, "list author restrictions")
	}
	for _, ar := range restrictions {
		f.AccountRestrictions[ar.AccountID] = append(f.AccountRestrictions[ar.AccountID], timeline.Restriction(ar.Restriction))
	}
	domains, err := jsonList(fq.Domains)
	if err != nil {
		return f, err
	}
	domainRestrictions, err := q.ListFeedDomainRestrictionsAmong(ctx, sqlitegen.ListFeedDomainRestrictionsAmongParams{Now: now, Domains: domains})
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
			Visibility: appcontent.Visibility(row.Visibility), IsLocal: row.IsLocal == 1, RevisedAt: fromMillis(row.RevisedAt),
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
			FollowerAccountID: row.FollowerAccountID, FolloweeAccountID: row.FolloweeAccountID, ShowBoosts: row.ShowBoosts == 1,
		})
	}
	exclusions, err := q.ListAllFeedExclusions(ctx)
	if err != nil {
		return p, wrap(err, "list feed exclusions")
	}
	for _, row := range exclusions {
		p.Exclusions = append(p.Exclusions, apptimeline.Exclusion{
			ViewerAccountID: row.ViewerAccountID, TargetAccountID: row.TargetAccountID,
			Reason: timeline.ExclusionReason(row.Reason), ExpiresAt: fromNullMillis(row.ExpiresAt),
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

func entryFrom(row sqlitegen.FeedEntry) apptimeline.Entry {
	return apptimeline.Entry{
		ID: row.ID, Kind: timeline.Kind(row.Kind), StatusID: row.StatusID,
		AuthorAccountID: row.AuthorAccountID, AuthorDomain: row.AuthorDomain,
		OriginalAuthorAccountID: row.OriginalAuthorAccountID, OriginalAuthorDomain: row.OriginalAuthorDomain,
		Visibility: appcontent.Visibility(row.Visibility), IsLocal: row.IsLocal == 1,
		Language: row.Language, RevisionSeq: int(row.RevisionSeq), IsReply: row.IsReply == 1,
	}
}

func entriesFrom(rows []sqlitegen.FeedEntry, err error, what string) ([]apptimeline.Entry, error) {
	if err != nil {
		return nil, wrap(err, what)
	}
	out := make([]apptimeline.Entry, len(rows))
	for i, row := range rows {
		out[i] = entryFrom(row)
	}
	return out, nil
}

// languages returns the arguments of a language filter: all is 1 when langs keeps every language.
func languages(langs []string) (all int64, list string, err error) {
	list, err = jsonList(langs)
	if len(langs) == 0 {
		all = 1
	}
	return all, list, err
}

// jsonList encodes a list argument as the JSON array that the queries read with json_each.
func jsonList(s []string) (string, error) {
	if s == nil {
		s = []string{}
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", wrap(err, "encode a list argument")
	}
	return string(b), nil
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

func tagUsageFrom(rows []sqlitegen.TagUsage) []apptimeline.TagUsage {
	var out []apptimeline.TagUsage
	for _, row := range rows {
		out = append(out, apptimeline.TagUsage{TagID: row.TagID, StatusesCount: row.StatusesCount, LastUsedAt: fromMillis(row.LastUsedAt)})
	}
	return out
}
