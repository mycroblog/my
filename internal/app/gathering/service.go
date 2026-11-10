// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/rivo/uniseg"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The errors of the event use cases.
var (
	// ErrNotEligible reports an account that may not organize an event (ADR-0161 §3).
	ErrNotEligible = errors.New("gathering: the account may not organize events")
	// ErrNotOrganizer reports an operation of the organizers by someone who is neither the
	// organizer nor a co-organizer.
	ErrNotOrganizer = errors.New("gathering: only the organizer or a co-organizer may do this")
	// ErrOrganizerOnly reports an operation of the organizer by someone else: naming and removing
	// the co-organizers, changing the visibility, and cancelling (ADR-0161 §4).
	ErrOrganizerOnly = errors.New("gathering: only the organizer may do this")
	// ErrClosed reports a change of an event that was cancelled, or an answer after it started
	// (ADR-0161 §8).
	ErrClosed = errors.New("gathering: the event takes no more changes")
	// ErrFull reports an answer of going to an event whose attendees who answer going reached its
	// capacity (ADR-0161 §8).
	ErrFull = errors.New("gathering: the event is full")
	// ErrVoiceUnavailable reports a voice place that the organizer may not have: the voice chat is
	// off, or the organizer may not host its rooms (ADR-0161 §3).
	ErrVoiceUnavailable = errors.New("gathering: the organizer may not hold the event in a voice chat room")
	// ErrInvalidAnswer reports an answer that is not going, maybe, or declined.
	ErrInvalidAnswer = errors.New("gathering: invalid answer")
)

// CreationLimit bounds the events that each account creates (ADR-0161 §3).
var CreationLimit = kernel.Limit{Name: "gathering.create", Max: 10, Window: time.Hour}

// Member is what the events read of an account (ADR-0161 §3, §9).
type Member struct {
	// Local reports a local account of a user, not the instance actor.
	Local bool
	// Active reports an account that is neither suspended nor deleted.
	Active bool
	// Limited reports that a moderator limited the account.
	Limited bool
}

// Organizer is what the events read of an account that organizes one.
type Organizer struct {
	Member
	// MayCreate reports that the roles of the account's user grant create_events.
	MayCreate bool
	// MayHostVoice reports that the voice chat runs and that the account may host its rooms
	// (ADR-0100 §2), so that an event of it may be held in a voice chat room.
	MayHostVoice bool
	// TimeZone is the user's time zone (user_time_zones), "" for none.
	TimeZone string
}

// Members reads the accounts for the events from the account, moderation, identity, instance,
// and voice modules; the composition root implements it.
type Members interface {
	// Member returns the facts of an account; an unknown account is the zero Member.
	Member(ctx context.Context, accountID string) (Member, error)
	// Organizer returns the facts of an account that organizes an event; an unknown account is
	// the zero Organizer.
	Organizer(ctx context.Context, accountID string) (Organizer, error)
	// EveryoneCreates reports the instance setting event_creators = everyone.
	EveryoneCreates(ctx context.Context) (bool, error)
}

// Relations reads the follows and blocks between accounts from the graph module; the
// composition root implements it. Follows count only when accepted.
type Relations interface {
	// BlockedAmong returns which of ids are in a block relation, either way, with accountID.
	BlockedAmong(ctx context.Context, accountID string, ids []string) (map[string]bool, error)
	// Following returns which of ids accountID follows.
	Following(ctx context.Context, accountID string, ids []string) (map[string]bool, error)
	// FollowersAmong returns which of ids follow accountID.
	FollowersAmong(ctx context.Context, accountID string, ids []string) (map[string]bool, error)
}

// Deps are the dependencies of the Service.
type Deps struct {
	Repository Repository
	UnitOfWork uow.UnitOfWork
	Clock      kernel.Clock
	IDs        kernel.IDGenerator
	// RateLimiter bounds the creations (CreationLimit) and the invitations (InvitationLimit);
	// nil bounds neither.
	RateLimiter *kernel.RateLimiter
	Members     Members
	Relations   Relations
	// Outbox enqueues the jobs of the notifications and of gathering.tick in the transactions of
	// the commands, and Notifier creates the notifications (ADR-0161 §11, §12); with either nil the
	// events notify nobody.
	Outbox   queue.Outbox
	Notifier Notifier
	Logger   *slog.Logger
}

// Service runs the events (ADR-0161): creating, revising, and cancelling them, their
// visibility, places, invitations, answers and approvals, capacity, and co-organizers.
type Service struct{ d Deps }

// NewService returns the Service on d.
func NewService(d Deps) *Service {
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

func (s *Service) now() time.Time { return kernel.Normalize(s.d.Clock.Now()) }

// Graphemes counts the grapheme clusters of s under Unicode text segmentation (UAX #29), the
// counter that the settings of an event are validated with (gathering.Settings.Validate).
func Graphemes(s string) int { return uniseg.GraphemeClusterCount(s) }

var _ gathering.GraphemeCounter = Graphemes

// take counts one operation against l for key, and fails with a *kernel.RateLimitedError past it.
func (s *Service) take(ctx context.Context, l kernel.Limit, key string) error {
	if s.d.RateLimiter == nil {
		return nil
	}
	d, err := s.d.RateLimiter.Take(ctx, l, key)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return &kernel.RateLimitedError{RetryAfter: d.RetryAfter}
	}
	return nil
}

// ViewerState is what an event says of its viewer.
type ViewerState struct {
	Organizer   bool
	Coorganizer bool
	Invited     bool
	// Answer is the viewer's current answer, "" for none.
	Answer   gathering.Answer
	Approved bool
}

// View is an event as a viewer sees it: the record, what it says of the viewer, and the places
// of its current settings split into those the viewer sees and the kinds of those the viewer
// does not (ADR-0161 §9). A hidden place never leaves the Service.
type View struct {
	Record
	Viewer ViewerState
	// Places are the places that the viewer sees, in their order.
	Places []gathering.Place
	// HiddenPlaces are the kinds of the places that the viewer does not see, in their order.
	HiddenPlaces []gathering.PlaceKind
}

// Organizes reports whether the viewer is the organizer or a co-organizer.
func (v View) Organizes() bool { return v.Viewer.Organizer || v.Viewer.Coorganizer }

// viewer returns the policy's viewer for accountID.
func (s *Service) viewer(ctx context.Context, accountID string) (gathering.Viewer, error) {
	if accountID == "" {
		return gathering.Viewer{}, nil
	}
	m, err := s.d.Members.Member(ctx, accountID)
	if err != nil {
		return gathering.Viewer{}, err
	}
	return gathering.Viewer{AccountID: accountID, Local: m.Local, Active: m.Active}, nil
}

// facts gathers the facts of viewer for each of recs, by event, with one read of each source.
func (s *Service) facts(ctx context.Context, viewer gathering.Viewer, recs []Record) (map[string]gathering.Facts, map[string]ViewerFacts, error) {
	out := make(map[string]gathering.Facts, len(recs))
	if viewer.AccountID == "" || len(recs) == 0 {
		return out, map[string]ViewerFacts{}, nil
	}
	ids := make([]string, 0, len(recs))
	var people, organizers []string
	for _, r := range recs {
		ids = append(ids, r.ID)
		for _, id := range r.organizerIDs() {
			if !slices.Contains(people, id) {
				people = append(people, id)
			}
		}
		if !slices.Contains(organizers, r.OrganizerAccountID) {
			organizers = append(organizers, r.OrganizerAccountID)
		}
	}
	own, err := s.d.Repository.ViewerFacts(ctx, nil, viewer.AccountID, ids)
	if err != nil {
		return nil, nil, err
	}
	blocked, err := s.d.Relations.BlockedAmong(ctx, viewer.AccountID, people)
	if err != nil {
		return nil, nil, err
	}
	following, err := s.d.Relations.Following(ctx, viewer.AccountID, organizers)
	if err != nil {
		return nil, nil, err
	}
	followers, err := s.d.Relations.FollowersAmong(ctx, viewer.AccountID, organizers)
	if err != nil {
		return nil, nil, err
	}
	for _, r := range recs {
		f := gathering.Facts{
			Coorganizer: r.Coorganizes(viewer.AccountID), Invited: own[r.ID].Invited, Answered: own[r.ID].Answer != "",
			Approved: own[r.ID].Approved, Voted: own[r.ID].Voted, FollowsOrganizer: following[r.OrganizerAccountID],
			FollowedByOrganizer: followers[r.OrganizerAccountID],
		}
		for _, id := range r.organizerIDs() {
			// The organizers do not block themselves.
			if id != viewer.AccountID && blocked[id] {
				f.Blocked = true
			}
		}
		out[r.ID] = f
	}
	return out, own, nil
}

// present returns the views of recs that viewer sees, in their order.
func (s *Service) present(ctx context.Context, viewer gathering.Viewer, recs []Record) ([]View, error) {
	facts, own, err := s.facts(ctx, viewer, recs)
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(recs))
	for _, r := range recs {
		f := facts[r.ID]
		e := r.Event()
		if !gathering.CanSee(e, viewer, f) {
			continue
		}
		v := View{Record: r, Viewer: ViewerState{
			Organizer: viewer.AccountID == r.OrganizerAccountID, Coorganizer: f.Coorganizer, Invited: f.Invited,
			Answer: own[r.ID].Answer, Approved: f.Approved,
		}}
		for _, p := range r.Revision.Settings.Places {
			if gathering.CanSeePlace(e, p.Kind, viewer, f) {
				v.Places = append(v.Places, p)
			} else {
				v.HiddenPlaces = append(v.HiddenPlaces, p.Kind)
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// Get returns the event id as accountID sees it; ErrNotFound for an unknown event and for one
// that the account does not see.
func (s *Service) Get(ctx context.Context, accountID, id string) (View, error) {
	rec, err := s.d.Repository.FindGathering(ctx, nil, id)
	if err != nil {
		return View{}, err
	}
	viewer, err := s.viewer(ctx, accountID)
	if err != nil {
		return View{}, err
	}
	views, err := s.present(ctx, viewer, []Record{rec})
	if err != nil {
		return View{}, err
	}
	if len(views) == 0 {
		return View{}, ErrNotFound
	}
	return views[0], nil
}

// organizing returns the event id as accountID sees it, when the account organizes it or
// co-organizes it (ErrNotOrganizer otherwise) and it was not cancelled (ErrClosed).
func (s *Service) organizing(ctx context.Context, accountID, id string) (View, error) {
	v, err := s.Get(ctx, accountID, id)
	if err != nil {
		return View{}, err
	}
	if !v.Organizes() {
		return View{}, ErrNotOrganizer
	}
	if v.Cancellation != nil {
		return View{}, ErrClosed
	}
	return v, nil
}

// ListQuery asks for a page of the upcoming events.
type ListQuery struct {
	// After is the position of the last event of the previous page, nil for the first page.
	After *ListKey
	Limit int
}

// listBatch is how many events a list reads at a time, and listScans how many batches it reads
// at most for one page: the visibility is decided per viewer in the application (ADR-0161 §9),
// so a page of a viewer who sees few events stops early and its cursor goes on from there.
const (
	listBatch = 100
	listScans = 10
)

// List returns the events that accountID sees and that have not ended, by the nearest start
// first (ADR-0161 §9), and the position to read the next page from, nil after the last one.
func (s *Service) List(ctx context.Context, accountID string, q ListQuery) ([]View, *ListKey, error) {
	viewer, err := s.viewer(ctx, accountID)
	if err != nil {
		return nil, nil, err
	}
	now := s.now()
	after := q.After
	var out []View
	for range listScans {
		recs, err := s.d.Repository.UpcomingGatherings(ctx, nil, now, after, listBatch)
		if err != nil {
			return nil, nil, err
		}
		views, err := s.present(ctx, viewer, recs)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, views...)
		if len(out) > q.Limit {
			out = out[:q.Limit]
			last := out[len(out)-1]
			return out, &ListKey{StartsAt: last.Head.Schedule.StartsAt, GatheringID: last.ID}, nil
		}
		if len(recs) < listBatch {
			return out, nil, nil
		}
		last := recs[len(recs)-1]
		after = &ListKey{StartsAt: last.Head.Schedule.StartsAt, GatheringID: last.ID}
	}
	// The scans ran out before the page filled: the next page goes on after the last event read.
	return out, after, nil
}
