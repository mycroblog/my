// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/platform"
)

// panickingBus is a stream.Bus whose Listen panics, as a broken adapter would.
type panickingBus struct{}

func (panickingBus) Send(context.Context, []byte) error { return nil }

func (panickingBus) Listen(context.Context, func([]byte)) (stream.Listener, error) {
	panic("listen broke")
}

// TestServeStopsOnStreamListenerPanic sees a panic in the goroutine of kernel.Go that starts
// the streams' listener reported to the App's sinks under its name and stop Serve.
func TestServeStopsOnStreamListenerPanic(t *testing.T) {
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_QUEUE": "inline"})
	sink := &errorSink{}
	s := startApp(t, cfg, platform.AppOptions{StreamBus: panickingBus{}}, platform.ServeOptions{ShutdownTimeout: 2 * time.Second}, func(a *platform.App) {
		a.ErrorReporter.AddSink(sink)
	})
	waitServing(t, s)

	// The first subscription starts the listener; it fails once Serve has stopped.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if sub, err := s.app.Streams.Subscribe(ctx, "account"); err == nil {
			sub.Close()
		}
	}()
	select {
	case err := <-s.done:
		s.done <- err // for the cleanup of startApp
		if err == nil || !strings.Contains(err.Error(), "stream.relay.listen") {
			t.Errorf("Serve = %v, want the listener's panic", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not stop")
	}
	evs, _ := sink.got()
	if len(evs) != 1 || evs[0].Where != "stream.relay.listen" || !evs[0].Fatal || evs[0].Message != "listen broke" {
		t.Errorf("events = %+v", evs)
	}
}
