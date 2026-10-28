// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Unsubscribing from email (ADR-0086 §4, ADR-0109 §4). Every notification or announcement email
// carries its own link (user_tokens, purpose email_unsubscribe), issued by the job that sends it,
// that turns off the emails of its category, a group of the notification settings: the
// List-Unsubscribe header and the link of the signature. The link works without signing in, any number of times, for
// EmailUnsubscribeTTL. The authpages package serves it: the one-click POST of RFC 8058 turns the
// emails off, and the GET only asks, so that mail scanners that fetch links change nothing.

const (
	// EmailUnsubscribeTTL is how long an unsubscribe link works.
	EmailUnsubscribeTTL = 90 * 24 * time.Hour
	// EmailUnsubscribeTokenPrefix marks unsubscribe tokens for secret scanners.
	EmailUnsubscribeTokenPrefix = "my_eut_"
	// PathMailUnsubscribe is the endpoint of the unsubscribe links, with the token in the query
	// parameter "token". The authpages package serves it.
	PathMailUnsubscribe = "/mail/unsubscribe"
)

// EmailUnsubscriptions is where the categories of the unsubscribe links are kept: the email
// column of the notification settings, whose groups are the categories (ADR-0109 §4). The
// composition root implements it on the notification module.
type EmailUnsubscriptions interface {
	// Unsubscribable reports whether category is a category of email that a link turns off.
	Unsubscribable(category identity.UnsubscribeCategory) bool
	// Unsubscribe turns off the emails of category for the user. It fails with an error wrapping
	// ErrNotFound when the user has no local account.
	Unsubscribe(ctx context.Context, userID string, category identity.UnsubscribeCategory) error
}

// IssueUnsubscribeLink stores a new unsubscribe token of a user for the emails of category and
// returns the link that carries it. The secret is only in the link: the caller puts it in the
// email it sends and nowhere else.
func (s *Service) IssueUnsubscribeLink(ctx context.Context, userID string, category identity.UnsubscribeCategory) (string, error) {
	if s.d.Unsubscriptions == nil || !s.d.Unsubscriptions.Unsubscribable(category) {
		return "", fmt.Errorf("identity: unknown unsubscribe category %q", category)
	}
	now := s.d.Clock.Now()
	secret, hash := NewToken(EmailUnsubscribeTokenPrefix)
	t := identity.UserToken{
		ID: s.d.IDs.NewID(), UserID: userID, Purpose: identity.PurposeEmailUnsubscribe, UnsubscribeCategory: category,
		TokenHash: hash, ExpiresAt: now.Add(EmailUnsubscribeTTL), IssuedAt: now,
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.InsertUserToken(ctx, tx, t)
	})
	if err != nil {
		return "", err
	}
	return s.link(PathMailUnsubscribe, secret), nil
}

// unsubscribeToken returns the unsubscribe token of secret while it works: issued as an
// unsubscribe token and not expired. It fails with ErrInvalidToken otherwise.
func (s *Service) unsubscribeToken(ctx context.Context, secret string) (identity.UserToken, error) {
	if !strings.HasPrefix(secret, EmailUnsubscribeTokenPrefix) {
		return identity.UserToken{}, ErrInvalidToken
	}
	t, err := s.d.Repository.FindUserTokenByHash(ctx, HashToken(secret))
	if errors.Is(err, ErrNotFound) {
		return identity.UserToken{}, ErrInvalidToken
	}
	if err != nil {
		return identity.UserToken{}, err
	}
	if t.Purpose != identity.PurposeEmailUnsubscribe || t.Expired(s.d.Clock.Now()) {
		return identity.UserToken{}, ErrInvalidToken
	}
	return t, nil
}

// CheckUnsubscribeToken returns the category of email that an unsubscribe link turns off,
// without changing anything. It fails with ErrInvalidToken for a token that does not work.
func (s *Service) CheckUnsubscribeToken(ctx context.Context, secret string) (identity.UnsubscribeCategory, error) {
	t, err := s.unsubscribeToken(ctx, secret)
	if err != nil {
		return "", err
	}
	return t.UnsubscribeCategory, nil
}

// Unsubscribe turns off the emails of the category of an unsubscribe link and returns the
// category. Using a link again changes nothing more. It fails with ErrInvalidToken for a token
// that does not work, for a category that is no longer unsubscribable, and for a user who no
// longer has a local account.
func (s *Service) Unsubscribe(ctx context.Context, secret string) (identity.UnsubscribeCategory, error) {
	t, err := s.unsubscribeToken(ctx, secret)
	if err != nil {
		return "", err
	}
	if s.d.Unsubscriptions == nil || !s.d.Unsubscriptions.Unsubscribable(t.UnsubscribeCategory) {
		return "", ErrInvalidToken
	}
	err = s.d.Unsubscriptions.Unsubscribe(ctx, t.UserID, t.UnsubscribeCategory)
	if errors.Is(err, ErrNotFound) {
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", err
	}
	return t.UnsubscribeCategory, nil
}
