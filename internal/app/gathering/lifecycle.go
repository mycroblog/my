// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"slices"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// CreateInput is a new event as its organizer gives it. The empty visibility, disclosure, and
// speakers of a voice place take their defaults, and the empty time zone the organizer's
// (ADR-0161 §5).
type CreateInput struct {
	Settings gathering.Settings
	Schedule gathering.Schedule
}

// Patch changes an event; a nil field keeps its value. Places replaces every place.
type Patch struct {
	Title           *string
	Description     *string
	Visibility      *gathering.Visibility
	TimeZone        *string
	PlaceDisclosure *gathering.PlaceDisclosure
	Capacity        *int
	Places          *[]gathering.Place
	Schedule        *gathering.Schedule
}

// withDefaults fills the defaults of the empty settings.
func withDefaults(s gathering.Settings) gathering.Settings {
	if s.Visibility == "" {
		s.Visibility = gathering.DefaultVisibility
	}
	if s.PlaceDisclosure == "" {
		s.PlaceDisclosure = gathering.DefaultPlaceDisclosure
	}
	s.Places = placesWithDefaults(s.Places)
	return s
}

func placesWithDefaults(places []gathering.Place) []gathering.Place {
	out := slices.Clone(places)
	for i, p := range out {
		if p.Kind == gathering.PlaceVoice && p.Speakers == "" {
			out[i].Speakers = gathering.DefaultSpeakers
		}
	}
	return out
}

// organizer checks that accountID may organize an event (ADR-0161 §3), and that it may hold one
// in a voice chat room when voice is set.
func (s *Service) organizer(ctx context.Context, accountID string, voice bool) (Organizer, error) {
	o, err := s.d.Members.Organizer(ctx, accountID)
	if err != nil {
		return Organizer{}, err
	}
	everyone, err := s.d.Members.EveryoneCreates(ctx)
	if err != nil {
		return Organizer{}, err
	}
	if !gathering.CanOrganize(gathering.OrganizerFacts{
		Local: o.Local, Active: o.Active, Limited: o.Limited, EveryoneCreates: everyone, MayCreate: o.MayCreate,
	}) {
		return Organizer{}, ErrNotEligible
	}
	if voice && !o.MayHostVoice {
		return Organizer{}, ErrVoiceUnavailable
	}
	return o, nil
}

// Create creates an event of accountID with a fixed date (ADR-0161 §5). It fails with
// ErrNotEligible, ErrVoiceUnavailable, a *gathering.SettingsError, and a *kernel.RateLimitedError
// past CreationLimit.
func (s *Service) Create(ctx context.Context, accountID string, in CreateInput) (View, error) {
	settings := withDefaults(in.Settings)
	_, voice := settings.Place(gathering.PlaceVoice)
	o, err := s.organizer(ctx, accountID, voice)
	if err != nil {
		return View{}, err
	}
	if settings.TimeZone == "" {
		settings.TimeZone = o.TimeZone
	}
	if err := settings.Validate(Graphemes); err != nil {
		return View{}, err
	}
	now := s.now()
	if err := in.Schedule.Validate(now); err != nil {
		return View{}, err
	}
	if err := s.take(ctx, CreationLimit, accountID); err != nil {
		return View{}, err
	}
	g := Gathering{ID: s.d.IDs.NewID(), OrganizerAccountID: accountID, CreatedAt: now}
	rev := Revision{ID: s.d.IDs.NewID(), GatheringID: g.ID, Settings: settings, RevisedByAccountID: accountID, RevisedAt: now}
	conf := Confirmation{ID: s.d.IDs.NewID(), GatheringID: g.ID, Schedule: utc(in.Schedule), ConfirmedByAccountID: accountID, ConfirmedAt: now}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertGathering(ctx, tx, g); err != nil {
			return err
		}
		if err := s.d.Repository.InsertRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := s.d.Repository.InsertConfirmation(ctx, tx, conf); err != nil {
			return err
		}
		return tx.Emit(ctx, Created{Gathering: g, Revision: rev, Confirmation: conf})
	})
	if err != nil {
		return View{}, err
	}
	return s.Get(ctx, accountID, g.ID)
}

// utc returns sc in UTC, as the events keep their times (ADR-0161 §5).
func utc(sc gathering.Schedule) gathering.Schedule {
	return gathering.Schedule{StartsAt: sc.StartsAt.UTC(), EndsAt: sc.EndsAt.UTC()}
}

// patched returns cur changed by p, and whether anything changed.
func patched(cur gathering.Settings, p Patch) (gathering.Settings, bool) {
	next := cur
	next.Places = slices.Clone(cur.Places)
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	set(&next.Title, p.Title)
	set(&next.Description, p.Description)
	set(&next.TimeZone, p.TimeZone)
	if p.Visibility != nil {
		next.Visibility = *p.Visibility
	}
	if p.PlaceDisclosure != nil {
		next.PlaceDisclosure = *p.PlaceDisclosure
	}
	if p.Capacity != nil {
		next.Capacity = *p.Capacity
	}
	if p.Places != nil {
		next.Places = placesWithDefaults(*p.Places)
	}
	changed := next.Title != cur.Title || next.Description != cur.Description || next.TimeZone != cur.TimeZone ||
		next.Visibility != cur.Visibility || next.PlaceDisclosure != cur.PlaceDisclosure || next.Capacity != cur.Capacity ||
		!slices.Equal(next.Places, cur.Places)
	return next, changed
}

// Update revises the settings and the date of the event id, by its organizer or a co-organizer
// (ADR-0161 §4, §5). Only the organizer changes the visibility (ErrOrganizerOnly); the organizer's
// eligibility is checked when the organizer revises (ErrNotEligible), and the organizer's right to
// a voice chat room when a voice place is added (ErrVoiceUnavailable). A new date is a new
// confirmation, checked as a new event's. It fails with ErrNotFound, ErrNotOrganizer, ErrClosed,
// and a *gathering.SettingsError.
func (s *Service) Update(ctx context.Context, accountID, id string, p Patch) (View, error) {
	v, err := s.organizing(ctx, accountID, id)
	if err != nil {
		return View{}, err
	}
	cur := v.Revision.Settings
	next, changed := patched(cur, p)
	if next.Visibility != cur.Visibility && !v.Viewer.Organizer {
		return View{}, ErrOrganizerOnly
	}
	_, hadVoice := cur.Place(gathering.PlaceVoice)
	_, voice := next.Place(gathering.PlaceVoice)
	switch {
	case v.Viewer.Organizer && (changed || p.Schedule != nil):
		if _, err := s.organizer(ctx, accountID, voice && !hadVoice); err != nil {
			return View{}, err
		}
	case voice && !hadVoice:
		// A co-organizer adds the room of the organizer, who must be able to host it.
		o, err := s.d.Members.Organizer(ctx, v.OrganizerAccountID)
		if err != nil {
			return View{}, err
		}
		if !o.MayHostVoice {
			return View{}, ErrVoiceUnavailable
		}
	}
	if err := next.Validate(Graphemes); err != nil {
		return View{}, err
	}
	now := s.now()
	var sched *gathering.Schedule
	if p.Schedule != nil && utc(*p.Schedule) != v.Confirmation.Schedule {
		sc := utc(*p.Schedule)
		if err := sc.Validate(now); err != nil {
			return View{}, err
		}
		sched = &sc
	}
	if !changed && sched == nil {
		return v, nil
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.LockGathering(ctx, tx, id); err != nil {
			return err
		}
		rec, err := s.d.Repository.FindGathering(ctx, tx, id)
		if err != nil {
			return err
		}
		if rec.Cancellation != nil {
			return ErrClosed
		}
		if changed {
			rev := Revision{ID: s.d.IDs.NewID(), GatheringID: id, Settings: next, RevisedByAccountID: accountID, RevisedAt: now}
			if err := s.d.Repository.InsertRevision(ctx, tx, rev); err != nil {
				return err
			}
			if err := tx.Emit(ctx, Revised{Revision: rev, Previous: rec.Revision}); err != nil {
				return err
			}
		}
		if sched != nil {
			conf := Confirmation{ID: s.d.IDs.NewID(), GatheringID: id, Schedule: *sched, ConfirmedByAccountID: accountID, ConfirmedAt: now}
			if err := s.d.Repository.InsertConfirmation(ctx, tx, conf); err != nil {
				return err
			}
			if err := tx.Emit(ctx, Rescheduled{Confirmation: conf, Previous: rec.Confirmation}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return View{}, err
	}
	return s.Get(ctx, accountID, id)
}

// Cancel cancels the event id, by its organizer, with the reason note ("" for none, at most
// gathering.MaxNoteLength characters). It fails with ErrNotFound, ErrNotOrganizer,
// ErrOrganizerOnly, ErrClosed for an event cancelled already, and a *gathering.SettingsError of
// the note.
func (s *Service) Cancel(ctx context.Context, accountID, id, note string) (View, error) {
	v, err := s.organizing(ctx, accountID, id)
	if err != nil {
		return View{}, err
	}
	if !v.Viewer.Organizer {
		return View{}, ErrOrganizerOnly
	}
	if !gathering.ValidNote(note) {
		return View{}, &gathering.SettingsError{Field: gathering.FieldNote}
	}
	c := Cancellation{GatheringID: id, Reason: gathering.CancelOrganizer, CancelledByAccountID: accountID, Note: note, CancelledAt: s.now()}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ok, err := s.d.Repository.InsertCancellation(ctx, tx, c)
		if err != nil {
			return err
		}
		if !ok {
			return ErrClosed
		}
		return tx.Emit(ctx, Cancelled{Cancellation: c})
	})
	if err != nil {
		return View{}, err
	}
	return s.Get(ctx, accountID, id)
}
