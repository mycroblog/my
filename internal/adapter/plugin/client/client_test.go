// SPDX-License-Identifier: AGPL-3.0-only

package client_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect/v2"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	probev1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/probe/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/probe/v1/probev1connect"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/pkg/testkit"
)

func probeKind(t *testing.T) kind.Kind {
	t.Helper()
	k, ok := kind.ByName(kind.Probe)
	if !ok {
		t.Fatal("no probe kind")
	}
	return k
}

func call(t *testing.T, conn *client.Conn) (*probev1.EchoResponse, error) {
	t.Helper()
	return probev1connect.NewProbeServiceClient(conn.RPC).Echo(t.Context(), &probev1.EchoRequest{Payload: []byte{0, 255}, Number: 9007199254740993})
}

func wantEcho(t *testing.T, conn *client.Conn) {
	t.Helper()
	res, err := call(t, conn)
	if err != nil {
		t.Fatalf("Echo: %v", err)
	}
	if res.GetOutcome() != probev1.Outcome_OUTCOME_OK || string(res.GetPayload()) != "\x00\xff" || res.GetNumber() != 9007199254740993 {
		t.Fatalf("Echo = %v", res)
	}
}

func newConn(t *testing.T, o client.Options) *client.Conn {
	t.Helper()
	conn, err := client.New(o)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(conn.Close)
	return conn
}

// Every transport and authentication method, with every protocol that can reach the fake.
func TestCallsOverEveryTransportAndAuthentication(t *testing.T) {
	k := probeKind(t)
	const token = "s3cret"
	clientTLS := fakeplugin.NewTLSFiles(t, true)
	serverOnlyTLS := fakeplugin.NewTLSFiles(t, false)

	for _, protocol := range []string{kind.ProtocolJSON, kind.ProtocolProto, kind.ProtocolGRPC} {
		t.Run(protocol, func(t *testing.T) {
			open, _ := fakeplugin.Connect(fakeplugin.Options{})
			guarded, _ := fakeplugin.Connect(fakeplugin.Options{Token: token})
			cases := []struct {
				name string
				d    kind.Destination
			}{
				{"unix", kind.Destination{Address: fakeplugin.ServeUnix(t, open)}},
				{"unix with a token", kind.Destination{Address: fakeplugin.ServeUnix(t, guarded), Token: token}},
				{"plain tcp", kind.Destination{Address: fakeplugin.ServeTCP(t, open)}},
				{
					"tls with a token",
					kind.Destination{Address: fakeplugin.ServeTLS(t, guarded, serverOnlyTLS.Server), Token: token, TLSCAFile: serverOnlyTLS.CAFile},
				},
				{
					"mtls",
					kind.Destination{Address: fakeplugin.ServeTLS(t, open, clientTLS.Server), TLSCAFile: clientTLS.CAFile, TLSCertFile: clientTLS.CertFile, TLSKeyFile: clientTLS.KeyFile},
				},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					tc.d.Protocol = protocol
					wantEcho(t, newConn(t, client.Options{Kind: k, Destination: tc.d}))
				})
			}
		})
	}
}

func TestAPluginOfNetHTTPAndJSONAlone(t *testing.T) {
	k := probeKind(t)
	h, rec := fakeplugin.JSON(fakeplugin.Options{})
	for _, addr := range []string{fakeplugin.ServeUnix(t, h), fakeplugin.ServeTCP(t, h)} {
		wantEcho(t, newConn(t, client.Options{Kind: k, Destination: kind.Destination{Address: addr}}))
		if rec.Proto() != "HTTP/1.1" {
			t.Errorf("%s: proto = %q, want HTTP/1.1, which a plugin of net/http takes with no setup", addr, rec.Proto())
		}
	}
	// Binary Connect does not reach it: a configuration mistake that the report of my plugin test shows.
	conn := newConn(t, client.Options{Kind: k, Destination: kind.Destination{Address: fakeplugin.ServeTCP(t, h), Protocol: kind.ProtocolProto}})
	if _, err := call(t, conn); err == nil {
		t.Error("proto reached a plugin of JSON alone")
	}
}

// A kind that streams speaks HTTP/2 on every transport, since Connect streams both ways only there (ADR-0146 §13).
func TestAStreamingKindSpeaksHTTP2(t *testing.T) {
	k, _ := kind.ByName(kind.MediaVideo)
	files := fakeplugin.NewTLSFiles(t, false)
	h, rec := fakeplugin.Connect(fakeplugin.Options{})
	for _, d := range []kind.Destination{
		{Address: fakeplugin.ServeUnix(t, h)},
		{Address: fakeplugin.ServeTCP(t, h)},
		{Address: fakeplugin.ServeTLS(t, h, files.Server), TLSCAFile: files.CAFile},
	} {
		wantEcho(t, newConn(t, client.Options{Kind: k, Destination: d}))
		if rec.Proto() != "HTTP/2.0" {
			t.Errorf("%s: proto = %q", d.Address, rec.Proto())
		}
	}
}

func TestNewRefusesWhatTheKindRefuses(t *testing.T) {
	mailer, _ := kind.ByName(kind.Mailer)
	for _, addr := range []string{"http://127.0.0.1:1", "http://mailer.internal"} {
		if _, err := client.New(client.Options{Kind: mailer, Destination: kind.Destination{Address: addr}}); err == nil {
			t.Errorf("mailer at %s: no error", addr)
		}
	}
	k := probeKind(t)
	if _, err := client.New(client.Options{Kind: k, Destination: kind.Destination{Address: "http://127.0.0.1:1", Token: "t"}}); err == nil {
		t.Error("token over http: no error")
	}
	if _, err := client.New(client.Options{Kind: k, Destination: kind.Destination{Address: "https://p", TLSCAFile: "/does/not/exist"}}); err == nil {
		t.Error("missing CA file: no error")
	}
}

func TestTheServerCertificateIsAlwaysVerified(t *testing.T) {
	files := fakeplugin.NewTLSFiles(t, false)
	h, rec := fakeplugin.Connect(fakeplugin.Options{})
	addr := fakeplugin.ServeTLS(t, h, files.Server)
	conn := newConn(t, client.Options{Kind: probeKind(t), Destination: kind.Destination{Address: addr}})
	if _, err := call(t, conn); err == nil {
		t.Error("a certificate outside the system's roots passed")
	}
	if rec.Calls() != 0 {
		t.Errorf("calls = %d", rec.Calls())
	}
}

func TestNoConnectionUntilACall(t *testing.T) {
	h, rec := fakeplugin.Connect(fakeplugin.Options{})
	newConn(t, client.Options{Kind: probeKind(t), Destination: kind.Destination{Address: fakeplugin.ServeTCP(t, h)}})
	if rec.Calls() != 0 {
		t.Errorf("calls = %d", rec.Calls())
	}
}

func TestADeadlineFromTheKindOrTheDestination(t *testing.T) {
	h, _ := fakeplugin.Connect(fakeplugin.Options{Block: true})
	conn := newConn(t, client.Options{Kind: probeKind(t), Destination: kind.Destination{Address: fakeplugin.ServeTCP(t, h), Timeout: 50 * time.Millisecond}})
	start := time.Now()
	_, err := call(t, conn)
	if connect.CodeOf(err) != connect.CodeDeadlineExceeded {
		t.Errorf("err = %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s", d)
	}
}

func TestLimitsOfAMessage(t *testing.T) {
	k := probeKind(t)
	k.MaxSend = 16
	h, rec := fakeplugin.Connect(fakeplugin.Options{})
	conn := newConn(t, client.Options{Kind: k, Destination: kind.Destination{Address: fakeplugin.ServeTCP(t, h), Protocol: kind.ProtocolProto}})
	_, err := probev1connect.NewProbeServiceClient(conn.RPC).Echo(t.Context(), &probev1.EchoRequest{Payload: make([]byte, 64)})
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Errorf("sending too much: %v", err)
	}
	if rec.Calls() > 1 {
		t.Errorf("calls = %d", rec.Calls())
	}

	k = probeKind(t)
	k.MaxReceive = 16
	conn = newConn(t, client.Options{Kind: k, Destination: kind.Destination{Address: fakeplugin.ServeTCP(t, h), Protocol: kind.ProtocolProto}})
	_, err = probev1connect.NewProbeServiceClient(conn.RPC).Echo(t.Context(), &probev1.EchoRequest{Payload: make([]byte, 64)})
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Errorf("receiving too much: %v", err)
	}
}

// The turn of a call is bounded, whether it succeeds or fails.
func TestConcurrencyOfAKind(t *testing.T) {
	k := probeKind(t)
	k.Concurrency = 1
	var active, most atomic.Int64
	inner, _ := fakeplugin.Connect(fakeplugin.Options{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			m := most.Load()
			if n <= m || most.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		inner.ServeHTTP(w, r)
	})
	conn := newConn(t, client.Options{Kind: k, Destination: kind.Destination{Address: fakeplugin.ServeTCP(t, h)}})
	errs := make(chan error, 8)
	for range 8 {
		go func() {
			_, err := probev1connect.NewProbeServiceClient(conn.RPC).Echo(context.WithoutCancel(t.Context()), &probev1.EchoRequest{})
			errs <- err
		}()
	}
	for range 8 {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
	if most.Load() != 1 {
		t.Errorf("at most %d at once, want 1", most.Load())
	}

	// A failed call gives its turn back.
	dead := newConn(t, client.Options{Kind: k, Destination: kind.Destination{Address: "unix:///does/not/exist.sock", Timeout: time.Second}})
	for range 3 {
		if _, err := call(t, dead); err == nil {
			t.Fatal("reached a socket that does not exist")
		}
	}
	if _, err := call(t, dead); connect.CodeOf(err) == connect.CodeDeadlineExceeded {
		t.Errorf("a failed call kept its turn: %v", err)
	}
}

// The text of a plugin reaches no error of the server's, and the metric has the kind and the code (ADR-0146 §10, §15).
// How long a call waited for its turn is a metric of the kind.
func TestTheWaitForATurnIsMeasured(t *testing.T) {
	meter := testkit.NewMeter()
	inner, _ := fakeplugin.Connect(fakeplugin.Options{})
	conn := newConn(t, client.Options{Kind: probeKind(t), Destination: kind.Destination{Address: fakeplugin.ServeTCP(t, inner)}, Meter: meter})
	if _, err := call(t, conn); err != nil {
		t.Fatal(err)
	}
	if got := meter.Measurements("my.plugin.slot_wait", map[string]string{client.LabelKind: kind.Probe}); len(got) != 1 {
		t.Errorf("slot_wait records = %v, want 1", got)
	}
}

func TestAFailureCarriesTheCodeAlone(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"unavailable","message":"secret recipient alice@example.org"}`))
	})
	addr := fakeplugin.ServeTCP(t, h)
	meter := testkit.NewMeter()
	conn := newConn(t, client.Options{Kind: probeKind(t), Destination: kind.Destination{Address: addr}, Meter: meter})
	_, err := call(t, conn)
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "alice") {
		t.Errorf("the plugin's text is in the error: %v", err)
	}
	if got := meter.Sum("my.plugin.failures", map[string]string{client.LabelKind: kind.Probe, client.LabelCode: "unavailable"}); got != 1 {
		t.Errorf("failures = %v", got)
	}

	conn = newConn(t, client.Options{Kind: probeKind(t), Destination: kind.Destination{Address: addr}, KeepDetail: true})
	_, err = call(t, conn)
	var ce *connect.Error
	if !errors.As(err, &ce) || !strings.Contains(ce.Message(), "alice") {
		t.Errorf("KeepDetail: err = %v", err)
	}
}

func TestAnEmptyResponseIsNotASuccess(t *testing.T) {
	h, _ := fakeplugin.JSON(fakeplugin.Options{Broken: true})
	res, err := call(t, newConn(t, client.Options{Kind: probeKind(t), Destination: kind.Destination{Address: fakeplugin.ServeTCP(t, h)}}))
	if err != nil {
		t.Fatal(err)
	}
	if res.GetOutcome() != probev1.Outcome_OUTCOME_UNSPECIFIED {
		t.Errorf("outcome = %v", res.GetOutcome())
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	var target atomic.Int64
	after := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { target.Add(1) })
	to := fakeplugin.ServeTCP(t, after)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, to+"/elsewhere", http.StatusTemporaryRedirect)
	})
	conn := newConn(t, client.Options{Kind: probeKind(t), Destination: kind.Destination{Address: fakeplugin.ServeTCP(t, h)}})
	if _, err := call(t, conn); err == nil {
		t.Error("a redirect passed")
	}
	if target.Load() != 0 {
		t.Error("the redirect was followed")
	}
}
