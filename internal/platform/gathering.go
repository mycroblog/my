// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/content"
	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/gathering"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/voice"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// setUpGathering builds the events of the local users (ADR-0161): the projectors of their
// projections, their notifications and gathering.tick, their voice chat rooms, and the Service of
// the client API. It comes after the voice chat, whose hosts it reads and whose rooms it opens,
// and after the notifications, which it creates, and after the statuses, whose board posts go with
// the records of the events.
func (a *App) setUpGathering(repo appgathering.Repository, statuses content.Repository) {
	appgathering.RegisterProjectors(a.Projectors, repo)
	appgathering.RegisterDueProjectors(a.Projectors, repo)
	outbox := a.Outbox
	if outbox == nil {
		// The inline queue has no transactional enqueue; its jobs are enqueued after the commit.
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	d := appgathering.Deps{
		Repository: repo, UnitOfWork: a.UnitOfWork, Clock: a.Clock, IDs: a.IDs, RateLimiter: a.RateLimiter,
		Members: gatheringMembers{a}, Relations: gatheringRelations{a}, Outbox: outbox, Logger: a.Logger,
		Retention: gatheringRetention{a}, Store: a.KV, Rooms: gatheringRooms{a},
	}
	if a.gatheringNotifier != nil {
		d.Notifier = a.gatheringNotifier
	}
	a.Events = appgathering.NewService(d)
	a.Events.RegisterNotificationProjectors(a.Projectors)
	a.Events.RegisterNotificationJobs(a.Handlers, a.Scheduler)
	a.Events.RegisterModerationProjectors(a.Projectors)
	a.Events.RegisterModerationJobs(a.Handlers, a.Scheduler)
	a.Events.RegisterRoomProjectors(a.Projectors)
	a.Events.RegisterRoomJobs(a.Handlers)
	// The organizer of an event whose room ended unattended hears of it (ADR-0161 §10); voice
	// does not know the events.
	a.Projectors.Register(appvoice.EventRoomEnded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		end := e.(appvoice.RoomEnded).Ending
		return a.Events.RoomEnded(ctx, tx, end.RoomID, end.Reason == voice.EndUnattended)
	})
	// The readers of a board hear of its new posts (ADR-0161 §11, §17); the gathering module does
	// not know the posts.
	a.Projectors.Register(content.EventBoardStatusPosted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(content.BoardStatusPosted)
		return a.Events.BoardPosted(ctx, tx, appgathering.BoardPost{
			StatusID: ev.Status.ID, GatheringID: ev.GatheringID, WriterAccountID: ev.Status.AuthorAccountID,
			MentionedReaderIDs: ev.MentionedReaderIDs, PostedAt: ev.Revision.RevisedAt,
		})
	})
	// The posts of the boards go with the records of their events (ADR-0161 §17): the content
	// module asks the gathering module which boards are due, and the gathering module does not
	// know the posts.
	a.BoardPurge = content.NewBoardPurge(content.BoardPurgeDeps{
		UnitOfWork: a.UnitOfWork, Repository: statuses, Boards: contentBoards{a}, Clock: a.Clock,
	})
	a.BoardPurge.RegisterJobs(a.Handlers, a.Scheduler)
}

// eventBoards lists and takes the posts of the boards of the events for the client API
// (clientapi.EventBoards): the status reader lists them, and the content service posts them.
type eventBoards struct{ a *App }

func (b eventBoards) Board(ctx context.Context, viewer content.Viewer, gatheringID, before string, limit int) (content.BoardPage, error) {
	return b.a.Statuses.Board(ctx, viewer, gatheringID, before, limit)
}

func (b eventBoards) PostOnBoard(ctx context.Context, in content.BoardPostInput) (content.Post, error) {
	return b.a.Content.PostOnBoard(ctx, in)
}

// contentBoards tells the content module who reads and who writes on the boards of the events
// (ADR-0161 §17) through the gathering module, which is built after it; before that, no board.
type contentBoards struct{ a *App }

func (b contentBoards) Board(ctx context.Context, accountID, gatheringID string) (content.BoardAccess, error) {
	if b.a.Events == nil {
		return content.BoardAccess{}, nil
	}
	a, err := b.a.Events.Board(ctx, accountID, gatheringID)
	return content.BoardAccess{Sees: a.Sees, Reads: a.Reads, Open: a.Open, Writes: a.Writes}, err
}

func (b contentBoards) BoardsToPurge(ctx context.Context, gatheringIDs []string) ([]string, error) {
	if b.a.Events == nil {
		return nil, nil
	}
	return b.a.Events.BoardsToPurge(ctx, gatheringIDs)
}

func (b contentBoards) BoardReaders(ctx context.Context, gatheringID string, accountIDs []string) (map[string]bool, error) {
	if b.a.Events == nil {
		return map[string]bool{}, nil
	}
	return b.a.Events.BoardReaders(ctx, gatheringID, accountIDs)
}

// gatheringRooms opens and ends the voice chat rooms of the events through the voice module
// (ADR-0161 §10, ADR-0100 §16).
type gatheringRooms struct{ a *App }

// roomReasonOf returns the reason of an error of the voice module that keeps a room shut, "" for
// another error.
func roomReasonOf(err error) gathering.RoomReason {
	switch {
	case errors.Is(err, appvoice.ErrDisabled):
		return gathering.RoomDisabled
	case errors.Is(err, appvoice.ErrNotEligible):
		return gathering.RoomNotEligible
	case errors.Is(err, appvoice.ErrTooManyRooms):
		return gathering.RoomTooManyRooms
	case errors.Is(err, appvoice.ErrHostingElsewhere):
		return gathering.RoomHostingElsewhere
	}
	return ""
}

// roomVisibilities map the visibility of an event to that of its room (ADR-0161 §10).
var roomVisibilities = map[gathering.Visibility]voice.Visibility{
	gathering.VisibilityPublic:    voice.VisibilityLocal,
	gathering.VisibilityFollowers: voice.VisibilityFollowers,
	gathering.VisibilityFollowing: voice.VisibilityFollowing,
	gathering.VisibilityMutuals:   voice.VisibilityMutuals,
	gathering.VisibilityInvited:   voice.VisibilityInvited,
}

func (r gatheringRooms) CheckHost(ctx context.Context, accountID string) (gathering.RoomReason, error) {
	err := r.a.VoiceRooms.CheckScheduledHost(ctx, accountID)
	if reason := roomReasonOf(err); reason != "" || err == nil {
		return reason, nil
	}
	return "", err
}

func (r gatheringRooms) Open(ctx context.Context, in appgathering.RoomOpening) (string, gathering.RoomReason, error) {
	visibility, ok := roomVisibilities[in.Visibility]
	if !ok {
		visibility = voice.VisibilityInvited
	}
	v, err := r.a.VoiceRooms.OpenScheduled(ctx, appvoice.ScheduledInput{
		HostAccountID: in.HostAccountID, Key: in.Key, CohostCandidates: in.CohostCandidates, Invitees: in.Invitees,
		Settings: appvoice.SettingsInput{Name: in.Title, Visibility: visibility, Speakers: voice.Speakers(in.Speakers)},
	})
	if reason := roomReasonOf(err); reason != "" {
		return "", reason, nil
	}
	if err != nil {
		return "", "", err
	}
	return v.ID, "", nil
}

func (r gatheringRooms) End(ctx context.Context, in appgathering.RoomEnd) error {
	end := appvoice.EndScheduledInput{Key: in.Key, Reason: voice.EndHost, ByAccountID: in.ByAccountID}
	if in.Moderator && in.ByAccountID != "" {
		act, err := r.a.Accounts.FindByID(ctx, in.ByAccountID)
		if err != nil {
			return err
		}
		if act.Local != nil {
			end.Reason, end.ByUserID, end.Note = voice.EndModerator, act.Local.UserID, in.Note
		}
	}
	err := r.a.VoiceRooms.EndScheduled(ctx, end)
	if errors.Is(err, appvoice.ErrNotFound) {
		return nil
	}
	return err
}

// eventsAPI returns the events of the client API, nil before setUpGathering.
func (a *App) eventsAPI() clientapi.Events {
	if a.Events == nil {
		return nil
	}
	return a.Events
}

// eventModerationAPI returns the moderation of the events of the client API, nil before
// setUpGathering.
func (a *App) eventModerationAPI() clientapi.EventModeration {
	if a.Events == nil {
		return nil
	}
	return a.Events
}

// gatheringRetention reads what the purge of the answers and the invitations of the events keeps
// from the instance and moderation modules (ADR-0161 §14).
type gatheringRetention struct{ a *App }

func (r gatheringRetention) RecordsRetention(ctx context.Context) (time.Duration, error) {
	s, err := r.a.Instance.Settings(ctx)
	if err != nil {
		return 0, err
	}
	return s.Retention(instance.SettingEventRecordsRetentionDays), nil
}

func (r gatheringRetention) ReportedGatherings(ctx context.Context, ids []string) (map[string]bool, error) {
	return r.a.Moderation.ReportedGatherings(ctx, ids)
}

func (r gatheringRetention) ReportedGatheringsTx(ctx context.Context, tx uow.Tx, ids []string) (map[string]bool, error) {
	return r.a.Moderation.ReportedGatheringsTx(ctx, tx, ids)
}

func (r gatheringRetention) HeldAccountIDs(ctx context.Context) ([]string, error) {
	return r.a.Moderation.HeldAccountIDs(ctx)
}

func (r gatheringRetention) HeldAccountIDsTx(ctx context.Context, tx uow.Tx) ([]string, error) {
	return r.a.Moderation.HeldAccountIDsTx(ctx, tx)
}

// gatheringMembers reads the accounts for the events from the account, moderation, identity,
// instance, and voice modules (ADR-0161 §3, §9).
type gatheringMembers struct{ a *App }

func (m gatheringMembers) Member(ctx context.Context, accountID string) (appgathering.Member, error) {
	v, err := voiceMembers(m).Member(ctx, accountID)
	if err != nil {
		return appgathering.Member{}, err
	}
	return appgathering.Member{Local: v.Local, Active: v.Active, Limited: v.Limited}, nil
}

func (m gatheringMembers) Organizer(ctx context.Context, accountID string) (appgathering.Organizer, error) {
	v, err := voiceMembers(m).Member(ctx, accountID)
	if err != nil || !v.Local {
		return appgathering.Organizer{}, err
	}
	act, err := m.a.Accounts.FindByID(ctx, accountID)
	if err != nil {
		return appgathering.Organizer{}, err
	}
	userID := act.Local.UserID
	mayCreate, err := m.a.Identity.HasPermission(ctx, userID, domainidentity.PermCreateEvents)
	if err != nil {
		return appgathering.Organizer{}, err
	}
	o := appgathering.Organizer{Member: appgathering.Member{Local: v.Local, Active: v.Active, Limited: v.Limited}, MayCreate: mayCreate}
	// A voice place opens a room that the organizer hosts (ADR-0161 §3, ADR-0100 §2).
	if m.a.VoiceRooms != nil && m.a.VoiceRooms.Enabled() {
		everyone, err := voiceMembers(m).EveryoneStarts(ctx)
		if err != nil {
			return appgathering.Organizer{}, err
		}
		o.MayHostVoice = voice.CanHost(voice.HostFacts{
			Local: v.Local, Active: v.Active, Limited: v.Limited, EveryoneStarts: everyone, MayStart: v.MayStart,
		})
	}
	prefs, err := m.a.Identity.Preferences(ctx, userID)
	switch {
	case errors.Is(err, identity.ErrNotFound):
	case err != nil:
		return appgathering.Organizer{}, err
	case prefs.TimeZoneChosen:
		o.TimeZone = prefs.TimeZone
	}
	return o, nil
}

func (m gatheringMembers) EveryoneCreates(ctx context.Context) (bool, error) {
	s, err := m.a.Instance.Settings(ctx)
	if err != nil {
		return false, err
	}
	return s.EventCreators() == instance.EventCreatorsEveryone, nil
}

// gatheringRelations reads the follows and blocks of the events from the graph module.
type gatheringRelations struct{ a *App }

func (r gatheringRelations) BlockedAmong(ctx context.Context, accountID string, ids []string) (map[string]bool, error) {
	return voiceRelations(r).BlockedAmong(ctx, accountID, ids)
}

func (r gatheringRelations) Following(ctx context.Context, accountID string, ids []string) (map[string]bool, error) {
	return voiceRelations(r).Following(ctx, accountID, ids)
}

func (r gatheringRelations) FollowersAmong(ctx context.Context, accountID string, ids []string) (map[string]bool, error) {
	return voiceRelations(r).FollowersAmong(ctx, accountID, ids)
}
