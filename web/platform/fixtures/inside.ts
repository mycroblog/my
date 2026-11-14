// SPDX-License-Identifier: AGPL-3.0-only
// @my/platform is where the platform is read (ADR-0028 §17): nothing here is refused.

export const read = [
  navigator.credentials,
  navigator.userAgent,
  typeof PublicKeyCredential,
  "ontouchstart" in window,
  window.matchMedia("(display-mode: standalone)"),
];
