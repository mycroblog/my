// SPDX-License-Identifier: AGPL-3.0-only

package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/webhook"
)

// The messages (ADR-0060 "イベントとペイロード"): the events of the other modules become, in their
// transactions, a message to the enabled endpoints that subscribe to its type, and a
// webhook.deliver job per endpoint. Without such an endpoint nothing is written. The body is
// {"type", "timestamp", "data"}; data is the admin API's (/api/v1/admin/...) representation of
// the object of the event, presented in the event's transaction (docs/reference/webhooks.md,
// WebhookMessage in api/openapi.yaml).

// ErrNoPresenter reports a message that needs the admin API's representation of its object
// while no Presenter is set.
var ErrNoPresenter = errors.New("webhook: no presenter of the admin API representations")

// Presenter presents the object of a message as the admin API does, as a value that encodes to
// the JSON of the admin API's schema. It reads in tx, the transaction of the event, through the
// modules' reads that take a transaction, so it sees what the transaction wrote. The client
// API's server implements it with the serializers of its admin operations; each method fails
// as those operations do for an unknown ID.
type Presenter interface {
	// WebhookReport presents a report as AdminReport.
	WebhookReport(ctx context.Context, tx uow.Tx, id string) (any, error)
	// WebhookRegistration presents the registration of a user as AdminRegistration.
	WebhookRegistration(ctx context.Context, tx uow.Tx, userID string) (any, error)
	// WebhookAccount presents an account as AdminAccount.
	WebhookAccount(ctx context.Context, tx uow.Tx, id string) (any, error)
	// WebhookAccountAction presents an action on an account as AdminAccountAction.
	WebhookAccountAction(ctx context.Context, tx uow.Tx, id string) (any, error)
	// WebhookAppeal presents an appeal as AdminAppeal.
	WebhookAppeal(ctx context.Context, tx uow.Tx, id string) (any, error)
	// WebhookDomainAction presents an action on a domain as AdminDomainAction.
	WebhookDomainAction(ctx context.Context, tx uow.Tx, id string) (any, error)
	// WebhookCustomEmoji presents a custom emoji as AdminCustomEmoji.
	WebhookCustomEmoji(ctx context.Context, tx uow.Tx, id string) (any, error)
	// WebhookErrorGroup presents an error group as ErrorGroup.
	WebhookErrorGroup(ctx context.Context, tx uow.Tx, id string) (any, error)
}

// SetPresenter sets the presenter of the messages' data. The composition root calls it once,
// before the first transaction, with the client API's server, which it builds after the
// Service.
func (s *Service) SetPresenter(p Presenter) { s.presenter = p }

// envelope is the body of a message.
type envelope struct {
	Type      webhook.EventType `json:"type"`
	Timestamp string            `json:"timestamp"`
	Data      any               `json:"data"`
}

// timestamp formats a time of a body: RFC 3339 in UTC with milliseconds.
func timestamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z07:00") }

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// statusData is the data of status.created (WebhookStatus), which no admin operation presents.
type statusData struct {
	ID           string  `json:"id"`
	URI          string  `json:"uri"`
	AccountID    string  `json:"account_id"`
	Visibility   string  `json:"visibility"`
	InReplyToURI *string `json:"in_reply_to_uri,omitempty"`
	SpoilerText  string  `json:"spoiler_text"`
	Sensitive    bool    `json:"sensitive"`
	Language     string  `json:"language"`
	Content      string  `json:"content"`
	CreatedAt    string  `json:"created_at"`
}

// testData is the data of webhook.test (WebhookTest).
type testData struct {
	EndpointID string `json:"endpoint_id"`
}

// withOperation adds the field "operation" to the JSON object that v encodes to: the data of
// custom_emoji.* (WebhookCustomEmoji) is AdminCustomEmoji with the operation.
func withOperation(v any, operation string) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return nil, err
	}
	if fields["operation"], err = json.Marshal(operation); err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

// customEmojiEvents maps the operations on custom emoji to their events (ADR-0060).
var customEmojiEvents = map[string]webhook.EventType{
	appcontent.CustomEmojiCreated:       webhook.EventCustomEmojiCreated,
	appcontent.CustomEmojiCopied:        webhook.EventCustomEmojiCreated,
	appcontent.CustomEmojiImageReplaced: webhook.EventCustomEmojiUpdated,
	appcontent.CustomEmojiUpdated:       webhook.EventCustomEmojiUpdated,
	appcontent.CustomEmojiEnabled:       webhook.EventCustomEmojiUpdated,
	appcontent.CustomEmojiDisabled:      webhook.EventCustomEmojiUpdated,
	appcontent.CustomEmojiDeleted:       webhook.EventCustomEmojiDeleted,
}

// presentFunc presents the data of a message with the Presenter.
type presentFunc func(ctx context.Context, tx uow.Tx, p Presenter) (any, error)

// source is the message of a domain event: its type, when the event happened, and its data,
// either built from the event (Data) or presented with the Presenter (Present). A zero Type
// sends nothing.
type source struct {
	Type    webhook.EventType
	At      time.Time
	Data    any
	Present presentFunc
}

// sources lists the domain events that become messages.
var sources = map[string]func(e uow.Event) source{
	appmoderation.EventReportFiled: func(e uow.Event) source {
		r := e.(appmoderation.ReportFiled).Report
		return source{Type: webhook.EventReportCreated, At: r.FiledAt, Present: report(r.ID)}
	},
	appmoderation.EventReportResolved: func(e uow.Event) source {
		ev := e.(appmoderation.ReportResolved)
		return source{Type: webhook.EventReportResolved, At: ev.Resolution.ResolvedAt, Present: report(ev.Report.ID)}
	},
	appmoderation.EventReportReopened: func(e uow.Event) source {
		ev := e.(appmoderation.ReportReopened)
		return source{Type: webhook.EventReportReopened, At: ev.Reopening.ReopenedAt, Present: report(ev.Report.ID)}
	},
	appidentity.EventRegistrationPending: func(e uow.Event) source {
		ev := e.(appidentity.RegistrationPending)
		return source{Type: webhook.EventRegistrationPending, At: ev.SubmittedAt, Present: registration(ev.UserID)}
	},
	appidentity.EventRegistrationApproved: func(e uow.Event) source {
		ev := e.(appidentity.RegistrationApproved)
		return source{Type: webhook.EventAccountCreated, At: ev.ApprovedAt, Present: registration(ev.UserID)}
	},
	appaccount.EventProfileRevised: func(e uow.Event) source {
		ev := e.(appaccount.ProfileRevised)
		// Only the changes of a local profile after the one it is created with (ADR-0060).
		if !ev.Federates() {
			return source{}
		}
		r := ev.Revision
		return source{Type: webhook.EventAccountUpdated, At: r.RevisedAt, Present: func(ctx context.Context, tx uow.Tx, p Presenter) (any, error) {
			return p.WebhookAccount(ctx, tx, r.AccountID)
		}}
	},
	appmoderation.EventAccountActionIssued: func(e uow.Event) source {
		a := e.(appmoderation.AccountActionIssued).Action
		return source{Type: webhook.EventModerationActionCreated, At: a.IssuedAt, Present: accountAction(a.ID)}
	},
	appmoderation.EventAccountActionLifted: func(e uow.Event) source {
		ev := e.(appmoderation.AccountActionLifted)
		return source{Type: webhook.EventModerationActionLifted, At: ev.Lift.LiftedAt, Present: accountAction(ev.Action.ID)}
	},
	appmoderation.EventAppealFiled: func(e uow.Event) source {
		a := e.(appmoderation.AppealFiled).Appeal
		return source{Type: webhook.EventAppealCreated, At: a.SubmittedAt, Present: func(ctx context.Context, tx uow.Tx, p Presenter) (any, error) {
			return p.WebhookAppeal(ctx, tx, a.ID)
		}}
	},
	appmoderation.EventDomainActionIssued: func(e uow.Event) source {
		a := e.(appmoderation.DomainActionIssued).Action
		return source{Type: webhook.EventDomainModerationActionCreated, At: a.IssuedAt, Present: domainAction(a.ID)}
	},
	appmoderation.EventDomainActionLifted: func(e uow.Event) source {
		ev := e.(appmoderation.DomainActionLifted)
		return source{Type: webhook.EventDomainModerationActionLifted, At: ev.Lift.LiftedAt, Present: domainAction(ev.Action.ID)}
	},
	appcontent.EventStatusCreated: func(e uow.Event) source {
		ev := e.(appcontent.StatusCreated)
		st, rev := ev.Status, ev.Revision
		// Only the public posts of local accounts (ADR-0060, ADR-0017).
		if !st.IsLocal || st.Visibility != appcontent.VisibilityPublic {
			return source{}
		}
		d := statusData{
			ID: st.ID, URI: st.URI, AccountID: st.AuthorAccountID, Visibility: string(st.Visibility),
			InReplyToURI: optString(st.InReplyToURI), SpoilerText: rev.SpoilerText, Sensitive: rev.Sensitive,
			Language: rev.Language, CreatedAt: timestamp(st.PublishedAt),
		}
		for _, b := range rev.Bodies {
			if b.Language == rev.Language || d.Content == "" {
				d.Content = b.ContentHTML
			}
		}
		return source{Type: webhook.EventStatusCreated, At: st.PublishedAt, Data: d}
	},
	// The error groups (ADR-0103 §8) follow the rule of their notifications: a group that only
	// web reports without a session have seen is sent when a report with one confirms it, unless
	// it is muted by then, and a regression from such a report is not sent.
	appdiagnostics.EventGroupCreated: func(e uow.Event) source {
		g := e.(appdiagnostics.GroupCreated).Group
		if g.Unauthenticated {
			return source{}
		}
		return source{Type: webhook.EventErrorGroupCreated, At: g.FirstSeenAt, Present: errorGroup(g.ID)}
	},
	appdiagnostics.EventGroupConfirmed: func(e uow.Event) source {
		ev := e.(appdiagnostics.GroupConfirmed)
		if ev.Head.Muted() {
			return source{}
		}
		return source{Type: webhook.EventErrorGroupCreated, At: ev.ConfirmedAt, Present: errorGroup(ev.Group.ID)}
	},
	appdiagnostics.EventGroupRegressed: func(e uow.Event) source {
		ev := e.(appdiagnostics.GroupRegressed)
		if ev.Unauthenticated {
			return source{}
		}
		return source{Type: webhook.EventErrorGroupRegressed, At: ev.Event.OccurredAt, Present: errorGroup(ev.Group.ID)}
	},
	appcontent.EventCustomEmojiAdministered: func(e uow.Event) source {
		ev := e.(appcontent.CustomEmojiAdministered)
		t, ok := customEmojiEvents[ev.Action]
		if !ok {
			return source{}
		}
		// A deletion is emitted before the emoji's rows go, so that it is presented as it was.
		return source{Type: t, At: ev.At, Present: func(ctx context.Context, tx uow.Tx, p Presenter) (any, error) {
			v, err := p.WebhookCustomEmoji(ctx, tx, ev.Emoji.ID)
			if err != nil {
				return nil, err
			}
			return withOperation(v, ev.Action)
		}}
	},
}

func report(id string) presentFunc {
	return func(ctx context.Context, tx uow.Tx, p Presenter) (any, error) {
		return p.WebhookReport(ctx, tx, id)
	}
}

func registration(userID string) presentFunc {
	return func(ctx context.Context, tx uow.Tx, p Presenter) (any, error) {
		return p.WebhookRegistration(ctx, tx, userID)
	}
}

func accountAction(id string) presentFunc {
	return func(ctx context.Context, tx uow.Tx, p Presenter) (any, error) {
		return p.WebhookAccountAction(ctx, tx, id)
	}
}

func errorGroup(id string) presentFunc {
	return func(ctx context.Context, tx uow.Tx, p Presenter) (any, error) {
		return p.WebhookErrorGroup(ctx, tx, id)
	}
}

func domainAction(id string) presentFunc {
	return func(ctx context.Context, tx uow.Tx, p Presenter) (any, error) {
		return p.WebhookDomainAction(ctx, tx, id)
	}
}

// RegisterSources registers the projectors that turn the domain events of the other modules into
// messages, in the transactions that emit them. Register them after the projectors of those
// modules.
func (s *Service) RegisterSources(r *uow.Projectors) {
	for eventType, build := range sources {
		r.Register(eventType, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
			src := build(e)
			if src.Type == "" {
				return nil
			}
			endpoints, err := s.d.Repository.SubscribedEndpoints(ctx, tx, src.Type)
			if err != nil || len(endpoints) == 0 {
				return err
			}
			data := src.Data
			if src.Present != nil {
				if s.presenter == nil {
					return fmt.Errorf("%w: %s", ErrNoPresenter, src.Type)
				}
				if data, err = src.Present(ctx, tx, s.presenter); err != nil {
					return fmt.Errorf("webhook: the data of %s: %w", src.Type, err)
				}
			}
			_, err = s.send(ctx, tx, src.Type, data, src.At, endpoints)
			return err
		})
	}
}

// send records in tx a message of type t with data to the endpoints, and enqueues a
// webhook.deliver job per delivery. It returns the last delivery.
func (s *Service) send(ctx context.Context, tx uow.Tx, t webhook.EventType, data any, at time.Time, endpoints []string) (webhook.Delivery, error) {
	if at.IsZero() {
		at = s.d.Clock.Now()
	}
	body, err := json.Marshal(envelope{Type: t, Timestamp: timestamp(at), Data: data})
	if err != nil {
		return webhook.Delivery{}, fmt.Errorf("webhook: the body of %s: %w", t, err)
	}
	now := s.d.Clock.Now()
	m := webhook.Message{ID: s.d.IDs.NewID(), EventType: t, Body: body, CreatedAt: now}
	if err := s.d.Repository.InsertMessage(ctx, tx, m); err != nil {
		return webhook.Delivery{}, err
	}
	var last webhook.Delivery
	jobs := make([]queue.NewJob, 0, len(endpoints))
	for _, id := range endpoints {
		last = webhook.Delivery{ID: s.d.IDs.NewID(), MessageID: m.ID, EndpointID: id, CreatedAt: now}
		if err := s.d.Repository.InsertDelivery(ctx, tx, last); err != nil {
			return webhook.Delivery{}, err
		}
		job, err := deliverJob(last.ID, "")
		if err != nil {
			return webhook.Delivery{}, err
		}
		jobs = append(jobs, job)
	}
	return last, s.d.Outbox.EnqueueTx(ctx, tx, jobs...)
}
