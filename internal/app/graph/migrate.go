// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// The migration of the relationships of an account that moved (ADR-0089, FR-414). On
// account.moved, the job JobMigrateFollows walks, page by page, the local accounts that follow,
// block, or mute the account that moved (the source):
//
//   - A local follower that neither blocks the target nor is blocked by it, and does not mute
//     the target's domain, follows the target with the preferences of its follow of the source
//     (a Follow to a remote or locked target awaits its approval), and its follow of the source
//     ends with the reason moved (an Undo Follow to a remote source). Any other follower keeps
//     following the source. Each follower is told with FollowMigrated, which the notification
//     module turns into account_moved.
//   - A local account that blocks the source blocks the target too.
//   - A local account that mutes the source mutes the target too, with the same handling of
//     notifications and expiry, unless it already mutes the target.
//
// Each account is one short transaction, and each step is safe to run again: a follower that
// moved no longer follows the source, a block or mute that exists is kept, and the notification
// has one dedupe key per move.

const (
	// JobMigrateFollows migrates one page of the relationships of a moved account; its payload
	// is migratePayload. Its idempotency key is the pair of the source and the target.
	JobMigrateFollows = "graph.migrate_follows"
	// MigrateBatch is the number of accounts that one page of JobMigrateFollows reads.
	MigrateBatch = 100
	// MigratePageInterval is the wait between the pages of JobMigrateFollows, which spreads the
	// Follow and Undo Follow deliveries of a large account.
	MigratePageInterval = 10 * time.Second
)

// EventFollowMigrated follows the migration of each local follower of a moved account.
const EventFollowMigrated = "graph.follow_migrated"

// MigrationOutcome is what became of a local account's follow of an account that moved.
type MigrationOutcome string

// Migration outcomes. With MigrationFollowed and MigrationRequested the account follows the
// target instead of the source; with the others it still follows the source.
const (
	// MigrationFollowed: the account actively follows the target.
	MigrationFollowed MigrationOutcome = "followed"
	// MigrationRequested: the account's follow of the target awaits the target's approval.
	MigrationRequested MigrationOutcome = "requested"
	// MigrationBlocking: the account blocks the target.
	MigrationBlocking MigrationOutcome = "blocking"
	// MigrationBlockedBy: the target blocks the account.
	MigrationBlockedBy MigrationOutcome = "blocked_by"
	// MigrationDomainMuted: the account mutes the target's domain.
	MigrationDomainMuted MigrationOutcome = "domain_muted"
)

// FollowMigrated is emitted when the follow of a local account of a moved account is migrated,
// or left in place, in the transaction that does it.
type FollowMigrated struct {
	// AccountID is the local follower; SourceAccountID the account that moved, TargetAccountID
	// the account it moved to, and MoveID the move (account_moves).
	AccountID       string
	SourceAccountID string
	TargetAccountID string
	MoveID          string
	Outcome         MigrationOutcome
	At              time.Time
}

// EventType implements uow.Event.
func (FollowMigrated) EventType() string { return EventFollowMigrated }

// MigrationPhase is the relationships that a step of a migration walks.
type MigrationPhase string

// Migration phases, in their order.
const (
	MigrateFollowsPhase MigrationPhase = "follows"
	MigrateBlocksPhase  MigrationPhase = "blocks"
	MigrateMutesPhase   MigrationPhase = "mutes"
)

// MigrationStep is a page of the migration of a moved account's relationships.
type MigrationStep struct {
	MoveID          string         `json:"move_id"`
	SourceAccountID string         `json:"source_account_id"`
	TargetAccountID string         `json:"target_account_id"`
	Phase           MigrationPhase `json:"phase,omitempty"`
	// After is the position after which the page starts: a follow ID in the follows phase, a
	// block ID in the blocks phase, and an account ID in the mutes phase; "" for the first.
	After string `json:"after,omitempty"`
}

func (st MigrationStep) phase() MigrationPhase {
	if st.Phase == "" {
		return MigrateFollowsPhase
	}
	return st.Phase
}

// job returns the JobMigrateFollows of st, to run at runAt (zero for now). The first step's key
// is the pair of the source and the target, so that one move enqueues one migration; a later
// step's key adds its position.
func (st MigrationStep) job(runAt time.Time) (queue.NewJob, error) {
	b, err := json.Marshal(st)
	if err != nil {
		return queue.NewJob{}, err
	}
	key := JobMigrateFollows + ":" + st.SourceAccountID + ":" + st.TargetAccountID
	if st.Phase != "" {
		key += ":" + string(st.Phase) + ":" + st.After
	}
	return queue.NewJob{Type: JobMigrateFollows, Payload: b, IdempotencyKey: key, RunAt: runAt}, nil
}

// RegisterMoveProjector subscribes the graph module to account.moved: the transaction that
// records a move enqueues the first step of JobMigrateFollows through Deps.Outbox. Without an
// outbox it enqueues nothing.
func (s *Service) RegisterMoveProjector(r *uow.Projectors) {
	r.Register(appaccount.EventAccountMoved, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appaccount.Moved)
		if !ok || s.d.Outbox == nil {
			return nil
		}
		job, err := MigrationStep{MoveID: ev.Move.ID, SourceAccountID: ev.Move.AccountID, TargetAccountID: ev.Move.TargetAccountID}.job(time.Time{})
		if err != nil {
			return err
		}
		return s.d.Outbox.EnqueueTx(ctx, tx, job)
	})
}

// MigrateFollows runs every step of the migration of the relationships of sourceID, which
// moved to targetID with the move moveID, at once. The job runs the steps one by one instead.
func (s *Service) MigrateFollows(ctx context.Context, moveID, sourceID, targetID string) error {
	return s.migrateFrom(ctx, MigrationStep{MoveID: moveID, SourceAccountID: sourceID, TargetAccountID: targetID})
}

// migrateFrom runs st and every step after it.
func (s *Service) migrateFrom(ctx context.Context, st MigrationStep) error {
	for {
		next, done, err := s.MigrateStep(ctx, st)
		if err != nil || done {
			return err
		}
		st = next
	}
}

// MigrateStep migrates one page of st and returns the next step, done after the last one. A
// target that is not active (deleted meanwhile) ends the migration with nothing done.
func (s *Service) MigrateStep(ctx context.Context, st MigrationStep) (next MigrationStep, done bool, err error) {
	if st.MoveID == "" || st.SourceAccountID == "" || st.TargetAccountID == "" || st.SourceAccountID == st.TargetAccountID {
		return MigrationStep{}, false, fmt.Errorf("graph: migration step %+v: %w", st, ErrNotFound)
	}
	target, err := s.active(ctx, st.TargetAccountID)
	if errors.Is(err, ErrAccountUnavailable) {
		return MigrationStep{}, true, nil
	}
	if err != nil {
		return MigrationStep{}, false, err
	}
	next = st
	next.Phase = st.phase()
	var last string
	var n int
	switch next.Phase {
	case MigrateFollowsPhase:
		edges, err := s.d.Repository.ListFollowers(ctx, st.SourceAccountID, graph.StateActive, st.After, MigrateBatch)
		if err != nil {
			return MigrationStep{}, false, err
		}
		for _, e := range edges {
			if err := s.migrateFollower(ctx, st, target, e.FollowerAccountID); err != nil {
				return MigrationStep{}, false, err
			}
			last = e.FollowID
		}
		n = len(edges)
	case MigrateBlocksPhase:
		edges, err := s.d.Repository.ListBlockers(ctx, st.SourceAccountID, st.After, MigrateBatch)
		if err != nil {
			return MigrationStep{}, false, err
		}
		for _, e := range edges {
			if err := s.migrateBlock(ctx, st, e.BlockerAccountID); err != nil {
				return MigrationStep{}, false, err
			}
			last = e.BlockID
		}
		n = len(edges)
	case MigrateMutesPhase:
		mutes, err := s.d.Repository.ListMuters(ctx, st.SourceAccountID, s.d.Clock.Now(), st.After, MigrateBatch)
		if err != nil {
			return MigrationStep{}, false, err
		}
		for _, m := range mutes {
			if err := s.migrateMute(ctx, st, m); err != nil {
				return MigrationStep{}, false, err
			}
			last = m.AccountID
		}
		n = len(mutes)
	default:
		return MigrationStep{}, false, fmt.Errorf("graph: unknown migration phase %q: %w", st.Phase, ErrNotFound)
	}
	if n == MigrateBatch {
		next.After = last
		return next, false, nil
	}
	next.After = ""
	switch next.Phase {
	case MigrateFollowsPhase:
		next.Phase = MigrateBlocksPhase
	case MigrateBlocksPhase:
		next.Phase = MigrateMutesPhase
	default:
		return MigrationStep{}, true, nil
	}
	return next, false, nil
}

// migratingAccount returns the local, active account id, false for one that the migration
// skips: a remote or inactive account, or the target itself.
func (s *Service) migratingAccount(ctx context.Context, st MigrationStep, id string) (appaccount.Actor, bool, error) {
	if id == st.TargetAccountID {
		return appaccount.Actor{}, false, nil
	}
	a, err := s.active(ctx, id)
	switch {
	case errors.Is(err, ErrAccountUnavailable):
		return appaccount.Actor{}, false, nil
	case err != nil:
		return appaccount.Actor{}, false, err
	}
	return a, a.Account.IsLocal(), nil
}

// migrateFollower moves followerID's active follow of the source to the target, or leaves it in
// place when the follower blocks the target, is blocked by it, or mutes its domain, and emits
// FollowMigrated.
func (s *Service) migrateFollower(ctx context.Context, st MigrationStep, target appaccount.Actor, followerID string) error {
	if _, ok, err := s.migratingAccount(ctx, st, followerID); err != nil || !ok {
		return err
	}
	domainMuted := false
	if !target.Account.IsLocal() {
		var err error
		if domainMuted, err = s.MutesDomain(ctx, followerID, target.Account.Domain); err != nil {
			return err
		}
	}
	locked := !target.Account.IsLocal() || (target.Profile != nil && target.Profile.Locked)
	now := kernel.Normalize(s.d.Clock.Now())
	f := graph.Follow{ID: s.d.IDs.NewID(), FollowerAccountID: followerID, FolloweeAccountID: st.TargetAccountID, RequestedAt: now}
	f.URI = graph.FollowURI(s.d.Host, f.ID)
	repo := s.d.Repository
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ev := FollowMigrated{
			AccountID: followerID, SourceAccountID: st.SourceAccountID, TargetAccountID: st.TargetAccountID, MoveID: st.MoveID, At: now,
		}
		if err := repo.LockPair(ctx, tx, followerID, st.SourceAccountID); err != nil {
			return err
		}
		if err := repo.LockPair(ctx, tx, followerID, st.TargetAccountID); err != nil {
			return err
		}
		edge, from, err := s.liveFollow(ctx, tx, followerID, st.SourceAccountID)
		if errors.Is(err, ErrNotFound) || (err == nil && edge.State != graph.StateActive) {
			return nil // the follow ended, or was migrated, meanwhile
		}
		if err != nil {
			return err
		}
		if ev.Outcome, err = s.keepsFollow(ctx, tx, followerID, st.TargetAccountID, domainMuted); err != nil {
			return err
		}
		if ev.Outcome != "" {
			return tx.Emit(ctx, ev)
		}
		switch to, err := repo.Edge(ctx, tx, followerID, st.TargetAccountID); {
		case err == nil:
			ev.Outcome = MigrationFollowed
			if to.State != graph.StateActive {
				ev.Outcome = MigrationRequested
			}
		case errors.Is(err, ErrNotFound):
			if ev.Outcome, err = s.followTargetIn(ctx, tx, f, from, locked); err != nil {
				return err
			}
		default:
			return err
		}
		term := FollowTerminated{Termination: graph.Termination{Reason: graph.TerminatedMoved, TerminatedAt: now}}
		if _, err := s.endIn(ctx, tx, term, followerID, st.SourceAccountID, false); err != nil {
			return err
		}
		return tx.Emit(ctx, ev)
	})
}

// keepsFollow returns why followerID does not follow targetID in place of the source, "" when
// it does.
func (s *Service) keepsFollow(ctx context.Context, tx uow.Tx, followerID, targetID string, domainMuted bool) (MigrationOutcome, error) {
	blocking, err := found(s.d.Repository.BlockEdge(ctx, tx, followerID, targetID))
	if err != nil || blocking {
		return MigrationBlocking, err
	}
	blockedBy, err := found(s.d.Repository.BlockEdge(ctx, tx, targetID, followerID))
	if err != nil || blockedBy {
		return MigrationBlockedBy, err
	}
	if domainMuted {
		return MigrationDomainMuted, nil
	}
	return "", nil
}

// followTargetIn records f, the follow of the target in place of the follow from, with from's
// preferences: pending when the target is remote or locked, active at once otherwise.
func (s *Service) followTargetIn(ctx context.Context, tx uow.Tx, f, from graph.Follow, locked bool) (MigrationOutcome, error) {
	repo := s.d.Repository
	prefs, err := repo.Preferences(ctx, tx, from.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		prefs = graph.DefaultPreferences(f.ID)
	case err != nil:
		return "", err
	}
	prefs.FollowID = f.ID
	if err := repo.InsertFollow(ctx, tx, f); err != nil {
		return "", err
	}
	if err := repo.UpsertPreferences(ctx, tx, prefs); err != nil {
		return "", err
	}
	if locked {
		return MigrationRequested, tx.Emit(ctx, FollowRequested{Follow: f, Preferences: prefs})
	}
	a := graph.Acceptance{FollowID: f.ID, AcceptedBy: graph.AcceptedByAuto, AcceptedAt: f.RequestedAt}
	if err := repo.InsertAcceptance(ctx, tx, a); err != nil {
		return "", err
	}
	return MigrationFollowed, tx.Emit(ctx, Followed{Follow: f, Acceptance: a, Preferences: prefs})
}

// migrateBlock makes blockerID, which blocks the source, block the target too.
func (s *Service) migrateBlock(ctx context.Context, st MigrationStep, blockerID string) error {
	if _, ok, err := s.migratingAccount(ctx, st, blockerID); err != nil || !ok {
		return err
	}
	b := graph.Block{ID: s.d.IDs.NewID(), BlockerAccountID: blockerID, BlockedAccountID: st.TargetAccountID, CreatedAt: kernel.Normalize(s.d.Clock.Now())}
	b.URI = graph.BlockURI(s.d.Host, b.ID)
	if err := b.Validate(); err != nil {
		return err
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.blockIn(ctx, tx, b) })
}

// migrateMute makes the account of m, which mutes the source, mute the target with the same
// settings and expiry, unless it already mutes the target.
func (s *Service) migrateMute(ctx context.Context, st MigrationStep, m graph.Mute) error {
	if _, ok, err := s.migratingAccount(ctx, st, m.AccountID); err != nil || !ok {
		return err
	}
	now := kernel.Normalize(s.d.Clock.Now())
	mute := graph.Mute{
		AccountID: m.AccountID, TargetAccountID: st.TargetAccountID, HideNotifications: m.HideNotifications,
		ExpiresAt: m.ExpiresAt, CreatedAt: now,
	}
	if err := mute.Validate(); err != nil {
		if errors.Is(err, graph.ErrMuteExpiry) {
			return nil // the mute of the source expired meanwhile
		}
		return err
	}
	repo := s.d.Repository
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		switch _, err := repo.Mute(ctx, tx, m.AccountID, st.TargetAccountID, now); {
		case err == nil:
			return nil // the account already mutes the target: its own settings stay
		case !errors.Is(err, ErrNotFound):
			return err
		}
		if err := repo.UpsertMute(ctx, tx, mute); err != nil {
			return err
		}
		return tx.Emit(ctx, Muted{Mute: mute})
	})
}

// handleMigrate runs one step of JobMigrateFollows and enqueues the next one after
// MigratePageInterval; without a job queue it runs the remaining steps at once.
func (s *Service) handleMigrate(ctx context.Context, j queue.Job) queue.Result {
	var st MigrationStep
	if err := json.Unmarshal(j.Payload, &st); err != nil {
		return queue.Fail(fmt.Errorf("graph: payload of %s: %w", JobMigrateFollows, err))
	}
	next, done, err := s.MigrateStep(ctx, st)
	switch {
	case errors.Is(err, ErrNotFound):
		return queue.Fail(err)
	case err != nil:
		return queue.Retry(0, err)
	case done:
		return queue.Done()
	case s.d.Jobs == nil:
		if err := s.migrateFrom(ctx, next); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	}
	job, err := next.job(s.d.Clock.Now().Add(MigratePageInterval))
	if err != nil {
		return queue.Fail(err)
	}
	if err := s.d.Jobs.Enqueue(ctx, job); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}
