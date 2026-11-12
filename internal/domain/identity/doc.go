// SPDX-License-Identifier: AGPL-3.0-only

// Package identity is the pure domain model of the identity module: user registration, authentication, sessions, OAuth applications and access tokens, and two-factor authentication.
// It holds entities, value objects, and rules, and imports only the standard library.
//
// A user is the principal that signs in, and holds nothing but its identity; the local account
// that it acts as points at it. Around it, its credentials (passwords, TOTP, passkeys, linked
// identities of login providers), its email address, the review of its registration, its roles,
// and its preferences are separate concepts, each with its own events and projections, so that
// each axis of its state changes on its own (ADR-0020 rule 2). A user has one email address,
// added at sign-up or by an administrator and never changed; its delivery failures and lifts are
// events of that address, from which the suppression of the address is derived.
//
// Nothing that can be derived is stored as a state: a reset link is valid only when issued after
// the last change of the password, an import is finished when every item has a result, an export
// is ready, failed, or expired by its result, and an OAuth client is disabled or deleted by the
// events recorded on it. A credential that is replaced or removed keeps its row with a removal or
// a revocation, so that a removed passkey cannot be registered again and an old hash is never
// current again.
package identity
