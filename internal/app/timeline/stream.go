// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"slices"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// RegisterCommitHooks signals every new home timeline entry to pub once the command that adds
// it commits (FR-207): a status as posted (content.status_created) and a boost
// (content.boosted). The signal carries the entry ID and the account of the entry, the author or
// the booster; the streams ask HomeAudience which of their viewers' home timelines may list it,
// and HomeEntry whether each of those lists it. A status that the public timelines may list,
// local or remote, also signals a public timeline entry (ADR-0013 §20), whose streams ask
// LocalEntry or PublicEntry. A direct post signals its conversation to each of its parties,
// whose streams ask StreamConversation.
func RegisterCommitHooks(r *uow.Projectors, pub stream.Publisher) {
	r.OnCommit(appcontent.EventStatusCreated, func(_ context.Context, e uow.Event) {
		st := e.(appcontent.StatusCreated).Status
		pub.Publish(stream.Event{Kind: stream.KindHomeEntry, EntryID: st.ID, AuthorAccountID: st.AuthorAccountID})
		if onLocal, onFederated := publicTimelines(st.IsLocal, st.Visibility); onLocal || onFederated {
			pub.Publish(stream.Event{Kind: stream.KindPublicEntry, EntryID: st.ID, OnLocal: onLocal, OnFederated: onFederated})
		}
		if ev := e.(appcontent.StatusCreated); st.Visibility == appcontent.VisibilityDirect && ev.ConversationID != "" {
			// One signal for each party, whose streams read the conversation again (FR-212).
			for _, id := range directParties(st.AuthorAccountID, ev.Revision.MentionedAccountIDs) {
				pub.Publish(stream.Event{Kind: stream.KindConversation, AccountID: id, ConversationID: ev.ConversationID})
			}
		}
	})
	r.OnCommit(appcontent.EventBoosted, func(_ context.Context, e uow.Event) {
		b := e.(appcontent.Boosted).Boost
		pub.Publish(stream.Event{Kind: stream.KindHomeEntry, EntryID: b.ID, AuthorAccountID: b.BoosterAccountID})
	})
}

// publicTimelines returns whether the local and the federated timelines may list a post of a
// local or remote author with visibility v, as ListLocal and ListPublic select their
// candidates: the local timeline lists the public and local posts of local accounts, the
// federated one the public posts of every account (ADR-0017). Neither lists boosts.
func publicTimelines(isLocal bool, v appcontent.Visibility) (onLocal, onFederated bool) {
	onLocal = isLocal && (v == appcontent.VisibilityPublic || v == appcontent.VisibilityLocal)
	return onLocal, v == appcontent.VisibilityPublic
}

// HomeEntry returns the entry id when the home timeline of a logged-in viewer lists it, as Home
// would, in the timeline languages q.Languages (q.Before and q.Limit are ignored); false when it
// does not, because the viewer does not follow its author, may not see it, or it is in another
// language. It reads the entries from id up, so it is meant for an entry that has just been
// added, as the streaming endpoint asks.
func (s *Service) HomeEntry(ctx context.Context, viewer appcontent.Viewer, id string, q Query) (Entry, bool, error) {
	if viewer.Kind != appcontent.ViewerLocal || viewer.AccountID == "" {
		return Entry{}, false, ErrLoginRequired
	}
	w := s.window(viewer, Query{Languages: q.Languages, Limit: MaxLimit})
	rows, err := s.d.Repository.ListHome(ctx, HomeQuery{Window: w, Floor: id})
	if err != nil {
		return Entry{}, false, err
	}
	return s.entry(ctx, appcontent.ContextHome, viewer, w, id, rows)
}

// HomeAudience returns which of accountIDs may have in their home timelines an entry of the
// account authorID (a post by it, or a boost by it): authorID itself and the accounts whose home
// timelines follow it, with one query. HomeEntry decides for each of them.
func (s *Service) HomeAudience(ctx context.Context, authorID string, accountIDs []string) (map[string]bool, error) {
	out, err := s.d.Repository.FeedFollowersAmong(ctx, authorID, accountIDs)
	if err != nil {
		return nil, err
	}
	if slices.Contains(accountIDs, authorID) {
		out[authorID] = true
	}
	return out, nil
}

// LocalEntry returns the entry id when the local timeline of a logged-in viewer lists it, as
// Local would, in the timeline languages q.Languages (q.Before and q.Limit are ignored); false
// when it does not. It reads the entries from id up, so it is meant for an entry that has just
// been added, as the streaming endpoint asks.
func (s *Service) LocalEntry(ctx context.Context, viewer appcontent.Viewer, id string, q Query) (Entry, bool, error) {
	if !loggedIn(viewer) {
		return Entry{}, false, ErrLoginRequired
	}
	w := s.window(viewer, Query{Languages: q.Languages, Limit: MaxLimit})
	rows, err := s.d.Repository.ListLocal(ctx, LocalQuery{Window: w, IncludeLocal: true, Floor: id})
	if err != nil {
		return Entry{}, false, err
	}
	return s.entry(ctx, appcontent.ContextLocalTimeline, viewer, w, id, rows)
}

// PublicEntry returns the entry id when the federated timeline of a logged-in viewer lists it,
// as Public would, in the timeline languages q.Languages (q.Before and q.Limit are ignored);
// false when it does not. It reads the entries from id up, as LocalEntry does.
func (s *Service) PublicEntry(ctx context.Context, viewer appcontent.Viewer, id string, q Query) (Entry, bool, error) {
	if !loggedIn(viewer) {
		return Entry{}, false, ErrLoginRequired
	}
	w := s.window(viewer, Query{Languages: q.Languages, Limit: MaxLimit})
	rows, err := s.d.Repository.ListPublic(ctx, PublicQuery{Window: w, Floor: id})
	if err != nil {
		return Entry{}, false, err
	}
	return s.entry(ctx, appcontent.ContextFederatedTimeline, viewer, w, id, rows)
}

// entry returns the row id of rows when CanView lets the viewer see it in vctx.
func (s *Service) entry(ctx context.Context, vctx appcontent.ViewContext, viewer appcontent.Viewer, w Window, id string, rows []Entry) (Entry, bool, error) {
	for _, e := range rows {
		if e.ID != id {
			continue
		}
		p, err := s.page(ctx, vctx, viewer, w, []Entry{e})
		if err != nil || len(p.Entries) == 0 {
			return Entry{}, false, err
		}
		return p.Entries[0], true, nil
	}
	return Entry{}, false, nil
}

// StreamViewer is a viewer of a stream of a public timeline with its timeline languages.
type StreamViewer struct {
	Viewer    appcontent.Viewer
	Languages []string
}

// LocalEntryFor is LocalEntry of each of viewers, logged-in viewers, by viewer account, with a
// number of queries that does not grow with the viewers: the entry is read once, and the facts
// of all the viewers together.
func (s *Service) LocalEntryFor(ctx context.Context, viewers []StreamViewer, id string) (map[string]Entry, error) {
	return s.entryFor(ctx, appcontent.ContextLocalTimeline, viewers, id, false)
}

// PublicEntryFor is PublicEntry of each of viewers, as LocalEntryFor is of LocalEntry.
func (s *Service) PublicEntryFor(ctx context.Context, viewers []StreamViewer, id string) (map[string]Entry, error) {
	return s.entryFor(ctx, appcontent.ContextFederatedTimeline, viewers, id, true)
}

// entryFor decides the entry id for each of viewers as LocalEntry (public false) or PublicEntry
// (public true) decides it for one: the rows that the timeline lists for no viewer, then for each
// viewer the conditions of its own listing, which the SQL of one viewer applies (its exclusions,
// its muted domains on the federated timeline, its languages), and CanView over its facts.
func (s *Service) entryFor(ctx context.Context, vctx appcontent.ViewContext, viewers []StreamViewer, id string, public bool) (map[string]Entry, error) {
	out := make(map[string]Entry, len(viewers))
	for _, v := range viewers {
		if !loggedIn(v.Viewer) {
			return nil, ErrLoginRequired
		}
	}
	if len(viewers) == 0 {
		return out, nil
	}
	w := s.window(appcontent.Viewer{Kind: appcontent.ViewerAnonymous}, Query{Limit: MaxLimit})
	var rows []Entry
	var err error
	if public {
		rows, err = s.d.Repository.ListPublic(ctx, PublicQuery{Window: w, Floor: id})
	} else {
		rows, err = s.d.Repository.ListLocal(ctx, LocalQuery{Window: w, IncludeLocal: true, Floor: id})
	}
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(rows, func(e Entry) bool { return e.ID == id })
	if i < 0 {
		if len(rows) < w.Limit {
			return out, nil // no viewer's timeline lists it
		}
		// More entries than one read came after it: a viewer's own exclusions could bring it in,
		// so each viewer decides alone, as LocalEntry and PublicEntry do.
		for _, v := range viewers {
			lookup := s.LocalEntry
			if public {
				lookup = s.PublicEntry
			}
			e, ok, err := lookup(ctx, v.Viewer, id, Query{Languages: v.Languages})
			if err != nil {
				return nil, err
			}
			if ok {
				out[v.Viewer.AccountID] = e
			}
		}
		return out, nil
	}
	e := rows[i]
	ids := make([]string, len(viewers))
	for k, v := range viewers {
		ids[k] = v.Viewer.AccountID
	}
	fq := FactsQuery{Now: w.Now, AccountIDs: uniq([]string{e.AuthorAccountID, e.OriginalAuthorAccountID})}
	fq.Domains = uniq(append(timeline.DomainAndParents(e.AuthorDomain), timeline.DomainAndParents(e.OriginalAuthorDomain)...))
	if e.Visibility == appcontent.VisibilityDirect {
		fq.EntryIDs = []string{e.ID}
	}
	facts, err := s.d.Repository.FactsOfViewers(ctx, ids, fq)
	if err != nil {
		return nil, err
	}
	muted := map[string][]string{}
	if public && e.AuthorDomain != "" {
		if muted, err = s.d.Repository.MutedDomainsOf(ctx, ids, timeline.DomainAndParents(e.AuthorDomain)); err != nil {
			return nil, err
		}
	}
	for _, v := range viewers {
		f := facts[v.Viewer.AccountID]
		if len(f.Exclusions[e.AuthorAccountID]) > 0 || len(muted[v.Viewer.AccountID]) > 0 {
			continue // the viewer's listing leaves it out
		}
		if len(v.Languages) > 0 && e.Language != "und" && !slices.Contains(v.Languages, e.Language) {
			continue
		}
		if f.canView(vctx, v.Viewer, e) {
			out[v.Viewer.AccountID] = e
		}
	}
	return out, nil
}
