// SPDX-License-Identifier: AGPL-3.0-only

// Package client is the transport of every plugin adapter (ADR-0146 §3 to §7, §10, §12): the connection, the
// authentication, the protocol, the deadline, the limits, and how failures look to the rest of the server.
//
// An adapter takes a Conn and hands its RPC to the generated client of its kind:
//
//	conn, err := client.New(client.Options{Kind: k, Destination: d, Meter: meter})
//	mailer := mailerv1connect.NewMailerServiceClient(conn.RPC)
//
// The address is the operator's and neither remote servers nor users can influence it, so this does not go through the
// SSRF-guarded kernel.HTTPClient (ADR-0146 §7).
package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"

	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/kernel"
)

// The bounds of a new connection (ADR-0146 §7).
const (
	dialTimeout = 5 * time.Second
	tlsTimeout  = 5 * time.Second
	idleTimeout = 90 * time.Second
)

// Labels of the my.plugin.failures metric (ADR-0146 §15). Both take their values from sets fixed in code: the kinds and
// the Connect codes.
const (
	LabelKind = "my.plugin.kind"
	LabelCode = "my.plugin.code"
)

// Options say which plugin a Conn reaches.
type Options struct {
	Kind        kind.Kind
	Destination kind.Destination
	// Meter counts the failed calls; nil counts nothing.
	Meter kernel.Meter
	// KeepDetail leaves the message of a failure in the error. The server discards it, since a plugin's free text must
	// reach no log (ADR-0146 §10); only `my plugin test` keeps it, to show the person who runs it.
	KeepDetail bool
}

// Conn is one kind's connection to its plugin. It opens connections when a call needs one, not when it is created.
type Conn struct {
	// RPC is what the generated client of the kind takes. Every call through it has the kind's deadline, limits, and
	// concurrency. Errors carry a Connect code, which connect.CodeOf reads, and no text of the plugin's.
	RPC *connect.Client
	// HTTP and BaseURL reach the plugin outside the RPCs, with the same authentication.
	HTTP    *http.Client
	BaseURL string
	// Timeout is the deadline of one call.
	Timeout time.Duration

	transport *http.Transport
}

// New returns the Conn of o. It checks the destination and reads the TLS files, and opens no connection.
func New(o Options) (*Conn, error) {
	if problems := o.Kind.Check(o.Destination); len(problems) > 0 {
		p := problems[0]
		return nil, fmt.Errorf("plugin %s: %s %s", o.Kind.Name, p.Suffix, p.Message)
	}
	addr, err := kind.ParseAddress(o.Destination.Address)
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", o.Kind.Name, err)
	}
	protocol := o.Destination.Protocol
	if protocol == "" {
		protocol = o.Kind.Protocol
	}
	transport, err := newTransport(o.Destination, addr, o.Kind.Streaming || protocol == kind.ProtocolGRPC)
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", o.Kind.Name, err)
	}
	var rt http.RoundTripper = transport
	if o.Destination.Token != "" {
		rt = bearer{next: transport, value: "Bearer " + o.Destination.Token}
	}
	httpClient := &http.Client{
		Transport: rt,
		// An RPC client must not follow a redirect, and the destination is the operator's alone.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		// No Timeout: it would cover a whole stream. Deadlines come from the context.
	}

	opts := []connecthttp.Option{
		connecthttp.WithSendMaxBytes(o.Kind.MaxSend),
		connecthttp.WithReadMaxBytes(o.Kind.MaxReceive),
	}
	switch protocol {
	case kind.ProtocolJSON:
		opts = append(opts, connecthttp.WithProtoJSON())
	case kind.ProtocolGRPC:
		opts = append(opts, connecthttp.WithGRPC())
	}
	timeout := o.Destination.Timeout
	if timeout == 0 {
		timeout = o.Kind.Timeout
	}
	meter := o.Meter
	if meter == nil {
		meter = kernel.NopMeter{}
	}
	calls := &calls{
		kind:       o.Kind.Name,
		timeout:    timeout,
		slots:      make(chan struct{}, o.Kind.Concurrency),
		keepDetail: o.KeepDetail,
		failures: meter.Counter(kernel.Instrument{
			Name:        "my.plugin.failures",
			Unit:        "{call}",
			Description: "Calls of a plugin that failed, by kind and Connect code",
			LabelKeys:   []string{LabelKind, LabelCode},
		}),
		waits: meter.Histogram(kernel.Instrument{
			Name:        "my.plugin.slot_wait",
			Unit:        "s",
			Description: "How long a call of a plugin waited for its turn among the calls of the kind that run at once, by kind",
			LabelKeys:   []string{LabelKind},
			Buckets:     kernel.DurationBuckets,
		}),
	}
	return &Conn{
		RPC:       connect.NewClient(connecthttp.NewTransport(httpClient, addr.BaseURL, opts...), calls.intercept),
		HTTP:      httpClient,
		BaseURL:   addr.BaseURL,
		Timeout:   timeout,
		transport: transport,
	}, nil
}

// Close closes the idle connections. Calls in progress finish.
func (c *Conn) Close() { c.transport.CloseIdleConnections() }

// newTransport returns the transport of a destination. A plain connection speaks HTTP/1.1, which a plugin of net/http
// serves with no further setup, unless the calls need HTTP/2; TLS offers what the calls can use through ALPN.
func newTransport(d kind.Destination, addr kind.Address, needHTTP2 bool) (*http.Transport, error) {
	dialer := &net.Dialer{Timeout: dialTimeout}
	t := &http.Transport{
		Proxy:               nil,
		DialContext:         dialer.DialContext,
		TLSHandshakeTimeout: tlsTimeout,
		MaxConnsPerHost:     kind.DefaultConcurrency,
		MaxIdleConnsPerHost: kind.DefaultConcurrency,
		IdleConnTimeout:     idleTimeout,
		Protocols:           new(http.Protocols),
	}
	switch addr.Scheme {
	case kind.SchemeHTTPS:
		tlsConfig, err := newTLSConfig(d)
		if err != nil {
			return nil, err
		}
		t.TLSClientConfig = tlsConfig
		t.Protocols.SetHTTP2(true)
		t.Protocols.SetHTTP1(!needHTTP2)
	case kind.SchemeUnix:
		t.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", addr.SocketPath)
		}
		fallthrough
	default:
		t.Protocols.SetUnencryptedHTTP2(needHTTP2)
		t.Protocols.SetHTTP1(!needHTTP2)
	}
	return t, nil
}

// newTLSConfig always verifies the server, against the CA file or the system's roots; no setting turns that off
// (ADR-0146 §6). The minimum version is Go's default.
func newTLSConfig(d kind.Destination) (*tls.Config, error) {
	c := new(tls.Config)
	if d.TLSCAFile != "" {
		pem, err := os.ReadFile(d.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read the CA file: %w", err)
		}
		c.RootCAs = x509.NewCertPool()
		if !c.RootCAs.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("the CA file %s holds no certificate", d.TLSCAFile)
		}
	}
	if d.TLSCertFile != "" {
		cert, err := tls.LoadX509KeyPair(d.TLSCertFile, d.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("read the client certificate: %w", err)
		}
		c.Certificates = []tls.Certificate{cert}
	}
	return c, nil
}

// bearer adds the token to every request. kind.Check allows a token only on TLS or a Unix socket.
type bearer struct {
	next  http.RoundTripper
	value string
}

// RoundTrip implements http.RoundTripper.
func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", b.value)
	return b.next.RoundTrip(r)
}

// calls applies a kind's rules to each call.
type calls struct {
	kind       string
	timeout    time.Duration
	slots      chan struct{}
	keepDetail bool
	failures   kernel.Counter
	waits      kernel.Histogram
}

// intercept is the connect.ClientInterceptor of a Conn.
func (c *calls) intercept(next connect.ClientFunc) connect.ClientFunc {
	return func(ctx context.Context, spec connect.Spec) (connect.ClientStream, error) {
		// The deadline runs while the call waits for its turn too: the caller's wait is what it bounds.
		ctx, cancel := context.WithTimeout(ctx, c.timeout)
		start := time.Now()
		select {
		case c.slots <- struct{}{}:
			c.waits.Record(ctx, time.Since(start).Seconds(), kernel.NewLabels(LabelKind, c.kind))
		case <-ctx.Done():
			c.waits.Record(ctx, time.Since(start).Seconds(), kernel.NewLabels(LabelKind, c.kind))
			err := ctx.Err()
			cancel()
			return nil, c.fail(ctx, err)
		}
		s := &stream{calls: c, ctx: ctx}
		s.done = func() {
			cancel()
			<-c.slots
		}
		inner, err := next(ctx, spec)
		if err != nil {
			err = c.fail(ctx, err)
			s.finish()
			return nil, err
		}
		s.ClientStream = inner
		return s, nil
	}
}

// fail counts a failed call and returns the error the server sees.
func (c *calls) fail(ctx context.Context, err error) error {
	code := codeOf(err)
	c.failures.Add(ctx, 1, kernel.NewLabels(LabelKind, c.kind, LabelCode, code.String()))
	return c.mapped(err, code)
}

// mapped returns the error the server sees: the code alone, unless the Conn keeps the detail.
func (c *calls) mapped(err error, code connect.Code) error {
	if c.keepDetail {
		return err
	}
	return connect.NewError(code, "plugin "+c.kind+" call failed")
}

// codeOf returns the Connect code of err. A call that its context abandoned may carry no Connect error.
func codeOf(err error) connect.Code {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return connect.CodeDeadlineExceeded
	case errors.Is(err, context.Canceled):
		return connect.CodeCanceled
	}
	return connect.CodeOf(err)
}

// stream ends a call once: when the response is read to its end, on a failure of the read, or at Close, whichever comes
// first, and so gives back the call's turn and its context.
type stream struct {
	connect.ClientStream
	calls   *calls
	ctx     context.Context //nolint:containedctx // the context of this one call, for its metric
	done    func()
	once    sync.Once
	counted atomic.Bool
}

func (s *stream) finish() { s.once.Do(s.done) }

// failure maps an error of the stream and counts the call as failed once. io.EOF is how a stream ends and no failure.
func (s *stream) failure(err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return err
	}
	if s.counted.CompareAndSwap(false, true) {
		return s.calls.fail(s.ctx, err)
	}
	return s.calls.mapped(err, codeOf(err))
}

// SendHeaders implements connect.ClientStream.
func (s *stream) SendHeaders() error { return s.failure(s.ClientStream.SendHeaders()) }

// Send implements connect.ClientStream.
func (s *stream) Send(msg any) error { return s.failure(s.ClientStream.Send(msg)) }

// CloseSend implements connect.ClientStream.
func (s *stream) CloseSend() error { return s.failure(s.ClientStream.CloseSend()) }

// Receive implements connect.ClientStream.
func (s *stream) Receive(msg any) error {
	err := s.failure(s.ClientStream.Receive(msg))
	if err != nil {
		s.finish()
	}
	return err
}

// Close implements connect.ClientStream.
func (s *stream) Close() error {
	err := s.failure(s.ClientStream.Close())
	s.finish()
	return err
}
