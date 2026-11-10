// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/timelinegen"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// FactsOfViewers implements timeline.Repository.
func (r *Repository) FactsOfViewers(ctx context.Context, viewers []string, fq apptimeline.FactsQuery) (map[string]apptimeline.Facts, error) {
	fq.ViewerAccountID = ""
	shared, err := r.Facts(ctx, fq)
	if err != nil {
		return nil, err
	}
	out := make(map[string]apptimeline.Facts, len(viewers))
	for _, v := range viewers {
		out[v] = apptimeline.Facts{
			Followed: map[string]bool{}, Exclusions: map[string][]timeline.ExclusionReason{},
			AccountRestrictions: shared.AccountRestrictions, DomainRestrictions: shared.DomainRestrictions, Direct: map[string]bool{},
		}
	}
	if len(viewers) == 0 {
		return out, nil
	}
	q := r.db.reader()
	vs, err := sqlite.JSONList(viewers)
	if err != nil {
		return nil, err
	}
	accounts, err := sqlite.JSONList(fq.AccountIDs)
	if err != nil {
		return nil, err
	}
	entries, err := sqlite.JSONList(fq.EntryIDs)
	if err != nil {
		return nil, err
	}
	now := sqlite.ToMillis(fq.Now)
	followed, err := q.ListFeedFollowedAmongOf(ctx, timelinegen.ListFeedFollowedAmongOfParams{Viewers: vs, AccountIds: accounts})
	if err != nil {
		return nil, sqlite.Wrap(err, "list followed accounts of viewers")
	}
	for _, row := range followed {
		out[row.FollowerAccountID].Followed[row.FolloweeAccountID] = true
	}
	exclusions, err := q.ListFeedExclusionsAmongOf(ctx, timelinegen.ListFeedExclusionsAmongOfParams{Viewers: vs, Now: now, AccountIds: accounts})
	if err != nil {
		return nil, sqlite.Wrap(err, "list exclusions of viewers")
	}
	for _, x := range exclusions {
		f := out[x.ViewerAccountID]
		f.Exclusions[x.TargetAccountID] = append(f.Exclusions[x.TargetAccountID], timeline.ExclusionReason(x.Reason))
	}
	direct, err := q.ListFeedDirectEntriesAmongOf(ctx, timelinegen.ListFeedDirectEntriesAmongOfParams{Viewers: vs, EntryIds: entries})
	if err != nil {
		return nil, sqlite.Wrap(err, "list direct entries of viewers")
	}
	for _, row := range direct {
		out[row.RecipientAccountID].Direct[row.EntryID] = true
	}
	return out, nil
}

// MutedDomainsOf implements timeline.Repository.
func (r *Repository) MutedDomainsOf(ctx context.Context, viewers, domains []string) (map[string][]string, error) {
	out := make(map[string][]string, len(viewers))
	if len(viewers) == 0 || len(domains) == 0 {
		return out, nil
	}
	vs, err := sqlite.JSONList(viewers)
	if err != nil {
		return nil, err
	}
	ds, err := sqlite.JSONList(domains)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListFeedDomainMutesAmongOf(ctx, timelinegen.ListFeedDomainMutesAmongOfParams{Viewers: vs, Domains: ds})
	if err != nil {
		return nil, sqlite.Wrap(err, "list domain mutes of viewers")
	}
	for _, row := range rows {
		out[row.ViewerAccountID] = append(out[row.ViewerAccountID], row.Domain)
	}
	return out, nil
}
