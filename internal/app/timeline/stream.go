// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/uow"
)

// RegisterCommitHooks signals every new home timeline entry to pub once the command that adds
// it commits (FR-207): a status as posted (content.status_created) and a boost
// (content.boosted). The signal carries only the entry ID; each stream asks HomeEntry whether
// its viewer's home timeline lists the entry. A status that the public timelines may list,
// local or remote, also signals a public timeline entry (ADR-0127 §4), whose streams ask
// LocalEntry or PublicEntry.
func RegisterCommitHooks(r *uow.Projectors, pub stream.Publisher) {
	r.OnCommit(appcontent.EventStatusCreated, func(_ context.Context, e uow.Event) {
		st := e.(appcontent.StatusCreated).Status
		pub.Publish(stream.Event{Kind: stream.KindHomeEntry, EntryID: st.ID})
		if onLocal, onFederated := publicTimelines(st.IsLocal, st.Visibility); onLocal || onFederated {
			pub.Publish(stream.Event{Kind: stream.KindPublicEntry, EntryID: st.ID, OnLocal: onLocal, OnFederated: onFederated})
		}
	})
	r.OnCommit(appcontent.EventBoosted, func(_ context.Context, e uow.Event) {
		pub.Publish(stream.Event{Kind: stream.KindHomeEntry, EntryID: e.(appcontent.Boosted).Boost.ID})
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
