// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_cloudtasks

package config

import "github.com/mycroblog/my/internal/adapter/queue/cloudtasks"

func checkCloudTasksQueue(v string) error { return cloudtasks.CheckQueuePath(v) }
