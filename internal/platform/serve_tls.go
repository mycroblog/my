// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
	"github.com/mycroblog/my/internal/platform/tlsserve"
)

// tlsMaxHeaderBytes is the MaxHeaderBytes of the server with tls other than off, which faces
// the internet without a proxy (ADR-0163 §9); Go's default is 1 MiB.
const tlsMaxHeaderBytes = 64 << 10

// tlsMaxConcurrentStreams is the HTTP/2 MaxConcurrentStreams of the server with tls other than
// off (ADR-0163 §9); Go's default is 250.
const tlsMaxConcurrentStreams = 100

// setUpTLS makes the holder of the certificates when this process listens with TLS: the web and
// all roles with tls other than off. Its hosts are host and, when it differs, the host of
// web_origin. With tls = acme it also makes the manager that obtains their certificates, and
// answers the CA's challenges on both listeners (ADR-0163 §3).
func (a *App) setUpTLS(cfg *config.Config, opts AppOptions) error {
	if !cfg.ServesTLS() {
		return nil
	}
	hosts := []string{cfg.Host}
	if u, err := url.Parse(cfg.WebOrigin); err == nil && u.Host != "" {
		hosts = append(hosts, u.Host)
	}
	a.TLSCertificates = tlsserve.NewHolder(hosts...)
	if cfg.TLS != config.TLSACME {
		return nil
	}
	client := opts.ACMEClient
	if client == nil {
		var err error
		if client, err = newACMEClient(cfg, a.Logger); err != nil {
			return err
		}
	}
	// TLS-ALPN-01 needs only the TLS listener; HTTP-01 needs port 80.
	challenges := []string{instance.ACMEChallengeTLSALPN01}
	if cfg.TLSHTTPListen != "" {
		challenges = append(challenges, instance.ACMEChallengeHTTP01)
	}
	m, err := instance.NewTLSManager(instance.TLSManagerOptions{
		Service: a.Instance, Client: client, KV: a.KV, Certificates: a.TLSCertificates, Hosts: a.TLSCertificates.Hosts(),
		Email: cfg.TLSACMEEmail, Profile: cfg.TLSACMEProfile, Challenges: challenges, Clock: a.Clock, Logger: a.Logger,
		CheckInterval: opts.TLSCheckInterval, RetryInterval: opts.TLSRetryInterval,
	})
	if err != nil {
		return err
	}
	a.TLSManager = m
	answers := m.Challenges()
	a.TLSCertificates.SetALPNChallenge(func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return answers.TLSALPN01Certificate(hello.Context(), hello.ServerName)
	})
	a.TLSHTTPChallenge = tlsserve.HTTP01Handler(answers.HTTP01)
	return nil
}

// startACME loads the stored certificates of tls = acme into the holder, so that a restarted
// server serves them at once, and runs the manager once verified closes, that is once the
// listeners are up and the database is known to be this server's. Nothing here waits for an
// order: the first certificates come while the server runs (ADR-0163 §7).
func (a *App) startACME(ctx context.Context, verified <-chan struct{}) {
	if a.TLSManager == nil {
		return
	}
	if err := a.TLSManager.ReloadAll(ctx); err != nil {
		a.Logger.WarnContext(ctx, "the stored TLS certificates are not loaded yet", slog.Any("error", err))
	}
	kernel.Go(ctx, "tls.acme", a.ErrorReporter, func(ctx context.Context) {
		select {
		case <-ctx.Done():
			return
		case <-verified:
		}
		a.TLSManager.Run(ctx)
	})
}

// tlsListener is what Serve runs besides the HTTPS server with tls other than off: the source of
// the certificates and the plain HTTP server of port 80.
type tlsListener struct {
	files      *tlsserve.FileSource
	redirect   *http.Server
	redirectLn net.Listener
}

// startTLS loads the certificates into the holder and opens port 80 (tls_http_listen, or ln).
// It returns nil when the process does not listen with TLS.
func (a *App) startTLS(ctx context.Context, ln net.Listener) (*tlsListener, error) {
	if a.TLSCertificates == nil {
		if ln != nil {
			_ = ln.Close()
		}
		return nil, nil
	}
	t := &tlsListener{}
	if a.Config.TLS == config.TLSFiles {
		t.files = tlsserve.NewFileSource(a.TLSCertificates, a.Config.TLSCertFile, a.Config.TLSKeyFile, a.Logger)
		if err := t.files.Load(); err != nil {
			if ln != nil {
				_ = ln.Close()
			}
			return nil, err
		}
	}
	if ln == nil && a.Config.TLSHTTPListen != "" {
		var err error
		if ln, err = (&net.ListenConfig{}).Listen(ctx, "tcp", a.Config.TLSHTTPListen); err != nil {
			return nil, fmt.Errorf("listen for http: %w", err)
		}
	}
	if ln != nil {
		_, httpsPort, _ := net.SplitHostPort(a.Config.Listen)
		t.redirect = &http.Server{
			Handler:           tlsserve.RedirectHandler(a.TLSCertificates, a.Config.Host, httpsPort, a.TLSHTTPChallenge),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      httpWriteTimeout,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    tlsMaxHeaderBytes,
			ErrorLog:          slog.NewLogLogger(a.Logger.Handler(), slog.LevelWarn),
		}
		t.redirectLn = ln
	}
	return t, nil
}

// configureTLS makes srv serve HTTPS with the certificates of the holder, with the limits of a
// server that faces the internet without a proxy (ADR-0163 §9).
func (a *App) configureTLS(srv *http.Server) {
	srv.TLSConfig = a.TLSCertificates.Config()
	srv.MaxHeaderBytes = tlsMaxHeaderBytes
	srv.HTTP2 = &http.HTTP2Config{MaxConcurrentStreams: tlsMaxConcurrentStreams}
	srv.ErrorLog = quietHandshakeLog(a.Logger)
}

// quietHandshakeLog is the ErrorLog of the HTTPS server: the failed handshakes, which every
// scanner of the internet causes (an unknown server name, a plain HTTP request), go to the debug
// level, and the rest to the warning level.
func quietHandshakeLog(l *slog.Logger) *log.Logger {
	return log.New(logWriter(func(p []byte) {
		msg := strings.TrimSuffix(string(p), "\n")
		level := slog.LevelWarn
		if strings.HasPrefix(msg, "http: TLS handshake error") {
			level = slog.LevelDebug
		}
		l.Log(context.Background(), level, msg)
	}), "", 0)
}

// logWriter is an io.Writer of whole log lines.
type logWriter func(p []byte)

func (w logWriter) Write(p []byte) (int, error) {
	w(p)
	return len(p), nil
}

// run serves port 80 and watches the certificate files every fileCheck until ctx ends; failures
// of port 80 go to serveErr.
func (t *tlsListener) run(ctx context.Context, a *App, fileCheck time.Duration, serveErr chan<- error) {
	if t.files != nil {
		kernel.Go(ctx, "tls.watch_files", a.ErrorReporter, func(ctx context.Context) {
			t.files.Watch(ctx, fileCheck)
		})
	}
	if t.redirect != nil {
		a.Logger.InfoContext(ctx, "redirecting http to https", slog.String("addr", t.redirectLn.Addr().String()))
		kernel.Go(ctx, "http.serve_redirect", a.ErrorReporter, func(context.Context) {
			if err := t.redirect.Serve(t.redirectLn); !errors.Is(err, http.ErrServerClosed) {
				serveErr <- fmt.Errorf("http redirect: %w", err)
			}
		})
	}
}

// close closes port 80 of a server that never started.
func (t *tlsListener) close() {
	if t != nil && t.redirectLn != nil {
		_ = t.redirectLn.Close()
	}
}

// shutdown stops port 80.
func (t *tlsListener) shutdown(ctx context.Context) {
	if t != nil && t.redirect != nil {
		_ = t.redirect.Shutdown(ctx)
	}
}

// plainWithoutProxyMessage is the warning of a server that answers in plain HTTP with nothing in
// front to terminate TLS; `my config check` prints the same one for its listen address.
const plainWithoutProxyMessage = "tls is off and trusted_proxies is empty, and a plain HTTP request came from an address other than loopback: nothing seems to terminate TLS in front of the server; set tls, or put a reverse proxy in front and list it in trusted_proxies"

// warnPlainWithoutProxy logs plainWithoutProxyMessage once, on the first plain HTTP request from
// an address other than loopback, when tls is off and trusted_proxies is empty (ADR-0163 §10).
// Otherwise it returns next as it is.
func (a *App) warnPlainWithoutProxy(next http.Handler) http.Handler {
	if (a.Config.TLS != "" && a.Config.TLS != config.TLSOff) || len(a.Config.TrustedProxies) > 0 {
		return next
	}
	var warned atomic.Bool
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil && !warned.Load() && !loopbackPeer(r.RemoteAddr) && warned.CompareAndSwap(false, true) {
			a.Logger.WarnContext(r.Context(), plainWithoutProxyMessage)
		}
		next.ServeHTTP(w, r)
	})
}

// loopbackPeer reports whether remoteAddr, a host:port, is a loopback address. An address that
// does not parse counts as loopback, so that it never warns by mistake.
func loopbackPeer(remoteAddr string) bool {
	ap, err := netip.ParseAddrPort(remoteAddr)
	if err != nil {
		return true
	}
	return ap.Addr().Unmap().IsLoopback()
}
