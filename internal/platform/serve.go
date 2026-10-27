// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/platform/config"
)

// DefaultShutdownTimeout is how long Serve waits for in-flight requests and jobs after ctx
// ends. It fits in the 30-second grace period that Kubernetes and ECS give after SIGTERM.
const DefaultShutdownTimeout = 25 * time.Second

// ServeOptions configure Serve.
type ServeOptions struct {
	// Listener, when set, is used instead of listening on the configured address.
	Listener net.Listener
	// MetricsListener, when set, is used instead of listening on metrics_listen.
	MetricsListener net.Listener
	// ShutdownTimeout bounds the graceful shutdown; the default is DefaultShutdownTimeout.
	ShutdownTimeout time.Duration
}

// Serve runs the App in its configured role until ctx ends, then shuts down gracefully:
//
//   - every role serves HTTP on the listen address (the worker role only the health checks);
//   - with metrics_exporter = prometheus, every role serves GET /metrics on metrics_listen;
//   - the worker and all roles run the job runner and the scheduler of periodic jobs;
//   - every role first ensures the built-in roles, giving admin every permission.
//
// When ctx ends or a goroutine of kernel.Go panics, Serve stops accepting connections and claiming jobs, and waits up to
// ShutdownTimeout for the requests and the job in progress to finish. After the timeout it
// cancels their contexts, closes the remaining connections, and waits for the job runner to
// return. The open streams end when the shutdown starts; the broker of the streams closes then
// in the web and all roles, and after the jobs in the worker role. Serve returns nil after a graceful shutdown and an error when it cannot listen,
// the server fails, a goroutine panicked, or the timeout was hit.
func Serve(ctx context.Context, a *App, opts ServeOptions) error {
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = DefaultShutdownTimeout
	}
	ln := opts.Listener
	if ln == nil {
		var err error
		if ln, err = (&net.ListenConfig{}).Listen(ctx, "tcp", a.Config.Listen); err != nil {
			return fmt.Errorf("listen: %w", err)
		}
	}

	// Requests and jobs run under hardCtx, which outlives ctx by the shutdown timeout.
	hardCtx, hardStop := context.WithCancel(context.WithoutCancel(ctx))
	defer hardStop()

	srv := &http.Server{
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(a.Logger.Handler(), slog.LevelWarn),
		BaseContext:       func(net.Listener) context.Context { return hardCtx },
	}
	// Open streams never finish on their own: end them when the shutdown starts, so that it
	// waits only for the other requests. A worker has no streams and keeps sending the signals
	// of its jobs to the other processes until the jobs stop.
	if a.Config.Role != config.RoleWorker {
		srv.RegisterOnShutdown(func() {
			closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opts.ShutdownTimeout)
			defer cancel()
			a.shutDownStreams(closeCtx)
		})
	}
	// A new version may add permissions that the built-in admin role must hold (ADR-0058 §4).
	// Every role does this, so an upgrade needs no manual step whichever process starts first;
	// the transaction is short and idempotent, and concurrent starts converge on the same rows.
	if err := a.Identity.EnsureBuiltinRoles(ctx); err != nil {
		a.Logger.WarnContext(ctx, "built-in roles not ensured yet", slog.Any("error", err))
	}
	if a.Config.Role != config.RoleWorker {
		// The authorization server also registers the web client on its first request, so a
		// schema applied after the start only delays this.
		if err := a.Identity.EnsureWebClient(ctx); err != nil {
			a.Logger.WarnContext(ctx, "web client not registered yet", slog.Any("error", err))
		}
	}
	metricsSrv, err := a.serveMetrics(ctx, opts.MetricsListener)
	if err != nil {
		_ = ln.Close()
		return err
	}
	serveErr := make(chan error, 2)
	// The panic of a goroutine of kernel.Go stops the server as a failure (ADR-0103 §5).
	fatal := make(chan error, 1)
	defer a.ErrorReporter.setStop(func(cause error) {
		select {
		case fatal <- cause:
		default: // a shutdown has already been asked for
		}
	})()
	go func() { serveErr <- srv.Serve(ln) }()
	if metricsSrv != nil {
		go func() {
			if err := metricsSrv.srv.Serve(metricsSrv.ln); !errors.Is(err, http.ErrServerClosed) {
				serveErr <- fmt.Errorf("metrics: %w", err)
			}
		}()
	}

	var jobs sync.WaitGroup
	stopJobs := make(chan struct{})
	if a.Config.Role != config.RoleWeb {
		jobs.Go(func() { _ = a.runJobs(hardCtx, stopJobs) })
		if a.Scheduler != nil {
			jobs.Go(func() { a.Scheduler.RunUntil(hardCtx, stopJobs, 0) })
		}
	}
	a.Logger.InfoContext(ctx, "started", slog.String("role", string(a.Config.Role)), slog.String("addr", ln.Addr().String()))

	select {
	case <-ctx.Done():
	case err = <-serveErr:
		err = fmt.Errorf("http server: %w", err)
	case err = <-fatal:
	}
	a.Logger.InfoContext(ctx, "shutting down", slog.Duration("timeout", opts.ShutdownTimeout))
	close(stopJobs)

	graceCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opts.ShutdownTimeout)
	defer cancel()
	if shutdownErr := srv.Shutdown(graceCtx); shutdownErr != nil {
		err = errors.Join(err, fmt.Errorf("http server shutdown: %w", shutdownErr))
	}
	if metricsSrv != nil {
		// A scrape is short; the metrics stay available while the requests finish.
		_ = metricsSrv.srv.Shutdown(graceCtx)
	}
	jobsDone := make(chan struct{})
	go func() { jobs.Wait(); close(jobsDone) }()
	select {
	case <-jobsDone:
	case <-graceCtx.Done():
		err = errors.Join(err, errors.New("job runner did not stop within the shutdown timeout"))
	}
	hardStop()
	_ = srv.Close()
	<-jobsDone
	a.shutDownStreams(graceCtx)
	if err == nil {
		a.Logger.InfoContext(ctx, "stopped")
	}
	return err
}

// metricsServer is the listener of the Prometheus scrape.
type metricsServer struct {
	srv *http.Server
	ln  net.Listener
}

// serveMetrics listens on metrics_listen (or ln) for GET /metrics when the App has a scrape, and
// returns nil otherwise. The metrics are never served on the public listener (ADR-0055 §2).
func (a *App) serveMetrics(ctx context.Context, ln net.Listener) (*metricsServer, error) {
	if a.metricsHandler == nil {
		if ln != nil {
			_ = ln.Close()
		}
		return nil, nil
	}
	if ln == nil {
		var err error
		if ln, err = (&net.ListenConfig{}).Listen(ctx, "tcp", a.Config.MetricsListen); err != nil {
			return nil, fmt.Errorf("listen for metrics: %w", err)
		}
	}
	mux := http.NewServeMux()
	mux.Handle("GET "+PathMetrics, a.metricsHandler)
	a.Logger.InfoContext(ctx, "serving metrics", slog.String("addr", ln.Addr().String()))
	return &metricsServer{
		srv: &http.Server{
			Handler: mux, ReadHeaderTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second,
			ErrorLog: slog.NewLogLogger(a.Logger.Handler(), slog.LevelWarn),
		},
		ln: ln,
	}, nil
}
