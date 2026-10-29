// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqs

package platform

import (
	"context"
	"fmt"
	"io"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	sqsqueue "github.com/mycroblog/my/internal/adapter/queue/sqs"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

// newSQSWaker returns the waker of queue = sqs, which sends the signals to the standard queue of
// queue_sqs_queue_url; its client holds nothing to release.
func newSQSWaker(ctx context.Context, cfg *config.Config, clock kernel.Clock) (queue.Waker, io.Closer, error) {
	// The credentials and the HTTP client come from the SDK's default chain: the execution
	// role, the environment, the shared files. The SDK reaches only the AWS endpoints, so it
	// does not go through the SSRF-guarded kernel.HTTPClient, whose guard is for the
	// destinations that others choose.
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("queue = sqs: load the AWS configuration: %w", err)
	}
	w, err := sqsqueue.NewFromConfig(awsCfg, sqsqueue.Options{QueueURL: cfg.QueueSQSQueueURL, Clock: clock})
	if err != nil {
		return nil, nil, fmt.Errorf("queue = sqs: %w", err)
	}
	return w, nil, nil
}
