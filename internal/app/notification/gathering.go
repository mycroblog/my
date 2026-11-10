// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/i18n"
)

// The notifications of the events of the local users (ADR-0161 §11): the gathering module calls
// GatheringNotifier inside the transactions of its jobs, which decide the recipients when they
// run and check that each one sees the event. Every notification of an event but
// event_room_unavailable, the server's notice to the organizer of its own event, is from an
// account (the organizer, or the account that invited), so the exclusions of the other
// notifications from accounts apply: none to the source itself or to a remote or deactivated account, none when
// either blocks the other or the recipient mutes the source with hide_notifications, none from a
// source with unsolicited_reach to a recipient who does not follow it. The notification policy
// (ADR-0158) does not hold them: they are about events the recipient answered or was invited to.
// Their messages and emails name the title of the event and never its places.

// EventMail is what words a notification of an event as of the notification: the title, the start
// in the event's time zone, its hosts, and for a reminder how long before the start it is.
type EventMail struct {
	Title    string    `json:"title"`
	StartsAt time.Time `json:"starts_at"`
	// TimeZone is the IANA name of the event's time zone.
	TimeZone string `json:"time_zone"`
	// OrganizerID and CoorganizerIDs are the hosts that the email names.
	OrganizerID    string   `json:"organizer_id,omitempty"`
	CoorganizerIDs []string `json:"coorganizer_ids,omitempty"`
	// MinutesBefore is how long before the start an event_reminder is.
	MinutesBefore int `json:"minutes_before,omitempty"`
}

// EventKind is which notification of an event an EventNotice is.
type EventKind string

// Event kinds, one per notification type of the events.
const (
	EventInvited           EventKind = "invited"
	EventScheduleConfirmed EventKind = "schedule_confirmed"
	EventChanged           EventKind = "changed"
	EventCancelled         EventKind = "cancelled"
	EventReminder          EventKind = "reminder"
	EventStarted           EventKind = "started"
	EventRoomUnavailable   EventKind = "room_unavailable"
)

// EventNotice is a notification of an event to many recipients: everything but the recipient.
type EventNotice struct {
	Kind        EventKind
	GatheringID string
	// SourceAccountID is the account the notification is from.
	SourceAccountID string
	// Ref is what the notification is about, which its dedupe key names: the invitation of an
	// invited, the revision or the confirmation of a changed, and the confirmation of the others.
	// The event of a cancelled needs none.
	Ref string
	// Reason and Checkpoint are why the room of a room_unavailable cannot open and when the server
	// found it out.
	Reason, Checkpoint string
	At                 time.Time
	Mail               EventMail
}

// typeAndKey returns the notification type of n and its dedupe key; ok is false for an unknown
// kind.
func (n EventNotice) typeAndKey() (notification.Type, string, bool) {
	switch n.Kind {
	case EventInvited:
		return notification.TypeEventInvited, notification.EventInvitedKey(n.Ref), true
	case EventScheduleConfirmed:
		return notification.TypeEventScheduleConfirmed, notification.EventScheduleConfirmedKey(n.Ref), true
	case EventChanged:
		return notification.TypeEventChanged, notification.EventChangedKey(n.Ref), true
	case EventCancelled:
		return notification.TypeEventCancelled, notification.EventCancelledKey(n.GatheringID), true
	case EventReminder:
		return notification.TypeEventReminder, notification.EventReminderKey(n.Ref, n.Mail.MinutesBefore), true
	case EventStarted:
		return notification.TypeEventStarted, notification.EventStartedKey(n.Ref), true
	case EventRoomUnavailable:
		return notification.TypeEventRoomUnavailable, notification.EventRoomUnavailableKey(n.Ref, n.Reason, n.Checkpoint), true
	}
	return "", "", false
}

// ErrUnknownEventKind reports an EventNotice of no known kind.
var ErrUnknownEventKind = errors.New("notification: unknown kind of a notification of an event")

// GatheringNotifier creates the notifications of the events.
type GatheringNotifier struct {
	p *projector
}

// NewGatheringNotifier returns a GatheringNotifier on the ports of the projectors.
func NewGatheringNotifier(d ProjectorDeps) *GatheringNotifier {
	return &GatheringNotifier{p: &projector{d: d}}
}

// NotifyEventTx tells each of recipientIDs inside tx of the notice n, once per recipient and
// dedupe key, with a number of queries that does not grow with them.
func (g *GatheringNotifier) NotifyEventTx(ctx context.Context, tx uow.Tx, n EventNotice, recipientIDs []string) error {
	t, key, ok := n.typeAndKey()
	if !ok {
		return ErrUnknownEventKind
	}
	mail := n.Mail
	return g.p.notifyAllWith(ctx, tx, notification.Notification{
		SourceAccountID: n.SourceAccountID, Type: t, GatheringID: n.GatheringID, DedupeKey: key, CreatedAt: n.At,
	}, recipientIDs, func(job *pushJob) { job.Event = &mail })
}

// EventPath is the web client's page of an event.
func EventPath(id string) string { return "/events/" + id }

// The units of the time before the start of a reminder, as push.event_reminder selects them.
const (
	reminderDay    = "day"
	reminderHour   = "hour"
	reminderMinute = "minute"
)

// reminderUnit returns how a reminder minutes before the start reads: in days or hours when it is
// a whole number of them, in minutes otherwise.
func reminderUnit(minutes int) (unit string, count int) {
	switch {
	case minutes > 0 && minutes%(24*60) == 0:
		return reminderDay, minutes / (24 * 60)
	case minutes > 0 && minutes%60 == 0:
		return reminderHour, minutes / 60
	}
	return reminderMinute, minutes
}

// describeEvent returns the title of the Web Push message of a notification of an event, from
// name (the source account) when it names one.
func describeEvent(l *i18n.Localizer, n Wording, name string) string {
	title := l.ServerPushEventUntitled()
	minutes := 0
	if n.Event != nil {
		if n.Event.Title != "" {
			title = n.Event.Title
		}
		minutes = n.Event.MinutesBefore
	}
	switch n.Type {
	case notification.TypeEventInvited:
		return l.ServerPushEventInvited(i18n.ServerPushEventInvitedArgs{Name: name, Title: title})
	case notification.TypeEventScheduleConfirmed:
		return l.ServerPushEventScheduleConfirmed(title)
	case notification.TypeEventChanged:
		return l.ServerPushEventChanged(title)
	case notification.TypeEventCancelled:
		return l.ServerPushEventCancelled(title)
	case notification.TypeEventReminder:
		unit, count := reminderUnit(minutes)
		return l.ServerPushEventReminder(i18n.ServerPushEventReminderArgs{Unit: unit, Count: count, Title: title})
	case notification.TypeEventStarted:
		return l.ServerPushEventStarted(title)
	default:
		return l.ServerPushEventRoomUnavailable(title)
	}
}

// eventWhenLayout is how an email writes the start of an event: in the event's time zone, whose
// name follows it.
const eventWhenLayout = "2006-01-02 15:04"

// EventWhen returns the start of an event as its emails write it: in the time zone named zone,
// followed by the zone's name, or in UTC when the zone does not load.
func EventWhen(startsAt time.Time, zone string) string {
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" {
		loc, zone = time.UTC, "UTC"
	}
	return startsAt.In(loc).Format(eventWhenLayout) + " (" + zone + ")"
}

// EventDetails returns the paragraphs that the email of a notification of an event adds below
// its subject: the start, the organizer, and the co-organizers, named by names. It never says
// where the event is held, since emails are forwarded and kept (ADR-0161 §11).
func EventDetails(l *i18n.Localizer, m *EventMail, names map[string]string) []string {
	if m == nil {
		return nil
	}
	out := []string{l.ServerMailEventWhen(EventWhen(m.StartsAt, m.TimeZone))}
	if name := names[m.OrganizerID]; name != "" {
		out = append(out, l.ServerMailEventOrganizer(name))
	}
	for _, id := range m.CoorganizerIDs {
		if name := names[id]; name != "" {
			out = append(out, l.ServerMailEventCoorganizer(name))
		}
	}
	return out
}

// eventHostNames returns the names of the hosts of m, as a message names an account; a host that
// is gone has none.
func eventHostNames(ctx context.Context, audience PushAudience, m *EventMail) (map[string]string, error) {
	out := map[string]string{}
	if m == nil {
		return out, nil
	}
	for _, id := range append([]string{m.OrganizerID}, m.CoorganizerIDs...) {
		if id == "" {
			continue
		}
		a, err := audience.Account(ctx, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		name := a.DisplayName
		if name == "" {
			name = "@" + a.Acct
		}
		out[id] = name
	}
	return out, nil
}
