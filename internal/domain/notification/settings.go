// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"errors"
	"slices"
	"time"
)

// The notification settings (FR-210, ADR-0109 §2, §6): for each group of notification types and
// each channel that the server keeps per user (in the app and by email), whether the user gets
// them. A channel of a group either takes the user's choice, with a default for a user who has
// not chosen, or is fixed on. Web Push stays the alerts of each subscription and the sound a
// setting of each device, so neither is a channel here.

// Channel is a way a notification reaches its recipient that the settings choose
// (notification_settings.channel).
type Channel string

// Channels.
const (
	// ChannelInApp creates the notification: the list, the unread count, and the real-time
	// updates. Turned off, the notification is not created, so no other channel delivers it.
	ChannelInApp Channel = "in_app"
	// ChannelEmail sends the notification by email to the user's primary address.
	ChannelEmail Channel = "email"
)

// Channels are the channels of the settings, in the order of the table's columns.
var Channels = []Channel{ChannelInApp, ChannelEmail}

// Valid reports whether c is a known channel.
func (c Channel) Valid() bool { return c == ChannelInApp || c == ChannelEmail }

// Group is a row of the notification settings: one or more notification types that the user
// turns on and off together (notification_settings.notification_group). The ID of an
// unsubscribable group is also the category of its emails' unsubscribe links (ADR-0109 §4).
type Group string

// Groups. The ordinary notifications are one group per type, except the voice chat
// notifications; the moderator notifications are one group per type; the instance's notices to
// the user are three groups.
const (
	GroupMention        Group = "mention"
	GroupFollow         Group = "follow"
	GroupFollowRequest  Group = "follow_request"
	GroupFollowAccepted Group = "follow_accepted"
	GroupBoost          Group = "boost"
	GroupFavourite      Group = "favourite"
	GroupReaction       Group = "reaction"
	GroupQuote          Group = "quote"
	GroupPollClosed     Group = "poll_closed"
	GroupAccountMoved   Group = "account_moved"
	// GroupVoice is voice_room_started and voice_room_invited.
	GroupVoice Group = "voice"
	// GroupStatus is the new posts of the accounts that the user subscribes to (ADR-0123 §3); it
	// has no email channel.
	GroupStatus Group = "status"

	GroupAdminReport             Group = "admin_report"
	GroupAdminSignUp             Group = "admin_sign_up"
	GroupAdminAppeal             Group = "admin_appeal"
	GroupAdminWebhookDisabled    Group = "admin_webhook_disabled"
	GroupAdminCustomEmojiRequest Group = "admin_custom_emoji_request"
	GroupAdminAccountMoved       Group = "admin_account_moved"
	GroupAdminServiceClosureDue  Group = "admin_service_closure_due"
	GroupAdminErrorGroup         Group = "admin_error_group"

	// GroupModeration is moderation_notice, report_resolved, appeal_decided,
	// relationships_severed, and custom_emoji_request_decided.
	GroupModeration     Group = "moderation"
	GroupServiceClosure Group = "service_closure"
	// GroupSupporter is the supporter notifications (Type.Supporter).
	GroupSupporter Group = "supporter"
	// GroupAnnouncement is the server-wide announcements (ADR-0125 §2): unlike the other notices
	// of the instance, the user turns it off in the app (on by default), and it has no email
	// channel.
	GroupAnnouncement Group = "announcement"
)

// Section is the kind of a group, which decides its defaults and how its emails are sent.
type Section string

// Sections.
const (
	// SectionOrdinary is what other users did. Both channels take the user's choice: in the app
	// on and email off by default. The email waits for the notification to stay unread
	// (EmailDelay).
	SectionOrdinary Section = "ordinary"
	// SectionModerator is the moderator notifications, to the holders of their permission. Both
	// channels take the user's choice and are on by default; the email is sent at once.
	SectionModerator Section = "moderator"
	// SectionInstance is the instance's notices to the user: always in the app. The moderation
	// notices take the user's choice of email, off by default, and wait as the ordinary ones do;
	// the service closure and the supporter notifications are always emailed, as transactional
	// email.
	SectionInstance Section = "instance"
)

// EmailDelay is how long after a notification its email waits to see whether the recipient
// read it (ADR-0109 §4): for the groups whose email is DelayedEmail.
const EmailDelay = 15 * time.Minute

// Rule is how a channel of a group is set: Default is its value until the user chooses, a
// Fixed channel is always on and cannot be chosen, and an Absent channel is never offered: it is
// always off and cannot be chosen.
type Rule struct {
	Default bool
	Fixed   bool
	Absent  bool
}

// GroupSpec is a row of the settings: its group, its section, its notification types, and the
// rules of its channels.
type GroupSpec struct {
	Group   Group
	Section Section
	Types   []Type
	InApp   Rule
	Email   Rule
}

// Rule returns the rule of channel c.
func (g GroupSpec) Rule(c Channel) Rule {
	if c == ChannelEmail {
		return g.Email
	}
	return g.InApp
}

// DelayedEmail reports whether the notifications of g are emailed by the job that waits
// EmailDelay for them to stay unread: the ordinary notifications with an email channel and the
// moderation notices.
// The moderator notifications are emailed at once, and the service closure and supporter
// notifications by their own transactional emails.
func (g GroupSpec) DelayedEmail() bool {
	return !g.Email.Absent && (g.Section == SectionOrdinary || g.Group == GroupModeration)
}

// Unsubscribable reports whether the emails of g carry an unsubscribe link, which turns off the
// email of g: the groups whose email the user chooses.
func (g GroupSpec) Unsubscribable() bool { return !g.Email.Fixed && !g.Email.Absent }

// Permission returns the permission that the recipient of the group's notifications needs, for
// a moderator group.
func (g GroupSpec) Permission() (Permission, bool) {
	if g.Section != SectionModerator || len(g.Types) == 0 {
		return "", false
	}
	return ModeratorPermission(g.Types[0])
}

// Supporter reports whether g is the supporter notifications, which only a server with the
// supporter program offers (ADR-0072 §14).
func (g GroupSpec) Supporter() bool { return g.Group == GroupSupporter }

var (
	ordinaryRules  = [2]Rule{{Default: true}, {Default: false}}
	moderatorRules = [2]Rule{{Default: true}, {Default: true}}
)

func ordinary(g Group, types ...Type) GroupSpec {
	if len(types) == 0 {
		types = []Type{Type(g)}
	}
	return GroupSpec{Group: g, Section: SectionOrdinary, Types: types, InApp: ordinaryRules[0], Email: ordinaryRules[1]}
}

func moderator(t Type) GroupSpec {
	return GroupSpec{Group: Group(t), Section: SectionModerator, Types: []Type{t}, InApp: moderatorRules[0], Email: moderatorRules[1]}
}

// Groups are the rows of the settings, in the order of the table (ADR-0109 §2). Every
// notification type is in exactly one group.
var Groups = []GroupSpec{
	ordinary(GroupMention), ordinary(GroupFollow), ordinary(GroupFollowRequest), ordinary(GroupFollowAccepted),
	ordinary(GroupBoost), ordinary(GroupFavourite), ordinary(GroupReaction), ordinary(GroupQuote),
	ordinary(GroupPollClosed), ordinary(GroupAccountMoved),
	ordinary(GroupVoice, TypeVoiceRoomStarted, TypeVoiceRoomInvited),
	{Group: GroupStatus, Section: SectionOrdinary, Types: []Type{TypeStatus}, InApp: ordinaryRules[0], Email: Rule{Absent: true}},
	moderator(TypeAdminReport), moderator(TypeAdminSignUp), moderator(TypeAdminAppeal), moderator(TypeAdminWebhookDisabled),
	moderator(TypeAdminCustomEmojiRequest), moderator(TypeAdminAccountMoved), moderator(TypeAdminServiceClosureDue),
	moderator(TypeAdminErrorGroup),
	{
		Group: GroupModeration, Section: SectionInstance,
		Types: []Type{TypeModerationNotice, TypeReportResolved, TypeAppealDecided, TypeRelationshipsSevered, TypeCustomEmojiRequestDecided},
		InApp: Rule{Default: true, Fixed: true}, Email: Rule{Default: false},
	},
	{
		Group: GroupServiceClosure, Section: SectionInstance, Types: []Type{TypeServiceClosure},
		InApp: Rule{Default: true, Fixed: true}, Email: Rule{Default: true, Fixed: true},
	},
	{
		Group: GroupSupporter, Section: SectionInstance,
		Types: []Type{TypeSupporterPaymentFailed, TypeSupporterSubscriptionEnded, TypeSupporterPlanRetired, TypeSupporterServiceClosure},
		InApp: Rule{Default: true, Fixed: true}, Email: Rule{Default: true, Fixed: true},
	},
	{
		Group: GroupAnnouncement, Section: SectionInstance, Types: []Type{TypeAnnouncement},
		InApp: Rule{Default: true}, Email: Rule{Absent: true},
	},
}

// FindGroup returns the row of group g.
func FindGroup(g Group) (GroupSpec, bool) {
	i := slices.IndexFunc(Groups, func(s GroupSpec) bool { return s.Group == g })
	if i < 0 {
		return GroupSpec{}, false
	}
	return Groups[i], true
}

// GroupOf returns the row of the notifications of type t.
func GroupOf(t Type) (GroupSpec, bool) {
	i := slices.IndexFunc(Groups, func(s GroupSpec) bool { return slices.Contains(s.Types, t) })
	if i < 0 {
		return GroupSpec{}, false
	}
	return Groups[i], true
}

// Setting is a user's choice of one channel of one group (notification_settings, a current
// value): the row exists once the user chose, and the group's default applies until then.
type Setting struct {
	AccountID string
	Group     Group
	Channel   Channel
	Enabled   bool
}

// Errors of the settings.
var (
	// ErrInvalidSetting reports a group or a channel that is unknown, or a group that the user
	// is not offered.
	ErrInvalidSetting = errors.New("notification: invalid notification setting")
	// ErrFixedSetting reports a choice of a channel that is fixed on.
	ErrFixedSetting = errors.New("notification: the notification setting is fixed")
)

// Validate checks that s names a known group and channel that the user can choose.
func (s Setting) Validate() error {
	g, ok := FindGroup(s.Group)
	if !ok || !s.Channel.Valid() {
		return ErrInvalidSetting
	}
	if g.Rule(s.Channel).Absent {
		return ErrInvalidSetting
	}
	if g.Rule(s.Channel).Fixed {
		return ErrFixedSetting
	}
	return nil
}

// Choices are a user's settings, which answer for every group and channel.
type Choices map[ChoiceKey]bool

// ChoiceKey is a group and a channel.
type ChoiceKey struct {
	Group   Group
	Channel Channel
}

// NewChoices returns the choices of the settings ss.
func NewChoices(ss []Setting) Choices {
	c := make(Choices, len(ss))
	for _, s := range ss {
		c[ChoiceKey{Group: s.Group, Channel: s.Channel}] = s.Enabled
	}
	return c
}

// Enabled reports whether channel ch of group g is on: always for a fixed channel, never for an
// absent one, the user's choice when there is one, and the group's default otherwise. An unknown
// group is on in the app and off by email.
func (c Choices) Enabled(g Group, ch Channel) bool {
	spec, ok := FindGroup(g)
	if !ok {
		return ch == ChannelInApp
	}
	rule := spec.Rule(ch)
	if rule.Absent {
		return false
	}
	if rule.Fixed {
		return true
	}
	if v, ok := c[ChoiceKey{Group: g, Channel: ch}]; ok {
		return v
	}
	return rule.Default
}

// TypeEnabled reports whether channel ch is on for the notifications of type t.
func (c Choices) TypeEnabled(t Type, ch Channel) bool {
	g, ok := GroupOf(t)
	if !ok {
		return ch == ChannelInApp
	}
	return c.Enabled(g.Group, ch)
}
