// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/app/dbupgrade"
)

// DataUpgrades are the data upgrade steps of `my db upgrade` and the prerequisites of
// `my db upgrade --check` (ADR-0087 §2, §4).
type DataUpgrades struct {
	Steps         []dbupgrade.Step
	Prerequisites []dbupgrade.Prerequisite
}

// dataUpgrades returns the data upgrade steps of this release, in the order that they run, and
// the steps that its contract assumes, with the release that runs each.
//
// A step rewrites the rows of an expand: it is idempotent and works in small transactions
// without external I/O. Its key never changes once released. When a later release contracts
// the schema, the step stays here with Run set to nil, so that new databases record it too, and
// that release lists it in Prerequisites with the release whose `my db upgrade` runs it.
func (a *App) dataUpgrades() DataUpgrades {
	return DataUpgrades{}
}

// setUpDataUpgrades builds the service of `my db upgrade`, whose record the readiness check
// also reads. A step rebuilds its projections with the service of `my projection`.
func (a *App) setUpDataUpgrades(repo dbupgrade.Repository, override *DataUpgrades) error {
	ups := a.dataUpgrades()
	if override != nil {
		ups = *override
	}
	for _, s := range ups.Steps {
		for _, name := range s.Projections {
			if _, err := a.Projections.Lookup(name); err != nil {
				return fmt.Errorf("data upgrade step %s: %w", s.Key, err)
			}
		}
	}
	a.DataUpgrades = dbupgrade.NewService(dbupgrade.Options{
		UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock,
		Steps: ups.Steps, Prerequisites: ups.Prerequisites,
		RebuildProjection: func(ctx context.Context, name string) error {
			p, err := a.Projections.Lookup(name)
			if err != nil {
				return err
			}
			_, err = a.Projections.Rebuild(ctx, p)
			return err
		},
	})
	return nil
}
