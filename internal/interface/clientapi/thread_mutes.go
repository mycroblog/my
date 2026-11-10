// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
)

// ThreadMutes is the part of the notification module's thread mutes that the client API uses
// (ADR-0158 §8, §10): muting and unmuting the thread (conversation) of a status, and reading
// which statuses' threads viewers mute, for Status.muted. *appnotification.ThreadMutes implements
// it.
type ThreadMutes interface {
	Mute(ctx context.Context, accountID, statusID string) error
	Unmute(ctx context.Context, accountID, statusID string) error
	MutedAmong(ctx context.Context, accountIDs, statusIDs []string) (map[string]map[string]bool, error)
}

// MuteStatusThread serves POST /api/v1/statuses/{id}/mute.
func (s *Server) MuteStatusThread(ctx context.Context, req MuteStatusThreadRequestObject) (MuteStatusThreadResponseObject, error) {
	if s.o.ThreadMutes == nil {
		return MuteStatusThreaddefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	res, p, err := s.act(ctx, req.Id, func(a string) error { return s.o.ThreadMutes.Mute(ctx, a, req.Id) })
	if p != nil {
		return MuteStatusThreaddefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return MuteStatusThread200JSONResponse{res}, err
}

// UnmuteStatusThread serves DELETE /api/v1/statuses/{id}/mute. As with the other interactions
// taken back, a status that the viewer may no longer see still loses the mute (unact).
func (s *Server) UnmuteStatusThread(ctx context.Context, req UnmuteStatusThreadRequestObject) (UnmuteStatusThreadResponseObject, error) {
	if s.o.ThreadMutes == nil {
		return UnmuteStatusThreaddefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	res, p, err := s.unact(ctx, req.Id, func(a string) error { return s.o.ThreadMutes.Unmute(ctx, a, req.Id) })
	if p != nil {
		return UnmuteStatusThreaddefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return UnmuteStatusThread200JSONResponse{res}, err
}

// threadMuted reads, for the viewers of views (ViewerRelation.AccountID), which of the statuses
// they see, those quoted included, are in a thread they mute, with the reads of
// ThreadMutes.MutedAmong; nil without ThreadMutes or a logged-in viewer.
func (s *Server) threadMuted(ctx context.Context, views []statusViewRef) (map[string]map[string]bool, error) {
	if s.o.ThreadMutes == nil {
		return nil, nil
	}
	var accounts, statuses []string
	for _, v := range views {
		accounts = append(accounts, v.accountID)
		statuses = append(statuses, v.statusID)
	}
	if len(accounts) == 0 {
		return nil, nil
	}
	return s.o.ThreadMutes.MutedAmong(ctx, accounts, statuses)
}

// statusViewRef is a status that a logged-in viewer sees.
type statusViewRef struct{ accountID, statusID string }
