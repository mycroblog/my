// SPDX-License-Identifier: AGPL-3.0-only
// The design system cannot import @my/platform, and reads no platform either (ADR-0028 §17): the app passes in what
// it asked. The reader's preferences and the viewport are the design system's (ADR-0025 §4).

export const direct = [
  /mac/i.test(navigator.platform), // error: platform navigator
  window.matchMedia("(hover: hover)"), // error: platform matchMedia(hover)
];

export const own = [
  window.matchMedia("(prefers-reduced-motion: reduce)"),
  window.matchMedia("(prefers-color-scheme: dark)"),
  window.matchMedia("(orientation: portrait)"),
];
