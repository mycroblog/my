// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// Retention of the expired sign-in records (data-model §4.1, §8).
const (
	// JobPurgeAuthRecords deletes the expired sessions, authorization codes, token families, and
	// one-time tokens past their retention; no payload.
	JobPurgeAuthRecords = "identity.purge_auth_records"
	// AuthRecordPurgeInterval is the period of the purge's schedule.
	AuthRecordPurgeInterval = time.Hour
	// AuthRecordRetention is how long these records stay after they expire. A token family's
	// clock starts when its last token expires, so that the reuse of a rotated refresh token is
	// detected for as long as any token of the family works.
	AuthRecordRetention = 30 * 24 * time.Hour
	// AuthRecordPurgeBatch is the number of parent rows (families, codes, sessions, or tokens) one
	// transaction deletes; authRecordPurgeMaxBatches bounds the batches of one kind in one run, and
	// the next run continues.
	AuthRecordPurgeBatch      = 100
	authRecordPurgeMaxBatches = 20
)

// AuthRecordPurgeRepository is the part of the identity module's persistence that the purge uses.
// Each method deletes inside tx at most limit parent rows with their children and returns how many
// parents it deleted. The database adapters implement it and pass testkit.AccountErasureContract.
type AuthRecordPurgeRepository interface {
	// DeleteExpiredTokenFamilies deletes the token families whose every access and refresh token
	// expired before t, with their revocations, last use, and the authorization code that started
	// them with its redemption.
	DeleteExpiredTokenFamilies(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	// DeleteExpiredUnredeemedCodes deletes the authorization codes that expired before t without
	// being exchanged.
	DeleteExpiredUnredeemedCodes(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	// DeleteExpiredSessions deletes the sessions that expired before t with their revocations and
	// activity, except those that an authorization code still names. Their origins have a
	// retention of their own.
	DeleteExpiredSessions(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
	// DeleteExpiredUserTokens deletes the one-time tokens that expired before t with their
	// redemptions.
	DeleteExpiredUserTokens(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
}

// AuthRecordPurgeDeps are the ports of the purge.
type AuthRecordPurgeDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository AuthRecordPurgeRepository
	Clock      kernel.Clock
	// Batch is the number of parent rows per transaction; zero means AuthRecordPurgeBatch.
	Batch int
}

// AuthRecordPurger runs JobPurgeAuthRecords. Each batch runs in a transaction of its own. Token
// families go before sessions, because a session stays as long as a code of a live family names it.
type AuthRecordPurger struct{ d AuthRecordPurgeDeps }

// NewAuthRecordPurger returns the purge on d.
func NewAuthRecordPurger(d AuthRecordPurgeDeps) *AuthRecordPurger {
	if d.Batch <= 0 {
		d.Batch = AuthRecordPurgeBatch
	}
	return &AuthRecordPurger{d: d}
}

// RegisterJobs registers the JobPurgeAuthRecords handler and its hourly schedule.
func (p *AuthRecordPurger) RegisterJobs(h *queue.JobHandlerRegistry, s *queue.Scheduler) {
	h.Register(JobPurgeAuthRecords, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := p.Run(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	s.Add(queue.Schedule{Name: JobPurgeAuthRecords, Every: AuthRecordPurgeInterval, Job: queue.NewJob{Type: JobPurgeAuthRecords}})
}

// Run deletes the records past their retention, at most authRecordPurgeMaxBatches batches of each
// kind, and returns how many parent rows it deleted. It is safe to repeat.
func (p *AuthRecordPurger) Run(ctx context.Context) (int64, error) {
	before := p.d.Clock.Now().Add(-AuthRecordRetention)
	r := p.d.Repository
	steps := []struct {
		name string
		del  func(context.Context, uow.Tx, time.Time, int) (int64, error)
	}{
		{"token families", r.DeleteExpiredTokenFamilies},
		{"authorization codes", r.DeleteExpiredUnredeemedCodes},
		{"sessions", r.DeleteExpiredSessions},
		{"user tokens", r.DeleteExpiredUserTokens},
	}
	var total int64
	for _, s := range steps {
		for range authRecordPurgeMaxBatches {
			var n int64
			err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				var err error
				n, err = s.del(ctx, tx, before, p.d.Batch)
				return err
			})
			if err != nil {
				return total, fmt.Errorf("identity: purge %s: %w", s.name, err)
			}
			total += n
			if n < int64(p.d.Batch) {
				break
			}
		}
	}
	return total, nil
}
