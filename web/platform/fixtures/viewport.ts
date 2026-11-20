// SPDX-License-Identifier: AGPL-3.0-only
// restrict/viewport-breakpoints below the design system: only the design system and the app decide by the viewport
// (ADR-0025 §4), so @my/platform, @my/rtc and @my/app-data read none of it. The platform's own media features and the
// reader's preferences are no viewport.

export const read = [
  window.matchMedia("(min-width: 40rem)"), // error: viewport matchMedia(min-width)
  window.matchMedia("(orientation: portrait)"), // error: viewport matchMedia(orientation)
  window.innerWidth, // error: viewport innerWidth
  window.innerHeight,
  window.matchMedia("(display-mode: standalone)"),
  window.matchMedia("(prefers-reduced-motion: reduce)"),
];
