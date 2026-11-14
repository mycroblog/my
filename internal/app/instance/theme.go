// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"errors"
	"fmt"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The instance theme (ADR-0059 §15, FR-605): one token set per server, the administrator's
// overrides of the web client's design tokens. Each save records a set and makes it the draft;
// publishing records a publication of the draft, which becomes the theme in effect, and restoring
// publishes again the set published before the current one.

// ThemeRepository is the part of Repository about the theme.
type ThemeRepository interface {
	// InsertThemeSet inserts a set as its Document; a set whose ID is already there stays as it is.
	InsertThemeSet(ctx context.Context, tx uow.Tx, set instance.ThemeSet) error
	// InsertThemePublication inserts a publication; one whose ID is already there stays as it is.
	InsertThemePublication(ctx context.Context, tx uow.Tx, p instance.ThemePublication) error
	// FindThemeSet returns a set, its overrides read with instance.ReadThemeSet.
	FindThemeSet(ctx context.Context, tx uow.Tx, id string) (instance.ThemeSet, error)
	// FindThemePublication returns a publication.
	FindThemePublication(ctx context.Context, tx uow.Tx, id string) (instance.ThemePublication, error)
	// PreviousThemePublication returns the latest publication before p of a set other than p's.
	PreviousThemePublication(ctx context.Context, tx uow.Tx, p instance.ThemePublication) (instance.ThemePublication, error)

	// ThemeHead returns the instance_theme row: the publication in effect.
	ThemeHead(ctx context.Context, tx uow.Tx) (instance.ThemeHead, error)
	UpsertThemeHead(ctx context.Context, tx uow.Tx, h instance.ThemeHead) error
	DeleteThemeHead(ctx context.Context, tx uow.Tx) error
	// LatestThemeRevisionID returns the greatest publication ID, for rebuilding.
	LatestThemeRevisionID(ctx context.Context, tx uow.Tx) (string, error)
	// ThemeDraftHead returns the instance_theme_draft row: the ID of the draft set.
	ThemeDraftHead(ctx context.Context, tx uow.Tx) (string, error)
	UpsertThemeDraftHead(ctx context.Context, tx uow.Tx, setID string) error
	DeleteThemeDraftHead(ctx context.Context, tx uow.Tx) error
	// LatestThemeSetID returns the greatest set ID, for rebuilding.
	LatestThemeSetID(ctx context.Context, tx uow.Tx) (string, error)

	// CurrentTheme returns the publication that the head points at, with its set.
	CurrentTheme(ctx context.Context) (instance.ThemeRevision, error)
	// FindThemeRevision returns a publication, current or not, with its set.
	FindThemeRevision(ctx context.Context, id string) (instance.ThemeRevision, error)
	// ThemeDraft returns the set that the draft head points at.
	ThemeDraft(ctx context.Context) (instance.ThemeSet, error)
}

// Event types of the theme.
const (
	EventThemeSetSaved  = "instance.theme_set_saved"
	EventThemePublished = "instance.theme_published"
)

// ThemeSetSaved is emitted when a set is inserted, which becomes the draft.
type ThemeSetSaved struct {
	Set instance.ThemeSet
}

// EventType implements uow.Event.
func (ThemeSetSaved) EventType() string { return EventThemeSetSaved }

// ThemePublished is emitted when a publication is inserted, which becomes the theme in effect.
type ThemePublished struct {
	Revision instance.ThemeRevision
}

// EventType implements uow.Event.
func (ThemePublished) EventType() string { return EventThemePublished }

// AuditTargetTheme is the target ID (and label) of the audit log entries of theme publications,
// which are setting changes of the setting named "theme".
const AuditTargetTheme = "theme"

func registerThemeProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventThemePublished, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		rev := e.(ThemePublished).Revision
		head, err := repo.ThemeHead(ctx, tx)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if !rev.Supersedes(head) {
			return nil
		}
		return repo.UpsertThemeHead(ctx, tx, instance.ThemeHead{RevisionID: rev.ID})
	})
	r.Register(EventThemeSetSaved, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		set := e.(ThemeSetSaved).Set
		draft, err := repo.ThemeDraftHead(ctx, tx)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if draft != "" && set.ID <= draft {
			return nil
		}
		return repo.UpsertThemeDraftHead(ctx, tx, set.ID)
	})
}

func rebuildThemeHead(ctx context.Context, tx uow.Tx, repo Repository) error {
	if err := repo.DeleteThemeHead(ctx, tx); err != nil {
		return err
	}
	id, err := repo.LatestThemeRevisionID(ctx, tx)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return repo.UpsertThemeHead(ctx, tx, instance.ThemeHead{RevisionID: id})
}

func rebuildThemeDraft(ctx context.Context, tx uow.Tx, repo Repository) error {
	if err := repo.DeleteThemeDraftHead(ctx, tx); err != nil {
		return err
	}
	id, err := repo.LatestThemeSetID(ctx, tx)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return repo.UpsertThemeDraftHead(ctx, tx, id)
}

// Theme returns the theme in effect. It reports false when no set has been published or the
// published set overrides nothing, so that the web client loads no stylesheet.
func (s *Service) Theme(ctx context.Context) (instance.ThemeRevision, bool, error) {
	rev, err := s.d.Repository.CurrentTheme(ctx)
	if errors.Is(err, ErrNotFound) {
		return instance.ThemeRevision{}, false, nil
	}
	if err != nil {
		return instance.ThemeRevision{}, false, err
	}
	return rev, !rev.Empty(), nil
}

// ThemeRevision returns a publication, current or not, with its set, for its stylesheet, which
// never changes (ADR-0059 §5). It fails with an error wrapping ErrNotFound for an unknown ID.
func (s *Service) ThemeRevision(ctx context.Context, id string) (instance.ThemeRevision, error) {
	return s.d.Repository.FindThemeRevision(ctx, id)
}

// ThemeDraft returns the draft set, and false when no set has been saved.
func (s *Service) ThemeDraft(ctx context.Context) (instance.ThemeSet, bool, error) {
	set, err := s.d.Repository.ThemeDraft(ctx)
	if errors.Is(err, ErrNotFound) {
		return instance.ThemeSet{}, false, nil
	}
	if err != nil {
		return instance.ThemeSet{}, false, err
	}
	return set, true, nil
}

// SaveThemeDraftInput replaces the draft.
type SaveThemeDraftInput struct {
	// Tokens are every overridden token; the others keep their defaults.
	Tokens []instance.ThemeTokenValue
	// SavedByUserID is the user saving the draft, or "" for the CLI.
	SavedByUserID string
}

// SaveThemeDraft validates a whole token set (instance.NormalizeTheme) and records it as a new set,
// which becomes the draft; the theme in effect does not change until PublishTheme. No tokens make
// a draft that resets the theme to the defaults. It fails with an *instance.ThemeError listing
// every problem.
func (s *Service) SaveThemeDraft(ctx context.Context, in SaveThemeDraftInput) (instance.ThemeSet, error) {
	tokens, err := instance.NormalizeTheme(in.Tokens)
	if err != nil {
		return instance.ThemeSet{}, err
	}
	set := instance.ThemeSet{ID: s.d.IDs.NewID(), Tokens: tokens, SavedByUserID: in.SavedByUserID, SavedAt: s.d.Clock.Now()}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertThemeSet(ctx, tx, set); err != nil {
			return err
		}
		return tx.Emit(ctx, ThemeSetSaved{Set: set})
	})
	if err != nil {
		return instance.ThemeSet{}, fmt.Errorf("save theme draft: %w", err)
	}
	return set, nil
}

// PublishTheme publishes the draft: it becomes the theme in effect under a new revision. The draft
// is validated again as a whole, since an upgrade may have changed the defaults it is checked
// with; its overrides that no longer apply are left out. It fails with an error wrapping
// ErrNotFound when no draft has been saved, and with an *instance.ThemeError. The publication is
// audited in the same transaction.
func (s *Service) PublishTheme(ctx context.Context, publishedByUserID string) (instance.ThemeRevision, error) {
	rev, err := s.publish(ctx, publishedByUserID, func(tx uow.Tx) (string, error) {
		return s.d.Repository.ThemeDraftHead(ctx, tx)
	})
	if err != nil {
		return instance.ThemeRevision{}, fmt.Errorf("publish theme: %w", err)
	}
	return rev, nil
}

// RestoreTheme publishes again the set published before the one in effect (the latest earlier
// publication of another set), under a new revision; the draft stays. It fails with an error
// wrapping ErrNotFound when no other set has been published, and with an *instance.ThemeError
// when the set no longer meets its contrast pairs. The publication is audited in the same
// transaction.
func (s *Service) RestoreTheme(ctx context.Context, publishedByUserID string) (instance.ThemeRevision, error) {
	rev, err := s.publish(ctx, publishedByUserID, func(tx uow.Tx) (string, error) {
		head, err := s.d.Repository.ThemeHead(ctx, tx)
		if err != nil {
			return "", err
		}
		cur, err := s.d.Repository.FindThemePublication(ctx, tx, head.RevisionID)
		if err != nil {
			return "", err
		}
		prev, err := s.d.Repository.PreviousThemePublication(ctx, tx, cur)
		if err != nil {
			return "", err
		}
		return prev.SetID, nil
	})
	if err != nil {
		return instance.ThemeRevision{}, fmt.Errorf("restore theme: %w", err)
	}
	return rev, nil
}

// publish records a publication of the set that pick chooses in the transaction.
func (s *Service) publish(ctx context.Context, by string, pick func(tx uow.Tx) (string, error)) (instance.ThemeRevision, error) {
	var rev instance.ThemeRevision
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		setID, err := pick(tx)
		if err != nil {
			return err
		}
		set, err := s.d.Repository.FindThemeSet(ctx, tx, setID)
		if err != nil {
			return err
		}
		if _, err := instance.NormalizeTheme(set.Tokens); err != nil {
			return err
		}
		rev = instance.ThemeRevision{
			ThemePublication: instance.ThemePublication{ID: s.d.IDs.NewID(), SetID: set.ID, PublishedByUserID: by, PublishedAt: s.d.Clock.Now()},
			Set:              set,
		}
		if err := s.d.Repository.InsertThemePublication(ctx, tx, rev.ThemePublication); err != nil {
			return err
		}
		if err := tx.Emit(ctx, ThemePublished{Revision: rev}); err != nil {
			return err
		}
		return tx.Emit(ctx, s.audit(appmoderation.AuditSettingChanged, appmoderation.AuditTargetSetting, AuditTargetTheme, AuditTargetTheme,
			rev.ID, by, rev.PublishedAt))
	})
	return rev, err
}
