// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"
	"fmt"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The rules of the link card crawler (FR-117): for a domain and its subdomains, the crawler
// fetches nothing or fetches no images. The rows are current values, and the audit log entry that
// each change records in the same transaction (ADR-0037) keeps who changed what and when.

// LinkCardDomainRepository is the persistence port of the rules. It is part of Repository.
type LinkCardDomainRepository interface {
	// UpsertLinkCardDomainRule records the rule of its domain, replacing the one it had.
	UpsertLinkCardDomainRule(ctx context.Context, tx uow.Tx, r instance.LinkCardDomainRule) error
	// DeleteLinkCardDomainRule deletes the rule of a domain, and reports whether it had one.
	DeleteLinkCardDomainRule(ctx context.Context, tx uow.Tx, domain string) (bool, error)
	// CountLinkCardDomainRules returns the number of rules, read in tx.
	CountLinkCardDomainRules(ctx context.Context, tx uow.Tx) (int, error)
	// HasLinkCardDomainRule reports whether a domain has a rule, read in tx.
	HasLinkCardDomainRule(ctx context.Context, tx uow.Tx, domain string) (bool, error)
	// ListLinkCardDomainRules returns every rule, by domain.
	ListLinkCardDomainRules(ctx context.Context) ([]instance.LinkCardDomainRule, error)
	// LinkCardDomainModes returns the modes of the rules of the domains.
	LinkCardDomainModes(ctx context.Context, domains []string) ([]instance.LinkCardDomainMode, error)
}

// LinkCardDomainRules returns every rule of the link card crawler, by domain.
func (s *Service) LinkCardDomainRules(ctx context.Context) ([]instance.LinkCardDomainRule, error) {
	return s.d.Repository.ListLinkCardDomainRules(ctx)
}

// SetLinkCardDomainRule records the rule of a canonical domain, replacing the one it had. It fails
// with a *instance.LinkCardDomainError for a bad domain, mode, or note, and with
// instance.ErrTooManyLinkCardDomainRules for a new rule beyond the limit. byUserID is the user
// making the change, or "" for the CLI.
func (s *Service) SetLinkCardDomainRule(ctx context.Context, r instance.LinkCardDomainRule, byUserID string) (instance.LinkCardDomainRule, error) {
	if !appmoderation.ValidDomain(r.Domain) {
		return instance.LinkCardDomainRule{}, &instance.LinkCardDomainError{Field: "domain"}
	}
	if err := r.Validate(); err != nil {
		return instance.LinkCardDomainRule{}, err
	}
	r.UpdatedAt = s.d.Clock.Now()
	action := appmoderation.AuditLinkCardDomainFetchStopped
	if r.Mode == instance.LinkCardNoImages {
		action = appmoderation.AuditLinkCardDomainImagesStopped
	}
	audit := s.audit(action, appmoderation.AuditTargetLinkCardDomain, r.Domain, r.Domain, "", byUserID, r.UpdatedAt)
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		has, err := s.d.Repository.HasLinkCardDomainRule(ctx, tx, r.Domain)
		if err != nil {
			return err
		}
		if !has {
			n, err := s.d.Repository.CountLinkCardDomainRules(ctx, tx)
			if err != nil {
				return err
			}
			if n >= instance.MaxLinkCardDomainRules {
				return instance.ErrTooManyLinkCardDomainRules
			}
		}
		if err := s.d.Repository.UpsertLinkCardDomainRule(ctx, tx, r); err != nil {
			return err
		}
		return tx.Emit(ctx, audit)
	})
	if err != nil {
		return instance.LinkCardDomainRule{}, fmt.Errorf("set the link card rule of %s: %w", r.Domain, err)
	}
	return r, nil
}

// RemoveLinkCardDomainRule deletes the rule of a canonical domain. A domain without a rule is not
// an error and records nothing. byUserID is the user making the change, or "" for the CLI.
func (s *Service) RemoveLinkCardDomainRule(ctx context.Context, domain, byUserID string) error {
	now := s.d.Clock.Now()
	audit := s.audit(appmoderation.AuditLinkCardDomainRuleRemoved, appmoderation.AuditTargetLinkCardDomain, domain, domain, "", byUserID, now)
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		deleted, err := s.d.Repository.DeleteLinkCardDomainRule(ctx, tx, domain)
		if err != nil || !deleted {
			return err
		}
		return tx.Emit(ctx, audit)
	})
	if err != nil {
		return fmt.Errorf("remove the link card rule of %s: %w", domain, err)
	}
	return nil
}

// LinkCardCrawl returns what the rules that cover a canonical host stop: the rule of the host and
// those of its parents, at the time of the call.
func (s *Service) LinkCardCrawl(ctx context.Context, host string) (instance.LinkCardCrawl, error) {
	domains := appmoderation.DomainAndParents(host)
	if len(domains) == 0 {
		return instance.LinkCardCrawl{}, nil
	}
	modes, err := s.d.Repository.LinkCardDomainModes(ctx, domains)
	if err != nil {
		return instance.LinkCardCrawl{}, fmt.Errorf("read the link card rules of %s: %w", host, err)
	}
	return instance.LinkCardCrawlOf(modes), nil
}
