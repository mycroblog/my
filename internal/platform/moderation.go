// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// setUpModeration builds the moderation use cases, the policy that federation and the media
// proxy read the domain and account restrictions through (docs/architecture/federation.md §11),
// and the policy of the accounts' restrictions (data-model §5.3). It needs the account module,
// and comes before the identity module and everything that federates. It also registers the
// job that deletes the accounts whose deletion is due, which bans schedule, and
// the audit log projector. Suspensions cut follows through the graph module (FR-717), which is
// built after it.
func (a *App) setUpModeration(repo moderation.Repository) {
	outbox := a.Outbox
	if outbox == nil {
		// The inline queue has no transactional enqueue.
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	a.Moderation = moderation.NewService(moderation.Deps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Accounts: a.Accounts, Clock: a.Clock, IDs: a.IDs,
		Statuses: moderationStatuses{a}, Lifecycles: a.Accounts, AppealWindow: appealWindow{a},
		Retention: retentionSettings{a},
		Follows:   moderationFollows{a}, Directory: a.Accounts, Outbox: outbox,
		Emails: moderationEmails{a}, CanonicalDomain: federation.CanonicalHost,
		Evidence: moderationEvidence{a}, AuditTargets: moderationAuditTargets{a},
	})
	a.ModerationPolicy = moderation.NewFederationPolicy(repo, a.Clock)
	a.AccountPolicy = moderation.NewAccountPolicy(repo, a.Accounts, a.Clock)
	// The removal of expired restrictions, which ends expired suspensions on other servers
	// (federation.md §11.1).
	a.Moderation.RegisterJobs(a.Handlers, a.Scheduler)
	a.Accounts.RegisterDeletionJobs(a.Handlers, a.Scheduler)
	// The administrative use cases of every module emit their audit log entries; the projector
	// writes them in the transaction of the operation (ADR-0037).
	moderation.RegisterAuditProjector(a.Projectors, repo)
}

// appealWindow reads the appeal window of the moderation module from the instance setting
// appeal_window (ADR-0038).
type appealWindow struct{ a *App }

func (w appealWindow) AppealWindow(ctx context.Context) (time.Duration, error) {
	settings, err := w.a.Instance.Settings(ctx)
	if err != nil {
		return 0, err
	}
	return settings.AppealWindow(), nil
}

// retentionSettings reads the retention periods of the moderation records from the instance
// settings (ADR-0038).
type retentionSettings struct{ a *App }

func (r retentionSettings) RetentionPeriods(ctx context.Context) (moderation.RetentionPeriods, error) {
	settings, err := r.a.Instance.Settings(ctx)
	if err != nil {
		return moderation.RetentionPeriods{}, err
	}
	return moderation.RetentionPeriods{
		Reports:  settings.Retention(instance.SettingRetentionReports),
		Actions:  settings.Retention(instance.SettingRetentionModerationActions),
		AuditLog: settings.Retention(instance.SettingRetentionAuditLog),
	}, nil
}

// originRetention reads the policy of the identity module's origin purge: the periods of the
// instance settings and the users of the local accounts that a retention hold preserves
// (ADR-0038).
type originRetention struct{ a *App }

func (o originRetention) OriginRetention(ctx context.Context) (identity.OriginRetentionPolicy, error) {
	settings, err := o.a.Instance.Settings(ctx)
	if err != nil {
		return identity.OriginRetentionPolicy{}, err
	}
	held, err := o.a.Moderation.HeldAccountIDs(ctx)
	if err != nil {
		return identity.OriginRetentionPolicy{}, err
	}
	pol := identity.OriginRetentionPolicy{
		SessionPeriod:      settings.Retention(instance.SettingRetentionSessionOrigins),
		RegistrationPeriod: settings.Retention(instance.SettingRetentionRegistrationOrigins),
	}
	for _, id := range held {
		actor, err := o.a.Accounts.FindByID(ctx, id)
		switch {
		case errors.Is(err, account.ErrNotFound):
			continue
		case err != nil:
			return identity.OriginRetentionPolicy{}, err
		}
		if actor.Local != nil && actor.Local.UserID != "" {
			pol.HeldUserIDs = append(pol.HeldUserIDs, actor.Local.UserID)
		}
	}
	return pol, nil
}

// moderationStatuses gives the moderation module the content module, which is built after it
// because it reads the moderation restrictions.
type moderationStatuses struct{ a *App }

func (m moderationStatuses) Find(ctx context.Context, statusID string) (content.Post, error) {
	return m.a.Content.Find(ctx, statusID)
}

func (m moderationStatuses) DeleteByModeration(ctx context.Context, tx uow.Tx, st content.Status, actionID string, at time.Time) error {
	return m.a.Content.DeleteByModeration(ctx, tx, st, actionID, at)
}

func (m moderationStatuses) MarkSensitiveByModeration(ctx context.Context, tx uow.Tx, statusID, actionID string) error {
	return m.a.Content.MarkSensitiveByModeration(ctx, tx, statusID, actionID)
}

func (m moderationStatuses) ClearSensitivityOf(ctx context.Context, tx uow.Tx, actionID string) error {
	return m.a.Content.ClearSensitivityOf(ctx, tx, actionID)
}

// moderationEvidence gives the moderation module the status reader, which is built after it,
// to present the statuses of a report to the moderators.
type moderationEvidence struct{ a *App }

func (m moderationEvidence) Review(ctx context.Context, statusID string) (content.ReviewedStatus, error) {
	return m.a.Statuses.Review(ctx, statusID)
}

// moderationAuditTargets gives the moderation module the roles, login providers, custom emoji, webhooks, and relays that
// the audit log names, of modules that are built after it.
type moderationAuditTargets struct{ a *App }

func (m moderationAuditTargets) ExistingAuditTargets(ctx context.Context, kind moderation.AuditTargetKind, ids []string) ([]string, error) {
	switch {
	case kind == moderation.AuditTargetRole && m.a.Identity != nil:
		return m.a.Identity.ExistingRoleIDs(ctx, ids)
	case kind == moderation.AuditTargetLoginProvider && m.a.Identity != nil:
		return m.a.Identity.ExistingLoginProviderIDs(ctx, ids)
	case kind == moderation.AuditTargetCustomEmoji && m.a.CustomEmojis != nil:
		return m.a.CustomEmojis.ExistingIDs(ctx, ids)
	case kind == moderation.AuditTargetCustomEmojiRequest && m.a.CustomEmojis != nil:
		return m.a.CustomEmojis.ExistingRequestIDs(ctx, ids)
	case kind == moderation.AuditTargetWebhook && m.a.Webhooks != nil:
		return m.a.Webhooks.ExistingIDs(ctx, ids)
	case kind == moderation.AuditTargetRelay && m.a.Relays != nil:
		return m.a.Relays.ExistingIDs(ctx, ids)
	}
	// A module that is not built has removed nothing.
	return ids, nil
}

// moderationFollows gives the moderation module the graph module, which is built after it
// because it reads the moderation restrictions.
type moderationFollows struct{ a *App }

func (m moderationFollows) Counterparts(ctx context.Context, tx uow.Tx, accountID string) ([]string, error) {
	return m.a.Graph.Counterparts(ctx, tx, accountID)
}

func (m moderationFollows) SeverByModeration(ctx context.Context, tx uow.Tx, accountID string, others []string, announce bool, now time.Time) (graph.Severed, error) {
	return m.a.Graph.SeverByModeration(ctx, tx, accountID, others, announce, now)
}

// moderationEmails gives the moderation module the identity module, which is built after it
// because it reads the moderation restrictions, for the sign-up block that a ban makes of the
// banned user's email address.
type moderationEmails struct{ a *App }

func (m moderationEmails) PrimaryEmailAddress(ctx context.Context, userID string) (string, error) {
	return m.a.Identity.PrimaryEmailAddress(ctx, userID)
}

// setUpLocalReports builds the filing of local users' reports, which checks the reported
// statuses through the status reader and the cited rules through the instance module, so it
// comes after both.
func (a *App) setUpLocalReports(repo moderation.Repository) {
	a.LocalReports = moderation.NewLocalReports(moderation.LocalReportDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Accounts: a.Accounts, Statuses: a.Statuses,
		Rules: a.Instance, Clock: a.Clock, IDs: a.IDs,
	})
}
