// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_sqs

package platform

import (
	"context"
	"io"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

// newSQSWaker reports that the build tag no_sqs left the SQS queue out.
func newSQSWaker(context.Context, *config.Config, kernel.Clock) (queue.Waker, io.Closer, error) {
	return nil, nil, config.ExcludedError("queue", "sqs", "no_sqs")
}
