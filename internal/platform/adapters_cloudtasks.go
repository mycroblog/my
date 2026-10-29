// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_cloudtasks

package platform

import (
	"context"
	"fmt"
	"io"

	"github.com/mycroblog/my/internal/adapter/oidc"
	"github.com/mycroblog/my/internal/adapter/queue/cloudtasks"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/interface/queuepush"
	"github.com/mycroblog/my/internal/platform/config"
)

// newCloudTasksWaker returns the waker of queue = cloudtasks on Application Default Credentials,
// whose tasks call the jobs endpoint of queue_cloudtasks_url, and what releases its client.
func newCloudTasksWaker(ctx context.Context, cfg *config.Config, clock kernel.Clock, reporter kernel.ErrorReporter) (queue.Waker, io.Closer, error) {
	w, err := cloudtasks.New(ctx, cloudtasks.Options{
		Queue: cfg.QueueCloudTasksQueue, URL: cfg.QueueCloudTasksURL + queuepush.PathJobs,
		ServiceAccount: cfg.QueueCloudTasksServiceAccount, Audience: cfg.QueueCloudTasksAudience,
		DispatchDeadline: cfg.QueueCloudTasksDispatchDeadline, Clock: clock, Reporter: reporter,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("queue = cloudtasks: %w", err)
	}
	return w, w, nil
}

// newCloudTasksAuthenticator returns the check of the OIDC ID tokens that Cloud Tasks and Cloud
// Scheduler send, which fetches Google's JWKS through h.
func newCloudTasksAuthenticator(cfg *config.Config, h kernel.HTTPClient, clock kernel.Clock) queue.PushAuthenticator {
	return cloudtasks.NewAuthenticator(cloudtasks.AuthOptions{
		OIDC:           oidc.New(oidc.Options{HTTP: h, Clock: clock}),
		ServiceAccount: cfg.QueueCloudTasksServiceAccount, Audience: cfg.QueueCloudTasksAudience,
	})
}
