// SPDX-License-Identifier: AGPL-3.0-only

package moderationtest

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/pkg/testkit"
)

// stubModerationStatuses stands in for the content module in ModerationFlows: live statuses
// by ID, the statuses deleted by moderation (status ID → action ID), and the statuses each
// mark_statuses_sensitive action marks.
type stubModerationStatuses struct {
	mu      sync.Mutex
	live    map[string]appcontent.Status
	deleted map[string]string
	marked  map[string][]string
}

func (s *stubModerationStatuses) add(st appcontent.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.live[st.ID] = st
}

// Find implements appmoderation.Statuses.
func (s *stubModerationStatuses) Find(_ context.Context, id string) (appcontent.Post, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.live[id]
	if !ok {
		return appcontent.Post{}, appcontent.ErrNotFound
	}
	return appcontent.Post{Status: st}, nil
}

// DeleteByModeration implements appmoderation.Statuses.
func (s *stubModerationStatuses) DeleteByModeration(_ context.Context, _ uow.Tx, st appcontent.Status, actionID string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.live[st.ID]; !ok {
		return appcontent.ErrAlreadyDeleted
	}
	delete(s.live, st.ID)
	s.deleted[st.ID] = actionID
	return nil
}

// MarkSensitiveByModeration implements appmoderation.Statuses.
func (s *stubModerationStatuses) MarkSensitiveByModeration(_ context.Context, _ uow.Tx, statusID, actionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.marked[actionID] = append(s.marked[actionID], statusID)
	return nil
}

// ClearSensitivityOf implements appmoderation.Statuses.
func (s *stubModerationStatuses) ClearSensitivityOf(_ context.Context, _ uow.Tx, actionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.marked, actionID)
	return nil
}

// stubLifecycles stands in for the account module in ModerationFlows: the due time of each
// account's scheduled deletion and the action that scheduled it, the deleted accounts, and the
// cancellations (account ID -> the action that canceled).
type stubLifecycles struct {
	mu       sync.Mutex
	due      map[string]time.Time
	action   string
	deleted  map[string]bool
	canceled map[string]string
}

// CancelDeletionByModeration implements appmoderation.Lifecycles.
func (s *stubLifecycles) CancelDeletionByModeration(_ context.Context, _ uow.Tx, accountID, actionID, _ string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleted[accountID] {
		return false, appaccount.ErrAlreadyDeleted
	}
	if _, ok := s.due[accountID]; !ok {
		return false, nil
	}
	delete(s.due, accountID)
	if s.canceled == nil {
		s.canceled = map[string]string{}
	}
	s.canceled[accountID] = actionID
	return true, nil
}

// ScheduleDeletionByModeration implements appmoderation.Lifecycles.
func (s *stubLifecycles) ScheduleDeletionByModeration(_ context.Context, _ uow.Tx, accountID, actionID string, due time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.due[accountID], s.action = due, actionID
	return nil
}

// moderationStatusesAndBans checks the actions on statuses and the ban on a local account: they
// name and record statuses of the target only, delete_statuses deletes them, a lifted
// mark_statuses_sensitive no longer marks them, neither takes an end, and a ban restricts like a
// suspension, schedules the deletion of the account, and is never lifted.
func moderationStatusesAndBans(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	status := func(author string) appcontent.Status {
		st := content.Status{ID: e.ids.NewID(), URI: "https://social.example/statuses/" + e.ids.NewID(), AuthorAccountID: author, IsLocal: true}
		e.statuses.add(st)
		return st
	}
	s1, s2, other := status(e.local), status(e.local), status(e.remote)
	issue := func(kind moderation.ActionKind, d time.Duration, ids ...string) (moderation.Action, error) {
		return e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
			TargetAccountID: e.local, Kind: kind, ReasonCategory: moderation.CategoryViolation, Duration: d, StatusIDs: ids,
		})
	}
	for _, c := range []struct {
		name string
		kind moderation.ActionKind
		d    time.Duration
		ids  []string
		want error
	}{
		{"no statuses", moderation.ActionDeleteStatuses, 0, nil, moderation.ErrInvalidActionStatuses},
		{"a repeated status", moderation.ActionDeleteStatuses, 0, []string{s1.ID, s1.ID}, moderation.ErrInvalidActionStatuses},
		{"statuses on warn", moderation.ActionWarn, 0, []string{s1.ID}, moderation.ErrInvalidActionStatuses},
		{"an unknown status", moderation.ActionDeleteStatuses, 0, []string{s1.ID, e.ids.NewID()}, appmoderation.ErrInvalidStatusReference},
		{"another account's status", moderation.ActionMarkStatusesSensitive, 0, []string{other.ID}, appmoderation.ErrInvalidStatusReference},
		{"an end on statuses", moderation.ActionMarkStatusesSensitive, time.Hour, []string{s1.ID}, moderation.ErrActionNeverExpires},
		{"an end on a ban", moderation.ActionBan, time.Hour, nil, moderation.ErrActionNeverExpires},
	} {
		if _, err := issue(c.kind, c.d, c.ids...); !errors.Is(err, c.want) {
			t.Errorf("IssueAccountAction(%s) = %v, want %v", c.name, err, c.want)
		}
	}
	if v, err := e.svc.AccountActions(ctx, appmoderation.AccountActionFilter{AccountID: e.local}, 10); err != nil || len(v) != 0 {
		t.Errorf("actions after refused ones = %+v, %v; want none", v, err)
	}

	// mark_statuses_sensitive marks its statuses until it is lifted, and records them in order.
	mark, err := issue(moderation.ActionMarkStatusesSensitive, 0, s2.ID, s1.ID)
	sorted := slices.Sorted(slices.Values([]string{s1.ID, s2.ID}))
	if err != nil || !reflect.DeepEqual(mark.StatusIDs, sorted) {
		t.Fatalf("IssueAccountAction(mark_statuses_sensitive) = %+v, %v", mark, err)
	}
	if got := e.statuses.marked[mark.ID]; !reflect.DeepEqual(got, sorted) {
		t.Errorf("marked = %v, want %v", got, sorted)
	}
	if v, err := e.svc.AccountAction(ctx, mark.ID); err != nil || !reflect.DeepEqual(v.Action, mark) {
		t.Errorf("AccountAction(mark) = %+v, %v; want %+v", v.Action, err, mark)
	}
	if err := e.svc.LiftAccountAction(ctx, mark.ID, testkit.NamedID("user-1"), ""); err != nil {
		t.Fatalf("LiftAccountAction(mark) = %v", err)
	}
	if got, ok := e.statuses.marked[mark.ID]; ok {
		t.Errorf("marked after the lift = %v, want none", got)
	}

	// delete_statuses deletes its statuses; a deleted status can no longer be named.
	del, err := issue(moderation.ActionDeleteStatuses, 0, s1.ID)
	if err != nil || e.statuses.deleted[s1.ID] != del.ID {
		t.Fatalf("IssueAccountAction(delete_statuses) = %+v, %v; deleted %v", del, err, e.statuses.deleted)
	}
	if _, err := issue(moderation.ActionDeleteStatuses, 0, s1.ID); !errors.Is(err, appmoderation.ErrInvalidStatusReference) {
		t.Errorf("IssueAccountAction(a deleted status) = %v, want ErrInvalidStatusReference", err)
	}
	list, err := e.svc.AccountActions(ctx, appmoderation.AccountActionFilter{AccountID: e.local}, 10)
	if err != nil || len(list) != 2 || !reflect.DeepEqual(list[0].Action.StatusIDs, []string{s1.ID}) ||
		!reflect.DeepEqual(list[1].Action.StatusIDs, sorted) {
		t.Errorf("AccountActions = %+v, %v; want both actions with their statuses", list, err)
	}

	// A ban restricts like a suspension, schedules the deletion, and is never lifted.
	ban, err := issue(moderation.ActionBan, 0)
	if err != nil || ban.ExpiresAt != nil || ban.StatusIDs != nil {
		t.Fatalf("IssueAccountAction(ban) = %+v, %v", ban, err)
	}
	if due := e.lifecycles.due[e.local]; !due.Equal(ban.IssuedAt.Add(moderation.BanDeletionDelay)) || e.lifecycles.action != ban.ID {
		t.Errorf("scheduled deletion = %v by %s, want %v by %s", due, e.lifecycles.action, ban.IssuedAt.Add(moderation.BanDeletionDelay), ban.ID)
	}
	if hidden, err := e.accounts.ProfileHidden(ctx, e.local); err != nil || !hidden {
		t.Errorf("ProfileHidden(banned) = %v, %v; want true", hidden, err)
	}
	if login, err := e.accounts.LoginRestricted(ctx, e.local); err != nil || !login {
		t.Errorf("LoginRestricted(banned) = %v, %v; want true", login, err)
	}
	if err := e.svc.LiftAccountAction(ctx, ban.ID, testkit.NamedID("user-1"), "mistake"); !errors.Is(err, appmoderation.ErrNotLiftable) {
		t.Errorf("LiftAccountAction(ban) = %v, want ErrNotLiftable", err)
	}
	if v, err := e.svc.AccountAction(ctx, ban.ID); err != nil || v.State != moderation.ActionActive || v.Lift != nil {
		t.Errorf("ban after a refused lift = %+v, %v; want active", v, err)
	}
}
