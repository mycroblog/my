// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// Event types of the instance module, for uow.Projectors.
const (
	EventSettingRevised = "instance.setting_revised"
	EventTextRevised    = "instance.text_revised"
)

// SettingRevised is emitted when a setting revision is inserted.
type SettingRevised struct {
	Revision instance.SettingRevision
}

// EventType implements uow.Event.
func (SettingRevised) EventType() string { return EventSettingRevised }

// TextRevised is emitted when a text revision is inserted.
type TextRevised struct {
	Revision instance.TextRevision
}

// EventType implements uow.Event.
func (TextRevised) EventType() string { return EventTextRevised }

// RegisterProjectors registers the instance module's synchronous projectors (data-model §9):
// instance_settings from setting revisions, instance_texts from text revisions,
// instance_rule_heads from rule revisions and retirements, instance_theme from theme revisions,
// instance_brands from brand revisions,
// announcement_heads from announcement revisions and state changes, and service_closure_heads
// from the service closure events. Each applies the same rule as RebuildProjections (the
// revision with the greatest ID wins; a retired rule has no head; a withdrawn announcement stays
// withdrawn; the closure events apply in order, instance.ClosureHead.Apply).
func RegisterProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventSettingRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		rev := e.(SettingRevised).Revision
		cur, err := repo.CurrentSetting(ctx, tx, rev.Key)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if !rev.Supersedes(cur) {
			return nil
		}
		return repo.UpsertCurrentSetting(ctx, tx, instance.CurrentSetting{Key: rev.Key, RevisionID: rev.ID, Value: rev.Value})
	})
	r.Register(EventTextRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		rev := e.(TextRevised).Revision
		cur, err := repo.CurrentText(ctx, tx, rev.Key)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if !rev.Supersedes(cur) {
			return nil
		}
		return repo.UpsertCurrentText(ctx, tx, instance.CurrentText{Key: rev.Key, RevisionID: rev.ID})
	})
	registerRuleProjectors(r, repo)
	registerThemeProjectors(r, repo)
	registerBrandProjectors(r, repo)
	registerAnnouncementProjectors(r, repo)
	registerClosureProjectors(r, repo)
}

// RebuildProjections empties the instance module's projections and derives them again from
// their revisions (data-model §9), in one transaction per projection.
func RebuildProjections(ctx context.Context, u uow.UnitOfWork, repo Repository) error {
	steps := []struct {
		name string
		fn   func(tx uow.Tx) error
	}{
		{"instance_settings", func(tx uow.Tx) error { return rebuildSettings(ctx, tx, repo) }},
		{"instance_texts", func(tx uow.Tx) error { return rebuildTexts(ctx, tx, repo) }},
		{"instance_rule_heads", func(tx uow.Tx) error { return rebuildRuleHeads(ctx, tx, repo) }},
		{"instance_theme", func(tx uow.Tx) error { return rebuildThemeHead(ctx, tx, repo) }},
		{"instance_brands", func(tx uow.Tx) error { return rebuildBrandHeads(ctx, tx, repo) }},
		{"announcement_heads", func(tx uow.Tx) error { return rebuildAnnouncementHeads(ctx, tx, repo) }},
		{"service_closure_heads", func(tx uow.Tx) error { return rebuildClosureHead(ctx, tx, repo) }},
	}
	for _, s := range steps {
		if err := u.Tx(ctx, s.fn); err != nil {
			return fmt.Errorf("rebuild %s: %w", s.name, err)
		}
	}
	return nil
}

func rebuildSettings(ctx context.Context, tx uow.Tx, repo Repository) error {
	if err := repo.DeleteAllCurrentSettings(ctx, tx); err != nil {
		return err
	}
	revs, err := repo.ListSettingRevisions(ctx, tx)
	if err != nil {
		return err
	}
	current := map[instance.SettingKey]instance.CurrentSetting{}
	var order []instance.SettingKey
	for _, r := range revs {
		c, seen := current[r.Key]
		if !seen {
			order = append(order, r.Key)
		}
		if r.Supersedes(c) {
			current[r.Key] = instance.CurrentSetting{Key: r.Key, RevisionID: r.ID, Value: r.Value}
		}
	}
	for _, k := range order {
		if err := repo.UpsertCurrentSetting(ctx, tx, current[k]); err != nil {
			return err
		}
	}
	return nil
}

func rebuildTexts(ctx context.Context, tx uow.Tx, repo Repository) error {
	if err := repo.DeleteAllCurrentTexts(ctx, tx); err != nil {
		return err
	}
	revs, err := repo.ListTextRevisionHeads(ctx, tx)
	if err != nil {
		return err
	}
	current := map[instance.TextKey]instance.CurrentText{}
	var order []instance.TextKey
	for _, r := range revs {
		c, seen := current[r.Key]
		if !seen {
			order = append(order, r.Key)
		}
		if (instance.TextRevision{ID: r.RevisionID}).Supersedes(c) {
			current[r.Key] = r
		}
	}
	for _, k := range order {
		if err := repo.UpsertCurrentText(ctx, tx, current[k]); err != nil {
			return err
		}
	}
	return nil
}
