// SPDX-License-Identifier: AGPL-3.0-only

package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/webhook"
)

// Jobs of the webhook module.
const (
	// JobDeliver sends one delivery (ADR-0060 "配送と再送").
	JobDeliver = "webhook.deliver"
	// JobPurgeMessages deletes the messages past webhook.Retention with their deliveries and attempts.
	JobPurgeMessages = "webhook.purge_messages"
)

// PurgeInterval is how often JobPurgeMessages runs, and PurgeBatch how many messages one transaction
// deletes.
const (
	PurgeInterval = time.Hour
	PurgeBatch    = 500
	// purgeMaxBatches bounds one run; the next run continues.
	purgeMaxBatches = 100
)

// deliverPayload is the payload of JobDeliver. Redeliver sends the delivery even when an attempt
// already succeeded.
type deliverPayload struct {
	DeliveryID string `json:"delivery_id"`
	Redeliver  bool   `json:"redeliver,omitempty"`
}

// deliverJob returns the job that sends a delivery: the first one when redelivery is "", and a
// redelivery from an administrator, keyed by redelivery, otherwise.
func deliverJob(deliveryID, redelivery string) (queue.NewJob, error) {
	payload, err := json.Marshal(deliverPayload{DeliveryID: deliveryID, Redeliver: redelivery != ""})
	if err != nil {
		return queue.NewJob{}, err
	}
	key := JobDeliver + ":" + deliveryID
	if redelivery != "" {
		key += ":" + redelivery
	}
	return queue.NewJob{Type: JobDeliver, Payload: payload, IdempotencyKey: key}, nil
}

// RetryPolicy is the retry policy of JobDeliver: webhook.RetryDelays over webhook.MaxAttempts
// attempts, then a dead job that an administrator can send again (Redeliver).
func RetryPolicy() queue.RetryPolicy {
	return queue.RetryPolicy{MaxAttempts: webhook.MaxAttempts, Delays: webhook.RetryDelays}
}

// RegisterJobs registers the handlers of JobDeliver and JobPurgeMessages, and the hourly schedule of
// JobPurgeMessages.
func (s *Service) RegisterJobs(h *queue.JobHandlerRegistry, sc *queue.Scheduler) {
	h.RegisterWithPolicy(JobDeliver, func(ctx context.Context, j queue.Job) queue.Result {
		var p deliverPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.DeliveryID == "" {
			return queue.Fail(fmt.Errorf("webhook: the payload of %s: %w", j.ID, err))
		}
		return s.Deliver(ctx, p.DeliveryID, p.Redeliver)
	}, RetryPolicy())
	h.Register(JobPurgeMessages, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := s.Purge(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	if sc != nil {
		sc.Add(queue.Schedule{Name: JobPurgeMessages, Every: PurgeInterval, Job: queue.NewJob{Type: JobPurgeMessages}})
	}
}

// Deliver sends a delivery once, outside any transaction, and records the attempt. It sends
// nothing for a delivery that is gone, whose endpoint is not enabled, or that already succeeded
// (unless redeliver). A 2xx answer succeeds; 410 Gone disables the endpoint; every other answer
// and failure asks for a retry, after the Retry-After of the answer when it has one. A run of
// failures that webhook.Failures.ShouldDisable disables the endpoint.
func (s *Service) Deliver(ctx context.Context, deliveryID string, redeliver bool) queue.Result {
	send, err := s.d.Repository.FindSend(ctx, deliveryID)
	switch {
	case errors.Is(err, ErrNotFound):
		return queue.Done()
	case err != nil:
		return queue.Retry(0, err)
	case send.State != webhook.StateEnabled, send.Succeeded && !redeliver:
		return queue.Done()
	}
	secrets, err := s.signingSecrets(ctx, send.Delivery.EndpointID)
	if err != nil {
		return queue.Retry(0, err)
	}
	now := s.d.Clock.Now()
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	header.Set(webhook.HeaderID, send.Message.ID)
	header.Set(webhook.HeaderTimestamp, strconv.FormatInt(now.Unix(), 10))
	header.Set(webhook.HeaderSignature, webhook.SignatureHeader(secrets, send.Message.ID, now, send.Message.Body))
	attempt := webhook.Attempt{ID: s.d.IDs.NewID(), DeliveryID: deliveryID, AttemptedAt: kernel.Normalize(now)}
	start := time.Now()
	resp, sendErr := s.d.HTTP.Do(ctx, &kernel.HTTPRequest{
		Method: http.MethodPost, URL: send.URL, Header: header, Body: send.Message.Body, Purpose: kernel.HTTPPurposeWebhook,
	})
	var retryAfter time.Duration
	if sendErr == nil {
		// The answer's body is not read beyond what closing it takes.
		_, _ = resp.ReadAll()
		if !webhook.StandardStatus(resp.StatusCode) {
			sendErr = fmt.Errorf("webhook: status code %d outside HTTP's range", resp.StatusCode)
		}
	}
	if sendErr == nil {
		attempt.StatusCode = resp.StatusCode
		attempt.Outcome = webhook.OutcomeOf(resp.StatusCode)
		retryAfter, _ = webhook.RetryAfter(resp.Header.Get("Retry-After"), now)
	} else {
		attempt.Outcome = failureOutcome(sendErr)
	}
	attempt.Duration = time.Since(start).Truncate(time.Millisecond)
	host := webhook.Host(send.URL)
	label := webhook.AuditLabel(webhook.EndpointSettings{URL: send.URL, Description: send.Description})
	if err := s.recordAttempt(ctx, send.Delivery.EndpointID, label, attempt); err != nil {
		return queue.Retry(0, err)
	}
	switch {
	case attempt.Outcome == webhook.OutcomeSucceeded:
		return queue.Done()
	case webhook.Gone(attempt.StatusCode):
		s.d.Logger.InfoContext(ctx, "webhook endpoint answered 410 Gone; disabled it",
			slog.String("endpoint_id", send.Delivery.EndpointID), slog.String("host", host))
		return queue.Done()
	case sendErr != nil:
		return queue.Retry(0, fmt.Errorf("webhook: deliver %s to %s: %s", deliveryID, host, attempt.Outcome))
	}
	return queue.Retry(retryAfter, fmt.Errorf("webhook: deliver %s to %s: status %d", deliveryID, host, attempt.StatusCode))
}

// failureOutcome classifies a failure to get an answer.
func failureOutcome(err error) webhook.Outcome {
	var netErr net.Error
	switch {
	case errors.Is(err, kernel.ErrForbiddenDestination):
		return webhook.OutcomeForbidden
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return webhook.OutcomeTimeout
	}
	return webhook.OutcomeConnectionError
}

// signingSecrets opens the secrets that sign now, the current one first.
func (s *Service) signingSecrets(ctx context.Context, endpointID string) ([][]byte, error) {
	rows, err := s.d.Repository.Secrets(ctx, endpointID, 2)
	if err != nil {
		return nil, err
	}
	rows = webhook.SigningSecrets(rows, s.d.Clock.Now())
	if len(rows) == 0 {
		return nil, fmt.Errorf("webhook: endpoint %s has no secret", endpointID)
	}
	out := make([][]byte, len(rows))
	for i, r := range rows {
		if out[i], err = s.d.Sealer.Open(ctx, r.Sealed, SecretAAD(r.ID)); err != nil {
			return nil, fmt.Errorf("webhook: open secret %s: %w", r.ID, err)
		}
	}
	return out, nil
}

// recordAttempt records an attempt and, in the same transaction, disables its endpoint when the
// attempt got 410 Gone or ends a run of failures that disables it.
func (s *Service) recordAttempt(ctx context.Context, endpointID, label string, a webhook.Attempt) error {
	if err := a.Validate(); err != nil {
		return err
	}
	changeID, auditID := s.d.IDs.NewID(), s.d.IDs.NewID()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		repo := s.d.Repository
		if err := repo.InsertAttempt(ctx, tx, a); err != nil {
			return err
		}
		if err := tx.Emit(ctx, AttemptRecorded{EndpointID: endpointID, Attempt: a}); err != nil {
			return err
		}
		reason := webhook.StateReason("")
		if webhook.Gone(a.StatusCode) {
			reason = webhook.ReasonGone
		} else if a.Failed() {
			f, err := repo.Failures(ctx, tx, endpointID)
			if err != nil {
				return err
			}
			if f != nil && f.ShouldDisable(a.AttemptedAt) {
				reason = webhook.ReasonFailing
			}
		}
		if reason == "" {
			return nil
		}
		return s.disable(ctx, tx, endpointID, label, reason, changeID, auditID, a.AttemptedAt)
	})
}

// disable disables an enabled endpoint in tx as the server, audits it under label, and emits
// EndpointDisabled.
func (s *Service) disable(ctx context.Context, tx uow.Tx, endpointID, label string, reason webhook.StateReason, changeID, auditID string, at time.Time) error {
	h, err := s.d.Repository.HeadForUpdate(ctx, tx, endpointID)
	if err != nil || h.State != webhook.StateEnabled {
		return err
	}
	change := StateChange{ID: changeID, EndpointID: endpointID, State: webhook.StateDisabled, Reason: reason, ChangedAt: at}
	if err := s.d.Repository.InsertStateChange(ctx, tx, change); err != nil {
		return err
	}
	h.State = webhook.StateDisabled
	if err := tx.Emit(ctx, EndpointChanged{Head: h}); err != nil {
		return err
	}
	entry := appmoderation.AuditEntry{
		ID: auditID, ActorKind: appmoderation.AuditActorSystem, Action: appmoderation.AuditWebhookDisabled,
		TargetKind: appmoderation.AuditTargetWebhook, TargetID: endpointID, TargetLabel: label,
		SourceEventID: change.ID, OccurredAt: at,
	}
	if err := tx.Emit(ctx, appmoderation.AuditRecorded{Entry: entry}); err != nil {
		return err
	}
	return tx.Emit(ctx, EndpointDisabled{EndpointID: endpointID, StateChangeID: change.ID, Reason: reason, At: at})
}

// Purge deletes the messages older than webhook.Retention with their deliveries and attempts,
// and returns how many messages it deleted. It is safe to repeat.
func (s *Service) Purge(ctx context.Context) (int64, error) {
	before := s.d.Clock.Now().Add(-webhook.Retention)
	var total int64
	for range purgeMaxBatches {
		var n int64
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			n, err = s.d.Repository.DeleteMessagesBefore(ctx, tx, before, PurgeBatch)
			return err
		})
		if err != nil {
			return total, fmt.Errorf("webhook: purge the messages: %w", err)
		}
		total += n
		if n < PurgeBatch {
			break
		}
	}
	return total, nil
}
