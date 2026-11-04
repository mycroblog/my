// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// ErrCurrentBrowserSession reports an attempt to end the browser that asks, through the list of
// browsers; signing out ends it (ADR-0090 §12).
var ErrCurrentBrowserSession = errors.New("identity: the current browser session")

// SignedInBrowser is a browser signed in to a user's account (ADR-0090): one session with the
// first-party client's working token families that started from it. It is derived from them.
type SignedInBrowser struct {
	// SessionID identifies the browser in RevokeSignedInBrowser.
	SessionID string
	// Current reports that the token family that asks is one of this browser's.
	Current bool
	// IP and UserAgent are of the sign-in; "" when unknown or erased after their retention.
	IP        string
	UserAgent string
	// SignedInAt is when the session started.
	SignedInAt time.Time
	// LastUsedAt is the latest use of the session or its families, to the hour; nil when none
	// was recorded.
	LastUsedAt *time.Time
}

// SignedInBrowsers returns the browsers signed in to the user's account: the one with the token
// family currentFamilyID first, then the most recently used first.
func (s *Service) SignedInBrowsers(ctx context.Context, userID, currentFamilyID string) ([]SignedInBrowser, error) {
	now := s.d.Clock.Now()
	sessions, err := s.d.Repository.ListBrowserSessions(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	families, err := s.d.Repository.ListBrowserSessionFamilies(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	browsers := make([]SignedInBrowser, len(sessions))
	index := make(map[string]int, len(sessions))
	for i, session := range sessions {
		index[session.ID] = i
		browsers[i] = SignedInBrowser{
			SessionID: session.ID, IP: session.IP, UserAgent: session.UserAgent,
			SignedInAt: session.CreatedAt, LastUsedAt: session.LastSeenAt,
		}
	}
	for _, family := range families {
		i, ok := index[family.SessionID]
		if !ok {
			continue
		}
		b := &browsers[i]
		b.Current = b.Current || family.FamilyID == currentFamilyID
		if family.LastUsedAt != nil && (b.LastUsedAt == nil || family.LastUsedAt.After(*b.LastUsedAt)) {
			b.LastUsedAt = family.LastUsedAt
		}
	}
	slices.SortStableFunc(browsers, func(x, y SignedInBrowser) int {
		switch {
		case x.Current != y.Current:
			if x.Current {
				return -1
			}
			return 1
		case x.LastUsedAt != nil && y.LastUsedAt != nil && !x.LastUsedAt.Equal(*y.LastUsedAt):
			return y.LastUsedAt.Compare(*x.LastUsedAt)
		case x.LastUsedAt != nil && y.LastUsedAt == nil:
			return -1
		case x.LastUsedAt == nil && y.LastUsedAt != nil:
			return 1
		}
		return y.SignedInAt.Compare(x.SignedInAt)
	})
	return browsers, nil
}

// RevokeSignedInBrowser signs one of the user's browsers out (ADR-0090 §13). It fails with
// ErrNotFound for a session that is not a browser of the user's list, and with
// ErrCurrentBrowserSession for the browser with the token family currentFamilyID.
func (s *Service) RevokeSignedInBrowser(ctx context.Context, userID, currentFamilyID, sessionID string) error {
	browsers, err := s.SignedInBrowsers(ctx, userID, currentFamilyID)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(browsers, func(b SignedInBrowser) bool { return b.SessionID == sessionID })
	if i < 0 {
		return ErrNotFound
	}
	if browsers[i].Current {
		return ErrCurrentBrowserSession
	}
	return s.endBrowsers(ctx, userID, []string{sessionID})
}

// RevokeOtherSignedInBrowsers signs out every browser of the user but the one with the token
// family currentFamilyID (ADR-0090 §13).
func (s *Service) RevokeOtherSignedInBrowsers(ctx context.Context, userID, currentFamilyID string) error {
	browsers, err := s.SignedInBrowsers(ctx, userID, currentFamilyID)
	if err != nil {
		return err
	}
	var sessionIDs []string
	for _, b := range browsers {
		if !b.Current {
			sessionIDs = append(sessionIDs, b.SessionID)
		}
	}
	if len(sessionIDs) == 0 {
		return nil
	}
	return s.endBrowsers(ctx, userID, sessionIDs)
}

// endBrowsers ends the sessions and the first-party client's token families that started from
// them. Under the lock, a code exchange or a refresh of one of them either committed first, so
// that its token is revoked here, or comes later and finds the session or the token revoked.
func (s *Service) endBrowsers(ctx context.Context, userID string, sessionIDs []string) error {
	now := s.d.Clock.Now()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.LockTokenGrants(ctx, tx, userID); err != nil {
			return err
		}
		for _, id := range sessionIDs {
			rev := identity.SessionRevocation{SessionID: id, Reason: identity.RevokedLogout, RevokedAt: now}
			if err := s.d.Repository.InsertSessionRevocation(ctx, tx, rev); err != nil {
				return err
			}
			if err := s.d.Repository.RevokeFirstPartyTokensOfSession(ctx, tx, id, identity.TokenRevokedUser, now); err != nil {
				return err
			}
		}
		return nil
	})
}
