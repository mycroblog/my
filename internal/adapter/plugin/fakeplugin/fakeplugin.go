// SPDX-License-Identifier: AGPL-3.0-only

// Package fakeplugin is the plugin that the adapters and `my plugin test` are tested against (ADR-0146 §16), in two
// forms: one built on the Connect library, and one of net/http and encoding/json alone, as a small plugin is written.
// Only tests link it: scripts/go-build-tags.sh fails when cmd/my depends on it.
package fakeplugin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"sync/atomic"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"

	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/billing/v1/billingv1connect"
	hookv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/hook/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/hook/v1/hookv1connect"
	keymanagerv1connect "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/key_manager/v1/key_managerv1connect"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/mailer/v1/mailerv1connect"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/media/v1/mediav1connect"
	probev1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/probe/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/probe/v1/probev1connect"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/translator/v1/translatorv1connect"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/media"
	"github.com/mycroblog/my/internal/app/translation"
)

// Options make the fake misbehave or require credentials.
type Options struct {
	// Token, when set, is the bearer token a request must carry.
	Token string
	// Broken answers with an empty response, which a plugin that forgot its outcome sends.
	Broken bool
	// Block holds every call until its context ends.
	Block bool
	// Billing is what the fake's billing kind reports, under the source's own rules of requests and failures. Without it
	// the fake does not serve the kind.
	Billing appfinance.BillingSource
	// Mailbox is the mail service of the fake's mailer kind. Without it the fake does not serve the kind.
	Mailbox *Mailbox
	// Translator is the service behind the fake's translator kind, whose answers it hands on as they are. Without it the fake
	// does not serve the kind.
	Translator translation.Translator
	// Media is the processor behind the fake's media kind, whose output it hands on as it is. Without it the fake does not
	// serve the kind, which only the Connect form can: its calls stream both ways.
	Media media.Processor
	// MediaUnderstate declares each output of the media kind at half its size, as a plugin that lies about it.
	MediaUnderstate bool
	// KeyManager is the key store behind the fake's key manager kind, whose answers it hands on as they are, so that a test
	// can make the fake break the contract. Its key IDs are the plugin's own, without the server's prefix, and an error that
	// is no result of the port fails the call. Without it the fake does not serve the kind.
	KeyManager kernel.KeyManager
}

// Recorder tells what reached a fake.
type Recorder struct {
	calls atomic.Int64
	proto atomic.Value
}

// Calls returns how many calls reached the fake.
func (r *Recorder) Calls() int64 { return r.calls.Load() }

// Proto returns the HTTP version of the last call, such as HTTP/2.0.
func (r *Recorder) Proto() string {
	p, _ := r.proto.Load().(string)
	return p
}

func (r *Recorder) wrap(token string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.calls.Add(1)
		r.proto.Store(req.Proto)
		if token != "" && req.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, req)
	})
}

// Connect returns the fake built on the Connect library, which answers the Connect and gRPC protocols.
func Connect(o Options) (http.Handler, *Recorder) {
	server := connect.NewServer()
	probev1connect.RegisterProbeServiceHandler(server, &probeServer{o: o})
	billingv1connect.RegisterBillingServiceHandler(server, &billingServer{o: o})
	mailerv1connect.RegisterMailerServiceHandler(server, &mailerServer{o: o})
	translatorv1connect.RegisterTranslatorServiceHandler(server, &translatorServer{o: o})
	mediav1connect.RegisterMediaServiceHandler(server, &mediaServer{o: o})
	keymanagerv1connect.RegisterKeyManagerServiceHandler(server, &keyManagerServer{o: o})
	hookv1connect.RegisterHookServiceHandler(server, &hookServer{o: o})
	mux := http.NewServeMux()
	connecthttp.Mount(mux, server)
	r := new(Recorder)
	return r.wrap(o.Token, mux), r
}

type probeServer struct {
	probev1connect.UnimplementedProbeServiceHandler

	o Options
}

// Echo implements probev1connect.ProbeServiceHandler.
func (s *probeServer) Echo(ctx context.Context, req *probev1.EchoRequest) (*probev1.EchoResponse, error) {
	if s.o.Block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.o.Broken {
		return &probev1.EchoResponse{}, nil
	}
	return &probev1.EchoResponse{Outcome: probev1.Outcome_OUTCOME_OK, Payload: req.GetPayload(), Number: req.GetNumber()}, nil
}

type hookServer struct {
	hookv1connect.UnimplementedHookServiceHandler

	o Options
}

// Check implements hookv1connect.HookServiceHandler: it allows everything.
func (s *hookServer) Check(ctx context.Context, _ *hookv1.CheckRequest) (*hookv1.CheckResponse, error) {
	if s.o.Block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.o.Broken {
		return &hookv1.CheckResponse{}, nil
	}
	return &hookv1.CheckResponse{Verdict: hookv1.Verdict_VERDICT_ALLOW}, nil
}

// JSON returns the fake of net/http and encoding/json alone. It knows the Connect protocol only as a POST of JSON to the
// RPC's path, and answers anything but JSON with 415, as such a plugin does.
func JSON(o Options) (http.Handler, *Recorder) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /my.plugin.probe.v1.ProbeService/Echo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "send JSON", http.StatusUnsupportedMediaType)
			return
		}
		// The shape of proto3's JSON mapping: a decimal string for an int64, base64 for bytes, a name for an enum, and
		// fields at their zero value left out.
		var req struct {
			Payload string `json:"payload"`
			Number  string `json:"number"`
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
		payload, err := base64.StdEncoding.DecodeString(req.Payload)
		if err != nil {
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}
		number, err := parseInt(req.Number)
		if err != nil {
			http.Error(w, "bad number", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"outcome": "OUTCOME_OK",
			"payload": base64.StdEncoding.EncodeToString(payload),
			"number":  strconv.FormatInt(number, 10),
			"extra":   "a field the server does not know, which it ignores",
		})
	})
	serveBillingJSON(mux, o)
	serveMailerJSON(mux, o)
	serveTranslatorJSON(mux, o)
	serveKeyManagerJSON(mux, o)
	mux.HandleFunc("POST /my.plugin.hook.v1.HookService/Check", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "send JSON", http.StatusUnsupportedMediaType)
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
		_, _ = w.Write([]byte(`{"verdict":"VERDICT_ALLOW"}`))
	})
	r := new(Recorder)
	return r.wrap(o.Token, mux), r
}

func parseInt(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}
