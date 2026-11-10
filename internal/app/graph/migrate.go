// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
// The followers of a page move together in one short transaction, whose statements do not grow
// with them, and each blocking or muting account is one short transaction. Each step is safe to
// run again: a follower that moved no longer follows the source, a block or mute that exists is
// kept, and the notification has one dedupe key per move.

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

// EventFollowMigrated follows the migration of each local follower of a moved account, and
// EventFollowsMigrated that of a page of them.
const (
	EventFollowMigrated  = "graph.follow_migrated"
	EventFollowsMigrated = "graph.follows_migrated"
)

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

// FollowsMigrated is FollowMigrated of the followers of a page of a migration, emitted once for
// all of them; a module that projects FollowMigrated projects it too, as each of Migrations.
type FollowsMigrated struct{ Migrations []FollowMigrated }

// EventType implements uow.Event.
func (FollowsMigrated) EventType() string { return EventFollowsMigrated }

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
// target that is not active (deleted meanwhile), or a move no longer in force, ends the migration
// with nothing done.
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
	// A move that was canceled or replaced moves nobody more, when a dead job runs again.
	switch state, err := s.d.Accounts.MoveState(ctx, st.SourceAccountID); {
	case errors.Is(err, appaccount.ErrNotFound):
		return MigrationStep{}, true, nil
	case err != nil:
		return MigrationStep{}, false, err
	case state.MoveID != st.MoveID:
		return MigrationStep{}, true, nil
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
		followers := make([]string, len(edges))
		for i, e := range edges {
			followers[i], last = e.FollowerAccountID, e.FollowID
		}
		if err := s.migrateFollowers(ctx, st, target, followers); err != nil {
			return MigrationStep{}, false, err
		}
		n = len(edges)
	case MigrateBlocksPhase:
		edges, err := s.d.Repository.ListBlockers(ctx, st.SourceAccountID, st.After, MigrateBatch)
		if err != nil {
			return MigrationStep{}, false, err
		}
		blockers := make([]string, len(edges))
		for i, e := range edges {
			blockers[i], last = e.BlockerAccountID, e.BlockID
		}
		if err := s.migrateBlocks(ctx, st, blockers); err != nil {
			return MigrationStep{}, false, err
		}
		n = len(edges)
	case MigrateMutesPhase:
		mutes, err := s.d.Repository.ListMuters(ctx, st.SourceAccountID, s.d.Clock.Now(), st.After, MigrateBatch)
		if err != nil {
			return MigrationStep{}, false, err
		}
		for _, m := range mutes {
			last = m.AccountID
		}
		if err := s.migrateMutes(ctx, st, mutes); err != nil {
			return MigrationStep{}, false, err
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

// actorsBatch is an Accounts that reads many accounts at once, as *appaccount.Service does.
type actorsBatch interface {
	FindByIDs(ctx context.Context, ids []string) (map[string]appaccount.Actor, error)
}

// migratingAccounts returns those of ids that the migration moves (migratingAccount): with one
// read of the accounts when Accounts can, one by one otherwise.
func (s *Service) migratingAccounts(ctx context.Context, st MigrationStep, ids []string) ([]string, error) {
	var out []string
	b, ok := s.d.Accounts.(actorsBatch)
	if !ok {
		for _, id := range ids {
			_, moves, err := s.migratingAccount(ctx, st, id)
			if err != nil {
				return nil, err
			}
			if moves {
				out = append(out, id)
			}
		}
		return out, nil
	}
	actors, err := b.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if a, found := actors[id]; found && id != st.TargetAccountID && a.Lifecycle.Active() && a.Account.IsLocal() && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out, nil
}

// migrateFollowers is migrateFollower of each of followerIDs, together: one transaction, whose
// statements do not grow with them, moves the follows of all of them, and one FollowsCreated,
// one FollowsTerminated, and one FollowsMigrated tell the projectors.
func (s *Service) migrateFollowers(ctx context.Context, st MigrationStep, target appaccount.Actor, followerIDs []string) error {
	followers, err := s.migratingAccounts(ctx, st, followerIDs)
	if err != nil || len(followers) == 0 {
		return err
	}
	repo := s.d.Repository
	domainMuted := map[string]bool{}
	if !target.Account.IsLocal() {
		if domainMuted, err = repo.DomainMutersAmong(ctx, followers, graph.MutedDomains(target.Account.Domain)); err != nil {
			return err
		}
	}
	locked := !target.Account.IsLocal() || (target.Profile != nil && target.Profile.Locked)
	now := kernel.Normalize(s.d.Clock.Now())
	newFollows := make(map[string]graph.Follow, len(followers))
	for _, id := range followers {
		f := graph.Follow{ID: s.d.IDs.NewID(), FollowerAccountID: id, FolloweeAccountID: st.TargetAccountID, RequestedAt: now}
		f.URI = graph.FollowURI(s.d.Host, f.ID)
		newFollows[id] = f
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		pairs := make([]string, 0, 2*len(followers))
		others := make([]string, 0, 2*len(followers))
		for _, id := range followers {
			pairs, others = append(pairs, id, id), append(others, st.SourceAccountID, st.TargetAccountID)
		}
		if err := repo.LockPairs(ctx, tx, pairs, others); err != nil {
			return err
		}
		edges, err := repo.ListEdgesBetween(ctx, tx, pairs, others)
		if err != nil {
			return err
		}
		from, to := map[string]graph.Edge{}, map[string]graph.Edge{}
		var fromIDs []string
		for _, e := range edges {
			switch e.FolloweeAccountID {
			case st.SourceAccountID:
				if e.State == graph.StateActive {
					from[e.FollowerAccountID] = e
					fromIDs = append(fromIDs, e.FollowID)
				}
			case st.TargetAccountID:
				to[e.FollowerAccountID] = e
			}
		}
		if len(from) == 0 {
			return nil // the follows ended, or were migrated, meanwhile
		}
		fromFollows, err := repo.FollowsByIDs(ctx, tx, fromIDs)
		if err != nil {
			return err
		}
		prefs, err := repo.PreferencesOf(ctx, tx, fromIDs)
		if err != nil {
			return err
		}
		blocking, blockedBy, err := repo.BlockEdgesWithTx(ctx, tx, st.TargetAccountID, followers)
		if err != nil {
			return err
		}
		var (
			created     FollowsCreated
			ended       FollowsTerminated
			migrated    FollowsMigrated
			inserted    []graph.Follow
			insertedPrf []graph.Preferences
			accepted    []string
			endedIDs    []string
		)
		for _, id := range followers {
			e, live := from[id]
			fromFollow, ok := fromFollows[e.FollowID]
			if !live || !ok {
				continue
			}
			ev := FollowMigrated{AccountID: id, SourceAccountID: st.SourceAccountID, TargetAccountID: st.TargetAccountID, MoveID: st.MoveID, At: now}
			switch {
			case blockedBy[id]: // the follower blocks the target
				ev.Outcome = MigrationBlocking
			case blocking[id]: // the target blocks the follower
				ev.Outcome = MigrationBlockedBy
			case domainMuted[id]:
				ev.Outcome = MigrationDomainMuted
			}
			if ev.Outcome != "" {
				migrated.Migrations = append(migrated.Migrations, ev)
				continue
			}
			if toEdge, ok := to[id]; ok {
				ev.Outcome = MigrationFollowed
				if toEdge.State != graph.StateActive {
					ev.Outcome = MigrationRequested
				}
			} else {
				f := newFollows[id]
				p, ok := prefs[fromFollow.ID]
				if !ok {
					p = graph.DefaultPreferences(f.ID)
				}
				p.FollowID = f.ID
				inserted, insertedPrf = append(inserted, f), append(insertedPrf, p)
				if locked {
					ev.Outcome = MigrationRequested
					created.Requested = append(created.Requested, FollowRequested{Follow: f, Preferences: p})
				} else {
					ev.Outcome = MigrationFollowed
					accepted = append(accepted, f.ID)
					a := graph.Acceptance{FollowID: f.ID, AcceptedBy: graph.AcceptedByAuto, AcceptedAt: f.RequestedAt}
					created.Followed = append(created.Followed, Followed{Follow: f, Acceptance: a, Preferences: p})
				}
			}
			endedIDs = append(endedIDs, fromFollow.ID)
			ended.Terminations = append(ended.Terminations, FollowTerminated{
				Follow: fromFollow, PreviousState: graph.StateActive,
				Termination: graph.Termination{FollowID: fromFollow.ID, Reason: graph.TerminatedMoved, TerminatedAt: now},
			})
			migrated.Migrations = append(migrated.Migrations, ev)
		}
		if len(inserted) > 0 {
			if err := repo.InsertFollows(ctx, tx, inserted); err != nil {
				return err
			}
			if err := repo.UpsertManyPreferences(ctx, tx, insertedPrf); err != nil {
				return err
			}
			if err := repo.InsertAutoAcceptances(ctx, tx, accepted, now); err != nil {
				return err
			}
			if err := tx.Emit(ctx, created); err != nil {
				return err
			}
		}
		if len(endedIDs) > 0 {
			if err := repo.InsertTerminations(ctx, tx, endedIDs, graph.TerminatedMoved, now); err != nil {
				return err
			}
			if err := tx.Emit(ctx, ended); err != nil {
				return err
			}
		}
		if len(migrated.Migrations) == 0 {
			return nil
		}
		return tx.Emit(ctx, migrated)
	})
}

// migrateBlocks makes each of blockerIDs, which block the source, block the target too, together:
// the blocks of a page are inserted, their follows with the target ended (as blockIn ends them),
// and one BlocksCreated tells the projectors, with a number of statements that does not grow with
// them. A blocker that blocks the target already is left as it is.
func (s *Service) migrateBlocks(ctx context.Context, st MigrationStep, blockerIDs []string) error {
	blockers, err := s.migratingAccounts(ctx, st, blockerIDs)
	if err != nil || len(blockers) == 0 {
		return err
	}
	now := kernel.Normalize(s.d.Clock.Now())
	fresh := make(map[string]graph.Block, len(blockers))
	for _, id := range blockers {
		b := graph.Block{ID: s.d.IDs.NewID(), BlockerAccountID: id, BlockedAccountID: st.TargetAccountID, CreatedAt: now}
		b.URI = graph.BlockURI(s.d.Host, b.ID)
		if err := b.Validate(); err != nil {
			return err
		}
		fresh[id] = b
	}
	repo := s.d.Repository
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		targets := make([]string, len(blockers))
		for i := range blockers {
			targets[i] = st.TargetAccountID
		}
		if err := repo.LockPairs(ctx, tx, blockers, targets); err != nil {
			return err
		}
		_, blocking, err := repo.BlockEdgesWithTx(ctx, tx, st.TargetAccountID, blockers)
		if err != nil {
			return err
		}
		var bs []graph.Block
		ev := BlocksCreated{}
		others := map[string][]string{}
		for _, id := range blockers {
			if blocking[id] {
				continue // the account blocks the target already
			}
			bs = append(bs, fresh[id])
			ev.Blocks = append(ev.Blocks, Blocked{Block: fresh[id]})
			others[id] = []string{st.TargetAccountID}
		}
		if len(bs) == 0 {
			return nil
		}
		if err := repo.InsertBlocks(ctx, tx, bs); err != nil {
			return err
		}
		if _, err := s.endAmong(ctx, tx, others, SeverBoth, graph.TerminatedBlocked, true, now); err != nil {
			return err
		}
		return tx.Emit(ctx, ev)
	})
}

// migrateMutes makes the account of each of ms, which mutes the source, mute the target with the
// same settings and expiry, unless it already mutes the target, together: one statement inserts
// the mutes of a page and one MutesCreated tells the projectors.
func (s *Service) migrateMutes(ctx context.Context, st MigrationStep, ms []graph.Mute) error {
	ids := make([]string, len(ms))
	for i, m := range ms {
		ids[i] = m.AccountID
	}
	muters, err := s.migratingAccounts(ctx, st, ids)
	if err != nil || len(muters) == 0 {
		return err
	}
	now := kernel.Normalize(s.d.Clock.Now())
	var fresh []graph.Mute
	for _, m := range ms {
		if !slices.Contains(muters, m.AccountID) {
			continue
		}
		mute := graph.Mute{
			AccountID: m.AccountID, TargetAccountID: st.TargetAccountID, HideNotifications: m.HideNotifications,
			ExpiresAt: m.ExpiresAt, CreatedAt: now,
		}
		if err := mute.Validate(); err != nil {
			if errors.Is(err, graph.ErrMuteExpiry) {
				continue // the mute of the source expired meanwhile
			}
			return err
		}
		fresh = append(fresh, mute)
	}
	if len(fresh) == 0 {
		return nil
	}
	repo := s.d.Repository
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		accounts := make([]string, len(fresh))
		for i, m := range fresh {
			accounts[i] = m.AccountID
		}
		had, err := repo.ActiveMutesOfTargetTx(ctx, tx, st.TargetAccountID, accounts, now)
		if err != nil {
			return err
		}
		// An account that already mutes the target keeps its own settings.
		fresh = slices.DeleteFunc(fresh, func(m graph.Mute) bool { _, ok := had[m.AccountID]; return ok })
		if len(fresh) == 0 {
			return nil
		}
		if err := repo.UpsertMutes(ctx, tx, fresh); err != nil {
			return err
		}
		ev := MutesCreated{Mutes: make([]Muted, len(fresh))}
		for i, m := range fresh {
			ev.Mutes[i] = Muted{Mute: m}
		}
		return tx.Emit(ctx, ev)
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
