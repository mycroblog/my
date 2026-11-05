// SPDX-License-Identifier: AGPL-3.0-only

package fakeplugin

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"connectrpc.com/connect/v2"

	mailerv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/mailer/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/mailer/v1/mailerv1connect"
)

// Mailbox is the mail service behind the fake's mailer kind: it answers OUTCOME_INVALID to a request without a recipient,
// Answers[to] to a recipient listed there, and OUTCOME_SENT to any other. It is safe for concurrent use.
type Mailbox struct {
	// Answers are the responses by recipient.
	Answers map[string]*mailerv1.SendResponse

	mu        sync.Mutex
	requests  []*mailerv1.SendRequest
	delivered []*mailerv1.SendRequest
}

// Requests returns every request that reached the mailbox, in order.
func (b *Mailbox) Requests() []*mailerv1.SendRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*mailerv1.SendRequest(nil), b.requests...)
}

// Delivered returns the requests that it answered with OUTCOME_SENT, in order.
func (b *Mailbox) Delivered() []*mailerv1.SendRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*mailerv1.SendRequest(nil), b.delivered...)
}

func (b *Mailbox) send(req *mailerv1.SendRequest) *mailerv1.SendResponse {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.requests = append(b.requests, req)
	if req.GetTo() == "" {
		return &mailerv1.SendResponse{Outcome: mailerv1.Outcome_OUTCOME_INVALID}
	}
	if res, ok := b.Answers[req.GetTo()]; ok {
		return res
	}
	b.delivered = append(b.delivered, req)
	return &mailerv1.SendResponse{Outcome: mailerv1.Outcome_OUTCOME_SENT}
}

type mailerServer struct {
	mailerv1connect.UnimplementedMailerServiceHandler

	o Options
}

// Send implements mailerv1connect.MailerServiceHandler.
func (s *mailerServer) Send(ctx context.Context, req *mailerv1.SendRequest) (*mailerv1.SendResponse, error) {
	switch {
	case s.o.Mailbox == nil:
		return nil, connect.NewError(connect.CodeUnimplemented, "this fake has no mailbox")
	case s.o.Block:
		<-ctx.Done()
		return nil, ctx.Err()
	case s.o.Broken:
		return &mailerv1.SendResponse{}, nil
	}
	return s.o.Mailbox.send(req), nil
}

// serveMailerJSON adds the mailer kind of the fake of net/http and encoding/json alone.
func serveMailerJSON(mux *http.ServeMux, o Options) {
	if o.Mailbox == nil {
		return
	}
	mux.HandleFunc("POST "+mailerv1connect.MailerServiceSendProcedure, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "send JSON", http.StatusUnsupportedMediaType)
			return
		}
		// The shape of proto3's JSON mapping: camelCase names, a name for an enum, an object for a map, and fields at their
		// zero value left out.
		var req struct {
			Category     string            `json:"category"`
			To           string            `json:"to"`
			EnvelopeFrom string            `json:"envelopeFrom"`
			FromName     string            `json:"fromName"`
			ReplyTo      string            `json:"replyTo"`
			Subject      string            `json:"subject"`
			Text         string            `json:"text"`
			HTML         string            `json:"html"`
			Headers      map[string]string `json:"headers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad JSON", http.StatusBadRequest)
			return
		}
		if o.Block {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if o.Broken {
			_, _ = w.Write([]byte("{}"))
			return
		}
		res := o.Mailbox.send(&mailerv1.SendRequest{
			Category: mailerv1.Category(mailerv1.Category_value[req.Category]),
			To:       req.To, EnvelopeFrom: req.EnvelopeFrom, FromName: req.FromName, ReplyTo: req.ReplyTo,
			Subject: req.Subject, Text: req.Text, Html: req.HTML, Headers: req.Headers,
		})
		out := map[string]any{"outcome": res.GetOutcome().String(), "extra": "a field the server does not know, which it ignores"}
		if s := res.GetEnhancedStatus(); s != "" {
			out["enhancedStatus"] = s
		}
		_ = json.NewEncoder(w).Encode(out)
	})
}
