// SPDX-License-Identifier: AGPL-3.0-only

package webhook

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// EventType is the type of a webhook message (the "type" of its body, ADR-0060), dot-separated
// as Standard Webhooks recommends.
type EventType string

// Event types (ADR-0060 "イベントとペイロード").
const (
	EventReportCreated                 EventType = "report.created"
	EventReportResolved                EventType = "report.resolved"
	EventReportReopened                EventType = "report.reopened"
	EventRegistrationPending           EventType = "registration.pending"
	EventAccountCreated                EventType = "account.created"
	EventAccountUpdated                EventType = "account.updated"
	EventModerationActionCreated       EventType = "moderation_action.created"
	EventModerationActionLifted        EventType = "moderation_action.lifted"
	EventAppealCreated                 EventType = "appeal.created"
	EventDomainModerationActionCreated EventType = "domain_moderation_action.created"
	EventDomainModerationActionLifted  EventType = "domain_moderation_action.lifted"
	EventStatusCreated                 EventType = "status.created"
	EventCustomEmojiCreated            EventType = "custom_emoji.created"
	EventCustomEmojiUpdated            EventType = "custom_emoji.updated"
	EventCustomEmojiDeleted            EventType = "custom_emoji.deleted"
	EventErrorGroupCreated             EventType = "error_group.created"
	EventErrorGroupRegressed           EventType = "error_group.regressed"
	// EventTest is the test message of the administration screen, sent to one endpoint whatever
	// it subscribes to; an endpoint cannot subscribe to it.
	EventTest EventType = "webhook.test"
)

// Permission is a permission of the roles of the identity module (role_permissions.permission)
// that subscribing to an event needs.
type Permission string

// Permissions that the events need.
const (
	PermissionManageReports      Permission = "manage_reports"
	PermissionManageAccounts     Permission = "manage_accounts"
	PermissionManageDomains      Permission = "manage_domains"
	PermissionManageCustomEmojis Permission = "manage_custom_emojis"
	PermissionManageWebhooks     Permission = "manage_webhooks"
	PermissionManageErrors       Permission = "manage_errors"
)

// subscribable lists the events an endpoint can subscribe to, in the order of the schema's
// CHECK, with the permission each needs on top of manage_webhooks ("" for none).
var subscribable = []struct {
	event EventType
	perm  Permission
}{
	{EventReportCreated, PermissionManageReports},
	{EventReportResolved, PermissionManageReports},
	{EventReportReopened, PermissionManageReports},
	{EventRegistrationPending, PermissionManageAccounts},
	{EventAccountCreated, PermissionManageAccounts},
	{EventAccountUpdated, PermissionManageAccounts},
	{EventModerationActionCreated, PermissionManageAccounts},
	{EventModerationActionLifted, PermissionManageAccounts},
	{EventAppealCreated, PermissionManageAccounts},
	{EventDomainModerationActionCreated, PermissionManageDomains},
	{EventDomainModerationActionLifted, PermissionManageDomains},
	{EventStatusCreated, ""},
	{EventCustomEmojiCreated, PermissionManageCustomEmojis},
	{EventCustomEmojiUpdated, PermissionManageCustomEmojis},
	{EventCustomEmojiDeleted, PermissionManageCustomEmojis},
	{EventErrorGroupCreated, PermissionManageErrors},
	{EventErrorGroupRegressed, PermissionManageErrors},
}

// EventTypes returns the events an endpoint can subscribe to, in a stable order.
func EventTypes() []EventType {
	out := make([]EventType, len(subscribable))
	for i, s := range subscribable {
		out[i] = s.event
	}
	return out
}

// Subscribable reports whether an endpoint can subscribe to e.
func (e EventType) Subscribable() bool {
	_, ok := e.RequiredPermission()
	return ok
}

// RequiredPermission returns the permission that subscribing to e needs besides
// manage_webhooks ("" for none), and false for an event that no endpoint can subscribe to.
func (e EventType) RequiredPermission() (Permission, bool) {
	for _, s := range subscribable {
		if s.event == e {
			return s.perm, true
		}
	}
	return "", false
}

// MissingPermissions returns, sorted and without duplicates, the permissions that subscribing to
// events needs and that granted does not hold.
func MissingPermissions(events []EventType, granted []Permission) []Permission {
	var out []Permission
	for _, e := range events {
		p, ok := e.RequiredPermission()
		if !ok || p == "" || slices.Contains(granted, p) || slices.Contains(out, p) {
			continue
		}
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// Limits of an endpoint.
const (
	// MaxURL is the number of bytes of an endpoint's URL.
	MaxURL = 2048
	// MaxDescription is the number of characters of an endpoint's description.
	MaxDescription = 200
)

var (
	// ErrInvalidURL reports an endpoint URL that is not an absolute http or https URL without
	// user info and fragment, or that is too long.
	ErrInvalidURL = errors.New("webhook: invalid endpoint URL")
	// ErrInvalidDescription reports a description that is too long or has control characters.
	ErrInvalidDescription = errors.New("webhook: invalid description")
	// ErrInvalidEvents reports a subscription to no event, to an unknown one, or to one twice.
	ErrInvalidEvents = errors.New("webhook: invalid events")
)

// EndpointSettings are what an administrator sets on an endpoint: one revision of it.
type EndpointSettings struct {
	URL         string
	Description string // "" for none
	// Events are the subscribed events, sorted.
	Events []EventType
}

// Normalize trims the description and sorts the events.
func (s EndpointSettings) Normalize() EndpointSettings {
	s.URL = strings.TrimSpace(s.URL)
	s.Description = strings.TrimSpace(s.Description)
	s.Events = slices.Clone(s.Events)
	slices.Sort(s.Events)
	return s
}

// Validate checks the settings, normalized.
func (s EndpointSettings) Validate() error {
	if err := ValidateURL(s.URL); err != nil {
		return err
	}
	if utf8.RuneCountInString(s.Description) > MaxDescription || strings.ContainsFunc(s.Description, isControl) {
		return ErrInvalidDescription
	}
	if len(s.Events) == 0 {
		return fmt.Errorf("%w: no event", ErrInvalidEvents)
	}
	for i, e := range s.Events {
		if !e.Subscribable() {
			return fmt.Errorf("%w: %q", ErrInvalidEvents, e)
		}
		if i > 0 && s.Events[i-1] == e {
			return fmt.Errorf("%w: %q twice", ErrInvalidEvents, e)
		}
	}
	return nil
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }

// ValidateURL checks an endpoint URL: an absolute http or https URL with a host, without user
// info or a fragment, of at most MaxURL bytes. Whether the destination may be reached is the
// HTTP client's decision, made on every delivery (ADR-0060 "宛先の HTTP").
func ValidateURL(raw string) error {
	if raw == "" || len(raw) > MaxURL || strings.ContainsFunc(raw, isControl) {
		return ErrInvalidURL
	}
	u, err := url.Parse(raw)
	switch {
	case err != nil:
		return ErrInvalidURL
	case u.Scheme != "https" && u.Scheme != "http":
		return fmt.Errorf("%w: scheme %q", ErrInvalidURL, u.Scheme)
	case u.Host == "" || u.Hostname() == "":
		return fmt.Errorf("%w: no host", ErrInvalidURL)
	case u.User != nil:
		return fmt.Errorf("%w: user info", ErrInvalidURL)
	case u.Fragment != "" || strings.Contains(raw, "#"):
		return fmt.Errorf("%w: fragment", ErrInvalidURL)
	}
	return nil
}

// Host returns the host name of an endpoint URL, which the logs and the audit log show instead
// of the URL: Discord and Slack put a secret in the path or the query (ADR-0060).
func Host(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// AuditLabel returns the target label of an endpoint in the audit log: its description and the
// host name of its URL.
func AuditLabel(s EndpointSettings) string {
	host := Host(s.URL)
	if s.Description == "" {
		return host
	}
	return s.Description + " (" + host + ")"
}

// State is the state of an endpoint (webhook_endpoint_state_changes.state).
type State string

// States.
const (
	StateEnabled  State = "enabled"
	StateDisabled State = "disabled"
	StateDeleted  State = "deleted"
)

// StateReason is why an endpoint changed state (webhook_endpoint_state_changes.reason).
type StateReason string

// Reasons.
const (
	// ReasonAdmin: an administrator (or the CLI) changed it.
	ReasonAdmin StateReason = "admin"
	// ReasonGone: the endpoint answered 410 Gone.
	ReasonGone StateReason = "gone"
	// ReasonFailing: the deliveries kept failing (ShouldDisable).
	ReasonFailing StateReason = "failing"
)

// Endpoint is an endpoint with its current settings, state, and failures.
type Endpoint struct {
	ID              string
	CreatedByUserID string // "" for the CLI
	CreatedAt       time.Time
	RevisionID      string
	Settings        EndpointSettings
	RevisedAt       time.Time
	State           State
	// StateReason and StateChangedAt describe the last change of State.
	StateReason    StateReason
	StateChangedAt time.Time
	// SecretID is the current secret's row.
	SecretID string
	// Failures is the run of failed attempts since the last success, nil without one.
	Failures *Failures
}

// Failures is a run of failed delivery attempts of an endpoint since its last success
// (webhook_endpoint_failures).
type Failures struct {
	FirstFailureAttemptID string
	FirstFailureAt        time.Time
	Consecutive           int
}

// Rules of automatic disabling (ADR-0060 "自動の無効化").
const (
	// DisableAfter is how long after the first failure of a run, without a success, an endpoint
	// may be disabled.
	DisableAfter = 72 * time.Hour
	// DisableAfterFailures is how many consecutive failures it takes besides.
	DisableAfterFailures = 20
)

// ShouldDisable reports whether a run of failures disables its endpoint at now: at least
// DisableAfterFailures failures, the first of them at least DisableAfter ago.
func (f Failures) ShouldDisable(now time.Time) bool {
	return f.Consecutive >= DisableAfterFailures && !now.Before(f.FirstFailureAt.Add(DisableAfter))
}
