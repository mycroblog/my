// SPDX-License-Identifier: AGPL-3.0-only

package queuepush

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// DefaultEventsPath is the default path of the Lambda events, the default of the Lambda Web
// Adapter's AWS_LWA_PASS_THROUGH_PATH.
const DefaultEventsPath = "/events"

// The headers that the Lambda Web Adapter sets on every request it forwards, replacing any that
// the client sent. HeaderRequestContext is the JSON null for an event that is not an HTTP
// request (a pass-through, such as an SQS event) and the request context otherwise (a Function
// URL); HeaderLambdaContext is the context of the invocation, whose deadline bounds the batch.
const (
	HeaderRequestContext = "X-Amzn-Request-Context"
	HeaderLambdaContext  = "X-Amzn-Lambda-Context"
)

// TickType is the type of the tick message: an SQS message whose body is the JSON object
// {"type":"tick"}, which EventBridge Scheduler sends to the queue every minute. Every other
// body is the ID of a job to run.
const TickType = "tick"

// maxEventBytes bounds the body of an event: the payload of a synchronous Lambda invocation is
// at most 6 MB.
const maxEventBytes = 6 << 20

// recordMargin is the time past the lease that a record keeps for the record of its result.
const recordMargin = 30 * time.Second

// DefaultEventsBudget is the time a batch may take when its request carries no deadline of the
// Lambda invocation.
const DefaultEventsBudget = 10 * time.Minute

// LambdaOptions configure a LambdaHandler. Receiver and Trigger are required.
type LambdaOptions struct {
	Receiver Receiver
	Trigger  Ticker
	// Path is the path of the events, AWS_LWA_PASS_THROUGH_PATH; empty means DefaultEventsPath.
	Path string
	// Budget is how long a batch may take from its arrival when the request has no deadline of
	// the invocation in HeaderLambdaContext; zero means DefaultEventsBudget.
	Budget time.Duration
	// Now reads the wall clock that the deadline of the invocation is on; nil means time.Now.
	Now func() time.Time
	// Problem writes the 404 of the requests that are no Lambda event; nil writes a plain-text body.
	Problem ProblemWriter
	// Logger logs the refused requests and the failures; nil means slog.Default().
	Logger *slog.Logger
}

// LambdaHandler receives the SQS events that the Lambda Web Adapter passes through to the
// server (ADR-0120 §2, §4, §5): each record is a wake-up signal, whose body is the ID of a job of
// the database queue for the push receiver to run, or a tick message, which runs the trigger.
//
// The records run one after another. The answer lists in batchItemFailures the records whose
// result could not be recorded and every record left when the time to the deadline of the
// invocation fell short of a lease, for SQS to deliver them again; the others are done. The
// Lambda Web Adapter makes only the body of the answer the result of the invocation, dropping the
// status and the headers, and a body that is not JSON would be a success of the whole batch, so
// every answer to an event is JSON. Only the requests whose HeaderRequestContext is the JSON
// null, which the adapter sets for the events alone, are events; the others are a 404.
type LambdaHandler struct{ o LambdaOptions }

// NewLambda returns a LambdaHandler on o. It panics when a required option is missing.
func NewLambda(o LambdaOptions) *LambdaHandler {
	if o.Receiver == nil || o.Trigger == nil {
		panic("queuepush: LambdaOptions.Receiver and Trigger are required")
	}
	o.Path = cmp.Or(o.Path, DefaultEventsPath)
	if o.Budget <= 0 {
		o.Budget = DefaultEventsBudget
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Problem == nil {
		o.Problem = func(w http.ResponseWriter, _ *http.Request, status int) {
			http.Error(w, http.StatusText(status), status)
		}
	}
	return &LambdaHandler{o: o}
}

// Register registers the endpoint of the events on r.
func (h *LambdaHandler) Register(r Router) {
	r.Handle("POST "+h.o.Path, http.HandlerFunc(h.serveEvents))
}

// PassThrough reports whether r is an event that the Lambda Web Adapter passed through: its
// HeaderRequestContext is the JSON null.
func PassThrough(r *http.Request) bool {
	v := r.Header.Values(HeaderRequestContext)
	return len(v) == 1 && strings.TrimSpace(v[0]) == "null"
}

// sqsEvent is the part of an SQS event of Lambda that the handler reads.
type sqsEvent struct {
	Records []sqsRecord `json:"Records"`
}

type sqsRecord struct {
	MessageID string `json:"messageId"`
	Body      string `json:"body"`
}

// batchResponse is the partial batch response of an SQS event source mapping with
// ReportBatchItemFailures.
type batchResponse struct {
	BatchItemFailures []batchItemFailure `json:"batchItemFailures"`
}

type batchItemFailure struct {
	ItemIdentifier string `json:"itemIdentifier"`
}

// lambdaContext is the part of HeaderLambdaContext that the handler reads: the deadline of the
// invocation in milliseconds since the Unix epoch.
type lambdaContext struct {
	Deadline int64 `json:"deadline"`
}

func (h *LambdaHandler) serveEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !PassThrough(r) {
		h.o.Logger.WarnContext(r.Context(), "events endpoint refused a request that is no Lambda event", slog.String("url.path", h.o.Path))
		h.o.Problem(w, r, http.StatusNotFound)
		return
	}
	arrived := h.o.Now()
	deadline := h.deadline(r, arrived)
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(deadline.Sub(arrived) + writeMargin))
	ev, err := decodeEvent(w, r)
	if err != nil {
		// No record to report: nothing that SQS would deliver again.
		h.o.Logger.ErrorContext(r.Context(), "events endpoint received no SQS event", slog.Any("error", err))
		writeBatch(w, http.StatusOK, nil)
		return
	}
	writeBatch(w, http.StatusOK, h.run(r.Context(), ev.Records, deadline))
}

// deadline returns the deadline of the invocation of r, or arrived plus the budget when the
// request does not carry it.
func (h *LambdaHandler) deadline(r *http.Request, arrived time.Time) time.Time {
	var lc lambdaContext
	if v := r.Header.Get(HeaderLambdaContext); v != "" && json.Unmarshal([]byte(v), &lc) == nil && lc.Deadline > 0 {
		return time.UnixMilli(lc.Deadline)
	}
	return arrived.Add(h.o.Budget)
}

// run handles the records in order and returns those that SQS must deliver again.
func (h *LambdaHandler) run(ctx context.Context, records []sqsRecord, deadline time.Time) []string {
	var failed []string
	ticked := false
	need := h.o.Receiver.Lease() + recordMargin
	for i, rec := range records {
		if h.o.Now().Add(need).After(deadline) {
			for _, rest := range records[i:] {
				failed = append(failed, rest.MessageID)
			}
			h.o.Logger.WarnContext(ctx, "events endpoint returned the records left at the deadline of the invocation",
				slog.Int("messaging.batch.message_count", len(records)-i), slog.Time("deadline", deadline))
			break
		}
		if err := h.record(ctx, rec, &ticked); err != nil {
			h.o.Logger.ErrorContext(ctx, "SQS message not handled", slog.String("messaging.message.id", rec.MessageID), slog.Any("error", err))
			failed = append(failed, rec.MessageID)
		}
	}
	return failed
}

// record handles one record: a tick message runs the trigger, once a batch, and any other body
// is the ID of a job.
func (h *LambdaHandler) record(ctx context.Context, rec sqsRecord, ticked *bool) error {
	body := strings.TrimSpace(rec.Body)
	if strings.HasPrefix(body, "{") {
		var m struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(body), &m); err != nil || m.Type != TickType {
			// Not a message of this server: delivering it again would not change that.
			h.o.Logger.WarnContext(ctx, "SQS message dropped: neither a job ID nor a tick", slog.String("messaging.message.id", rec.MessageID))
			return nil
		}
		if *ticked {
			return nil
		}
		if err := h.o.Trigger.Tick(ctx); err != nil {
			return err
		}
		*ticked = true
		return nil
	}
	if body == "" {
		h.o.Logger.WarnContext(ctx, "SQS message dropped: empty body", slog.String("messaging.message.id", rec.MessageID))
		return nil
	}
	return h.o.Receiver.Receive(ctx, body)
}

func decodeEvent(w http.ResponseWriter, r *http.Request) (sqsEvent, error) {
	var ev sqsEvent
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxEventBytes))
	if err != nil {
		return ev, err
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		return ev, err
	}
	if ev.Records == nil {
		return ev, errors.New("the event has no Records")
	}
	return ev, nil
}

// writeBatch writes the partial batch response that lists ids as the failures. The Lambda Web
// Adapter drops the status, which only the access log sees.
func writeBatch(w http.ResponseWriter, status int, ids []string) {
	resp := batchResponse{BatchItemFailures: make([]batchItemFailure, 0, len(ids))}
	for _, id := range ids {
		resp.BatchItemFailures = append(resp.BatchItemFailures, batchItemFailure{ItemIdentifier: id})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

// FailEvent answers the event of r, a pass-through of the Lambda Web Adapter, with every record
// of its SQS event in batchItemFailures, for SQS to deliver them again: the answer of the
// maintenance mode, which runs nothing (ADR-0084), with the status 503.
func FailEvent(w http.ResponseWriter, r *http.Request) {
	ev, _ := decodeEvent(w, r)
	ids := make([]string, 0, len(ev.Records))
	for _, rec := range ev.Records {
		ids = append(ids, rec.MessageID)
	}
	writeBatch(w, http.StatusServiceUnavailable, ids)
}
