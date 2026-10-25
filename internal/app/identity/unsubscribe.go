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

// Unsubscribing from email (ADR-0086 §4). Every notification or announcement email carries its
// own link (user_tokens, purpose email_unsubscribe), issued by the job that sends it, that turns
// off the preference of its category: the List-Unsubscribe header and the link of the
// signature. The link works without signing in, any number of times, for
// EmailUnsubscribeTTL. The authpages package serves it: the one-click POST of RFC 8058 turns the
// preference off, and the GET only asks, so that mail scanners that fetch links change nothing.

const (
	// EmailUnsubscribeTTL is how long an unsubscribe link works.
	EmailUnsubscribeTTL = 90 * 24 * time.Hour
	// EmailUnsubscribeTokenPrefix marks unsubscribe tokens for secret scanners.
	EmailUnsubscribeTokenPrefix = "my_eut_"
	// PathMailUnsubscribe is the endpoint of the unsubscribe links, with the token in the query
	// parameter "token". The authpages package serves it.
	PathMailUnsubscribe = "/mail/unsubscribe"
)

// IssueUnsubscribeLink stores a new unsubscribe token of a user for the emails of category and
// returns the link that carries it. The secret is only in the link: the caller puts it in the
// email it sends and nowhere else.
func (s *Service) IssueUnsubscribeLink(ctx context.Context, userID string, category identity.UnsubscribeCategory) (string, error) {
	if category != identity.UnsubscribeModeratorNotifications {
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

// Unsubscribe turns off the preference of the category of an unsubscribe link, through the
// change of the user's preferences (UpdatePreferences), and returns the category. Using a link
// again changes nothing more. It fails with ErrInvalidToken for a token that does not work and
// for a user who no longer has preferences.
func (s *Service) Unsubscribe(ctx context.Context, secret string) (identity.UnsubscribeCategory, error) {
	t, err := s.unsubscribeToken(ctx, secret)
	if err != nil {
		return "", err
	}
	off := false
	var in PreferencesUpdate
	switch t.UnsubscribeCategory {
	case identity.UnsubscribeModeratorNotifications:
		in.EmailModeratorNotifications = &off
	default:
		return "", ErrInvalidToken
	}
	_, err = s.UpdatePreferences(ctx, t.UserID, in)
	if errors.Is(err, ErrNotFound) {
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", err
	}
	return t.UnsubscribeCategory, nil
}
