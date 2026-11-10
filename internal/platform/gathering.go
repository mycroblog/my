// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/identity"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/voice"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// setUpGathering builds the events of the local users (ADR-0161): the projectors of their
// projections and the Service of the client API. It comes after the voice chat, whose hosts it
// reads.
func (a *App) setUpGathering(repo appgathering.Repository) {
	appgathering.RegisterProjectors(a.Projectors, repo)
	a.Events = appgathering.NewService(appgathering.Deps{
		Repository: repo, UnitOfWork: a.UnitOfWork, Clock: a.Clock, IDs: a.IDs, RateLimiter: a.RateLimiter,
		Members: gatheringMembers{a}, Relations: gatheringRelations{a}, Logger: a.Logger,
	})
}

// eventsAPI returns the events of the client API, nil before setUpGathering.
func (a *App) eventsAPI() clientapi.Events {
	if a.Events == nil {
		return nil
	}
	return a.Events
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
