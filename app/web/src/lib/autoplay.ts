// SPDX-License-Identifier: AGPL-3.0-only
// Whether animated images (GIF, animated WebP) play by themselves (ADR-0005 §6). Like the theme (lib/theme), it is
// how this browser shows the app, so it is kept in localStorage (lib/storage) and not in the user's preferences on the server: it
// applies before anyone signs in and outlives the session. On by default; even on, an animation stays still while
// the system asks for reduced motion (@my/ui Picture).
import { autoplayKey, useStorage } from "./storage";

/** Turns the autoplay of animations on or off at once and keeps the choice for the next start on this browser. */
export function setAutoplay(autoplay: boolean): void {
  autoplayKey.write(autoplay);
}

/** Whether animations play by themselves, rendering again when the setting changes here or in another tab. */
export function useAutoplay(): boolean {
  return useStorage(autoplayKey);
}
