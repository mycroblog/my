// SPDX-License-Identifier: AGPL-3.0-only

package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	domain "github.com/mycroblog/my/internal/domain/identity"
)

// fullAppsRepository is the repository of an account that is at, and past, its cap of OAuth apps.
// The methods it does not override are not called.
type fullAppsRepository struct {
	identity.Repository
	owned int
}

func (r fullAppsRepository) CountOwnedOAuthClients(context.Context, uow.Tx, string) (int, error) {
	return r.owned, nil
}

func (r fullAppsRepository) ListOwnedOAuthClients(context.Context, string) ([]domain.OAuthClient, error) {
	return nil, nil
}

// The question whether users may register apps is answered by the setting alone: true and false as
// it says, its read error passed on, and true for an account at its cap, since the cap refuses the
// registration and not the right (ADR-0090 §1, #3206).
func TestMayRegisterOAuthApps(t *testing.T) {
	boom := errors.New("settings are down")
	tx := &recordingTx{}
	for _, tc := range []struct {
		name    string
		setting func(context.Context) (bool, error)
		want    bool
		wantErr error
	}{
		{"on, for an account at its cap of apps", func(context.Context) (bool, error) { return true, nil }, true, nil},
		{"off", func(context.Context) (bool, error) { return false, nil }, false, nil},
		{"the read fails", func(context.Context) (bool, error) { return false, boom }, false, boom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := identity.NewService(identity.Deps{
				UnitOfWork: directUnitOfWork{tx: tx}, Repository: fullAppsRepository{owned: 1 << 20},
				Clock: fixedClock(time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)), IDs: fixedIDs{},
				UserAppRegistration: tc.setting,
			})
			got, err := s.MayRegisterOAuthApps(t.Context())
			if got != tc.want || !errors.Is(err, tc.wantErr) {
				t.Errorf("MayRegisterOAuthApps = %t, %v; want %t, %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
