// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// The service closure (FR-808, FR-809, ADR-0088): the operator announces that the server closes
// on a date, may change the date or withdraw the announcement, closes the server after the date,
// and the server records when the Delete of every local account has been delivered. The state is
// read per request and per job; every entry point asks ClosureAllows about the operation it
// handles and never branches on the state's value.

// ClosureEventKind is the kind of a service closure event (service_closure_events.kind).
type ClosureEventKind string

// Kinds of service closure events.
const (
	// ClosureEventAnnounced announces the closure with its schedule; a later one changes the
	// schedule.
	ClosureEventAnnounced ClosureEventKind = "announced"
	// ClosureEventWithdrawn withdraws the announcement: the server runs on as before.
	ClosureEventWithdrawn ClosureEventKind = "withdrawn"
	// ClosureEventClosed closes the server, for good.
	ClosureEventClosed ClosureEventKind = "closed"
	// ClosureEventDeliveryCompleted records that the Delete of every local account has been
	// delivered.
	ClosureEventDeliveryCompleted ClosureEventKind = "delivery_completed"
)

// ClosureEventKinds returns every kind, in the order of the schema's CHECK.
func ClosureEventKinds() []ClosureEventKind {
	return []ClosureEventKind{ClosureEventAnnounced, ClosureEventWithdrawn, ClosureEventClosed, ClosureEventDeliveryCompleted}
}

// ClosureState is where the server is in its closure (the service_closure_heads projection).
type ClosureState string

// States of the service closure (ADR-0088 §1). The static page after the server stops is not a
// state: the server no longer runs then.
const (
	// ClosureNone is a server that runs with no closure announced.
	ClosureNone ClosureState = "none"
	// ClosureAnnounced is a server whose closure is announced.
	ClosureAnnounced ClosureState = "announced"
	// ClosureClosed is a closed server that delivers the Delete of its local accounts.
	ClosureClosed ClosureState = "closed"
	// ClosureDeliveryCompleted is a closed server whose Delete deliveries have all ended.
	ClosureDeliveryCompleted ClosureState = "delivery_completed"
)

// ClosureStates returns every state, in order.
func ClosureStates() []ClosureState {
	return []ClosureState{ClosureNone, ClosureAnnounced, ClosureClosed, ClosureDeliveryCompleted}
}

// DefaultClosureExportPeriod is how long after the closure the users may still export their
// data when the operator does not choose export_until (ADR-0088 §1).
const DefaultClosureExportPeriod = 30 * 24 * time.Hour

// ClosureNoticeWarning is the notice below which the admin screen and the CLI warn that the
// users have little time: from the announcement to closes_at (ADR-0088 §1).
const ClosureNoticeWarning = 30 * 24 * time.Hour

// MaxClosureMessageLength bounds the operator's message of a closure, in characters.
const MaxClosureMessageLength = 5000

var (
	// ErrClosureTransition reports a service closure event that the current state does not
	// allow: a withdrawal without an announcement, an announcement after the closure, a closure
	// without an announcement, or a completed delivery before the closure.
	ErrClosureTransition = errors.New("instance: the service closure does not allow that now")
	// ErrClosureNotDue reports a closure before closes_at. It wraps ErrClosureTransition.
	ErrClosureNotDue = fmt.Errorf("instance: the closing date has not come: %w", ErrClosureTransition)
	// ErrClosureSchedule reports a schedule without a closing date or whose export deadline is
	// before it. It wraps ErrInvalidValue.
	ErrClosureSchedule = fmt.Errorf("instance: invalid closure schedule: %w", ErrInvalidValue)
	// ErrClosureMessage reports a message that is not valid text or longer than
	// MaxClosureMessageLength. It wraps ErrInvalidValue.
	ErrClosureMessage = fmt.Errorf("instance: invalid closure message: %w", ErrInvalidValue)
)

// ClosureSchedule is what an announcement says (service_closure_schedules): the closing date,
// until when the users may export their data, and the operator's message to them.
type ClosureSchedule struct {
	ClosesAt    time.Time
	ExportUntil time.Time
	// Message is the operator's message in plain text, "" for none.
	Message string
}

// NewClosureSchedule validates and completes a schedule. closesAt has no lower bound, so that
// an operator can close at short notice. exportUntil is closesAt plus
// DefaultClosureExportPeriod when it is the zero time, and may not be before closesAt. It fails
// with ErrClosureSchedule or ErrClosureMessage.
func NewClosureSchedule(closesAt, exportUntil time.Time, message string) (ClosureSchedule, error) {
	switch {
	case closesAt.IsZero():
		return ClosureSchedule{}, fmt.Errorf("%w: no closing date", ErrClosureSchedule)
	case exportUntil.IsZero():
		exportUntil = closesAt.Add(DefaultClosureExportPeriod)
	case exportUntil.Before(closesAt):
		return ClosureSchedule{}, fmt.Errorf("%w: the export deadline is before the closing date", ErrClosureSchedule)
	}
	if !utf8.ValidString(message) || strings.ContainsRune(message, 0) || utf8.RuneCountInString(message) > MaxClosureMessageLength {
		return ClosureSchedule{}, ErrClosureMessage
	}
	return ClosureSchedule{ClosesAt: closesAt, ExportUntil: exportUntil, Message: strings.TrimSpace(message)}, nil
}

// ClosureEvent is one service closure event (service_closure_events).
type ClosureEvent struct {
	ID   string
	Kind ClosureEventKind
	// Schedule is the schedule of an announcement, the zero value for the other kinds.
	Schedule ClosureSchedule
	// ByUserID is the user who made the change, or "" for the CLI and the server itself.
	ByUserID   string
	OccurredAt time.Time
}

// ClosureHead is the current state (the service_closure_heads projection), the zero value
// while no closure is announced (no row).
type ClosureHead struct {
	State ClosureState
	// EventID is the latest event applied.
	EventID string
	// AnnouncementID is the announcement whose schedule is in effect.
	AnnouncementID string
}

// Apply returns the head after e, given h; events apply in the order of their IDs, and an event
// not newer than the head changes nothing. A withdrawal returns the zero head.
func (h ClosureHead) Apply(e ClosureEvent) ClosureHead {
	if h.EventID != "" && e.ID <= h.EventID {
		return h
	}
	switch e.Kind {
	case ClosureEventAnnounced:
		return ClosureHead{State: ClosureAnnounced, EventID: e.ID, AnnouncementID: e.ID}
	case ClosureEventWithdrawn:
		return ClosureHead{}
	case ClosureEventClosed:
		return ClosureHead{State: ClosureClosed, EventID: e.ID, AnnouncementID: h.AnnouncementID}
	case ClosureEventDeliveryCompleted:
		return ClosureHead{State: ClosureDeliveryCompleted, EventID: e.ID, AnnouncementID: h.AnnouncementID}
	}
	return h
}

// Closure is the current state of the service closure with the schedule in effect, as the entry
// points read it. The zero value is a server with no closure (ClosureNone after Normalize).
type Closure struct {
	State ClosureState
	// AnnouncementID is the announcement whose schedule is in effect, "" for ClosureNone.
	AnnouncementID string
	// Schedule is the schedule of the announcement in effect, the zero value for ClosureNone.
	Schedule ClosureSchedule
	// AnnouncedAt is when the announcement in effect was made (the latest change of the date),
	// the zero time for ClosureNone.
	AnnouncedAt time.Time
	// ChangedAt is when the state last changed, the zero time for ClosureNone.
	ChangedAt time.Time
}

// CurrentState returns the state, ClosureNone for the zero value.
func (c Closure) CurrentState() ClosureState {
	if c.State == "" {
		return ClosureNone
	}
	return c.State
}

// ShortNotice reports whether the announcement in effect gives the users less than
// ClosureNoticeWarning from AnnouncedAt to the closing date.
func (c Closure) ShortNotice() bool {
	if c.CurrentState() == ClosureNone {
		return false
	}
	return c.Schedule.ClosesAt.Sub(c.AnnouncedAt) < ClosureNoticeWarning
}

// Check reports whether an event of kind may be recorded at now (ADR-0088 §1): an announcement
// with no closure or during one (a change of the date); a withdrawal only during an
// announcement; the closure only during an announcement once now has reached closes_at; the
// completed delivery only while closed. Nothing goes back after the closure. It fails with an
// error wrapping ErrClosureTransition.
func (c Closure) Check(kind ClosureEventKind, now time.Time) error {
	state := c.CurrentState()
	ok := false
	switch kind {
	case ClosureEventAnnounced:
		ok = state == ClosureNone || state == ClosureAnnounced
	case ClosureEventWithdrawn:
		ok = state == ClosureAnnounced
	case ClosureEventClosed:
		if state == ClosureAnnounced && now.Before(c.Schedule.ClosesAt) {
			return ErrClosureNotDue
		}
		ok = state == ClosureAnnounced
	case ClosureEventDeliveryCompleted:
		ok = state == ClosureClosed
	}
	if !ok {
		return fmt.Errorf("%w: %s in the state %s", ErrClosureTransition, kind, state)
	}
	return nil
}

// ClosureOperation is a kind of operation that the service closure may stop (ADR-0088 §2).
type ClosureOperation string

// Operations of the service closure.
const (
	// ClosureSignIn is signing in and resetting a password.
	ClosureSignIn ClosureOperation = "sign_in"
	// ClosureRegister is signing up, using an invite, and approving a pending application.
	ClosureRegister ClosureOperation = "register"
	// ClosureSubscribe is starting a new supporter subscription.
	ClosureSubscribe ClosureOperation = "subscribe"
	// ClosureOwnData is what the users do with their own data to leave: reading their own
	// account, exporting it (FR-008 and the CSV files), handling their sessions, and reading
	// GET /instance.
	ClosureOwnData ClosureOperation = "own_data"
	// ClosureClientAPI is every other operation of the client API.
	ClosureClientAPI ClosureOperation = "client_api"
	// ClosureStream is the streaming API and Web Push.
	ClosureStream ClosureOperation = "stream"
	// ClosureFederate is federation besides the closure's own deliveries: delivering what was
	// queued, the inbox, GET of the posts and collections, fetching from other servers, and the
	// relays.
	ClosureFederate ClosureOperation = "federate"
	// ClosureRetract is delivering the Delete of the local accounts that the closure sends.
	ClosureRetract ClosureOperation = "retract"
	// ClosureServeActors is GET of the local actors and the instance actor, whose keys verify
	// the closure's Delete. A closed server serves the local actors as a suspension does.
	ClosureServeActors ClosureOperation = "serve_actors"
	// ClosureWebFinger is WebFinger and host-meta.
	ClosureWebFinger ClosureOperation = "webfinger"
	// ClosureNodeInfo is NodeInfo, which states the closure.
	ClosureNodeInfo ClosureOperation = "nodeinfo"
)

// ClosureOperations returns every operation, in order.
func ClosureOperations() []ClosureOperation {
	return []ClosureOperation{
		ClosureSignIn, ClosureRegister, ClosureSubscribe, ClosureOwnData, ClosureClientAPI, ClosureStream,
		ClosureFederate, ClosureRetract, ClosureServeActors, ClosureWebFinger, ClosureNodeInfo,
	}
}

// closureAllowed is the table of ADR-0088 §2: the states each operation runs in.
var closureAllowed = map[ClosureOperation][]ClosureState{
	ClosureSignIn:      {ClosureNone, ClosureAnnounced, ClosureClosed, ClosureDeliveryCompleted},
	ClosureRegister:    {ClosureNone},
	ClosureSubscribe:   {ClosureNone},
	ClosureOwnData:     {ClosureNone, ClosureAnnounced, ClosureClosed, ClosureDeliveryCompleted},
	ClosureClientAPI:   {ClosureNone, ClosureAnnounced},
	ClosureStream:      {ClosureNone, ClosureAnnounced},
	ClosureFederate:    {ClosureNone, ClosureAnnounced},
	ClosureRetract:     {ClosureNone, ClosureAnnounced, ClosureClosed},
	ClosureServeActors: {ClosureNone, ClosureAnnounced, ClosureClosed},
	ClosureWebFinger:   {ClosureNone, ClosureAnnounced},
	ClosureNodeInfo:    {ClosureNone, ClosureAnnounced, ClosureClosed, ClosureDeliveryCompleted},
}

// ClosureAllows reports whether the server in state runs op. An unknown state or operation runs
// nothing; the empty state is ClosureNone.
func ClosureAllows(state ClosureState, op ClosureOperation) bool {
	if state == "" {
		state = ClosureNone
	}
	for _, s := range closureAllowed[op] {
		if s == state {
			return true
		}
	}
	return false
}

// EffectiveRegistrationMode is the registration mode that sign-ups follow (FR-001, ADR-0088 §2):
// the setting's value while the service closure lets users register (ClosureRegister), and
// RegistrationClosed otherwise. The setting keeps its value, which applies again once the
// announcement is withdrawn.
func EffectiveRegistrationMode(mode RegistrationMode, state ClosureState) RegistrationMode {
	if !ClosureAllows(state, ClosureRegister) {
		return RegistrationClosed
	}
	return mode
}
