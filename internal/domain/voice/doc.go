// SPDX-License-Identifier: AGPL-3.0-only

// Package voice holds the rules of the voice chat rooms (ADR-0100): the settings of a room and
// their bounds, the values of its visibility, its speakers, the roles of its participants, and
// the reasons its records carry, and the pure policy functions that decide who sees a room
// (CanSee), who may speak in it without a grant (CanSpeakFreely), and who may host one
// (CanHost, NextHost). The facts these functions read (follows, blocks, invitations, bans, the
// state of the accounts) are gathered by internal/app/voice; this package knows nothing of the
// database, the media transport, or the other modules.
package voice
