// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_sqs

package config

func init() { exclude("no_sqs", "queue", "sqs") }

// checkSQSQueueURL accepts every value: no adapter of this build reads queue_sqs_queue_url.
func checkSQSQueueURL(string) error { return nil }
