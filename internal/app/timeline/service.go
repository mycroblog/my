// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"errors"
	"slices"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// Page sizes of a Query.
const (
	DefaultLimit = 20
	MaxLimit     = 40
)

// ErrLoginRequired reports a timeline that only a logged-in local account has: the home timeline.
var ErrLoginRequired = errors.New("timeline: the timeline needs a logged-in account")

// Deps are the ports a Service uses. UnitOfWork and IDs serve the commands of lists.
type Deps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Clock      kernel.Clock
	IDs        kernel.IDGenerator
}

// Service reads the home, local, hashtag, and profile timelines (FR-201 to FR-204, FR-208) from
// the timeline module's own projections, fan-out-on-read (ADR-0013). The SQL narrows the
// candidates as data-model §6 maps the visibility policy; before returning them, Service applies
// appcontent.CanView again to every entry (NFR-SEC-4) and drops those it denies. It also keeps
// the viewer's lists and reads their timelines (lists.go).
type Service struct {
	d Deps
}

// NewService returns a Service on d.
func NewService(d Deps) *Service { return &Service{d: d} }

// Query selects a page of a timeline, newest first.
type Query struct {
	// Before is the Next of the previous page; "" starts at the newest entry.
	Before string
	// Limit is the page size: DefaultLimit when zero or negative, at most MaxLimit.
	Limit int
	// Languages keeps only the entries in these languages and those whose language is
	// undetermined; empty keeps every language (FR-208). Callers pass the viewer's timeline
	// languages, which the identity module keeps.
	Languages []string
}

func (q Query) limit() int {
	if q.Limit <= 0 {
		return DefaultLimit
	}
	return min(q.Limit, MaxLimit)
}

// Page is one page of a timeline.
type Page struct {
	// Entries are the entries the viewer may see, newest first. A page may hold fewer than the
	// limit even when more follow, when CanView denies some of its candidates.
	Entries []Entry
	// Next is the Before of the next page, "" when this page is the last.
	Next string
}

// Home returns the home timeline of a logged-in viewer (FR-201): the viewer's own posts and
// boosts and those of the accounts the viewer follows, except the boosts of follows that hide
// them, the direct posts not addressed to the viewer, and the entries of accounts the viewer
// blocks, is blocked by, or mutes. The query first reads only the entries within
// timeline.HomeWindow before the cursor and widens the window when that does not fill the page.
func (s *Service) Home(ctx context.Context, viewer appcontent.Viewer, q Query) (Page, error) {
	if viewer.Kind != appcontent.ViewerLocal || viewer.AccountID == "" {
		return Page{}, ErrLoginRequired
	}
	w := s.window(viewer, q)
	base := w.Now
	if t, ok := kernel.UUIDv7Time(q.Before); ok && t.Before(base) {
		base = t
	}
	windows := timeline.HomeWindows()
	var rows []Entry
	for i := 0; ; i++ {
		hq := HomeQuery{Window: w}
		if i < len(windows) {
			hq.Floor = kernel.UUIDv7Floor(base.Add(-windows[i]))
		}
		var err error
		if rows, err = s.d.Repository.ListHome(ctx, hq); err != nil {
			return Page{}, err
		}
		if len(rows) >= w.Limit || hq.Floor == "" {
			break
		}
	}
	return s.page(ctx, appcontent.ContextHome, viewer, w, rows)
}

// Local returns the local timeline (FR-202): the public posts of local accounts, and their
// local posts for a logged-in viewer (ADR-0017), except those of authors kept from public
// discovery and of accounts the viewer blocks, is blocked by, or mutes.
func (s *Service) Local(ctx context.Context, viewer appcontent.Viewer, q Query) (Page, error) {
	w := s.window(viewer, q)
	rows, err := s.d.Repository.ListLocal(ctx, LocalQuery{Window: w, IncludeLocal: loggedIn(viewer)})
	if err != nil {
		return Page{}, err
	}
	return s.page(ctx, appcontent.ContextLocalTimeline, viewer, w, rows)
}

// Public returns the federated timeline (FR-202): the public posts of local and remote accounts
// that the instance holds, except those of authors and domains kept from public discovery and of
// accounts the viewer blocks, is blocked by, or mutes. Posts of visibility local never appear on it
// (ADR-0017).
func (s *Service) Public(ctx context.Context, viewer appcontent.Viewer, q Query) (Page, error) {
	w := s.window(viewer, q)
	rows, err := s.d.Repository.ListPublic(ctx, PublicQuery{Window: w})
	if err != nil {
		return Page{}, err
	}
	return s.page(ctx, appcontent.ContextFederatedTimeline, viewer, w, rows)
}

// Hashtag returns the timeline of the hashtag tagID (FR-203): the public posts with the tag, and
// its local posts for a logged-in viewer, with the exclusions of Local.
func (s *Service) Hashtag(ctx context.Context, viewer appcontent.Viewer, tagID string, q Query) (Page, error) {
	w := s.window(viewer, q)
	rows, err := s.d.Repository.ListTag(ctx, TagQuery{Window: w, TagID: tagID, IncludeLocal: loggedIn(viewer)})
	if err != nil {
		return Page{}, err
	}
	return s.page(ctx, appcontent.ContextHashtag, viewer, w, rows)
}

// ProfileFilter narrows a profile timeline.
type ProfileFilter struct {
	// ExcludeReplies leaves out the author's posts that reply to a status.
	ExcludeReplies bool
	// ExcludeBoosts leaves out the author's boosts.
	ExcludeBoosts bool
}

// Profile returns the profile timeline of an author (FR-204): the author's posts and boosts in
// the visibilities that the viewer's relation to the author and the author's restrictions
// allow, and the author's direct posts addressed to the viewer, narrowed by f.
func (s *Service) Profile(ctx context.Context, viewer appcontent.Viewer, authorAccountID string, f ProfileFilter, q Query) (Page, error) {
	w := s.window(viewer, q)
	facts, err := s.d.Repository.Facts(ctx, FactsQuery{
		ViewerAccountID: w.ViewerAccountID, AccountIDs: []string{authorAccountID}, Now: w.Now,
	})
	if err != nil {
		return Page{}, err
	}
	pq := ProfileQuery{Window: w, AuthorAccountID: authorAccountID, ExcludeReplies: f.ExcludeReplies, ExcludeBoosts: f.ExcludeBoosts}
	// The visibilities CanView allows on the profile, judged as for a local author: a remote
	// author has no local posts, and CanView below judges each entry by its real facts.
	restrictions := facts.restrictions(authorAccountID, "")
	rel := facts.relation(authorAccountID, "")
	for _, v := range []appcontent.Visibility{
		appcontent.VisibilityPublic, appcontent.VisibilityLocal, appcontent.VisibilityUnlisted, appcontent.VisibilityPrivate,
	} {
		st := appcontent.StatusFacts{AuthorAccountID: authorAccountID, AuthorIsLocal: true, Visibility: v}
		if appcontent.CanView(appcontent.ContextProfile, viewer, st, restrictions, rel) {
			pq.Visibilities = append(pq.Visibilities, v)
		}
	}
	direct := appcontent.StatusFacts{AuthorAccountID: authorAccountID, AuthorIsLocal: true, Visibility: appcontent.VisibilityDirect}
	mentioned := rel
	mentioned.Mentioned = true
	pq.IncludeDirect = appcontent.CanView(appcontent.ContextProfile, viewer, direct, restrictions, mentioned)
	if len(pq.Visibilities) == 0 && !pq.IncludeDirect {
		return Page{}, nil
	}
	rows, err := s.d.Repository.ListProfile(ctx, pq)
	if err != nil {
		return Page{}, err
	}
	return s.page(ctx, appcontent.ContextProfile, viewer, w, rows)
}

// window reads one more entry than the page holds, to learn whether a next page exists.
func (s *Service) window(viewer appcontent.Viewer, q Query) Window {
	w := Window{Before: q.Before, Limit: q.limit() + 1, Now: s.d.Clock.Now()}
	if loggedIn(viewer) {
		w.ViewerAccountID = viewer.AccountID
	}
	if len(q.Languages) > 0 {
		w.Languages = q.Languages
	}
	return w
}

func loggedIn(v appcontent.Viewer) bool { return v.Kind == appcontent.ViewerLocal && v.AccountID != "" }

// page cuts rows to the page and keeps the entries that CanView lets the viewer see in vctx.
func (s *Service) page(ctx context.Context, vctx appcontent.ViewContext, viewer appcontent.Viewer, w Window, rows []Entry) (Page, error) {
	var p Page
	if len(rows) >= w.Limit {
		rows = rows[:w.Limit-1]
		p.Next = rows[len(rows)-1].ID
	}
	if len(rows) == 0 {
		return p, nil
	}
	fq := FactsQuery{ViewerAccountID: w.ViewerAccountID, Now: w.Now}
	for _, e := range rows {
		fq.AccountIDs = append(fq.AccountIDs, e.AuthorAccountID, e.OriginalAuthorAccountID)
		fq.Domains = append(fq.Domains, timeline.DomainAndParents(e.AuthorDomain)...)
		fq.Domains = append(fq.Domains, timeline.DomainAndParents(e.OriginalAuthorDomain)...)
		if e.Visibility == appcontent.VisibilityDirect {
			fq.EntryIDs = append(fq.EntryIDs, e.ID)
		}
	}
	fq.AccountIDs, fq.Domains = uniq(fq.AccountIDs), uniq(fq.Domains)
	facts, err := s.d.Repository.Facts(ctx, fq)
	if err != nil {
		return Page{}, err
	}
	for _, e := range rows {
		if facts.canView(vctx, viewer, e) {
			p.Entries = append(p.Entries, e)
		}
	}
	return p, nil
}

// canView applies appcontent.CanView to an entry. A post is judged as itself; a boost is judged
// as a status of the booster in vctx, and its status on its own as a single status.
func (f Facts) canView(vctx appcontent.ViewContext, viewer appcontent.Viewer, e Entry) bool {
	if viewer.Kind == appcontent.ViewerLocal && viewer.AccountID != "" {
		for _, id := range []string{e.AuthorAccountID, e.OriginalAuthorAccountID} {
			// Mutes are not part of the policy, but a timeline never shows a muted account.
			if id != viewer.AccountID && slices.Contains(f.Exclusions[id], timeline.ExcludedMute) {
				return false
			}
		}
	}
	shown := appcontent.StatusFacts{AuthorAccountID: e.AuthorAccountID, AuthorIsLocal: e.AuthorDomain == "", Visibility: e.Visibility}
	if !appcontent.CanView(vctx, viewer, shown, f.restrictions(e.AuthorAccountID, e.AuthorDomain), f.relation(e.AuthorAccountID, e.ID)) {
		return false
	}
	if e.Kind != timeline.KindBoost {
		return true
	}
	original := appcontent.StatusFacts{
		AuthorAccountID: e.OriginalAuthorAccountID, AuthorIsLocal: e.OriginalAuthorDomain == "", Visibility: e.Visibility,
	}
	return appcontent.CanView(appcontent.ContextStatus, viewer, original,
		f.restrictions(e.OriginalAuthorAccountID, e.OriginalAuthorDomain), f.relation(e.OriginalAuthorAccountID, ""))
}

// restrictions returns the restrictions of an account on domain: its own and those of the domain
// and its parents.
func (f Facts) restrictions(accountID, domain string) appcontent.AuthorRestrictions {
	rs := slices.Clone(f.AccountRestrictions[accountID])
	for _, d := range timeline.DomainAndParents(domain) {
		rs = append(rs, f.DomainRestrictions[d]...)
	}
	return appcontent.AuthorRestrictions{
		ProfileView:              slices.Contains(rs, timeline.RestrictProfileView),
		VisibilityCeilingPrivate: slices.Contains(rs, timeline.RestrictVisibilityCeilingPrivate),
		PublicDiscovery:          slices.Contains(rs, timeline.RestrictPublicDiscovery),
	}
}

// relation returns the viewer's relation to an account, and whether the direct entry entryID
// ("" for none) is addressed to the viewer.
func (f Facts) relation(accountID, entryID string) appcontent.Relation {
	rel := appcontent.Relation{
		Blocking:  slices.Contains(f.Exclusions[accountID], timeline.ExcludedBlock),
		BlockedBy: slices.Contains(f.Exclusions[accountID], timeline.ExcludedBlockedBy),
		Mentioned: entryID != "" && f.Direct[entryID],
	}
	if f.Followed[accountID] {
		rel.Follow = appcontent.FollowActive
	}
	return rel
}

func uniq(s []string) []string {
	slices.Sort(s)
	return slices.Compact(s)
}
