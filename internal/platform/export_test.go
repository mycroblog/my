// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// SetReadyCheck replaces the database part of the readiness check of a.
func SetReadyCheck(a *App, ready func(ctx context.Context) error) { a.ready = ready }

// NewHealthCache returns the Check of a healthCache over check.
func NewHealthCache(check func(ctx context.Context) error, clock kernel.Clock) func(ctx context.Context) (bool, time.Time) {
	return newHealthCache(check, clock).Check
}

// HealthCacheTTL is healthCacheTTL.
const HealthCacheTTL = healthCacheTTL

// SupporterMailbox is the mailbox of the supporter emails of an account: its address ID and
// whether the emails may go to it.
func SupporterMailbox(ctx context.Context, a *App, accountID string) (string, bool, error) {
	box, ok, err := supporterUsers{a}.SupporterMailbox(ctx, accountID)
	return box.EmailAddressID, ok, err
}
