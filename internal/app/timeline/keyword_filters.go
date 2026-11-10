// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// Keyword filters (FR-209, ADR-0157, data-model §4.6): an account keeps up to
// timeline.MaxKeywordFilters filters, each with words, contexts, an action, and an optional
// expiry. The timeline module owns them: the account's commands write them, and every read that
// shows statuses in a context of the filters (home and lists, notifications, the public
// timelines, threads, profiles, and their streams) judges the statuses it shows through a
// StatusFilter right after CanView. Nothing of a match is stored: a change to a filter applies at
// once to every status, and an edit of a status is judged as it reads now.

// KeywordFilterRepository is the part of Repository that keeps the keyword filters.
type KeywordFilterRepository interface {
	// InsertKeywordFilter stores the new filter f with its settings, contexts, and words.
	InsertKeywordFilter(ctx context.Context, tx uow.Tx, f timeline.KeywordFilter) error
	// ReplaceKeywordFilter replaces the settings, contexts, and words of the stored filter f.
	ReplaceKeywordFilter(ctx context.Context, tx uow.Tx, f timeline.KeywordFilter) error
	// DeleteKeywordFilter deletes the filter id with its settings, contexts, and words; one that
	// does not exist changes nothing.
	DeleteKeywordFilter(ctx context.Context, tx uow.Tx, id string) error
	// FindKeywordFilter returns the filter id inside tx, ErrNotFound when it does not exist.
	FindKeywordFilter(ctx context.Context, tx uow.Tx, id string) (timeline.KeywordFilter, error)
	// KeywordFiltersOf returns the filters of accountID, oldest first.
	KeywordFiltersOf(ctx context.Context, accountID string) ([]timeline.KeywordFilter, error)
	// CountKeywordFilters returns how many filters accountID has and how many words they have
	// in all.
	CountKeywordFilters(ctx context.Context, tx uow.Tx, accountID string) (filters, words int, err error)
	// ActiveKeywordFilters returns, by account, the filters of accountIDs that apply in c at now
	// (no expiry, or one after now), oldest first, each with its ID, AccountID, Title, Action,
	// and Words; an account without one has no key.
	ActiveKeywordFilters(ctx context.Context, accountIDs []string, c timeline.FilterContext, now time.Time) (map[string][]timeline.KeywordFilter, error)
}

// StatusViews presents statuses as a viewer sees them, for the judgments that read no page of
// their own: the unread count, Web Push, and email of the notifications. *appcontent.Reader
// implements it.
type StatusViews interface {
	Views(ctx context.Context, viewer appcontent.Viewer, ids []string) ([]appcontent.StatusView, error)
}

// KeywordFilterInput is a keyword filter as the account writes it.
type KeywordFilterInput struct {
	Title    string
	Action   timeline.FilterAction
	Contexts []timeline.FilterContext
	Words    []KeywordInput
	// ExpiresAt is when the filter stops applying, zero for never.
	ExpiresAt time.Time
}

// OwnedKeywordFilter is a keyword filter as its owner reads it, with whether it has expired.
type OwnedKeywordFilter struct {
	timeline.KeywordFilter
	// Expired is whether the filter has expired and applies nowhere.
	Expired bool
}

// asOwned returns f as its owner reads it now.
func (s *Service) asOwned(f timeline.KeywordFilter) OwnedKeywordFilter {
	return OwnedKeywordFilter{KeywordFilter: f, Expired: !f.ActiveAt(s.d.Clock.Now())}
}

// KeywordInput is a word of a KeywordFilterInput.
type KeywordInput struct {
	Word      string
	WholeWord bool
}

// NFKC is the normalization form KC that keyword filters normalize with (ADR-0157 §4).
func NFKC(s string) string { return norm.NFKC.String(s) }

var _ timeline.NFKC = NFKC

// keywordFilter validates in and returns the filter it describes: the title trimmed, the
// contexts sorted without duplicates, and the words normalized, each normalized form once, in
// the order given.
func keywordFilter(in KeywordFilterInput) (timeline.KeywordFilter, error) {
	f := timeline.KeywordFilter{Title: strings.TrimSpace(in.Title), Action: in.Action, ExpiresAt: in.ExpiresAt}
	if !timeline.ValidFilterTitle(f.Title, Graphemes) {
		return timeline.KeywordFilter{}, timeline.ErrInvalidFilterTitle
	}
	if !f.Action.Valid() {
		return timeline.KeywordFilter{}, timeline.ErrInvalidFilterAction
	}
	for _, c := range in.Contexts {
		if !c.Valid() {
			return timeline.KeywordFilter{}, timeline.ErrInvalidFilterContext
		}
	}
	f.Contexts = slices.Compact(slices.Sorted(slices.Values(in.Contexts)))
	if len(f.Contexts) == 0 {
		return timeline.KeywordFilter{}, timeline.ErrInvalidFilterContext
	}
	if !f.ExpiresAt.IsZero() {
		f.ExpiresAt = kernel.Normalize(f.ExpiresAt)
	}
	seen := map[string]bool{}
	for _, w := range in.Words {
		fw, err := timeline.NewFilterWord(w.Word, w.WholeWord, NFKC, Graphemes)
		if err != nil {
			return timeline.KeywordFilter{}, err
		}
		if seen[fw.Normalized] {
			continue
		}
		seen[fw.Normalized] = true
		f.Words = append(f.Words, fw)
	}
	switch {
	case len(f.Words) == 0:
		return timeline.KeywordFilter{}, timeline.ErrNoFilterWords
	case len(f.Words) > timeline.MaxKeywordFilterWords:
		return timeline.KeywordFilter{}, timeline.ErrTooManyFilterWords
	}
	return f, nil
}

// KeywordFilters returns the keyword filters of accountID, oldest first, the expired ones too.
func (s *Service) KeywordFilters(ctx context.Context, accountID string) ([]OwnedKeywordFilter, error) {
	fs, err := s.d.Repository.KeywordFiltersOf(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]OwnedKeywordFilter, len(fs))
	for i, f := range fs {
		out[i] = s.asOwned(f)
	}
	return out, nil
}

// KeywordFilter returns the keyword filter id of accountID, ErrNotFound for a filter that
// accountID does not have.
func (s *Service) KeywordFilter(ctx context.Context, accountID, id string) (OwnedKeywordFilter, error) {
	var f timeline.KeywordFilter
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		f, err = s.ownedFilter(ctx, tx, accountID, id)
		return err
	})
	if err != nil {
		return OwnedKeywordFilter{}, err
	}
	return s.asOwned(f), nil
}

// ownedFilter returns the filter id of accountID inside tx, ErrNotFound when it does not exist or
// another account has it.
func (s *Service) ownedFilter(ctx context.Context, tx uow.Tx, accountID, id string) (timeline.KeywordFilter, error) {
	f, err := s.d.Repository.FindKeywordFilter(ctx, tx, id)
	if err != nil {
		return timeline.KeywordFilter{}, err
	}
	if f.AccountID != accountID {
		return timeline.KeywordFilter{}, ErrNotFound
	}
	return f, nil
}

// CreateKeywordFilter creates a keyword filter of accountID. It fails with the errors of the
// timeline package's filter rules for an input that is not one, with
// timeline.ErrTooManyKeywordFilters when the account has timeline.MaxKeywordFilters filters, and
// with timeline.ErrTooManyFilterWords when the words would pass a limit.
func (s *Service) CreateKeywordFilter(ctx context.Context, accountID string, in KeywordFilterInput) (OwnedKeywordFilter, error) {
	f, err := keywordFilter(in)
	if err != nil {
		return OwnedKeywordFilter{}, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		filters, words, err := s.d.Repository.CountKeywordFilters(ctx, tx, accountID)
		switch {
		case err != nil:
			return err
		case filters >= timeline.MaxKeywordFilters:
			return timeline.ErrTooManyKeywordFilters
		case words+len(f.Words) > timeline.MaxKeywordFilterWordsTotal:
			return timeline.ErrTooManyFilterWords
		}
		f.ID, f.AccountID, f.CreatedAt = s.d.IDs.NewID(), accountID, kernel.Normalize(s.d.Clock.Now())
		return s.d.Repository.InsertKeywordFilter(ctx, tx, f)
	})
	if err != nil {
		return OwnedKeywordFilter{}, err
	}
	return s.asOwned(f), nil
}

// UpdateKeywordFilter replaces the title, action, contexts, words, and expiry of the keyword filter
// id of accountID with in. It fails with ErrNotFound for a filter that accountID does not have,
// and as CreateKeywordFilter for an input that is not one or words that pass a limit.
func (s *Service) UpdateKeywordFilter(ctx context.Context, accountID, id string, in KeywordFilterInput) (OwnedKeywordFilter, error) {
	f, err := keywordFilter(in)
	if err != nil {
		return OwnedKeywordFilter{}, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		old, err := s.ownedFilter(ctx, tx, accountID, id)
		if err != nil {
			return err
		}
		_, words, err := s.d.Repository.CountKeywordFilters(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if words-len(old.Words)+len(f.Words) > timeline.MaxKeywordFilterWordsTotal {
			return timeline.ErrTooManyFilterWords
		}
		f.ID, f.AccountID, f.CreatedAt = old.ID, old.AccountID, old.CreatedAt
		return s.d.Repository.ReplaceKeywordFilter(ctx, tx, f)
	})
	if err != nil {
		return OwnedKeywordFilter{}, err
	}
	return s.asOwned(f), nil
}

// DeleteKeywordFilter deletes the keyword filter id of accountID. It fails with ErrNotFound for a
// filter that accountID does not have.
func (s *Service) DeleteKeywordFilter(ctx context.Context, accountID, id string) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.ownedFilter(ctx, tx, accountID, id); err != nil {
			return err
		}
		return s.d.Repository.DeleteKeywordFilter(ctx, tx, id)
	})
}

// StatusFilter judges the statuses that a read shows to one viewer against the viewer's keyword
// filters of one context. A nil *StatusFilter, the one of a visitor or of a viewer without
// filters there, judges nothing.
type StatusFilter struct {
	viewerAccountID string
	matcher         *timeline.FilterMatcher
}

// Judgement is what the keyword filters make of a status.
type Judgement struct {
	// Matches are the filters that the status matched, with their words; empty for none.
	Matches []timeline.FilterMatch
	// Hide is whether one of them hides the status: the read leaves it out, except where the
	// viewer opened it (ADR-0157 §6), which folds it as for the others.
	Hide bool
	// Quote are the filters that the quoted status matched, which fold only the quote: a quote
	// never leaves out the status that quotes it (ADR-0157 §3).
	Quote []timeline.FilterMatch
}

// StatusFilter returns the StatusFilter of viewer in c at now: nil for a visitor and for a
// viewer whose filters of c have no words or have expired.
func (s *Service) StatusFilter(ctx context.Context, viewer appcontent.Viewer, c timeline.FilterContext) (*StatusFilter, error) {
	if !loggedIn(viewer) {
		return nil, nil
	}
	fs, err := s.StatusFiltersOf(ctx, []string{viewer.AccountID}, c)
	if err != nil {
		return nil, err
	}
	return fs[viewer.AccountID], nil
}

// StatusFiltersOf returns the StatusFilter in c of each of accountIDs that has one, with one
// read, for a stream that sends a status to many viewers.
func (s *Service) StatusFiltersOf(ctx context.Context, accountIDs []string, c timeline.FilterContext) (map[string]*StatusFilter, error) {
	out := map[string]*StatusFilter{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	byAccount, err := s.d.Repository.ActiveKeywordFilters(ctx, uniq(slices.Clone(accountIDs)), c, s.d.Clock.Now())
	if err != nil {
		return nil, err
	}
	for id, filters := range byAccount {
		if m := timeline.NewFilterMatcher(filters, NFKC); m != nil {
			out[id] = &StatusFilter{viewerAccountID: id, matcher: m}
		}
	}
	return out, nil
}

// Judge judges the status v, which booster boosted ("" for a post as itself). The viewer's own
// statuses and boosts are never judged, nor what they quote: what the viewer wrote or chose is
// not hidden from the viewer (ADR-0157 §7). It matches the text of each body (every language),
// the content warning, each poll choice, and each alt text, each on its own; the quoted status
// is judged on its own.
func (f *StatusFilter) Judge(v appcontent.StatusView, booster string) Judgement {
	if f == nil || booster == f.viewerAccountID || v.Status.AuthorAccountID == f.viewerAccountID {
		return Judgement{}
	}
	j := Judgement{Matches: f.matcher.Match(filterFields(v)...)}
	j.Hide = slices.ContainsFunc(j.Matches, func(m timeline.FilterMatch) bool { return m.Filter.Action == timeline.FilterHide })
	if q := v.Quote; q != nil && q.Status != nil && q.Status.Status.AuthorAccountID != f.viewerAccountID {
		j.Quote = f.matcher.Match(filterFields(*q.Status)...)
	}
	return j
}

// filterFields returns the texts of v that keyword filters match.
func filterFields(v appcontent.StatusView) []string {
	rev := v.Revision
	fields := make([]string, 0, len(rev.Bodies)+1+len(v.Attachments)+4)
	for _, b := range rev.Bodies {
		fields = append(fields, timeline.TextOfHTML(b.ContentHTML))
	}
	fields = append(fields, rev.SpoilerText)
	if v.Poll != nil {
		for _, o := range v.Poll.Poll.Options {
			fields = append(fields, o.Title)
		}
	}
	for _, a := range v.Attachments {
		fields = append(fields, a.Description)
	}
	return fields
}

// errNoStatusViews reports a Service built without Deps.Statuses.
var errNoStatusViews = errors.New("timeline: judging statuses by ID needs the status views")

// JudgeStatuses judges the statuses ids, as accountID sees them now, against accountID's keyword
// filters of c, by status ID; a status that accountID may not see, or that no filter matches, has
// no key. The notification module calls it for what it reads without a page: the unread count,
// Web Push, and email of the notifications (ADR-0157 §7).
func (s *Service) JudgeStatuses(ctx context.Context, accountID string, c timeline.FilterContext, ids []string) (map[string]Judgement, error) {
	out := map[string]Judgement{}
	if len(ids) == 0 || accountID == "" {
		return out, nil
	}
	viewer := appcontent.Viewer{Kind: appcontent.ViewerLocal, AccountID: accountID}
	f, err := s.StatusFilter(ctx, viewer, c)
	if err != nil || f == nil {
		return out, err
	}
	if s.d.Statuses == nil {
		return nil, errNoStatusViews
	}
	views, err := s.d.Statuses.Views(ctx, viewer, uniq(slices.Clone(ids)))
	if err != nil {
		return nil, err
	}
	for _, v := range views {
		if j := f.Judge(v, ""); len(j.Matches) > 0 {
			out[v.Status.ID] = j
		}
	}
	return out, nil
}
