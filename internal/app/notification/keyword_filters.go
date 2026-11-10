// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/i18n"
)

// The keyword filters of the notifications (ADR-0157 §7): a notification whose status a hide
// filter of its recipient matches is in no list, stream, Web Push, email, or unread count; one
// whose status only warn filters match is marked in the list and the stream, and its Web Push and
// email name the filters instead of saying more. The timeline module owns the filters and judges.

// StatusFilters judges statuses, as their recipient sees them now, against the recipient's
// keyword filters of the notifications. The composition root implements it over the timeline
// service.
type StatusFilters interface {
	// FilterStatuses returns the verdict of each of statusIDs that a filter matches; a status
	// that none matches, or that accountID may not see, has no key.
	FilterStatuses(ctx context.Context, accountID string, statusIDs []string) (map[string]StatusVerdict, error)
}

// StatusVerdict is what the keyword filters of the notifications make of a status.
type StatusVerdict struct {
	// Hidden is whether a hide filter matches it.
	Hidden bool
	// Warned are the titles of the warn filters that match it, when none hides it.
	Warned []string
}

// maxFilteredUnread bounds the unread notifications whose statuses the unread count judges: past
// it, the count stops being exact, as it is shown capped long before.
const maxFilteredUnread = 1000

// unreadHidden returns how many of accountID's unread notifications after after a hide filter
// leaves out, judging the statuses of the newest maxFilteredUnread.
func (s *Service) unreadHidden(ctx context.Context, accountID, after string, hidden Hidden, now time.Time) (int64, error) {
	if s.d.Filters == nil {
		return 0, nil
	}
	ids, err := s.d.Repository.UnreadNotificationStatuses(ctx, accountID, after, hidden, now, maxFilteredUnread)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	verdicts, err := s.d.Filters.FilterStatuses(ctx, accountID, ids)
	if err != nil {
		return 0, fmt.Errorf("notification: judge the unread statuses of %s: %w", accountID, err)
	}
	var n int64
	for _, id := range ids {
		if verdicts[id].Hidden {
			n++
		}
	}
	return n, nil
}

// filterVerdict returns the verdict of the status of job's notification, zero for none.
func filterVerdict(ctx context.Context, filters StatusFilters, job pushJob) (StatusVerdict, error) {
	if filters == nil || job.StatusID == "" {
		return StatusVerdict{}, nil
	}
	vs, err := filters.FilterStatuses(ctx, job.AccountID, []string{job.StatusID})
	if err != nil {
		return StatusVerdict{}, fmt.Errorf("notification: judge the status of %s: %w", job.NotificationID, err)
	}
	return vs[job.StatusID], nil
}

// warned words p, a message about a status that warn filters matched, by its title and the title
// of the first of those filters alone (ADR-0157 §7).
func warned(l *i18n.Localizer, p PushPayload, v StatusVerdict) PushPayload {
	if len(v.Warned) > 0 {
		p.Title = l.ServerPushFiltered(i18n.ServerPushFilteredArgs{Title: p.Title, Filter: v.Warned[0]})
	}
	return p
}
