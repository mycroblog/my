// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// PolicyRepository is the persistence port of the notification policy and the requests of the
// filtered notifications (ADR-0158, data-model §4.7): the policy choices (current value), the
// filterings of the notifications (event), the accepted sources (resource), the dismissals of the
// requests (event), and the requests (projection). Repository embeds it; the database adapters
// pass notificationtest.NotificationContract.
type PolicyRepository interface {
	// SavePolicyChoices stores the account's choices, replacing an earlier choice of the same
	// condition.
	SavePolicyChoices(ctx context.Context, tx uow.Tx, choices []notification.PolicyChoice) error
	// PolicyChoices returns the choices that the account saved, in order of condition.
	PolicyChoices(ctx context.Context, accountID string) ([]notification.PolicyChoice, error)
	// PolicyChoicesOfTx returns inside tx the saved choices of each of accountIDs that has some,
	// by account, with one query.
	PolicyChoicesOfTx(ctx context.Context, tx uow.Tx, accountIDs []string) (map[string][]notification.PolicyChoice, error)
	// InsertFilterings records that the policy filtered each of ns, notifications with a source,
	// with one statement; recording one again changes nothing.
	InsertFilterings(ctx context.Context, tx uow.Tx, ns []notification.Notification) error
	// SourceAcceptedTx reports whether the recipient accepted the source's request.
	SourceAcceptedTx(ctx context.Context, tx uow.Tx, recipientAccountID, sourceAccountID string) (bool, error)
	// InsertAcceptedSource records that the recipient accepted the source; recording it again
	// changes nothing.
	InsertAcceptedSource(ctx context.Context, tx uow.Tx, a notification.AcceptedSource) error
	// InsertRequestDismissal records the dismissal of a source's request.
	InsertRequestDismissal(ctx context.Context, tx uow.Tx, d notification.RequestDismissal) error
	// RefreshRequests derives again the requests of the pairs (recipient, source): the filtered
	// notifications of each that are neither accepted nor dismissed. A pair without any has no
	// request. It is the only writer of the projection besides RebuildRequests.
	RefreshRequests(ctx context.Context, tx uow.Tx, pairs []RequestPair) error
	// RebuildRequests empties the requests and derives them all again.
	RebuildRequests(ctx context.Context, tx uow.Tx) error
	// RequestTx returns inside tx the request of the source, whatever hides it; ErrNotFound when
	// there is none.
	RequestTx(ctx context.Context, tx uow.Tx, recipientAccountID, sourceAccountID string) (notification.Request, error)
	// Requests returns the recipient's requests whose source no exclusion in force at now hides
	// (blocks, mutes with hide_notifications, domain mutes), newest notification first, starting
	// after the request whose newest notification is before ("" for the newest), at most limit,
	// each with its newest notification (Request.Last).
	Requests(ctx context.Context, recipientAccountID, before string, limit int, now time.Time) ([]notification.Request, error)
	// Request returns the request of the source as Requests lists it; ErrNotFound otherwise.
	Request(ctx context.Context, recipientAccountID, sourceAccountID string, now time.Time) (notification.Request, error)
	// RequestSummary counts the requests that Requests lists and their notifications.
	RequestSummary(ctx context.Context, recipientAccountID string, now time.Time) (notification.RequestSummary, error)
	// RequestNotifications returns the filtered notifications that wait in the source's request,
	// newest first, starting after the notification before ("" for the newest), at most limit:
	// neither accepted nor covered by a dismissal of the request, not dismissed one by one, and
	// not about a deleted status.
	RequestNotifications(ctx context.Context, recipientAccountID, sourceAccountID, before string, limit int) ([]notification.Notification, error)
}

// EventRequestsChanged is emitted in the transaction that changes an account's requests: a
// notification filtered into one, an acceptance, or a dismissal. Its commit hook signals the
// account's streams, which send the counts again (ADR-0158 §7).
const EventRequestsChanged = "notification.requests_changed"

// RequestsChanged is the event EventRequestsChanged of AccountID, the recipient.
type RequestsChanged struct{ AccountID string }

// EventType implements uow.Event.
func (RequestsChanged) EventType() string { return EventRequestsChanged }

// RequestPair names the request of a recipient and a source.
type RequestPair struct {
	RecipientAccountID string
	SourceAccountID    string
}

// AccountAges is the part of the account module's public API that ConditionNewAccount reads: when
// an account was created. *appaccount.Service implements it.
type AccountAges interface {
	CreatedAtTx(ctx context.Context, tx uow.Tx, accountID string) (time.Time, error)
}

// PrivateMentions is the part of the content module's public API that
// ConditionUnsolicitedPrivateMention reads: whether a status is direct, and whether its
// conversation holds a direct status of the recipient that mentions the source.
// *appcontent.Service implements it.
type PrivateMentions interface {
	PrivateMentionTx(ctx context.Context, tx uow.Tx, statusID, recipientID, sourceID string) (direct, solicited bool, err error)
}

// policies returns the policy of each of recipients, by recipient, with one query.
func (p *projector) policies(ctx context.Context, tx uow.Tx, recipients []string) (map[string]notification.Policy, error) {
	saved, err := p.d.Repository.PolicyChoicesOfTx(ctx, tx, recipients)
	if err != nil {
		return nil, fmt.Errorf("notification: the policies of %d recipients: %w", len(recipients), err)
	}
	out := make(map[string]notification.Policy, len(recipients))
	for _, id := range recipients {
		out[id] = notification.NewPolicy(saved[id])
	}
	return out, nil
}

// classify returns what the recipient's policy pol does with n (ADR-0158 §4): the facts of the
// conditions are read only when the policy needs them, so a policy that accepts everything, the
// default, reads nothing. following, when not nil, is whether the recipient follows the source,
// already read.
func (p *projector) classify(ctx context.Context, tx uow.Tx, n notification.Notification, pol notification.Policy, following *bool) (notification.Action, error) {
	if n.SourceAccountID == "" || !notification.PolicyApplies(n.Type) || !pol.Strict() {
		return notification.ActionAccept, nil
	}
	f, err := p.facts(ctx, tx, n, pol, following)
	if err != nil {
		return "", fmt.Errorf("notification: the policy facts of %s from %s: %w", n.RecipientAccountID, n.SourceAccountID, err)
	}
	return notification.Classify(n.Type, pol, f), nil
}

// facts reads the facts that pol needs of n's source (ADR-0158 §6), in tx, through the public
// APIs of the modules that own them.
func (p *projector) facts(ctx context.Context, tx uow.Tx, n notification.Notification, pol notification.Policy, following *bool) (notification.Facts, error) {
	var f notification.Facts
	recipient, source := n.RecipientAccountID, n.SourceAccountID
	accepted, err := p.d.Repository.SourceAcceptedTx(ctx, tx, recipient, source)
	if err != nil || accepted {
		return notification.Facts{Accepted: accepted}, err
	}
	strangers := []notification.Condition{
		notification.ConditionNotFollowing, notification.ConditionNotFollower, notification.ConditionNewAccount,
		notification.ConditionUnsolicitedPrivateMention,
	}
	if slices.ContainsFunc(strangers, pol.Needs) {
		if following != nil {
			f.Following = *following
		} else if f.Following, err = p.d.Relationships.Follows(ctx, tx, recipient, source); err != nil {
			return f, err
		}
	}
	if !f.Following {
		if pol.Needs(notification.ConditionNotFollower) {
			if f.Follower, err = p.d.Relationships.Follows(ctx, tx, source, recipient); err != nil {
				return f, err
			}
		}
		if pol.Needs(notification.ConditionNewAccount) && p.d.Ages != nil {
			created, err := p.d.Ages.CreatedAtTx(ctx, tx, source)
			if err != nil {
				return f, err
			}
			f.NewAccount = notification.NewAccountAt(created, n.CreatedAt)
		}
		if pol.Needs(notification.ConditionUnsolicitedPrivateMention) && p.d.Mentions != nil && n.Type == notification.TypeMention && n.StatusID != "" {
			direct, solicited, err := p.d.Mentions.PrivateMentionTx(ctx, tx, n.StatusID, recipient, source)
			if err != nil {
				return f, err
			}
			f.UnsolicitedPrivateMention = direct && !solicited
		}
	}
	if pol.Needs(notification.ConditionLimitedAccount) && p.d.Reach != nil {
		if f.LimitedAccount, err = p.d.Reach.UnsolicitedReachRestrictedTx(ctx, tx, source); err != nil {
			return f, err
		}
	}
	return f, nil
}

// filter records that the policy filtered ns, notifications just created, and derives the
// requests of their sources again. They are delivered nowhere: no Web Push, email, or sound, and
// the streams send nothing of them, since the list leaves them out (ADR-0158 §7).
func (p *projector) filter(ctx context.Context, tx uow.Tx, ns []notification.Notification) error {
	if len(ns) == 0 {
		return nil
	}
	if err := p.d.Repository.InsertFilterings(ctx, tx, ns); err != nil {
		return err
	}
	var pairs []RequestPair
	var recipients []string
	for _, n := range ns {
		pair := RequestPair{RecipientAccountID: n.RecipientAccountID, SourceAccountID: n.SourceAccountID}
		if !slices.Contains(pairs, pair) {
			pairs = append(pairs, pair)
		}
		if !slices.Contains(recipients, n.RecipientAccountID) {
			recipients = append(recipients, n.RecipientAccountID)
		}
	}
	if err := p.d.Repository.RefreshRequests(ctx, tx, pairs); err != nil {
		return err
	}
	for _, id := range recipients {
		if err := tx.Emit(ctx, RequestsChanged{AccountID: id}); err != nil {
			return err
		}
	}
	return nil
}

// classifyEach returns the action of each of ns, reading the policies of their recipients
// together; follows, when not nil, tells whether the recipient of ns[i] follows its source.
func (p *projector) classifyEach(ctx context.Context, tx uow.Tx, ns []notification.Notification, follows func(i int) *bool) ([]notification.Action, error) {
	out := make([]notification.Action, len(ns))
	var recipients []string
	for i, n := range ns {
		out[i] = notification.ActionAccept
		if n.SourceAccountID != "" && notification.PolicyApplies(n.Type) && !slices.Contains(recipients, n.RecipientAccountID) {
			recipients = append(recipients, n.RecipientAccountID)
		}
	}
	if len(recipients) == 0 {
		return out, nil
	}
	pols, err := p.policies(ctx, tx, recipients)
	if err != nil {
		return nil, err
	}
	for i, n := range ns {
		pol, ok := pols[n.RecipientAccountID]
		if !ok || !pol.Strict() {
			continue
		}
		var following *bool
		if follows != nil {
			following = follows(i)
		}
		if out[i], err = p.classify(ctx, tx, n, pol, following); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Page sizes of the requests and of their notifications.
const (
	DefaultRequestPageLimit = 40
	MaxRequestPageLimit     = 80
)

func requestLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultRequestPageLimit
	case limit > MaxRequestPageLimit:
		return MaxRequestPageLimit
	}
	return limit
}

// Policy returns accountID's notification policy (ADR-0158 §2); a condition the account has not
// chosen accepts.
func (s *Service) Policy(ctx context.Context, accountID string) (notification.Policy, error) {
	saved, err := s.d.Repository.PolicyChoices(ctx, accountID)
	if err != nil {
		return notification.Policy{}, fmt.Errorf("notification: the policy of %s: %w", accountID, err)
	}
	return notification.NewPolicy(saved), nil
}

// UpdatePolicy stores accountID's choices of actions, leaving the conditions it does not name as
// they are, and returns the policy. An unknown condition or action fails with
// notification.ErrInvalidPolicy and changes nothing. The notifications already created keep the
// list or the request they went to (ADR-0158 §4).
func (s *Service) UpdatePolicy(ctx context.Context, accountID string, choices []notification.PolicyChoice) (notification.Policy, error) {
	now := s.d.Clock.Now()
	out := make([]notification.PolicyChoice, len(choices))
	for i, c := range choices {
		c.AccountID, c.UpdatedAt = accountID, now
		if err := c.Validate(); err != nil {
			return notification.Policy{}, fmt.Errorf("policy choice %q=%q: %w", c.Condition, c.Action, err)
		}
		out[i] = c
	}
	if len(out) > 0 {
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.SavePolicyChoices(ctx, tx, out) })
		if err != nil {
			return notification.Policy{}, err
		}
	}
	return s.Policy(ctx, accountID)
}

// Requests returns a page of accountID's requests, newest notification first (ADR-0158 §7):
// the sources whose filtered notifications wait, with their count and newest notification. The
// sources that accountID blocks, is blocked by, or mutes with hide_notifications, or whose domain
// it mutes, are left out. before is the newest notification of the last request of the previous
// page.
func (s *Service) Requests(ctx context.Context, accountID, before string, limit int) ([]notification.Request, error) {
	return s.d.Repository.Requests(ctx, accountID, before, requestLimit(limit), s.d.Clock.Now())
}

// Request returns accountID's request of sourceID as Requests lists it; ErrNotFound otherwise.
func (s *Service) Request(ctx context.Context, accountID, sourceID string) (notification.Request, error) {
	return s.d.Repository.Request(ctx, accountID, sourceID, s.d.Clock.Now())
}

// RequestSummary counts accountID's requests that Requests lists, and their notifications.
func (s *Service) RequestSummary(ctx context.Context, accountID string) (notification.RequestSummary, error) {
	return s.d.Repository.RequestSummary(ctx, accountID, s.d.Clock.Now())
}

// RequestNotifications returns a page of the notifications that wait in accountID's request of
// sourceID, newest first; ErrNotFound when Request finds no request.
func (s *Service) RequestNotifications(ctx context.Context, accountID, sourceID, before string, limit int) ([]notification.Notification, error) {
	if _, err := s.Request(ctx, accountID, sourceID); err != nil {
		return nil, err
	}
	return s.d.Repository.RequestNotifications(ctx, accountID, sourceID, before, requestLimit(limit))
}

// AcceptRequest accepts accountID's request of sourceID (ADR-0158 §7): its filtered
// notifications join the list, unread when newer than the read marker, and the policy accepts
// the source's later notifications. Nothing is delivered for the notifications it lists. A
// source without a request fails with ErrNotFound.
func (s *Service) AcceptRequest(ctx context.Context, accountID, sourceID string) error {
	at := s.d.Clock.Now()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.d.Repository.RequestTx(ctx, tx, accountID, sourceID); err != nil {
			return err
		}
		err := s.d.Repository.InsertAcceptedSource(ctx, tx, notification.AcceptedSource{
			RecipientAccountID: accountID, SourceAccountID: sourceID, CreatedAt: at,
		})
		if err != nil {
			return err
		}
		if err := s.d.Repository.RefreshRequests(ctx, tx, []RequestPair{{RecipientAccountID: accountID, SourceAccountID: sourceID}}); err != nil {
			return err
		}
		return tx.Emit(ctx, RequestsChanged{AccountID: accountID})
	})
}

// DismissRequest dismisses accountID's request of sourceID (ADR-0158 §7): the notifications that
// wait in it leave it and are never listed, and the source is not accepted, so a later filtered
// notification opens the request again. A source without a request fails with ErrNotFound.
func (s *Service) DismissRequest(ctx context.Context, accountID, sourceID string) error {
	if s.d.IDs == nil {
		return errors.New("notification: dismissing a request needs an ID generator")
	}
	at := s.d.Clock.Now()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		r, err := s.d.Repository.RequestTx(ctx, tx, accountID, sourceID)
		if err != nil {
			return err
		}
		err = s.d.Repository.InsertRequestDismissal(ctx, tx, notification.RequestDismissal{
			ID: s.d.IDs.NewID(), RecipientAccountID: accountID, SourceAccountID: sourceID, LastNotificationID: r.LastNotificationID, DismissedAt: at,
		})
		if err != nil {
			return err
		}
		if err := s.d.Repository.RefreshRequests(ctx, tx, []RequestPair{{RecipientAccountID: accountID, SourceAccountID: sourceID}}); err != nil {
			return err
		}
		return tx.Emit(ctx, RequestsChanged{AccountID: accountID})
	})
}

// RebuildRequests empties notification_requests and derives it again, in one transaction, from
// the filterings, the accepted sources, and the dismissals of the requests (data-model §9).
func RebuildRequests(ctx context.Context, u uow.UnitOfWork, repo Repository) error {
	if err := u.Tx(ctx, func(tx uow.Tx) error { return repo.RebuildRequests(ctx, tx) }); err != nil {
		return fmt.Errorf("rebuild notification_requests: %w", err)
	}
	return nil
}
