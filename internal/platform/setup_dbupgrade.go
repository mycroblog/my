// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/app/dbupgrade"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
)

// DataUpgrades are the data upgrade steps of `my db upgrade` and the prerequisites of
// `my db upgrade --check` (ADR-0023 §5, §7).
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
//
// scripts/release-notes.sh reads the returned literal from this file for the release notes, at
// each release: keep the steps in it, with literal strings, or that script fails.
func (a *App) dataUpgrades(income appfinance.IncomeRepository, instances appinstance.Repository) DataUpgrades {
	return DataUpgrades{
		Steps: []dbupgrade.Step{
			{
				Key:         "finance.income_sources",
				Description: "Gives the income movements recorded before tips their source: the subscriptions' movements and their refunds and chargebacks become subscription, the rest stays unknown.",
				Run: func(ctx context.Context, u uow.UnitOfWork) error {
					return appfinance.UpgradeIncomeSources(ctx, u, income)
				},
			},
			{
				Key:         "theme.token_sets",
				Description: "Copies each saved theme into a token set and its publication under the same revision, so that the theme in effect stays and the earlier ones can be restored; a custom CSS is not carried over.",
				Run: func(ctx context.Context, u uow.UnitOfWork) error {
					return appinstance.UpgradeThemeSets(ctx, u, instances)
				},
				Projections: []string{"instance"},
			},
		},
	}
}

// setUpDataUpgrades builds the service of `my db upgrade`, whose record the readiness check
// also reads. A step rebuilds its projections with the service of `my projection`.
func (a *App) setUpDataUpgrades(repo dbupgrade.Repository, income appfinance.IncomeRepository, instances appinstance.Repository, override *DataUpgrades) error {
	ups := a.dataUpgrades(income, instances)
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
