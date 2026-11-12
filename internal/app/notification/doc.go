// SPDX-License-Identifier: AGPL-3.0-only

// Package notification holds the use cases and port definitions of the notification module: creation and retrieval of notifications and Web Push subscriptions.
// It is the module's public API; other modules reach the notification module only through this package.
//
// Whether a viewer muted the thread of a status (Status.muted of the client API) is read for a
// whole page with two reads: the conversations of the statuses, from the content module
// (ConversationsOf), and the viewer's mutes of those conversations.
package notification
