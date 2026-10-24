// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"errors"
	"fmt"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The audit log (FR-704, ADR-0037). The administrative use cases of every module emit
// AuditRecorded in the transaction of the operation, after its source event; the projector
// that RegisterAuditProjector registers writes the entry in that transaction, so an operation
// and its entry commit or roll back together. Nothing updates or deletes an entry: there is no
// use case for it, and only the retention cleanup (ADR-0038) removes old entries.

// AuditEntry is moderation.AuditEntry, an entry of the audit log as other modules emit it.
type AuditEntry = moderation.AuditEntry

// AuditAction is moderation.AuditAction.
type AuditAction = moderation.AuditAction

// AuditTargetKind is moderation.AuditTargetKind.
type AuditTargetKind = moderation.AuditTargetKind

// AuditActorKind is moderation.AuditActorKind.
type AuditActorKind = moderation.AuditActorKind

// The audited operations and their targets that other modules emit.
const (
	AuditRegistrationApproved          = moderation.AuditRegistrationApproved
	AuditRegistrationRejected          = moderation.AuditRegistrationRejected
	AuditSettingChanged                = moderation.AuditSettingChanged
	AuditTextChanged                   = moderation.AuditTextChanged
	AuditRuleAdded                     = moderation.AuditRuleAdded
	AuditRuleRevised                   = moderation.AuditRuleRevised
	AuditRuleMoved                     = moderation.AuditRuleMoved
	AuditRuleRetired                   = moderation.AuditRuleRetired
	AuditRoleGranted                   = moderation.AuditRoleGranted
	AuditRoleRevoked                   = moderation.AuditRoleRevoked
	AuditRoleCreated                   = moderation.AuditRoleCreated
	AuditRoleUpdated                   = moderation.AuditRoleUpdated
	AuditRoleDeleted                   = moderation.AuditRoleDeleted
	AuditWebhookCreated                = moderation.AuditWebhookCreated
	AuditWebhookUpdated                = moderation.AuditWebhookUpdated
	AuditWebhookDeleted                = moderation.AuditWebhookDeleted
	AuditWebhookEnabled                = moderation.AuditWebhookEnabled
	AuditWebhookDisabled               = moderation.AuditWebhookDisabled
	AuditWebhookSecretRotated          = moderation.AuditWebhookSecretRotated
	AuditAnnouncementCreated           = moderation.AuditAnnouncementCreated
	AuditAnnouncementUpdated           = moderation.AuditAnnouncementUpdated
	AuditAnnouncementPublished         = moderation.AuditAnnouncementPublished
	AuditAnnouncementWithdrawn         = moderation.AuditAnnouncementWithdrawn
	AuditInviteCreated                 = moderation.AuditInviteCreated
	AuditInviteRevoked                 = moderation.AuditInviteRevoked
	AuditRelayAdded                    = moderation.AuditRelayAdded
	AuditRelayFollowed                 = moderation.AuditRelayFollowed
	AuditRelayUnfollowed               = moderation.AuditRelayUnfollowed
	AuditRelayRemoved                  = moderation.AuditRelayRemoved
	AuditRelayAccepted                 = moderation.AuditRelayAccepted
	AuditRelayRejected                 = moderation.AuditRelayRejected
	AuditStatusRefetchQueued           = moderation.AuditStatusRefetchQueued
	AuditAccountStatusesRefetchQueued  = moderation.AuditAccountStatusesRefetchQueued
	AuditDomainStatusesRefetchQueued   = moderation.AuditDomainStatusesRefetchQueued
	AuditBillingImportQueued           = moderation.AuditBillingImportQueued
	AuditCostItemCreated               = moderation.AuditCostItemCreated
	AuditCostItemRelabeled             = moderation.AuditCostItemRelabeled
	AuditCostItemPriceRevised          = moderation.AuditCostItemPriceRevised
	AuditCostItemEndChanged            = moderation.AuditCostItemEndChanged
	AuditCostItemCanceled              = moderation.AuditCostItemCanceled
	AuditSupporterProgramEnabled       = moderation.AuditSupporterProgramEnabled
	AuditSupporterProgramDisabled      = moderation.AuditSupporterProgramDisabled
	AuditSupporterPlanCreated          = moderation.AuditSupporterPlanCreated
	AuditSupporterPlanRevised          = moderation.AuditSupporterPlanRevised
	AuditSupporterPlanRetired          = moderation.AuditSupporterPlanRetired
	AuditSupporterSubscriptionCanceled = moderation.AuditSupporterSubscriptionCanceled
	AuditCustomEmojiRequestSubmitted   = moderation.AuditCustomEmojiRequestSubmitted
	AuditCustomEmojiRequestWithdrawn   = moderation.AuditCustomEmojiRequestWithdrawn
	AuditCustomEmojiRequestApproved    = moderation.AuditCustomEmojiRequestApproved
	AuditCustomEmojiRequestRejected    = moderation.AuditCustomEmojiRequestRejected
	AuditPaymentReconcileQueued        = moderation.AuditPaymentReconcileQueued
	AuditOAuthClientDisabled           = moderation.AuditOAuthClientDisabled
	AuditOAuthClientEnabled            = moderation.AuditOAuthClientEnabled
	AuditOAuthClientHostDenied         = moderation.AuditOAuthClientHostDenied
	AuditOAuthClientHostAllowed        = moderation.AuditOAuthClientHostAllowed

	AuditTargetUser                  = moderation.AuditTargetUser
	AuditTargetSetting               = moderation.AuditTargetSetting
	AuditTargetRule                  = moderation.AuditTargetRule
	AuditTargetRole                  = moderation.AuditTargetRole
	AuditTargetWebhook               = moderation.AuditTargetWebhook
	AuditTargetAnnouncement          = moderation.AuditTargetAnnouncement
	AuditTargetInvite                = moderation.AuditTargetInvite
	AuditTargetCustomEmoji           = moderation.AuditTargetCustomEmoji
	AuditTargetRelay                 = moderation.AuditTargetRelay
	AuditTargetStatus                = moderation.AuditTargetStatus
	AuditTargetDomain                = moderation.AuditTargetDomain
	AuditTargetBillingSource         = moderation.AuditTargetBillingSource
	AuditTargetCostItem              = moderation.AuditTargetCostItem
	AuditTargetSupporterProgram      = moderation.AuditTargetSupporterProgram
	AuditTargetSupporterPlan         = moderation.AuditTargetSupporterPlan
	AuditTargetSupporterSubscription = moderation.AuditTargetSupporterSubscription
	AuditTargetCustomEmojiRequest    = moderation.AuditTargetCustomEmojiRequest
	AuditTargetPaymentProvider       = moderation.AuditTargetPaymentProvider
	AuditTargetOAuthClient           = moderation.AuditTargetOAuthClient
	AuditTargetOAuthClientHost       = moderation.AuditTargetOAuthClientHost
)

// AuditActorSystem is moderation.AuditActorSystem, the server itself, and AuditActorRemote
// moderation.AuditActorRemote, another server.
const (
	AuditActorSystem = moderation.AuditActorSystem
	AuditActorRemote = moderation.AuditActorRemote
)

// AuditActor returns the actor of an operation given the user ID that the use cases take: the
// user, or the CLI for "".
func AuditActor(userID string) (AuditActorKind, string) { return moderation.AuditActor(userID) }

// EventAuditRecorded follows every administrative operation, in its transaction.
const EventAuditRecorded = "moderation.audit_recorded"

// AuditRecorded carries the audit log entry of an operation. Emitters generate its ID before the
// transaction, like the IDs of the operation's rows, so that a retry writes the same entry.
type AuditRecorded struct{ Entry AuditEntry }

// EventType implements uow.Event.
func (AuditRecorded) EventType() string { return EventAuditRecorded }

// AuditRepository is the persistence port of the audit log, part of Repository.
type AuditRepository interface {
	// InsertAuditEntry appends an entry. There is no method to update or delete one.
	InsertAuditEntry(ctx context.Context, tx uow.Tx, e moderation.AuditEntry) error
	// ListAuditEntries returns up to limit entries matching q, newest first.
	ListAuditEntries(ctx context.Context, q AuditQuery, limit int) ([]moderation.AuditEntry, error)
	// ExistingAuditTargets returns, in ID order, the targets among ids of a kind of target that
	// this module keeps (a report, an appeal, or a sign-up block) that exist.
	ExistingAuditTargets(ctx context.Context, kind moderation.AuditTargetKind, ids []string) ([]string, error)
}

// AuditQuery selects the entries of the audit log. Empty fields select every entry.
type AuditQuery struct {
	// ActorUserID keeps the operations of one user.
	ActorUserID string
	// Action keeps one kind of operation.
	Action moderation.AuditAction
	// TargetKind keeps the operations on one kind of target, and TargetID, which needs
	// TargetKind, on one target.
	TargetKind moderation.AuditTargetKind
	TargetID   string
	// Before is the ID of the last entry of the previous page; "" starts from the newest.
	Before string
}

// RegisterAuditProjector registers the projector that writes the audit log entry of every
// AuditRecorded, and of every appcontent.CustomEmojiAdministered and
// appcontent.CustomEmojiRequestRecorded, into the transaction that
// emits it. An invalid entry fails the operation: an operation that cannot be audited does not
// happen.
func RegisterAuditProjector(r *uow.Projectors, repo AuditRepository) {
	r.Register(EventAuditRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		entry := e.(AuditRecorded).Entry
		entry.TargetLabel = moderation.TruncateAuditLabel(entry.TargetLabel)
		if err := entry.Validate(); err != nil {
			return err
		}
		return repo.InsertAuditEntry(ctx, tx, entry)
	})
	// The content module cannot emit AuditRecorded (this module depends on it), so the operations
	// on custom emoji (ADR-0051 §5) are audited from its own event.
	r.Register(appcontent.EventCustomEmojiAdministered, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return r.Project(ctx, tx, AuditRecorded{Entry: customEmojiAuditEntry(e.(appcontent.CustomEmojiAdministered))})
	})
	r.Register(appcontent.EventCustomEmojiRequestRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return r.Project(ctx, tx, AuditRecorded{Entry: customEmojiRequestAuditEntry(e.(appcontent.CustomEmojiRequestRecorded))})
	})
}

// customEmojiRequestAuditEntry returns the audit log entry of an operation on a custom emoji
// request (ADR-0070 §8): its label is the :shortcode: of the request, and its source the
// outcome, if any.
func customEmojiRequestAuditEntry(e appcontent.CustomEmojiRequestRecorded) AuditEntry {
	entry := AuditEntry{
		ID: e.AuditID, Action: moderation.AuditAction(e.Action), TargetKind: moderation.AuditTargetCustomEmojiRequest,
		TargetID: e.Request.ID, TargetLabel: e.Request.Label(), OccurredAt: e.At,
	}
	if e.Outcome != nil {
		entry.SourceEventID = e.Outcome.ID
	}
	entry.ActorKind, entry.ActorUserID = moderation.AuditActor(e.ByUserID)
	return entry
}

// customEmojiAuditEntry returns the audit log entry of an operation on a custom emoji: its
// label is :shortcode:, or :shortcode:@domain for a remote emoji.
func customEmojiAuditEntry(e appcontent.CustomEmojiAdministered) AuditEntry {
	entry := AuditEntry{
		ID: e.AuditID, Action: moderation.AuditAction(e.Action), TargetKind: moderation.AuditTargetCustomEmoji,
		TargetID: e.Emoji.ID, TargetLabel: e.Emoji.Label(), SourceEventID: e.SourceEventID, OccurredAt: e.At,
	}
	entry.ActorKind, entry.ActorUserID = moderation.AuditActor(e.ByUserID)
	return entry
}

// AuditLog lists up to limit entries of the audit log, newest first. It fails with
// ErrInvalidQuery for an unknown action or target kind, or a target ID without its kind.
func (s *Service) AuditLog(ctx context.Context, q AuditQuery, limit int) ([]moderation.AuditEntry, error) {
	switch {
	case q.Action != "" && !q.Action.Valid():
		return nil, fmt.Errorf("%w: action %q", ErrInvalidQuery, q.Action)
	case q.TargetKind != "" && !q.TargetKind.Valid():
		return nil, fmt.Errorf("%w: target kind %q", ErrInvalidQuery, q.TargetKind)
	case q.TargetID != "" && q.TargetKind == "":
		return nil, fmt.Errorf("%w: a target ID needs its target kind", ErrInvalidQuery)
	}
	return s.d.Repository.ListAuditEntries(ctx, q, limit)
}

// AuditTarget names the target of an entry of the audit log.
type AuditTarget struct {
	Kind moderation.AuditTargetKind
	ID   string
}

// GoneAuditTargets returns the targets of entries that no longer exist: accounts, reports,
// appeals, and sign-up blocks erased (past their retention, ADR-0038), and roles, custom emoji,
// custom emoji requests, webhooks, and relays deleted (through Deps.AuditTargets). It looks up
// each kind of target once for all the entries. A domain, a user, a setting, a rule, an announcement, and an invite are never
// removed, a canceled cost item keeps its rows, and a status has no operation recorded on it;
// none of these is reported gone.
func (s *Service) GoneAuditTargets(ctx context.Context, entries []moderation.AuditEntry) (map[AuditTarget]bool, error) {
	ids := map[moderation.AuditTargetKind][]string{}
	seen := map[AuditTarget]bool{}
	for _, e := range entries {
		t := AuditTarget{Kind: e.TargetKind, ID: e.TargetID}
		if seen[t] {
			continue
		}
		seen[t] = true
		switch e.TargetKind {
		case moderation.AuditTargetAccount, moderation.AuditTargetReport, moderation.AuditTargetAppeal, moderation.AuditTargetSignupBlock:
			ids[e.TargetKind] = append(ids[e.TargetKind], e.TargetID)
		case moderation.AuditTargetRole, moderation.AuditTargetCustomEmoji, moderation.AuditTargetCustomEmojiRequest, moderation.AuditTargetWebhook,
			moderation.AuditTargetRelay:
			if s.d.AuditTargets != nil {
				ids[e.TargetKind] = append(ids[e.TargetKind], e.TargetID)
			}
		}
	}
	gone := map[AuditTarget]bool{}
	for kind, list := range ids {
		var existing []string
		var err error
		switch kind {
		case moderation.AuditTargetAccount:
			existing, err = s.d.Accounts.ExistingIDs(ctx, list)
		case moderation.AuditTargetRole, moderation.AuditTargetCustomEmoji, moderation.AuditTargetCustomEmojiRequest, moderation.AuditTargetWebhook,
			moderation.AuditTargetRelay:
			existing, err = s.d.AuditTargets.ExistingAuditTargets(ctx, kind, list)
		default:
			existing, err = s.d.Repository.ExistingAuditTargets(ctx, kind, list)
		}
		if err != nil {
			return nil, fmt.Errorf("moderation: the %s targets of the audit log: %w", kind, err)
		}
		for _, id := range list {
			if !slices.Contains(existing, id) {
				gone[AuditTarget{Kind: kind, ID: id}] = true
			}
		}
	}
	return gone, nil
}

// emitAudit emits the audit log entry of one of this module's operations, performed by byUserID
// ("" for the CLI).
func (s *Service) emitAudit(ctx context.Context, tx uow.Tx, e moderation.AuditEntry, byUserID string) error {
	e.ActorKind, e.ActorUserID = moderation.AuditActor(byUserID)
	return tx.Emit(ctx, AuditRecorded{Entry: e})
}

// accountLabel returns the display name of an account by ID for the audit log: its handle
// (@alice for a local account, @bob@example.com for a remote one), its URI when it has none, or
// its ID when it is gone.
func (s *Service) accountLabel(ctx context.Context, id string) (string, error) {
	a, err := s.d.Accounts.FindByID(ctx, id)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return id, nil
	case err != nil:
		return "", err
	}
	return actorLabel(a), nil
}

// actorLabel returns the display name of an account for the audit log, as accountLabel does.
func actorLabel(a appaccount.Actor) string {
	switch {
	case a.Local != nil:
		return "@" + a.Local.Username
	case a.Handle != nil:
		return "@" + a.Handle.Username + "@" + a.Handle.Domain
	}
	return a.Account.URI
}
