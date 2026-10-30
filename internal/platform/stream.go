// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/adapter/telemetry/sysmetrics"
	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/platform/config"
)

// streamsCloseTimeout bounds the close of the streams' broker in App.Close: the signals still
// queued for the Bus and the deletion of this process's listener resources.
const streamsCloseTimeout = 10 * time.Second

// Streams is the broker of the streaming endpoint (FR-207): the timeline and notification commit
// hooks publish to it, and each open stream subscribes (docs/architecture/overview.md §4.4).
type Streams interface {
	stream.Publisher
	stream.Subscriber
	stream.RoomSubscriber
	stream.TimelineSubscriber
	// Len returns the number of open subscriptions of this process.
	Len() int
}

// setUpStreams builds the broker of the streams that cfg.Stream selects (ADR-0050): a
// stream.Hub for hub, which reaches only the streams of this process, and otherwise a
// stream.Relay on the Bus adapter, with the stream.sweep job when the adapter leaves
// per-process resources on the broker. bus, when not nil, replaces the adapter (tests).
func (a *App) setUpStreams(ctx context.Context, cfg *config.Config, bus stream.Bus) error {
	if bus == nil && cfg.Stream == "hub" {
		hub := stream.NewHub(stream.HubOptions{})
		a.Streams = hub
		a.closeStreams = func(context.Context) error { hub.Close(); return nil }
		if cfg.Role != config.RoleAll {
			a.Logger.WarnContext(ctx, "stream = hub delivers to the streams of this process only: with role "+string(cfg.Role)+
				", the changes made in the other processes do not reach its streams; choose a broker across processes (ADR-0050)",
				slog.String("role", string(cfg.Role)))
		}
		return nil
	}

	processID := stream.NewProcessID()
	var closeBus func() error
	measured := bus == nil
	if bus == nil {
		var err error
		if bus, closeBus, err = a.newStreamBus(ctx, cfg, processID); err != nil {
			return err
		}
	}
	o := stream.RelayOptions{ProcessID: processID, Logger: a.Logger.With(slog.String("component", "stream")), Reporter: a.ErrorReporter}
	sweeper, sweeps := bus.(stream.Sweeper)
	if measured {
		// The adapter operations and the listener's connection of ADR-0056 §1.
		bus = sysmetrics.Bus(bus, a.Meter, "stream."+cfg.Stream, a.ErrorReporter)
	}
	if sweeps {
		// The sweep keeps the resources of the processes whose marker is in the shared KV.
		o.Liveness = a.KV
		stream.RegisterSweepJob(a.Handlers, a.Scheduler, sweeper, a.KV, a.Logger)
	}
	relay := stream.NewRelay(bus, o)
	a.Streams = relay
	var once sync.Once
	var closeErr error
	a.closeStreams = func(ctx context.Context) error {
		once.Do(func() {
			closeErr = relay.Close(ctx)
			if closeBus != nil {
				closeErr = errors.Join(closeErr, closeBus())
			}
		})
		return closeErr
	}
	return nil
}

// registerStreamCommitHooks registers the hooks that signal the streams of the timelines, the
// notifications, and the instance after the commits; setUpStreams builds the broker they signal.
func (a *App) registerStreamCommitHooks() {
	timeline.RegisterCommitHooks(a.Projectors, a.Streams)
	notification.RegisterCommitHooks(a.Projectors, a.Streams)
	instance.RegisterCommitHooks(a.Projectors, a.Streams)
}

// newStreamBus returns the Bus adapter of cfg.Stream, on the process ID of this process, and
// what releases it after the Relay closed (nil when nothing does).
func (a *App) newStreamBus(ctx context.Context, cfg *config.Config, processID string) (stream.Bus, func() error, error) {
	switch cfg.Stream {
	case "postgres":
		if a.newPostgresBus == nil {
			return nil, nil, errors.New("stream = postgres requires db = postgres")
		}
		b, err := a.newPostgresBus(cfg.StreamPostgresURL)
		return b, nil, err
	case "valkey":
		b, err := a.newValkeyBus(ctx, cfg)
		return b, nil, err
	case "sns":
		b, err := a.newSNSBus(ctx, cfg, processID)
		return b, nil, err
	case "pubsub":
		return a.newPubSubBus(ctx, cfg, processID)
	default:
		return nil, nil, fmt.Errorf("stream %q is not implemented yet", cfg.Stream)
	}
}

// shutDownStreams closes the broker of the streams within ctx: it ends the open streams, sends
// the signals still queued for the other processes, and closes this process's listener. It
// does nothing after the first call.
func (a *App) shutDownStreams(ctx context.Context) {
	if a.closeStreams == nil {
		return
	}
	if err := a.closeStreams(ctx); err != nil {
		a.Logger.WarnContext(ctx, "the streams' broker did not close cleanly", slog.Any("error", err))
	}
}
