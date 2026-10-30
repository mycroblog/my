// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/rivo/uniseg"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// Lists (ADR-0122, data-model §4.6): an account groups the accounts it actively follows into
// lists that only it sees, and reads a timeline of each. The timeline module owns them: the
// owner's commands write lists, list_names, and list_members, the projector of
// graph.follow_terminated takes an ended follow's followee out of every list of the follower, and
// only the owner reads them. Every lookup of a list that another account owns answers as for a
// list that does not exist (ErrNotFound), so that its existence does not show.

// List is a list (lists with its current name, list_names).
type List struct {
	ID             string
	OwnerAccountID string
	Name           string
	CreatedAt      time.Time
}

// ListMember is a member of a list (list_members): an account that the owner actively follows.
type ListMember struct {
	ListID          string
	MemberAccountID string
	OwnerAccountID  string
	AddedAt         time.Time
}

// ListTimelineQuery selects a page of the timeline of a list that Window.ViewerAccountID owns.
type ListTimelineQuery struct {
	Window
	ListID string
	// Floor bounds the read window as HomeQuery.Floor does.
	Floor string
}

// Page sizes of the members of a list.
const (
	DefaultListMembersLimit = 40
	MaxListMembersLimit     = 80
)

// Graphemes counts the grapheme clusters of s under Unicode text segmentation (UAX #29), the
// counter that list names are validated with (timeline.ValidListName).
func Graphemes(s string) int { return uniseg.GraphemeClusterCount(s) }

var _ timeline.GraphemeCounter = Graphemes

// listName trims the white space around name and validates the rest.
func listName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !timeline.ValidListName(name, Graphemes) {
		return "", timeline.ErrInvalidListName
	}
	return name, nil
}

// owned returns the list id of owner inside tx, ErrNotFound when it does not exist or another
// account owns it.
func (s *Service) owned(ctx context.Context, tx uow.Tx, owner, id string) (List, error) {
	l, err := s.d.Repository.FindList(ctx, tx, id)
	if err != nil {
		return List{}, err
	}
	if l.OwnerAccountID != owner {
		return List{}, ErrNotFound
	}
	return l, nil
}

// CreateList creates a list of owner named name (trimmed). It fails with
// timeline.ErrInvalidListName for a name that is not one and timeline.ErrTooManyLists when the
// owner already has timeline.MaxLists lists.
func (s *Service) CreateList(ctx context.Context, owner, name string) (List, error) {
	name, err := listName(name)
	if err != nil {
		return List{}, err
	}
	var l List
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		n, err := s.d.Repository.CountLists(ctx, tx, owner)
		if err != nil {
			return err
		}
		if n >= timeline.MaxLists {
			return timeline.ErrTooManyLists
		}
		l = List{ID: s.d.IDs.NewID(), OwnerAccountID: owner, Name: name, CreatedAt: kernel.Normalize(s.d.Clock.Now())}
		return s.d.Repository.InsertList(ctx, tx, l)
	})
	if err != nil {
		return List{}, err
	}
	return l, nil
}

// RenameList gives the list id of owner the name name (trimmed). It fails with ErrNotFound for a
// list that owner does not own and timeline.ErrInvalidListName for a name that is not one.
func (s *Service) RenameList(ctx context.Context, owner, id, name string) (List, error) {
	name, err := listName(name)
	if err != nil {
		return List{}, err
	}
	var l List
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		if l, err = s.owned(ctx, tx, owner, id); err != nil {
			return err
		}
		l.Name = name
		return s.d.Repository.RenameList(ctx, tx, id, name)
	})
	if err != nil {
		return List{}, err
	}
	return l, nil
}

// DeleteList deletes the list id of owner with its members. It fails with ErrNotFound for a list
// that owner does not own.
func (s *Service) DeleteList(ctx context.Context, owner, id string) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.owned(ctx, tx, owner, id); err != nil {
			return err
		}
		return s.d.Repository.DeleteList(ctx, tx, id)
	})
}

// List returns the list id of owner, ErrNotFound for a list that owner does not own.
func (s *Service) List(ctx context.Context, owner, id string) (List, error) {
	l, err := s.d.Repository.List(ctx, id)
	if err != nil {
		return List{}, err
	}
	if l.OwnerAccountID != owner {
		return List{}, ErrNotFound
	}
	return l, nil
}

// Lists returns the lists of owner, oldest first.
func (s *Service) Lists(ctx context.Context, owner string) ([]List, error) {
	return s.d.Repository.ListsOf(ctx, owner)
}

// ListsWithMember returns the lists of owner that have member, oldest first.
func (s *Service) ListsWithMember(ctx context.Context, owner, member string) ([]List, error) {
	ids, err := s.d.Repository.ListIDsWithMember(ctx, owner, member)
	if err != nil || len(ids) == 0 {
		return []List{}, err
	}
	lists, err := s.d.Repository.ListsOf(ctx, owner)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(lists, func(l List) bool { return !slices.Contains(ids, l.ID) }), nil
}

// AddListMember adds member to the list id of owner; adding a member again changes nothing. It
// fails with ErrNotFound for a list that owner does not own, timeline.ErrNotFollowed for an
// account that owner does not actively follow (a pending follow request is not enough), and
// timeline.ErrTooManyListMembers when the list has timeline.MaxListMembers members.
func (s *Service) AddListMember(ctx context.Context, owner, id, member string) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.owned(ctx, tx, owner, id); err != nil {
			return err
		}
		following, err := s.d.Repository.Following(ctx, tx, owner, member)
		if err != nil {
			return err
		}
		if !following {
			return timeline.ErrNotFollowed
		}
		inserted, err := s.d.Repository.InsertListMember(ctx, tx, ListMember{
			ListID: id, MemberAccountID: member, OwnerAccountID: owner, AddedAt: kernel.Normalize(s.d.Clock.Now()),
		})
		if err != nil || !inserted {
			return err
		}
		// Counting after the insert keeps adding an existing member idempotent on a full list;
		// the error rolls the insert back.
		n, err := s.d.Repository.CountListMembers(ctx, tx, id)
		if err == nil && n > timeline.MaxListMembers {
			err = timeline.ErrTooManyListMembers
		}
		return err
	})
}

// RemoveListMember takes member out of the list id of owner; a member that is not in it changes
// nothing. It fails with ErrNotFound for a list that owner does not own.
func (s *Service) RemoveListMember(ctx context.Context, owner, id, member string) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.owned(ctx, tx, owner, id); err != nil {
			return err
		}
		return s.d.Repository.DeleteListMember(ctx, tx, id, member)
	})
}

// ListMembers returns the account IDs of up to limit members of the list id of owner after the
// member after ("" starts at the first), in the order of their IDs. It fails with ErrNotFound for
// a list that owner does not own.
func (s *Service) ListMembers(ctx context.Context, owner, id, after string, limit int) ([]string, error) {
	if _, err := s.List(ctx, owner, id); err != nil {
		return nil, err
	}
	return s.d.Repository.ListMembers(ctx, id, after, limit)
}

// ListTimeline returns the timeline of the list id of a logged-in viewer (ADR-0122 §4): the rules
// of Home with the authors (posters and boosters) limited to the members of the list, so never the
// viewer's own posts. It reads in the widening windows of Home. It fails with ErrNotFound for a
// list that the viewer does not own.
func (s *Service) ListTimeline(ctx context.Context, viewer appcontent.Viewer, id string, q Query) (Page, error) {
	if !loggedIn(viewer) {
		return Page{}, ErrLoginRequired
	}
	if _, err := s.List(ctx, viewer.AccountID, id); err != nil {
		return Page{}, err
	}
	w := s.window(viewer, q)
	base := w.Now
	if t, ok := kernel.UUIDv7Time(q.Before); ok && t.Before(base) {
		base = t
	}
	windows := timeline.HomeWindows()
	var rows []Entry
	for i := 0; ; i++ {
		lq := ListTimelineQuery{Window: w, ListID: id}
		if i < len(windows) {
			lq.Floor = kernel.UUIDv7Floor(base.Add(-windows[i]))
		}
		var err error
		if rows, err = s.d.Repository.ListListEntries(ctx, lq); err != nil {
			return Page{}, err
		}
		if len(rows) >= w.Limit || lq.Floor == "" {
			break
		}
	}
	return s.page(ctx, appcontent.ContextHome, viewer, w, rows)
}

// EntryLists returns the IDs of the lists of a logged-in viewer whose timelines list e, an entry
// that the viewer's home timeline lists (HomeEntry): those that have its author as a member. A
// list timeline applies the rules of home to its members, so a home entry by a member is on it.
func (s *Service) EntryLists(ctx context.Context, viewer appcontent.Viewer, e Entry) ([]string, error) {
	if !loggedIn(viewer) {
		return nil, ErrLoginRequired
	}
	if e.AuthorAccountID == viewer.AccountID {
		return nil, nil
	}
	return s.d.Repository.ListIDsWithMember(ctx, viewer.AccountID, e.AuthorAccountID)
}
