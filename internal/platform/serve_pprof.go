// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// PathPprof is the path of the index of the Go runtime's profiles on pprof_listen; each profile
// is below it.
const PathPprof = "/debug/pprof/"

// servePprof listens on pprof_listen (or ln) for the Go runtime's profiles when the setting names
// an address, and returns nil otherwise. The block and mutex profiles sample only while a request
// that names seconds takes them (sampled). The profiles are never served on the public listener:
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
	mux.Handle(PathPprof+"block", sampled("block", func(on bool) { runtime.SetBlockProfileRate(onOff(on, blockProfileRate)) }))
	mux.Handle(PathPprof+"mutex", sampled("mutex", func(on bool) { runtime.SetMutexProfileFraction(onOff(on, mutexProfileFraction)) }))
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

// The sampling of the block and mutex profiles while a request of sampled asks for one: a
// blocking event is sampled about once per 10 µs that goroutines spend blocked, and one mutex
// contention event in 10.
const (
	blockProfileRate     = 10_000
	mutexProfileFraction = 10
)

// sampled serves the block or mutex profile named name. The runtime samples neither unless asked,
// so a request with ?seconds=N turns the sampling on (set(true)) for the N seconds over which it
// takes the profile's difference, and off again after; the requests of one profile take turns.
// Without seconds it serves the profile as net/http/pprof does, which holds nothing unless a
// request with seconds sampled before.
func sampled(name string, set func(on bool)) http.Handler {
	var mu sync.Mutex
	h := pprof.Handler(name)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sec, err := strconv.Atoi(r.FormValue("seconds")); err != nil || sec <= 0 {
			h.ServeHTTP(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		set(true)
		defer set(false)
		h.ServeHTTP(w, r)
	})
}

// onOff returns rate when on, and 0, which turns a sampling off, otherwise.
func onOff(on bool, rate int) int {
	if on {
		return rate
	}
	return 0
}
