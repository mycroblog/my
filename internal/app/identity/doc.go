// SPDX-License-Identifier: AGPL-3.0-only

// Package identity holds the use cases and port definitions of the identity module: user registration, authentication, sessions, OAuth applications and access tokens, and two-factor authentication.
// It is the module's public API; other modules reach the identity module only through this package.
//
// Registration is the one command that writes two modules: so that a user never exists without
// the account it acts as, nor an account without its user, it calls the account module's
// PrepareLocalAccount and WriteLocalAccount inside its own transaction, and the account tables
// stay written by the account module's code alone. The keys and the password hash are computed
// before the transaction. The email that verifies the address is a job, which writes its token
// before it sends.
//
// What lives only minutes is not in the database: the TOTP steps already used and the challenge
// of a sign-in that waits for its second step are in the KeyValueStore namespace two_factor; the
// state of a passkey ceremony is sealed into the browser, and the KeyValueStore namespace passkey
// records a used sign-in or confirmation challenge until it expires, so that it works once.
//
// Permissions are read from the user's roles on every request, so a grant or a revocation applies
// from the next one, and the moderators that a notification goes to are those of the moment it is
// created. A reset link reaches the user by email from the sign-in screen when the server sends
// email; administrators can issue one on any server, from the admin screen or the CLI, and the
// audit log keeps it.
//
// The search of the accounts administration joins no tables across modules: each condition asks
// the module that owns it for account IDs newest first by keyset (identity for the email
// addresses, the IP addresses, and the registration states; moderation for the restrictions;
// account for the lifecycle, the handles, and the rest), and AccountAdmin checks the other
// conditions account by account. A page reads at most 400 accounts and returns where it stopped as
// the cursor, so a short or empty page may have more after it.
//
// The archive of a user's data holds the actor, the outbox of every undeleted status of theirs
// whatever its visibility, the moderation records about them without private notes or
// reporters, their keyword filters, their notes on accounts, and the CSV files of every
// ImportKind, which the CSV export writes from the data of the moment and Mastodon reads.
package identity
