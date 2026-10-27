// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_pubsub

package platform

import (
	"context"
	"fmt"
	"log/slog"

	pubsubstream "github.com/mycroblog/my/internal/adapter/stream/pubsub"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/platform/config"
)

// newPubSubBus returns the stream.Bus of stream = pubsub, on the process ID of this process,
// and what releases it after the Relay closed.
func (a *App) newPubSubBus(ctx context.Context, cfg *config.Config, processID string) (stream.Bus, func() error, error) {
	b, err := pubsubstream.New(ctx, pubsubstream.Options{
		Topic: cfg.StreamPubSubTopic, SubscriptionPrefix: cfg.StreamPubSubSubscriptionPrefix, ProcessID: processID,
		Logger: a.Logger.With(slog.String("component", "stream")), Reporter: a.ErrorReporter,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("stream = pubsub: %w", err)
	}
	return b, b.Close, nil
}
