// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/instancegen"
	"github.com/mycroblog/my/internal/domain/instance"
)

// FindAnnouncements implements instance.Repository.
func (r *Repository) FindAnnouncements(ctx context.Context, ids []string) (map[string]instance.Announcement, error) {
	out := map[string]instance.Announcement{}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListAnnouncementsByIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list announcements")
	}
	for _, row := range rows {
		out[row.ID] = announcementOf(instancegen.ListAnnouncementsRow(row))
	}
	return out, nil
}

// FindClosureEvents implements instance.Repository.
func (r *Repository) FindClosureEvents(ctx context.Context, ids []string) (map[string]instance.ClosureEvent, error) {
	out := map[string]instance.ClosureEvent{}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListServiceClosureEventsByIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list service closure events")
	}
	for _, row := range rows {
		out[row.ID] = closureEventFrom(instancegen.GetServiceClosureEventRow(row))
	}
	return out, nil
}
