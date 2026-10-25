// SPDX-License-Identifier: AGPL-3.0-only

// Package mailevents serves the endpoint that Amazon SNS calls with the notifications of Amazon
// SES (ADR-0086 §5): POST /mail/events/ses. It is not part of the client API: no user calls it.
// The endpoint exists only when the topic of the notifications (ses_sns_topic_arn) is set. The
// body is read up to MaxBodyBytes before anything else, and the SNS signature is verified before
// the message is used. A subscription confirmation is confirmed by a GET of its SubscribeURL; a
// notification of a permanent bounce or a complaint is recorded by the identity module as a
// delivery failure of the recipient's address, which suppresses it.
package mailevents

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/idna"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Path is the path of the endpoint.
const Path = "/mail/events/ses"

// MaxBodyBytes is the largest body the endpoint reads. An SNS message is at most 256 KiB, and
// escaping it as a JSON string in the envelope at most about doubles it.
const MaxBodyBytes = 1 << 20

// Recorder is what the endpoint needs from the identity module; *identity.Service implements it.
type Recorder interface {
	RecordAddressFailure(ctx context.Context, in appidentity.AddressFailure) (bool, error)
}

// ProblemWriter writes the error response of an HTTP status, the same one the rest of the
// server uses for that status.
type ProblemWriter func(w http.ResponseWriter, r *http.Request, status int)

// Options configure a Handler.
type Options struct {
	// Topic is the SNS topic of the notifications (ses_sns_topic_arn); messages of any other
	// topic are refused.
	Topic TopicARN
	// Recorder records the bounces and complaints.
	Recorder Recorder
	// HTTPClient fetches the signing certificates and confirms the subscriptions; it is the
	// SSRF-guarded client (ADR-0019 §1).
	HTTPClient kernel.HTTPClient
	// KV caches the signing certificates.
	KV kernel.KeyValueStore
	// Clock dates a notification that carries no time of its own.
	Clock kernel.Clock
	// Problem writes error responses; nil writes a plain-text body.
	Problem ProblemWriter
	// Logger logs unexpected errors; nil means slog.Default().
	Logger *slog.Logger
}

// Handler serves the endpoint.
type Handler struct {
	o        Options
	verifier verifier
}

// New returns a Handler on o.
func New(o Options) *Handler {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Problem == nil {
		o.Problem = func(w http.ResponseWriter, _ *http.Request, status int) {
			http.Error(w, http.StatusText(status), status)
		}
	}
	return &Handler{o: o, verifier: verifier{topic: o.Topic, client: o.HTTPClient, kv: o.KV, logger: o.Logger}}
}

// Router registers handlers for http.ServeMux patterns.
type Router interface {
	Handle(pattern string, h http.Handler)
}

// Register registers the endpoint on r.
func (h *Handler) Register(r Router) {
	r.Handle("POST "+Path, h)
}

// ServeHTTP implements http.Handler: 200 for a message that is handled or ignored; 400 for a
// body that is not a signed message of the topic; 413 beyond the body's limit; and 503 when the
// certificate could not be fetched, the subscription not confirmed, or the notification not
// recorded, for SNS to send it again.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		status := http.StatusBadRequest
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		h.o.Problem(w, r, status)
		return
	}
	ctx := r.Context()
	var m snsMessage
	if err := json.Unmarshal(body, &m); err != nil {
		h.refuse(w, r, rejected("the body is not an SNS message"))
		return
	}
	if err := h.verifier.verify(ctx, &m); err != nil {
		h.refuse(w, r, err)
		return
	}
	switch m.Type {
	case typeSubscriptionConfirmation:
		err = h.confirm(ctx, &m)
	case typeUnsubscribeConfirmation:
		h.o.Logger.WarnContext(ctx, "SES notifications unsubscribed from the SNS topic", slog.String("topic", m.TopicArn))
	case typeNotification:
		err = h.record(ctx, &m)
	}
	if err != nil {
		h.refuse(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// refuse answers a message that failed: 400 for one refused for good, 503 otherwise.
func (h *Handler) refuse(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errRejected) {
		// The reason, never the body or the signature.
		h.o.Logger.WarnContext(r.Context(), "SES notification refused", slog.Any("error", err))
		h.o.Problem(w, r, http.StatusBadRequest)
		return
	}
	h.o.Logger.ErrorContext(r.Context(), "SES notification not handled", slog.Any("error", err))
	h.o.Problem(w, r, http.StatusServiceUnavailable)
}

// confirm confirms the subscription of m by a GET of its SubscribeURL on the topic's SNS host.
func (h *Handler) confirm(ctx context.Context, m *snsMessage) error {
	u, ok := h.o.Topic.snsURL(m.SubscribeURL)
	if !ok {
		return rejected("SubscribeURL is not on %s", h.o.Topic.snsHost())
	}
	if _, err := h.verifier.fetch(ctx, u.String()); err != nil {
		return err
	}
	h.o.Logger.InfoContext(ctx, "SNS subscription of the SES notifications confirmed", slog.String("topic", m.TopicArn))
	return nil
}

// sesNotification is the part of an SES notification that the endpoint reads, from either an
// identity's notifications (notificationType) or a configuration set's event destination
// (eventType) (https://docs.aws.amazon.com/ses/latest/dg/notification-contents.html).
type sesNotification struct {
	NotificationType string `json:"notificationType"`
	EventType        string `json:"eventType"`
	Bounce           *struct {
		BounceType        string         `json:"bounceType"`
		BouncedRecipients []sesRecipient `json:"bouncedRecipients"`
		Timestamp         string         `json:"timestamp"`
		FeedbackID        string         `json:"feedbackId"`
	} `json:"bounce"`
	Complaint *struct {
		ComplainedRecipients  []sesRecipient `json:"complainedRecipients"`
		ComplaintFeedbackType string         `json:"complaintFeedbackType"`
		Timestamp             string         `json:"timestamp"`
		FeedbackID            string         `json:"feedbackId"`
	} `json:"complaint"`
}

// sesRecipient is one recipient of a bounce or a complaint.
type sesRecipient struct {
	EmailAddress string `json:"emailAddress"`
	// Status is the enhanced status code of a bounce, such as 5.1.1.
	Status string `json:"status"`
}

// record records the permanent bounces and the complaints of the SES notification in m; other
// notifications, transient and undetermined bounces, and complaints of the type not-spam change
// nothing.
func (h *Handler) record(ctx context.Context, m *snsMessage) error {
	var n sesNotification
	if err := json.Unmarshal([]byte(m.Message), &n); err != nil {
		// A signed message of the topic that is not an SES notification: nothing to record.
		h.o.Logger.WarnContext(ctx, "SNS message of the SES topic is not an SES notification", slog.String("message_id", m.MessageID))
		return nil
	}
	kind := n.NotificationType
	if kind == "" {
		kind = n.EventType
	}
	var (
		failure    identity.EmailFailureKind
		recipients []sesRecipient
		timestamp  string
		feedbackID string
	)
	switch {
	case kind == "Bounce" && n.Bounce != nil && n.Bounce.BounceType == "Permanent":
		failure, recipients, timestamp, feedbackID = identity.EmailHardBounce, n.Bounce.BouncedRecipients, n.Bounce.Timestamp, n.Bounce.FeedbackID
	case kind == "Complaint" && n.Complaint != nil && n.Complaint.ComplaintFeedbackType != "not-spam":
		failure, recipients, timestamp, feedbackID = identity.EmailComplaint, n.Complaint.ComplainedRecipients, n.Complaint.Timestamp, n.Complaint.FeedbackID
	default:
		return nil
	}
	if feedbackID == "" {
		h.o.Logger.WarnContext(ctx, "SES notification without a feedbackId", slog.String("message_id", m.MessageID))
		return nil
	}
	at := h.occurredAt(timestamp, m.Timestamp)
	for i, rcpt := range recipients {
		// The server sends each email to one recipient, so a notification has one; a second
		// one gets a key of its own.
		key := feedbackID
		if i > 0 {
			key += ":" + strconv.Itoa(i)
		}
		_, err := h.o.Recorder.RecordAddressFailure(ctx, appidentity.AddressFailure{
			Addresses: addressForms(rcpt.EmailAddress), Kind: failure, Source: identity.EmailFailureSES,
			FeedbackKey: key, EnhancedStatus: rcpt.Status, OccurredAt: at,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// occurredAt returns the first of the times that parses, or now.
func (h *Handler) occurredAt(times ...string) time.Time {
	for _, s := range times {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t.UTC()
		}
	}
	return h.o.Clock.Now()
}

// addressForms returns the forms of an address to look up: as SES wrote it, and with its domain
// in Unicode when SES wrote an internationalized domain in Punycode, as the mailers send it.
func addressForms(address string) []string {
	address = strings.TrimSpace(address)
	forms := []string{address}
	at := strings.LastIndexByte(address, '@')
	if at < 0 {
		return forms
	}
	domain, err := idna.ToUnicode(address[at+1:])
	if err == nil && domain != address[at+1:] {
		forms = append(forms, address[:at+1]+domain)
	}
	return forms
}
