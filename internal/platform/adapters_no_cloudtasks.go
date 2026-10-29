// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_cloudtasks

package platform

import (
	"context"
	"io"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

// newCloudTasksWaker reports that the build tag no_cloudtasks left the Cloud Tasks queue out.
func newCloudTasksWaker(context.Context, *config.Config, kernel.Clock, kernel.ErrorReporter) (queue.Waker, io.Closer, error) {
	return nil, nil, config.ExcludedError("queue", "cloudtasks", "no_cloudtasks")
}

// newCloudTasksAuthenticator is never reached: without a waker there are no push endpoints.
func newCloudTasksAuthenticator(*config.Config, kernel.HTTPClient, kernel.Clock) queue.PushAuthenticator {
	return nil
}
