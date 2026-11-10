// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/gatheringgen"
	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// InsertVoiceRoom implements appgathering.Repository.
func (r *Repository) InsertVoiceRoom(ctx context.Context, tx uow.Tx, v appgathering.VoiceRoom) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertGatheringVoiceRoom(ctx, gatheringgen.InsertGatheringVoiceRoomParams{
		GatheringID: v.GatheringID, ConfirmationID: v.ConfirmationID, RoomID: v.RoomID, OpenedAt: toTime(v.OpenedAt),
	})
	return n > 0, wrap(err, "insert gathering voice room")
}

// InsertRoomFailure implements appgathering.Repository.
func (r *Repository) InsertRoomFailure(ctx context.Context, tx uow.Tx, f appgathering.RoomFailure) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertGatheringVoiceRoomFailure(ctx, gatheringgen.InsertGatheringVoiceRoomFailureParams{
		GatheringID: f.GatheringID, ConfirmationID: f.ConfirmationID, Reason: string(f.Reason), FailedAt: toTime(f.FailedAt),
	})
	return n > 0, wrap(err, "insert gathering voice room failure")
}

func voiceRoomOf(row gatheringgen.GatheringVoiceRoom) appgathering.VoiceRoom {
	return appgathering.VoiceRoom{GatheringID: row.GatheringID, ConfirmationID: row.ConfirmationID, RoomID: row.RoomID, OpenedAt: fromTime(row.OpenedAt)}
}

// VoiceRooms implements appgathering.Repository.
func (r *Repository) VoiceRooms(ctx context.Context, tx uow.Tx, gatheringIDs []string) ([]appgathering.VoiceRoom, error) {
	if len(gatheringIDs) == 0 {
		return nil, nil
	}
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	list, err := listOf(gatheringIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListGatheringVoiceRooms(ctx, list)
	if err != nil {
		return nil, wrap(err, "list gathering voice rooms")
	}
	out := make([]appgathering.VoiceRoom, len(rows))
	for i, row := range rows {
		out[i] = voiceRoomOf(row)
	}
	return out, nil
}

// RoomFailures implements appgathering.Repository.
func (r *Repository) RoomFailures(ctx context.Context, tx uow.Tx, gatheringIDs []string) ([]appgathering.RoomFailure, error) {
	if len(gatheringIDs) == 0 {
		return nil, nil
	}
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	list, err := listOf(gatheringIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListGatheringVoiceRoomFailures(ctx, list)
	if err != nil {
		return nil, wrap(err, "list gathering voice room failures")
	}
	out := make([]appgathering.RoomFailure, len(rows))
	for i, row := range rows {
		out[i] = appgathering.RoomFailure{
			GatheringID: row.GatheringID, ConfirmationID: row.ConfirmationID, Reason: gathering.RoomReason(row.Reason), FailedAt: fromTime(row.FailedAt),
		}
	}
	return out, nil
}

// FindVoiceRoomByRoom implements appgathering.Repository.
func (r *Repository) FindVoiceRoomByRoom(ctx context.Context, tx uow.Tx, roomID string) (appgathering.VoiceRoom, error) {
	q, err := r.q(tx)
	if err != nil {
		return appgathering.VoiceRoom{}, err
	}
	row, err := q.GetGatheringVoiceRoomByRoom(ctx, roomID)
	if err != nil {
		return appgathering.VoiceRoom{}, notFound(err, appgathering.ErrNotFound, "gathering voice room")
	}
	return voiceRoomOf(row), nil
}
