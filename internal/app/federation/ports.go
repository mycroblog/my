// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// ErrNotFound reports that no row matches a lookup.
var ErrNotFound = errors.New("federation: not found")

// Repository is the persistence port of the federation module's outbound side: outbound
// activities, their documents, their deliveries, and the health of remote servers
// (data-model §4.8). The database adapters implement it and pass
// testkit.FederationRepositoryContract.
//
// Methods that take a uow.Tx run inside that transaction; the others read committed data outside
// any transaction. Lookups that find nothing return an error wrapping ErrNotFound.
type Repository interface {
	// InsertOutboundActivity records an outbound activity.
	InsertOutboundActivity(ctx context.Context, tx uow.Tx, a federation.OutboundActivity) error
	// InsertOutboundDocument stores the body of an outbound activity unless it has one, and
	// reports whether it did: the first body stays.
	InsertOutboundDocument(ctx context.Context, tx uow.Tx, outboundActivityID string, document []byte, builtAt time.Time) (bool, error)
	// InsertDelivery adds an inbox to an outbound activity unless it has it, and reports whether
	// it did.
	InsertDelivery(ctx context.Context, tx uow.Tx, d federation.Delivery) (bool, error)
	// UpsertInstanceHealth replaces the health of a remote server; only this module's
	// projector of delivery results writes it.
	UpsertInstanceHealth(ctx context.Context, tx uow.Tx, h federation.InstanceHealth) error

	FindOutboundActivity(ctx context.Context, id string) (federation.OutboundActivity, error)
	// ListOutboundActivitiesOf returns the activities sent about an object, oldest first.
	ListOutboundActivitiesOf(ctx context.Context, kind federation.OutboundObjectKind, objectID string) ([]federation.OutboundActivity, error)
	FindOutboundDocument(ctx context.Context, outboundActivityID string) ([]byte, error)
	// ListDeliveries returns the deliveries of an outbound activity in ID order, starting after
	// the delivery ID after ("" for the first) and returning at most limit deliveries.
	ListDeliveries(ctx context.Context, outboundActivityID, after string, limit int) ([]federation.Delivery, error)
	// InstanceState returns the health state of a remote server; a server without a row is
	// available.
	InstanceState(ctx context.Context, domain string) (federation.InstanceState, error)

	// InsertDeliveryAttempt records one attempt of a delivery.
	InsertDeliveryAttempt(ctx context.Context, tx uow.Tx, a federation.DeliveryAttempt) error
	// LockInstanceHealth reads the health of a remote server in tx, locking its row where the
	// database needs it, so that concurrent updates of one server do not lose each other.
	LockInstanceHealth(ctx context.Context, tx uow.Tx, domain string) (federation.InstanceHealth, error)
	// InsertAvailabilityEvent records that a server was marked unavailable or available.
	InsertAvailabilityEvent(ctx context.Context, tx uow.Tx, e federation.AvailabilityEvent) error

	FindDelivery(ctx context.Context, id string) (federation.Delivery, error)
	// ListDeliveryAttempts returns the attempts of a delivery, oldest first.
	ListDeliveryAttempts(ctx context.Context, deliveryID string) ([]federation.DeliveryAttempt, error)
	FindInstanceHealth(ctx context.Context, domain string) (federation.InstanceHealth, error)
	// ListAvailabilityEvents returns at most limit availability events of a server, newest first,
	// starting before the event ID before ("" for the newest).
	ListAvailabilityEvents(ctx context.Context, domain, before string, limit int) ([]federation.AvailabilityEvent, error)
	// ListUnavailableInstances returns at most limit unavailable servers in domain order,
	// starting after the domain after ("" for the first).
	ListUnavailableInstances(ctx context.Context, after string, limit int) ([]federation.InstanceHealth, error)
	// ListInstanceHealth returns at most limit servers with a health row in domain order, starting
	// after the domain after ("" for the first): those in state, or every one when state is "".
	ListInstanceHealth(ctx context.Context, state federation.InstanceState, after string, limit int) ([]federation.InstanceHealth, error)

	// InsertFanOutCompletion records that the fan-out of an outbound activity enqueued its last
	// batch; a repeated one keeps the first.
	InsertFanOutCompletion(ctx context.Context, tx uow.Tx, outboundActivityID string, completedAt time.Time) error
	// InsertClosureDelete records the service closure's Delete of an account unless the account
	// has one, and reports whether it did.
	InsertClosureDelete(ctx context.Context, tx uow.Tx, d federation.ClosureDelete) (bool, error)
	// ClosureDeleteAccounts returns, in ID order, the accounts among ids whose closure Delete is
	// recorded.
	ClosureDeleteAccounts(ctx context.Context, ids []string) ([]string, error)
	// ClosureProgress counts the closure's Deletes, those whose fan-out has not ended, and their
	// deliveries by state; a delivery whose latest attempt failed and that made maxAttempts
	// attempts is dead.
	ClosureProgress(ctx context.Context, maxAttempts int) (federation.ClosureProgress, error)
	// CountPendingMoves counts the Moves recorded since since whose fan-out has not ended or
	// that have a delivery with no attempt yet or with a retryable failure and fewer than
	// maxAttempts attempts.
	CountPendingMoves(ctx context.Context, since time.Time, maxAttempts int) (int, error)
}

// Statuses is the part of the content module's public API that delivery reads: the local
// statuses and boosts that outbound activities are about, and the visibility facts of their
// authors. *appcontent.Service and *appcontent.Interactions implement it together
// (ContentStatuses).
type Statuses interface {
	FindStatus(ctx context.Context, id string) (appcontent.Status, error)
	LiveRevision(ctx context.Context, statusID, revisionID string) (appcontent.Status, appcontent.Revision, error)
	AuthorRestrictions(ctx context.Context, accountID string) (appcontent.AuthorRestrictions, error)
	FindBoost(ctx context.Context, id string) (appcontent.Boost, error)
	// PollVoterIDs returns at most limit accounts that voted in a poll, ordered by ID, after
	// after: the Update of a local poll goes to the remote ones too.
	PollVoterIDs(ctx context.Context, pollID, after string, limit int) ([]string, error)
}

// ContentStatuses joins the content module's services into Statuses.
type ContentStatuses struct {
	*appcontent.Service
	Interactions *appcontent.Interactions
}

// FindBoost implements Statuses.
func (c ContentStatuses) FindBoost(ctx context.Context, id string) (appcontent.Boost, error) {
	return c.Interactions.FindBoost(ctx, id)
}

// Accounts is the part of the account module's public API that delivery reads: the IRIs and
// documents of actors, the inboxes of remote ones, and the shared inboxes known to the
// instance. *appaccount.Service implements it.
type Accounts interface {
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
	// RemoteInbox fails with appaccount.ErrNotFound for a local account.
	RemoteInbox(ctx context.Context, accountID string) (appaccount.RemoteInbox, error)
	// KnownSharedInboxes returns at most limit distinct shared inboxes that remote actors
	// declare, in order, after the shared inbox after ("" for the first batch).
	KnownSharedInboxes(ctx context.Context, after string, limit int) ([]string, error)
}

// Graph is the part of the graph module's public API that delivery reads: an author's active
// followers in batches, and the relationship of a mentioned account with the author.
// *appgraph.Service implements it.
type Graph interface {
	ActiveFollowerIDs(ctx context.Context, accountID, before string, limit int) (ids []string, next string, err error)
	Relationship(ctx context.Context, accountID, targetID string) (appgraph.Relationship, error)
}

// DomainPolicy says whether a domain block keeps activities from a remote server: the
// federation restriction (suspend) of a domain or one of its parents (federation.md §11).
// *moderation.FederationPolicy implements it.
type DomainPolicy interface {
	FederationSuspended(ctx context.Context, domain string) (bool, error)
}

// NoDomainRestrictions is the DomainPolicy that suspends no server: the default of the use cases
// built without a policy, such as in tests.
type NoDomainRestrictions struct{}

// FederationSuspended implements DomainPolicy.
func (NoDomainRestrictions) FederationSuspended(context.Context, string) (bool, error) {
	return false, nil
}

// AccountPolicy says whether a restriction of a remote account keeps its activities: the
// federation restriction (suspend, ban) of the account (federation.md §11).
// *moderation.FederationPolicy implements it.
type AccountPolicy interface {
	AccountFederationSuspended(ctx context.Context, accountID string) (bool, error)
}

// NoAccountRestrictions is the AccountPolicy that suspends no account: the default of the use
// cases built without a policy, such as in tests.
type NoAccountRestrictions struct{}

// AccountFederationSuspended implements AccountPolicy.
func (NoAccountRestrictions) AccountFederationSuspended(context.Context, string) (bool, error) {
	return false, nil
}
