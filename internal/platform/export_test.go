// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"crypto"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	domainaccount "github.com/mycroblog/my/internal/domain/account"
)

// The tests of this package assemble the app hundreds of times, and each account they create
// would hash a password with the production Argon2id cost and generate an RSA key: most of the time
// of a test. They use the lowest cost and one key per algorithm instead (ADR-0153 §2); the
// production values are tested by the identity and account packages.
func init() {
	argon2Params = identity.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
	var keys sync.Map
	generateKey = func(alg domainaccount.KeyAlgorithm) (crypto.Signer, error) {
		if k, ok := keys.Load(alg); ok {
			return k.(crypto.Signer), nil
		}
		k, err := account.GenerateKey(alg)
		if err != nil {
			return nil, err
		}
		k2, _ := keys.LoadOrStore(alg, k)
		return k2.(crypto.Signer), nil
	}
}

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
