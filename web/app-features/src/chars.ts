// SPDX-License-Identifier: AGPL-3.0-only

// How the server counts the characters of a text it limits by code points (a display name, a bio, a field of the
// profile, an article's title and summary): what the forms of several features count a text's room with.

/** The characters of a text, as the server counts them (code points). */
export const charsOf = (text: string): number => Array.from(text).length;
