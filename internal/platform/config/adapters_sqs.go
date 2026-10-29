// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqs

package config

import "github.com/mycroblog/my/internal/adapter/queue/sqs"

func checkSQSQueueURL(v string) error { return sqs.CheckQueueURL(v) }
