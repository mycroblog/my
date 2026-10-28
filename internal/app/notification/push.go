// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/i18n"
)

// Web Push (FR-206): a local account's browsers subscribe through the client API, one
// subscription per OAuth grant, choosing the notification types to receive. When a projector
// creates a notification, the same transaction enqueues JobPush for each subscription of the
// recipient that takes its type (Transactional Outbox, ADR-0004), so a message goes out only
// once the notification is committed. The job encrypts the message for the browser and posts
// it to the push service through the PushSender (RFC 8030, RFC 8291, RFC 8292), whose adapter
// uses the SSRF-guarded kernel.HTTPClient (ADR-0019 §1).

// PushRepository is the persistence of the Web Push subscriptions (push_subscriptions and
// push_subscription_alerts, data-model §4.7). The database adapters' notification repositories
// implement it and pass testkit.PushSubscriptionContract.
type PushRepository interface {
	// SavePushSubscription stores s with its alerts, replacing the subscription of the same
	// token family and the one with the same endpoint, if any, with their alerts.
	SavePushSubscription(ctx context.Context, tx uow.Tx, s notification.PushSubscription) error
	// DeletePushSubscription deletes a subscription with its alerts and reports whether there
	// was one.
	DeletePushSubscription(ctx context.Context, tx uow.Tx, id string) (bool, error)
	// PushSubscriptionIDs returns inside tx the subscriptions of the account that take
	// notifications of type t, in order.
	PushSubscriptionIDs(ctx context.Context, tx uow.Tx, accountID string, t notification.Type) ([]string, error)
	// FindPushSubscription returns a subscription by ID, and FindPushSubscriptionOfFamily the
	// subscription of a token family. They fail with an error wrapping ErrNotFound when there is
	// none.
	FindPushSubscription(ctx context.Context, id string) (notification.PushSubscription, error)
	FindPushSubscriptionOfFamily(ctx context.Context, tokenFamilyID string) (notification.PushSubscription, error)
}

// PushMessage is one message to a push service.
type PushMessage struct {
	// Payload is the plaintext that the sender encrypts for the subscription (RFC 8291).
	Payload []byte
	// TTL is how long the push service keeps the message for an offline browser (RFC 8030 §5.2).
	TTL time.Duration
	// Topic replaces an undelivered message of the same topic (RFC 8030 §5.4): at most 32
	// characters of the base64url alphabet.
	Topic string
}

// PushOutcome is how the push service answered a message.
type PushOutcome int

// Outcomes.
const (
	// PushDelivered: the push service accepted the message (201).
	PushDelivered PushOutcome = iota + 1
	// PushGone: the subscription no longer exists (404, 410), or its endpoint is a destination
	// the server refuses (ADR-0019 §1); the subscription is deleted.
	PushGone
	// PushRetry: a temporary failure (429, 5xx, a network error); the message is sent again
	// after RetryAfter, or the job's backoff when it is zero.
	PushRetry
	// PushRejected: the push service refused the message for good (another 4xx).
	PushRejected
)

// PushResult is the answer of PushSender.Send.
type PushResult struct {
	Outcome PushOutcome
	// StatusCode is the push service's status code; 0 when there was no response.
	StatusCode int
	// RetryAfter is the push service's Retry-After for a PushRetry; zero when it gave none.
	RetryAfter time.Duration
}

// PushSender encrypts and sends Web Push messages. The adapter (internal/adapter/webpush)
// holds the server's VAPID key pair, sends through kernel.HTTPClient, and passes
// testkit.PushSenderContract.
type PushSender interface {
	// ServerKey returns the server's VAPID public key, an uncompressed P-256 point in unpadded
	// base64url: the applicationServerKey that browsers subscribe with (RFC 8292 §3.2).
	ServerKey() string
	// Send encrypts m.Payload for s (RFC 8291) and posts it to s.Endpoint with a VAPID
	// authorization (RFC 8292). A transport failure is a PushRetry result, not an error; the
	// error reports a message that cannot be built: invalid keys (ErrInvalidPushKey) or a
	// payload beyond the 3993 bytes of one record (RFC 8291 §4).
	Send(ctx context.Context, s notification.PushSubscription, m PushMessage) (PushResult, error)
}

// PushAccount is how a push message names an account.
type PushAccount struct {
	// DisplayName is the account's display name; "" when it has none.
	DisplayName string
	// Acct is the account's handle without the leading @: username for a local account and
	// username@domain for a remote one, as the web client's profile paths use it.
	Acct string
}

// PushAudience is what the push job reads of the identity and account modules. The
// composition root implements it.
type PushAudience interface {
	// GrantActive reports whether an OAuth grant (its token family) can still be used: a token of
	// it is neither revoked nor expired (counting its refresh token), so that signing out of an
	// app, or a revocation by a moderator or a password change, ends its subscription.
	GrantActive(ctx context.Context, tokenFamilyID string) (bool, error)
	// Locale returns the UI locale of the user of a local account; "" when the user has none.
	Locale(ctx context.Context, accountID string) (string, error)
	// Account returns how a message names an account; ErrNotFound when the account is gone.
	Account(ctx context.Context, accountID string) (PushAccount, error)
	// StatusVisible reports whether the local account may see the status now, by the content
	// module's single visibility policy; false for a missing or deleted status.
	StatusVisible(ctx context.Context, accountID, statusID string) (bool, error)
}

// PushNotifications tells whether a notification is still in its recipient's list.
// *Service implements it.
type PushNotifications interface {
	Listed(ctx context.Context, accountID, id string) (bool, error)
}

// JobPush is the job that sends one notification to one Web Push subscription.
const JobPush = "notification.push"

// pushJob is the payload of JobPush: the subscription, and the notification as far as the
// message needs it, so that the job reads no notification row. Without the subscription, it is
// the payload of JobNotificationEmail.
type pushJob struct {
	SubscriptionID  string            `json:"subscription_id"`
	NotificationID  string            `json:"notification_id"`
	AccountID       string            `json:"account_id"`
	Type            notification.Type `json:"type"`
	SourceAccountID string            `json:"source_account_id,omitempty"`
	StatusID        string            `json:"status_id,omitempty"`
	ReportID        string            `json:"report_id,omitempty"`
	AppealID        string            `json:"appeal_id,omitempty"`
	// ServiceClosureChange is what a service_closure tells of.
	ServiceClosureChange notification.ClosureChange `json:"service_closure_change,omitempty"`
	// VoiceRoomID is the room of a voice_room_started or a voice_room_invited.
	VoiceRoomID string `json:"voice_room_id,omitempty"`
	// ErrorGroupChange and ErrorGroupID are what an admin_error_group tells of and its group.
	ErrorGroupChange notification.ErrorGroupChange `json:"error_group_change,omitempty"`
	ErrorGroupID     string                        `json:"error_group_id,omitempty"`
}

// jobOf returns the payload of a job about n: JobPush to the subscription subscriptionID, or
// JobNotificationEmail without one.
func jobOf(n notification.Notification, subscriptionID string) pushJob {
	return pushJob{
		SubscriptionID: subscriptionID, NotificationID: n.ID, AccountID: n.RecipientAccountID, Type: n.Type,
		SourceAccountID: n.SourceAccountID, StatusID: n.StatusID, ReportID: n.ReportID, AppealID: n.AppealID,
		ServiceClosureChange: n.ServiceClosureChange, VoiceRoomID: n.VoiceRoomID,
		ErrorGroupChange: n.ErrorGroupChange, ErrorGroupID: n.ErrorGroupID,
	}
}

// PushTTL is how long a push service keeps a message for a browser that is offline.
const PushTTL = 48 * time.Hour

// Pusher enqueues the Web Push jobs of new notifications inside the transaction that creates
// them. The projectors call it through ProjectorDeps.Push and ModeratorDeps.Push.
type Pusher struct {
	Repository PushRepository
	Outbox     queue.Outbox
}

// enqueue enqueues JobPush for each subscription of n's recipient that takes n's type. A nil
// Pusher pushes nothing. The idempotency key is per notification and subscription, so that a
// retried transaction or a reprojected event enqueues each job once.
func (p *Pusher) enqueue(ctx context.Context, tx uow.Tx, n notification.Notification) error {
	if p == nil || p.Repository == nil || p.Outbox == nil {
		return nil
	}
	ids, err := p.Repository.PushSubscriptionIDs(ctx, tx, n.RecipientAccountID, n.Type)
	if err != nil || len(ids) == 0 {
		return err
	}
	jobs := make([]queue.NewJob, 0, len(ids))
	for _, id := range ids {
		payload, err := json.Marshal(jobOf(n, id))
		if err != nil {
			return err
		}
		jobs = append(jobs, queue.NewJob{Type: JobPush, Payload: payload, IdempotencyKey: JobPush + ":" + n.ID + ":" + id})
	}
	return p.Outbox.EnqueueTx(ctx, tx, jobs...)
}

// PushDeps are the ports of the Web Push use cases and job.
type PushDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository PushRepository
	Clock      kernel.Clock
	IDs        kernel.IDGenerator
	// Sender sends the messages; nil turns Web Push off: subscribing fails with
	// ErrPushUnavailable and the server key is "".
	Sender   PushSender
	Audience PushAudience
	// Notifications rechecks, when a message is sent, that its notification is still listed:
	// a block, a mute, a domain mute, a dismissal, or a deleted status since the notification
	// was created drops the message. nil skips the check.
	Notifications PushNotifications
	// Messages words the messages; nil means the embedded catalogs (i18n.Load).
	Messages *i18n.Bundle
	// Host names the instance in a message whose source is the instance, when InstanceName
	// gives no name.
	Host string
	// InstanceName returns the instance's display name; nil or "" names
	// it by Host.
	InstanceName func(ctx context.Context) string
	// Stopped reports whether the closure of the service stops Web Push (ADR-0088 §2), read
	// per message; nil never stops it.
	Stopped func(ctx context.Context) (bool, error)
}

// PushService runs the Web Push use cases of the client API (FR-206): reading, registering, and
// deleting the subscription of the caller's grant.
type PushService struct {
	d PushDeps
}

// NewPushService returns a PushService on d.
func NewPushService(d PushDeps) *PushService { return &PushService{d: d} }

// ErrPushUnavailable reports that the instance has no VAPID key, so it sends no Web Push.
var ErrPushUnavailable = errors.New("notification: Web Push is not configured")

// ServerKey returns the server's VAPID public key, "" when Web Push is off.
func (s *PushService) ServerKey() string {
	if s == nil || s.d.Sender == nil {
		return ""
	}
	return s.d.Sender.ServerKey()
}

// Subscription returns the subscription of a token family, or an error wrapping ErrNotFound.
func (s *PushService) Subscription(ctx context.Context, tokenFamilyID string) (notification.PushSubscription, error) {
	return s.d.Repository.FindPushSubscriptionOfFamily(ctx, tokenFamilyID)
}

// SubscribeInput is a registration of a subscription.
type SubscribeInput struct {
	AccountID     string
	TokenFamilyID string
	Endpoint      string
	KeyP256DH     string
	KeyAuth       string
	Alerts        []notification.Type
}

// Subscribe registers the subscription of the grant tokenFamilyID, replacing the grant's
// earlier subscription and any other subscription with the same endpoint. It fails with
// ErrPushUnavailable when Web Push is off, notification.ErrInvalidPushEndpoint,
// notification.ErrInvalidPushKey, or notification.ErrInvalidType.
func (s *PushService) Subscribe(ctx context.Context, in SubscribeInput) (notification.PushSubscription, error) {
	if s.d.Sender == nil {
		return notification.PushSubscription{}, ErrPushUnavailable
	}
	alerts, ok := notification.NormalizeAlerts(in.Alerts)
	if !ok {
		return notification.PushSubscription{}, notification.ErrInvalidType
	}
	p256dh, auth := notification.NormalizePushKeys(in.KeyP256DH, in.KeyAuth)
	now := s.d.Clock.Now()
	sub := notification.PushSubscription{
		ID: s.d.IDs.NewID(), AccountID: in.AccountID, TokenFamilyID: in.TokenFamilyID, Endpoint: in.Endpoint,
		KeyP256DH: p256dh, KeyAuth: auth, Alerts: alerts, CreatedAt: now, UpdatedAt: now,
	}
	if err := sub.Validate(); err != nil {
		return notification.PushSubscription{}, err
	}
	if prev, err := s.d.Repository.FindPushSubscriptionOfFamily(ctx, in.TokenFamilyID); err == nil && prev.Endpoint == sub.Endpoint {
		sub.ID, sub.CreatedAt = prev.ID, prev.CreatedAt // the same browser changes its alerts
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return notification.PushSubscription{}, err
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.SavePushSubscription(ctx, tx, sub)
	})
	if err != nil {
		return notification.PushSubscription{}, err
	}
	return sub, nil
}

// Unsubscribe deletes the subscription of a token family, if any.
func (s *PushService) Unsubscribe(ctx context.Context, tokenFamilyID string) error {
	sub, err := s.d.Repository.FindPushSubscriptionOfFamily(ctx, tokenFamilyID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.delete(ctx, sub.ID)
}

func (s *PushService) delete(ctx context.Context, id string) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := s.d.Repository.DeletePushSubscription(ctx, tx, id)
		return err
	})
}

// RegisterPushJobs registers the handler of JobPush on reg. The handler is idempotent: it
// sends nothing when the subscription is gone, no longer takes the type, or its grant has
// ended (deleting it then); a message sent again after a crash carries the same Topic, which
// replaces it at the push service while undelivered, and the same notification ID, which the
// Service Worker uses as the tag that replaces it on screen. A subscription that the push
// service reports gone (404, 410) is deleted.
func (s *PushService) RegisterPushJobs(reg *queue.JobHandlerRegistry) {
	reg.Register(JobPush, func(ctx context.Context, j queue.Job) queue.Result {
		var job pushJob
		if err := json.Unmarshal(j.Payload, &job); err != nil {
			return queue.Fail(fmt.Errorf("notification: the payload of %s: %w", j.ID, err))
		}
		return s.push(ctx, job)
	})
}

func (s *PushService) push(ctx context.Context, job pushJob) queue.Result {
	if s.d.Sender == nil {
		return queue.Done()
	}
	if s.d.Stopped != nil {
		stopped, err := s.d.Stopped(ctx)
		if err != nil {
			return queue.Retry(0, err)
		}
		if stopped {
			return queue.Done() // the server is closed: the message is dropped
		}
	}
	sub, err := s.d.Repository.FindPushSubscription(ctx, job.SubscriptionID)
	if errors.Is(err, ErrNotFound) {
		return queue.Done()
	}
	if err != nil {
		return queue.Retry(0, err)
	}
	if sub.AccountID != job.AccountID || !sub.Alerted(job.Type) {
		return queue.Done()
	}
	active, err := s.d.Audience.GrantActive(ctx, sub.TokenFamilyID)
	if err != nil {
		return queue.Retry(0, err)
	}
	if !active {
		return s.retire(ctx, sub.ID)
	}
	switch shown, err := stillShown(ctx, s.d.Notifications, s.d.Audience, job); {
	case err != nil:
		return queue.Retry(0, err)
	case !shown:
		return queue.Done() // the recipient no longer sees the notification or its status
	}
	payload, err := s.message(ctx, job)
	if errors.Is(err, ErrNotFound) {
		return queue.Done() // the account the message names is gone
	}
	if err != nil {
		return queue.Retry(0, err)
	}
	res, err := s.d.Sender.Send(ctx, sub, PushMessage{Payload: payload, TTL: PushTTL, Topic: pushTopic(job.NotificationID)})
	if err != nil {
		return queue.Fail(fmt.Errorf("notification: push %s to %s: %w", job.NotificationID, sub.ID, err))
	}
	switch res.Outcome {
	case PushDelivered:
		return queue.Done()
	case PushGone:
		return s.retire(ctx, sub.ID)
	case PushRejected:
		return queue.Fail(fmt.Errorf("notification: the push service refused %s with %d", job.NotificationID, res.StatusCode))
	}
	return queue.Retry(res.RetryAfter, fmt.Errorf("notification: the push service answered %s with %d", job.NotificationID, res.StatusCode))
}

// stillShown reports whether job's notification may still reach its recipient at the time of
// sending, which can be long after the notification was created: the notification list still
// shows it (no block, mute, or dismissal since), and the recipient may still see its status.
// A nil notifications skips the check of the list. The email of a notification checks the same.
func stillShown(ctx context.Context, notifications PushNotifications, audience PushAudience, job pushJob) (bool, error) {
	if notifications != nil {
		listed, err := notifications.Listed(ctx, job.AccountID, job.NotificationID)
		if err != nil || !listed {
			return false, err
		}
	}
	if job.StatusID == "" {
		return true, nil
	}
	return audience.StatusVisible(ctx, job.AccountID, job.StatusID)
}

// retire deletes a subscription that can take no more messages.
func (s *PushService) retire(ctx context.Context, id string) queue.Result {
	if err := s.delete(ctx, id); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// pushTopic is the Topic of a notification's messages: its ID without the hyphens, 32
// hexadecimal digits, which the base64url alphabet includes.
func pushTopic(notificationID string) string {
	t := strings.ReplaceAll(notificationID, "-", "")
	return t[:min(len(t), 32)]
}

// PushPayload is the JSON document that a push message carries to the Service Worker
// (app/web/sw/push.ts), which shows it as a notification.
type PushPayload struct {
	// NotificationID is the notification's ID; the Service Worker shows the message with it
	// as the tag, so that a message sent twice replaces itself.
	NotificationID string `json:"notification_id"`
	// Type is the notification's type.
	Type notification.Type `json:"type"`
	// Title is the notification's text, localized for the recipient.
	Title string `json:"title"`
	// Lang is the locale of Title.
	Lang string `json:"lang"`
	// Path is the page of the web client that tapping the notification opens.
	Path string `json:"path"`
}

// message returns the payload of job's message, worded in the recipient's locale.
func (s *PushService) message(ctx context.Context, job pushJob) ([]byte, error) {
	p, _, err := describer{
		audience: s.d.Audience, messages: s.d.Messages, host: s.d.Host, instanceName: s.d.InstanceName,
	}.describe(ctx, job)
	if err != nil {
		return nil, err
	}
	return json.Marshal(p)
}

// describer words a notification for its recipient: the title and the page of a Web Push
// message, which are also the subject and the link of the notification's email.
type describer struct {
	audience     PushAudience
	messages     *i18n.Bundle
	host         string
	instanceName func(ctx context.Context) string
}

// describe returns how job's notification reads in the recipient's locale, with the localizer
// of that locale. It fails with ErrNotFound when an account it names is gone.
func (d describer) describe(ctx context.Context, job pushJob) (PushPayload, *i18n.Localizer, error) {
	locale, err := d.audience.Locale(ctx, job.AccountID)
	if err != nil {
		return PushPayload{}, nil, err
	}
	bundle := d.messages
	if bundle == nil {
		if bundle, err = i18n.Load(); err != nil {
			return PushPayload{}, nil, err
		}
	}
	users := &i18n.Resolver{Bundle: bundle, UserLocale: func(context.Context) string { return locale }}
	l := users.Localizer(ctx, "")
	var source, recipient PushAccount
	if job.SourceAccountID != "" {
		if source, err = d.audience.Account(ctx, job.SourceAccountID); err != nil {
			return PushPayload{}, nil, err
		}
	}
	if job.Type == notification.TypeBoost || job.Type == notification.TypeFavourite || job.Type == notification.TypeReaction ||
		(job.Type == notification.TypePollClosed && job.SourceAccountID == "") {
		if recipient, err = d.audience.Account(ctx, job.AccountID); err != nil {
			return PushPayload{}, nil, err
		}
	}
	name := source.DisplayName
	if name == "" && source.Acct != "" {
		name = "@" + source.Acct
	}
	instance := d.host
	if d.instanceName != nil {
		if n := d.instanceName(ctx); n != "" {
			instance = n
		}
	}
	p := PushPayload{NotificationID: job.NotificationID, Type: job.Type, Lang: l.RequestedLocale(), Path: "/notifications"}
	switch job.Type {
	case notification.TypeFollow:
		p.Title, p.Path = l.ServerPushFollow(name), "/@"+source.Acct
	case notification.TypeFollowRequest:
		p.Title, p.Path = l.ServerPushFollowRequest(name), "/settings/follow-requests"
	case notification.TypeFollowAccepted:
		p.Title, p.Path = l.ServerPushFollowAccepted(name), "/@"+source.Acct
	case notification.TypeMention:
		p.Title, p.Path = l.ServerPushMention(name), statusPath(source.Acct, job.StatusID)
	case notification.TypeBoost:
		p.Title, p.Path = l.ServerPushBoost(name), statusPath(recipient.Acct, job.StatusID)
	case notification.TypeFavourite:
		p.Title, p.Path = l.ServerPushFavourite(name), statusPath(recipient.Acct, job.StatusID)
	case notification.TypeReaction:
		p.Title, p.Path = l.ServerPushReaction(name), statusPath(recipient.Acct, job.StatusID)
	case notification.TypeQuote:
		p.Title, p.Path = l.ServerPushQuote(name), statusPath(source.Acct, job.StatusID)
	case notification.TypePollClosed:
		// The voters' notifications are from the poll's author; the author's own has no source.
		author := source.Acct
		if job.SourceAccountID == "" {
			author = recipient.Acct
		}
		p.Title, p.Path = l.ServerPushPollClosed(), statusPath(author, job.StatusID)
	case notification.TypeModerationNotice:
		p.Title, p.Path = l.ServerPushModerationNotice(instance), "/settings/account-status"
	case notification.TypeAppealDecided:
		p.Title, p.Path = l.ServerPushAppealDecided(instance), "/settings/account-status"
	case notification.TypeAdminReport:
		p.Title, p.Path = l.ServerPushAdminReport(instance), "/moderation/reports/"+job.ReportID
	case notification.TypeAdminSignUp:
		p.Title, p.Path = l.ServerPushAdminSignUp(instance), "/moderation/registrations"
	case notification.TypeAdminAppeal:
		p.Title, p.Path = l.ServerPushAdminAppeal(instance), "/moderation/appeals/"+job.AppealID
	case notification.TypeAdminWebhookDisabled:
		p.Title, p.Path = l.ServerPushAdminWebhookDisabled(instance), "/admin/webhooks"
	case notification.TypeAdminCustomEmojiRequest:
		p.Title, p.Path = l.ServerPushAdminCustomEmojiRequest(instance), "/admin/custom-emojis?tab=requests"
	case notification.TypeCustomEmojiRequestDecided:
		p.Title, p.Path = l.ServerPushCustomEmojiRequestDecided(instance), "/settings/custom-emojis"
	case notification.TypeSupporterPaymentFailed:
		// The supporter notifications open the notifications, never the subscription screen,
		// which the Tauri app does not show (ADR-0072 §14).
		p.Title = l.ServerPushSupporterPaymentFailed(instance)
	case notification.TypeSupporterSubscriptionEnded:
		p.Title = l.ServerPushSupporterSubscriptionEnded(instance)
	case notification.TypeSupporterPlanRetired:
		p.Title = l.ServerPushSupporterPlanRetired(instance)
	case notification.TypeReportResolved:
		p.Title = l.ServerPushReportResolved(instance)
	case notification.TypeRelationshipsSevered:
		p.Title = l.ServerPushRelationshipsSevered(instance)
	case notification.TypeAccountMoved:
		p.Title, p.Path = l.ServerPushAccountMoved(name), "/@"+source.Acct
	case notification.TypeAdminAccountMoved:
		p.Title = l.ServerPushAdminAccountMoved(instance)
	case notification.TypeServiceClosure:
		switch job.ServiceClosureChange {
		case notification.ClosureWithdrawn:
			p.Title = l.ServerPushServiceClosureWithdrawn(instance)
		case notification.ClosureRescheduled:
			p.Title = l.ServerPushServiceClosureRescheduled(instance)
		default:
			p.Title = l.ServerPushServiceClosureAnnounced(instance)
		}
	case notification.TypeAdminServiceClosureDue:
		p.Title = l.ServerPushAdminServiceClosureDue(instance)
	case notification.TypeAdminErrorGroup:
		p.Path = "/admin/errors"
		switch job.ErrorGroupChange {
		case notification.ErrorGroupCreated:
			p.Title, p.Path = l.ServerPushAdminErrorGroupCreated(instance), "/admin/errors/"+job.ErrorGroupID
		case notification.ErrorGroupRegressed:
			p.Title, p.Path = l.ServerPushAdminErrorGroupRegressed(instance), "/admin/errors/"+job.ErrorGroupID
		default:
			p.Title = l.ServerPushAdminErrorGroupOthers(instance)
		}
	case notification.TypeSupporterServiceClosure:
		p.Title = l.ServerPushSupporterServiceClosure(instance)
	case notification.TypeVoiceRoomStarted:
		p.Title, p.Path = l.ServerPushVoiceRoomStarted(name), "/voice/"+job.VoiceRoomID
	case notification.TypeVoiceRoomInvited:
		p.Title, p.Path = l.ServerPushVoiceRoomInvited(name), "/voice/"+job.VoiceRoomID
	default:
		p.Title = l.ServerPushOther(instance)
	}
	return p, l, nil
}

// statusPath is the web client's page of a status by the account acct (ADR-0053).
func statusPath(acct, statusID string) string { return "/@" + acct + "/status/" + statusID }
