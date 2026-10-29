// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_cloudtasks

package config

func init() { exclude("no_cloudtasks", "queue", "cloudtasks") }

// checkCloudTasksQueue accepts every value: no adapter of this build reads queue_cloudtasks_queue.
func checkCloudTasksQueue(string) error { return nil }
