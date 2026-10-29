// SPDX-License-Identifier: AGPL-3.0-only

// Package sqs carries the wake-up signals of the database queue through an Amazon SQS standard
// queue (`queue = sqs`, ADR-0120): the jobs stay in the database, and Waker sends one message
// per signal, whose body is the job ID and whose DelaySeconds is the time until the signal, at
// most 15 minutes. Several signals go in SendMessageBatch calls of at most 10 messages. No
// process polls the queue (ADR-0120 §6). The client takes its credentials from the SDK's
// default chain.
package sqs
