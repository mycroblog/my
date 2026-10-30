// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"fmt"
	"slices"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/i18n"
)

// The notification settings (FR-210, ADR-0109 §2, §3, §6): a local account chooses, for each
// group of notification types it gets, whether it gets them in the app and by email. The
// projectors read the choices in the transaction that creates a notification: a group turned
// off in the app creates nothing, so neither Web Push nor email delivers it; the email of a
// group turned on is enqueued with the notification (email.go).

// Cell is one channel of one row of the settings as a user sees it.
type Cell struct {
	Enabled bool
	// Fixed reports a channel that is always on, which the user cannot choose.
	Fixed bool
	// Absent reports a channel that the row does not have (the email of status): always off,
	// and not shown.
	Absent bool
}

// GroupSettings is one row of a user's settings: the group and its channels.
type GroupSettings struct {
	Spec  notification.GroupSpec
	InApp Cell
	Email Cell
}

// Settings returns the rows of accountID's notification settings, in the order of the table: the
// ordinary notifications and the instance's notices, without the supporter row when the
// supporter program is off (Deps.SupportersEnabled), and the moderator notifications whose
// permission the roles of accountID's user grant.
func (s *Service) Settings(ctx context.Context, accountID string) ([]GroupSettings, error) {
	offered, err := s.offeredGroups(ctx, accountID)
	if err != nil {
		return nil, err
	}
	saved, err := s.d.Repository.NotificationSettings(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return settingsRows(offered, notification.NewChoices(saved)), nil
}

// UpdateSettings stores accountID's choices of settings and returns the rows as Settings does.
// Every choice must name a row the account is offered and a channel it can choose: an unknown or
// unoffered group or an unknown channel fails with notification.ErrInvalidSetting, a fixed one
// with notification.ErrFixedSetting, and nothing is stored then.
func (s *Service) UpdateSettings(ctx context.Context, accountID string, choices []notification.Setting) ([]GroupSettings, error) {
	offered, err := s.offeredGroups(ctx, accountID)
	if err != nil {
		return nil, err
	}
	for i := range choices {
		choices[i].AccountID = accountID
		if err := choices[i].Validate(); err != nil {
			return nil, fmt.Errorf("%s %s: %w", choices[i].Group, choices[i].Channel, err)
		}
		if !slices.ContainsFunc(offered, func(g notification.GroupSpec) bool { return g.Group == choices[i].Group }) {
			return nil, fmt.Errorf("%s: %w", choices[i].Group, notification.ErrInvalidSetting)
		}
	}
	if len(choices) > 0 {
		err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return s.d.Repository.SaveNotificationSettings(ctx, tx, choices, s.d.Clock.Now())
		})
		if err != nil {
			return nil, err
		}
	}
	return s.Settings(ctx, accountID)
}

// TurnOffEmail turns off the email of group g for accountID: the one-click unsubscribe of the
// group's emails (ADR-0109 §4). It does not check that the account is offered g, since the email
// came from it. A group whose email the user cannot choose fails with
// notification.ErrFixedSetting, an unknown one with notification.ErrInvalidSetting.
func (s *Service) TurnOffEmail(ctx context.Context, accountID string, g notification.Group) error {
	off := notification.Setting{AccountID: accountID, Group: g, Channel: notification.ChannelEmail}
	if err := off.Validate(); err != nil {
		return err
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.SaveNotificationSettings(ctx, tx, []notification.Setting{off}, s.d.Clock.Now())
	})
}

// EmailEnabled reports whether accountID gets the notifications of type t by email now: its
// group is on in the app and by email.
func (s *Service) EmailEnabled(ctx context.Context, accountID string, t notification.Type) (bool, error) {
	saved, err := s.d.Repository.NotificationSettings(ctx, accountID)
	if err != nil {
		return false, err
	}
	c := notification.NewChoices(saved)
	return c.TypeEnabled(t, notification.ChannelInApp) && c.TypeEnabled(t, notification.ChannelEmail), nil
}

// Unsubscribable reports whether g is a group whose emails carry an unsubscribe link, which
// turns its email off: a known group whose email the user chooses.
func Unsubscribable(g notification.Group) bool {
	spec, ok := notification.FindGroup(g)
	return ok && spec.Unsubscribable()
}

// offeredGroups returns the rows that accountID is offered.
func (s *Service) offeredGroups(ctx context.Context, accountID string) ([]notification.GroupSpec, error) {
	var granted []notification.Permission
	if s.d.Staff != nil {
		var err error
		if granted, err = s.d.Staff.GrantedPermissions(ctx, accountID); err != nil {
			return nil, fmt.Errorf("notification: the permissions of %s: %w", accountID, err)
		}
	}
	supporters := false
	if s.d.SupportersEnabled != nil {
		var err error
		if supporters, err = s.d.SupportersEnabled(ctx); err != nil {
			return nil, err
		}
	}
	out := make([]notification.GroupSpec, 0, len(notification.Groups))
	for _, g := range notification.Groups {
		if p, ok := g.Permission(); ok && !slices.Contains(granted, p) {
			continue
		}
		if g.Supporter() && !supporters {
			continue
		}
		out = append(out, g)
	}
	return out, nil
}

func settingsRows(groups []notification.GroupSpec, c notification.Choices) []GroupSettings {
	out := make([]GroupSettings, len(groups))
	for i, g := range groups {
		out[i] = GroupSettings{
			Spec:  g,
			InApp: Cell{Enabled: c.Enabled(g.Group, notification.ChannelInApp), Fixed: g.InApp.Fixed},
			Email: Cell{Enabled: c.Enabled(g.Group, notification.ChannelEmail), Fixed: g.Email.Fixed, Absent: g.Email.Absent},
		}
	}
	return out
}

// choicesTx returns inside tx the choices of accountID.
func choicesTx(ctx context.Context, repo Repository, tx uow.Tx, accountID string) (notification.Choices, error) {
	saved, err := repo.NotificationSettingsTx(ctx, tx, accountID)
	if err != nil {
		return nil, fmt.Errorf("notification: the settings of %s: %w", accountID, err)
	}
	return notification.NewChoices(saved), nil
}

// GroupLabel returns the name of group g as the notification settings show it, for the emails
// and the unsubscribe page; "" for a group that has no email to name it in.
func GroupLabel(l *i18n.Localizer, g notification.Group) string {
	switch g {
	case notification.GroupMention:
		return l.ServerMailGroupMention()
	case notification.GroupFollow:
		return l.ServerMailGroupFollow()
	case notification.GroupFollowRequest:
		return l.ServerMailGroupFollowRequest()
	case notification.GroupFollowAccepted:
		return l.ServerMailGroupFollowAccepted()
	case notification.GroupBoost:
		return l.ServerMailGroupBoost()
	case notification.GroupFavourite:
		return l.ServerMailGroupFavourite()
	case notification.GroupReaction:
		return l.ServerMailGroupReaction()
	case notification.GroupQuote:
		return l.ServerMailGroupQuote()
	case notification.GroupPollClosed:
		return l.ServerMailGroupPollClosed()
	case notification.GroupAccountMoved:
		return l.ServerMailGroupAccountMoved()
	case notification.GroupVoice:
		return l.ServerMailGroupVoice()
	case notification.GroupAdminReport:
		return l.ServerMailGroupAdminReport()
	case notification.GroupAdminSignUp:
		return l.ServerMailGroupAdminSignUp()
	case notification.GroupAdminAppeal:
		return l.ServerMailGroupAdminAppeal()
	case notification.GroupAdminWebhookDisabled:
		return l.ServerMailGroupAdminWebhookDisabled()
	case notification.GroupAdminCustomEmojiRequest:
		return l.ServerMailGroupAdminCustomEmojiRequest()
	case notification.GroupAdminAccountMoved:
		return l.ServerMailGroupAdminAccountMoved()
	case notification.GroupAdminServiceClosureDue:
		return l.ServerMailGroupAdminServiceClosureDue()
	case notification.GroupAdminErrorGroup:
		return l.ServerMailGroupAdminErrorGroup()
	case notification.GroupModeration:
		return l.ServerMailGroupModeration()
	}
	return ""
}
