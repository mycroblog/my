// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sns

package platform

import (
	"context"
	"fmt"
	"log/slog"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	snsstream "github.com/mycroblog/my/internal/adapter/stream/sns"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/platform/config"
)

// newSNSBus returns the stream.Bus of stream = sns, on the process ID of this process.
func (a *App) newSNSBus(ctx context.Context, cfg *config.Config, processID string) (stream.Bus, error) {
	// The credentials and the HTTP client come from the SDK's default chain: the execution
	// role, the environment, the shared files. The SDK reaches only the AWS endpoints, so it
	// does not go through the SSRF-guarded kernel.HTTPClient, whose guard is for the
	// destinations that others choose.
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("stream = sns: load the AWS configuration: %w", err)
	}
	return snsstream.NewFromConfig(awsCfg, snsstream.Options{
		TopicARN: cfg.StreamSNSTopicARN, QueuePrefix: cfg.StreamSQSQueuePrefix, ProcessID: processID,
		Logger: a.Logger.With(slog.String("component", "stream")), Reporter: a.ErrorReporter,
	})
}
