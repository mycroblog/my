// SPDX-License-Identifier: AGPL-3.0-only

// Package account holds the use cases and port definitions of the account module: local and remote accounts (actors), their profiles, and their keys.
// It is the module's public API; other modules reach the account module only through this package.
//
// A remote profile gets a new revision only when the fetched document changes it, its set of
// custom emoji included. Its HTML is sanitized when it is taken in, its names are kept as plain
// text, and its emoji are recorded through the content module (Deps.RecordRemoteEmojis), as
// those of a remote status are. The counts that the snapshot of a remote actor carries
// (totalItems) are what a remote account shows; the counters of this server count only what it
// knows of, and are kept apart.
//
// A move and its cancellation emit EventAccountMoved and EventMoveCanceled in their
// transactions. The projector of the module points the account's move state at the move, or
// removes it; moderation tells whether the source is limited, notification tells the moderators,
// graph queues the migration of the follows, and federation sends the Update of the actor,
// without movedTo after a cancellation.
package account
