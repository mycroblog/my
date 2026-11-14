// SPDX-License-Identifier: AGPL-3.0-only

// One-time codes (FR-005): the field a code goes in, marked so that the browser and the OS offer the code they
// received. A screen keeps the field for everyone; the autofill only fills it.

/** The attributes of the field of a one-time code, whose autofill offers the code the device received. */
export const oneTimeCodeField = { autoComplete: "one-time-code" } as const;
