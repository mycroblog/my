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

// The instance theme (ADR-0059, FR-605): the administrator's overrides of the web client's design
// tokens and custom CSS, versioned like the settings.

// ThemeRepository is the part of Repository about the theme.
type ThemeRepository interface {
	// InsertThemeRevision inserts a revision with its token rows.
	InsertThemeRevision(ctx context.Context, tx uow.Tx, rev instance.ThemeRevision) error
	// ThemeHead returns the instance_theme row.
	ThemeHead(ctx context.Context, tx uow.Tx) (instance.ThemeHead, error)
	UpsertThemeHead(ctx context.Context, tx uow.Tx, h instance.ThemeHead) error
	DeleteThemeHead(ctx context.Context, tx uow.Tx) error
	// LatestThemeRevisionID returns the greatest revision ID, for rebuilding.
	LatestThemeRevisionID(ctx context.Context, tx uow.Tx) (string, error)

	// CurrentTheme returns the revision that the head points at.
	CurrentTheme(ctx context.Context) (instance.ThemeRevision, error)
	// FindThemeRevision returns a revision, current or not, with its tokens.
	FindThemeRevision(ctx context.Context, id string) (instance.ThemeRevision, error)
}

// EventThemeRevised is the event type of ThemeRevised.
const EventThemeRevised = "instance.theme_revised"

// ThemeRevised is emitted when a theme revision is inserted.
type ThemeRevised struct {
	Revision instance.ThemeRevision
}

// EventType implements uow.Event.
func (ThemeRevised) EventType() string { return EventThemeRevised }

// AuditTargetTheme is the target ID (and label) of the audit log entries of theme changes, which
// are setting changes of the setting named "theme".
const AuditTargetTheme = "theme"

func registerThemeProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventThemeRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		rev := e.(ThemeRevised).Revision
		head, err := repo.ThemeHead(ctx, tx)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if !rev.Supersedes(head) {
			return nil
		}
		return repo.UpsertThemeHead(ctx, tx, instance.ThemeHead{RevisionID: rev.ID})
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

// Theme returns the current theme. It reports false when the theme has never been saved or was
// reset (its revision changes nothing), so that the web client loads no stylesheet.
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

// ThemeRevision returns a theme revision, current or not, for its stylesheet, which never changes
// (ADR-0059 §5). It fails with an error wrapping ErrNotFound for an unknown ID.
func (s *Service) ThemeRevision(ctx context.Context, id string) (instance.ThemeRevision, error) {
	return s.d.Repository.FindThemeRevision(ctx, id)
}

// SetThemeInput replaces the theme.
type SetThemeInput struct {
	// Tokens are every overridden token; the others go back to their defaults.
	Tokens []instance.ThemeTokenValue
	// CustomCSS is the administrator's stylesheet; "" for none.
	CustomCSS string
	// RevisedByUserID is the user making the change, or "" for the CLI.
	RevisedByUserID string
}

// SetTheme validates a theme (instance.NormalizeTheme) and records it as a new revision, which
// becomes the current theme; no tokens and no CSS reset the theme. It fails with an
// *instance.ThemeError listing every problem. The change is audited in the same transaction.
func (s *Service) SetTheme(ctx context.Context, in SetThemeInput) (instance.ThemeRevision, error) {
	tokens, err := instance.NormalizeTheme(in.Tokens, in.CustomCSS)
	if err != nil {
		return instance.ThemeRevision{}, err
	}
	rev := instance.ThemeRevision{
		ID: s.d.IDs.NewID(), Tokens: tokens, CustomCSS: in.CustomCSS,
		RevisedByUserID: in.RevisedByUserID, RevisedAt: s.d.Clock.Now(),
	}
	audit := s.audit(appmoderation.AuditSettingChanged, appmoderation.AuditTargetSetting, AuditTargetTheme, AuditTargetTheme,
		rev.ID, in.RevisedByUserID, rev.RevisedAt)
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertThemeRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := tx.Emit(ctx, ThemeRevised{Revision: rev}); err != nil {
			return err
		}
		return tx.Emit(ctx, audit)
	})
	if err != nil {
		return instance.ThemeRevision{}, fmt.Errorf("set theme: %w", err)
	}
	return rev, nil
}
