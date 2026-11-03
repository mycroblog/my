// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_sns

package config

func init() { exclude("no_sns", "stream", "sns") }

// checkSNSTopicARN accepts every value: no adapter of this build reads stream_sns_topic_arn.
func checkSNSTopicARN(string) error { return nil }

// checkSQSQueuePrefix accepts every value: no adapter of this build reads stream_sns_queue_prefix.
func checkSQSQueuePrefix(string) error { return nil }
