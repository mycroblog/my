// SPDX-License-Identifier: AGPL-3.0-only

// The pictures of a server's page in the stories of the layout parts: its banner and its wordmark, as data URLs.

/** A server's banner: a sea under a pale sky. */
export const banner = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 90" width="640" height="360"><rect width="160" height="90" fill="#7fb3d5"/><path d="M0 60 L40 42 L80 56 L120 38 L160 52 V90 H0Z" fill="#3f6f86"/></svg>',
)}`;
/** The wordmark of a server's page. */
export const wordmark = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 32" width="120" height="32"><text x="0" y="24" font-family="sans-serif" font-size="24" fill="#767676">mycroblog</text></svg>',
)}`;
