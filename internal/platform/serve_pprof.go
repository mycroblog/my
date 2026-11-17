// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"time"
)

// PathPprof is the path of the index of the Go runtime's profiles on pprof_listen; each profile
// is below it.
const PathPprof = "/debug/pprof/"

// servePprof listens on pprof_listen (or ln) for the Go runtime's profiles when the setting names
// an address, and returns nil otherwise. The profiles are never served on the public listener:
// the handlers are on a mux of their own, and nothing serves http.DefaultServeMux, where the
// import of net/http/pprof also registers them.
func (a *App) servePprof(ctx context.Context, ln net.Listener) (*sideServer, error) {
	if a.Config.PprofListen == "" {
		if ln != nil {
			_ = ln.Close()
		}
		return nil, nil
	}
	if ln == nil {
		var err error
		if ln, err = (&net.ListenConfig{}).Listen(ctx, "tcp", a.Config.PprofListen); err != nil {
			return nil, fmt.Errorf("listen for pprof: %w", err)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc(PathPprof, pprof.Index)
	mux.HandleFunc(PathPprof+"cmdline", pprof.Cmdline)
	mux.HandleFunc(PathPprof+"profile", pprof.Profile)
	mux.HandleFunc(PathPprof+"symbol", pprof.Symbol)
	mux.HandleFunc(PathPprof+"trace", pprof.Trace)
	a.Logger.InfoContext(ctx, "serving profiles", slog.String("addr", ln.Addr().String()))
	return &sideServer{
		// No WriteTimeout: a CPU profile or a trace lasts as long as its request asks.
		srv: &http.Server{
			Handler: mux, ReadHeaderTimeout: 10 * time.Second,
			ErrorLog: slog.NewLogLogger(a.Logger.Handler(), slog.LevelWarn),
		},
		ln: ln,
	}, nil
}
