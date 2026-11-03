// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// DomainsRepository is the persistence port of the domains of the instance (instance_domains).
type DomainsRepository interface {
	// InsertDomains records d unless domains are recorded, and reports whether it did.
	InsertDomains(ctx context.Context, tx uow.Tx, d instance.Domains) (bool, error)
	// FindDomains returns the recorded domains, ErrNotFound when there are none.
	FindDomains(ctx context.Context) (instance.Domains, error)
}

// EnsureDomains records configured when the database has no domains yet, and otherwise returns
// a *instance.DomainMismatchError when they differ from it. Concurrent first calls agree on the
// one that was inserted first (ADR-0018).
func (s *Service) EnsureDomains(ctx context.Context, configured instance.Domains) error {
	recorded, err := s.d.Repository.FindDomains(ctx)
	if errors.Is(err, ErrNotFound) {
		configured.RecordedAt = s.d.Clock.Now()
		err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			_, err := s.d.Repository.InsertDomains(ctx, tx, configured)
			return err
		})
		if err != nil {
			return fmt.Errorf("instance: record domains: %w", err)
		}
		recorded, err = s.d.Repository.FindDomains(ctx)
	}
	if err != nil {
		return fmt.Errorf("instance: read domains: %w", err)
	}
	return compareDomains(recorded, configured)
}

// CheckDomains is EnsureDomains for the commands that change nothing: domains that are not
// recorded yet are no mismatch, and stay unrecorded.
func (s *Service) CheckDomains(ctx context.Context, configured instance.Domains) error {
	recorded, err := s.d.Repository.FindDomains(ctx)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("instance: read domains: %w", err)
	}
	return compareDomains(recorded, configured)
}

func compareDomains(recorded, configured instance.Domains) error {
	if recorded.Same(configured) {
		return nil
	}
	return &instance.DomainMismatchError{Recorded: recorded, Configured: configured}
}
