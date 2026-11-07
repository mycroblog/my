// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
)

// IndexCallTimeout bounds one call of the backend: the default worker runs one job at a time, so
// a slow engine must not take the time of federation.inbox for long (ADR-0149 §7).
const IndexCallTimeout = 10 * time.Second

const (
	// syncAttempts bounds how many times one status is written in one run when the database keeps
	// changing under it; after that the job retries.
	syncAttempts = 3
)

// StatusReader is what the module reads of statuses. *appcontent.Service implements it.
type StatusReader interface {
	// ObjectStatus returns a status with its current revision and whether it is deleted. It fails
	// with appcontent.ErrNotFound when there is no status.
	ObjectStatus(ctx context.Context, id string) (appcontent.Status, appcontent.Revision, bool, error)
	// AuthorRestrictions returns the restrictions of an author that bear on who sees its statuses,
	// with its lifecycle mapped to ProfileView.
	AuthorRestrictions(ctx context.Context, accountID string) (appcontent.AuthorRestrictions, error)
	// SearchableStatusIDs pages the public and local statuses of the server in ID order.
	SearchableStatusIDs(ctx context.Context, after string, limit int) ([]string, error)
	// AuthorSearchableStatusIDs pages the public and local statuses of an author in ID order.
	AuthorSearchableStatusIDs(ctx context.Context, authorAccountID, after string, limit int) ([]string, error)
}

// AccountReader is what the module reads of accounts. *appaccount.Service implements it.
type AccountReader interface {
	// ProfileIndexable reports the indexable of an account's current profile, false without one.
	ProfileIndexable(ctx context.Context, id string) (bool, error)
	// AccountIDsOnDomain pages the accounts on a domain or its subdomains in ID order.
	AccountIDsOnDomain(ctx context.Context, domain, after string, limit int) ([]string, error)
	// AccountDomain returns the domain of an account, "" when it is local.
	AccountDomain(ctx context.Context, id string) (string, error)
}

// SettingsReader reads the instance setting search_visibilities. *appinstance.Service implements it.
type SettingsReader interface {
	SearchVisibilities(ctx context.Context) ([]string, error)
}

// Viewing decides which statuses a viewer may see in the search. *apptimeline.Service implements it.
type Viewing interface {
	SearchVisible(ctx context.Context, viewer appcontent.Viewer, candidates []apptimeline.SearchCandidate) ([]string, error)
}

// Deps are what the module needs. With search = none only Index is meaningful, and it is nil.
type Deps struct {
	Index      Index
	UnitOfWork uow.UnitOfWork
	Outbox     queue.Outbox
	Statuses   StatusReader
	Accounts   AccountReader
	Settings   SettingsReader
	Viewing    Viewing
	// Sanitizer takes the HTML out of the bodies of remote statuses.
	Sanitizer kernel.HTMLSanitizer
	// Rebuilds keeps the progress of a rebuild.
	Rebuilds Repository
	Clock    kernel.Clock
	IDs      kernel.IDGenerator
	// RebuildBatch is how many statuses one job of a rebuild covers. Tests only; rebuildBatch when 0.
	RebuildBatch int
}

// Service is the search module. Its index is nil with search = none (ADR-0149 §2).
type Service struct {
	d Deps
}

// NewService returns the module over d.
func NewService(d Deps) *Service {
	return &Service{d: d}
}

// Enabled reports whether the server has a backend, which is false with search = none.
func (s *Service) Enabled() bool {
	return s.d.Index != nil
}

// Visibilities returns the visibilities the full-text search covers: the setting
// search_visibilities (ADR-0149 §5), and none with search = none.
func (s *Service) Visibilities(ctx context.Context) ([]Visibility, error) {
	if !s.Enabled() {
		return nil, nil
	}
	values, err := s.d.Settings.SearchVisibilities(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Visibility, 0, len(values))
	for _, v := range values {
		out = append(out, Visibility(v))
	}
	return out, nil
}

// Candidate is what the rule of what qualifies reads of a status and its author (ADR-0149 §5).
type Candidate struct {
	// Deleted reports that the status is deleted.
	Deleted bool
	// Visibility is the stored visibility of the status.
	Visibility appcontent.Visibility
	// Indexable is the indexable of the author's current profile, false without one.
	Indexable bool
	// Author is what restricts the author, on its account or its domain.
	Author appcontent.AuthorRestrictions
}

// Eligible is the one rule of what the full-text search holds (ADR-0149 §5), which the jobs, the
// reads, and the rebuild share. It returns the visibility of the document, and whether the status
// qualifies given the visibilities that search_visibilities covers. unlisted, private, and direct
// never qualify, whatever covered says.
func Eligible(c Candidate, covered []Visibility) (Visibility, bool) {
	if c.Deleted || !authorQualifies(c.Indexable, c.Author) {
		return "", false
	}
	var v Visibility
	switch appcontent.EffectiveVisibility(c.Visibility, c.Author) {
	case appcontent.VisibilityPublic:
		v = VisibilityPublic
	case appcontent.VisibilityLocal:
		v = VisibilityLocal
	default:
		return "", false
	}
	if !slices.Contains(covered, v) {
		return "", false
	}
	return v, true
}

// authorQualifies reports whether any status of an author can qualify. A ceiling makes every status
// of the author private.
func authorQualifies(indexable bool, author appcontent.AuthorRestrictions) bool {
	return indexable && !author.ProfileView && !author.PublicDiscovery && !author.VisibilityCeilingPrivate
}

// mayEverQualify reports whether a status of this stored visibility can ever qualify. The stored
// visibility never changes after posting, so the others need no job at all.
func mayEverQualify(v appcontent.Visibility) bool {
	return v == appcontent.VisibilityPublic || v == appcontent.VisibilityLocal
}

// authorState reads whether an author can have documents at all.
func (s *Service) authorState(ctx context.Context, accountID string) (bool, appcontent.AuthorRestrictions, error) {
	indexable, err := s.d.Accounts.ProfileIndexable(ctx, accountID)
	if err != nil {
		return false, appcontent.AuthorRestrictions{}, err
	}
	author, err := s.d.Statuses.AuthorRestrictions(ctx, accountID)
	if err != nil {
		return false, appcontent.AuthorRestrictions{}, err
	}
	return indexable, author, nil
}

// current reads a status as it is now and returns its document, the ID of the revision that document
// shows, and whether it qualifies.
func (s *Service) current(ctx context.Context, statusID string) (Document, string, bool, error) {
	st, rev, deleted, err := s.d.Statuses.ObjectStatus(ctx, statusID)
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		return Document{}, "", false, nil
	case err != nil:
		return Document{}, "", false, err
	case deleted:
		return Document{}, "", false, nil
	}
	indexable, author, err := s.authorState(ctx, st.AuthorAccountID)
	if err != nil {
		return Document{}, "", false, err
	}
	covered, err := s.Visibilities(ctx)
	if err != nil {
		return Document{}, "", false, err
	}
	v, ok := Eligible(Candidate{Visibility: st.Visibility, Indexable: indexable, Author: author}, covered)
	if !ok {
		return Document{}, "", false, nil
	}
	language := rev.Language
	if language == "und" {
		language = ""
	}
	return Document{
		StatusID: st.ID, AuthorID: st.AuthorAccountID, Visibility: v, Language: language,
		PublishedAt: st.PublishedAt, Text: s.documentText(rev),
	}, rev.ID, true, nil
}

// documentText is what the search looks in: the content warning, the body, and the alternative texts
// of the revision. Polls, profiles, and older revisions stay out (ADR-0149 §5).
func (s *Service) documentText(rev appcontent.Revision) string {
	parts := make([]string, 0, 2+len(rev.Bodies)+len(rev.Attachments))
	parts = append(parts, rev.SpoilerText)
	if rev.Source != nil {
		parts = append(parts, rev.Source.Text)
	} else {
		for _, b := range rev.Bodies {
			// A space before each tag keeps the words of two paragraphs apart.
			parts = append(parts, s.d.Sanitizer.PlainText(strings.ReplaceAll(b.ContentHTML, "<", " <")))
		}
	}
	for _, a := range rev.Attachments {
		parts = append(parts, a.Description)
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

// syncStatus brings the document of one status in line with the database. Having written a document,
// it reads the status again: a job that read the older state may write after the job of a newer one,
// so a status that stopped qualifying or has another revision by then is written again, whatever the
// engine does about versions (ADR-0149 §7). A deletion commits before its own job runs, so a document
// written before the second read is always removed.
func (s *Service) syncStatus(ctx context.Context, statusID string) error {
	for range syncAttempts {
		doc, revisionID, ok, err := s.current(ctx, statusID)
		if err != nil {
			return err
		}
		if !ok {
			return s.call(ctx, func(ctx context.Context) error { return s.d.Index.Remove(ctx, statusID) })
		}
		if err := s.call(ctx, func(ctx context.Context) error { return s.d.Index.Replace(ctx, doc) }); err != nil {
			return err
		}
		after, revisionAfter, okAfter, err := s.current(ctx, statusID)
		if err != nil {
			return err
		}
		if okAfter && revisionAfter == revisionID && after.Visibility == doc.Visibility {
			return nil
		}
	}
	return fmt.Errorf("search: status %s kept changing while it was synced", statusID)
}

// call runs one call of the backend with its deadline.
func (s *Service) call(ctx context.Context, f func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, IndexCallTimeout)
	defer cancel()
	return f(ctx)
}
