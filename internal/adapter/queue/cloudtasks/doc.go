// SPDX-License-Identifier: AGPL-3.0-only

// Package cloudtasks carries the wake-up signals of the database queue through Google Cloud
// Tasks (`queue = cloudtasks`, ADR-0120): the jobs stay in the database, and Waker creates one
// HTTP-target task per signal, whose delivery to POST /_internal/jobs reaches the push receiver.
// Authenticator checks the OIDC ID token that Cloud Tasks and Cloud Scheduler send with each
// delivery. The client authenticates with Application Default Credentials.
package cloudtasks
