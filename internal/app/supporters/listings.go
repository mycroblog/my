// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
)

// The supporter list (ADR-0070 §13, §15): a member of a plan (a subscription in a member state)
// chooses to be listed, nobody is listed by default, and the choice goes with the plan's role
// (the transition that takes the role away deletes it, a change of plan keeps it). The list
// shows the members of the plans that are not retired, by the plan's rank and newest first
// within a plan, and never an amount, an interval, or a date; who may read it is the instance's
// setting, which the interface checks.

// ErrNotMember reports a user without a subscription in a member state (active, canceling, or
// past due), who has no choice of the supporter list.
var ErrNotMember = errors.New("supporters: the user is not a member of a plan")

// Listing is a member on the supporter list.
type Listing struct {
	UserID string
	PlanID string
	// Rank is the rank of the member's plan, and ListedAt when the member chose to be listed:
	// with UserID, the position of the member in the list.
	Rank     int
	ListedAt time.Time
}

// ListingFilter narrows a page of the supporter list.
type ListingFilter struct {
	// PlanID narrows to the members of a plan; "" for every plan.
	PlanID string
	// After is the last member of the previous page; nil for the first page.
	After *Listing
	Limit int
}

// ListedSupporter is a member on the supporter list with the account to show.
type ListedSupporter struct {
	Listing
	// AccountID is the member's local account; "" when the user has none any more.
	AccountID string
}

// SupporterList is a page of the supporter list with the plans for its headings.
type SupporterList struct {
	// Plans are the plans that are not retired, by rank.
	Plans []PlanRecord
	// Members are the page's members; Last is the last member read, the cursor of the next
	// page, which is nil when there is none.
	Members []ListedSupporter
	Last    *Listing
}

// member reports inside tx whether a user has a subscription in a member state, locking it so
// that its change of state and the user's choice run one after the other.
func (s *Service) member(ctx context.Context, tx uow.Tx, userID string) (bool, error) {
	id, err := s.d.Repository.LiveSubscriptionID(ctx, tx, userID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rec, err := s.d.Repository.LockSubscription(ctx, tx, id)
	if err != nil {
		return false, err
	}
	return rec.Current.State.Member(), nil
}

// Listed reports whether a user chose to be on the supporter list. It fails with ErrDisabled
// while the program is disabled and with ErrNotMember for a user who is not a member of a plan.
func (s *Service) Listed(ctx context.Context, userID string) (bool, error) {
	enabled, err := s.Enabled(ctx)
	if err != nil {
		return false, err
	}
	if !enabled {
		return false, ErrDisabled
	}
	v, err := s.liveView(ctx, userID)
	if errors.Is(err, ErrNoLiveSubscription) || (err == nil && !v.Current.State.Member()) {
		return false, ErrNotMember
	}
	if err != nil {
		return false, err
	}
	return s.d.Repository.HasListing(ctx, nil, userID)
}

// SetListed puts a member of a plan on the supporter list or takes them off it, and returns the
// choice. It fails as Listed does; taking off a user who is not listed changes nothing.
func (s *Service) SetListed(ctx context.Context, userID string, listed bool) (bool, error) {
	enabled, err := s.Enabled(ctx)
	if err != nil {
		return false, err
	}
	if !enabled {
		return false, ErrDisabled
	}
	now := s.now()
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ok, err := s.member(ctx, tx, userID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotMember
		}
		if listed {
			return s.d.Repository.InsertListing(ctx, tx, userID, now)
		}
		return s.d.Repository.DeleteListing(ctx, tx, userID)
	})
	if err != nil {
		return false, err
	}
	return listed, nil
}

// SupporterList returns a page of the supporter list (GET /instance/supporters); ErrDisabled
// while the program is disabled. A plan that the filter names and that is not on the list
// (unknown or retired) gives an empty page.
func (s *Service) SupporterList(ctx context.Context, f ListingFilter) (SupporterList, error) {
	enabled, err := s.Enabled(ctx)
	if err != nil {
		return SupporterList{}, err
	}
	if !enabled {
		return SupporterList{}, ErrDisabled
	}
	plans, err := s.d.Repository.ListPlans(ctx, nil)
	if err != nil {
		return SupporterList{}, err
	}
	out := SupporterList{Plans: slices.DeleteFunc(plans, func(p PlanRecord) bool { return p.Retired })}
	rows, err := s.d.Repository.ListListings(ctx, ListingFilter{PlanID: f.PlanID, After: f.After, Limit: f.Limit + 1})
	if err != nil {
		return SupporterList{}, err
	}
	if len(rows) > f.Limit {
		rows = rows[:f.Limit]
		last := rows[len(rows)-1]
		out.Last = &last
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.UserID
	}
	accounts, err := s.d.Users.AccountIDs(ctx, ids)
	if err != nil {
		return SupporterList{}, err
	}
	out.Members = make([]ListedSupporter, len(rows))
	for i, r := range rows {
		out.Members[i] = ListedSupporter{Listing: r, AccountID: accounts[r.UserID]}
	}
	return out, nil
}
