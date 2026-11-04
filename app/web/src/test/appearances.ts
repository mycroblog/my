// SPDX-License-Identifier: AGPL-3.0-only
// Whether something was ever on screen while a flow ran, not only at its end: a page that renders what a
// write has made stale and replaces it a moment later passes an assertion made after it (ADR-0028).
import { onTestFinished } from "vitest";

/**
 * Starts watching the document for `isShown` and returns a function that tells whether it has held at any
 * change of the document since. React commits each render to the DOM at once, so every render is seen.
 */
export function watchAppearances(isShown: () => boolean): () => boolean {
  let seen = isShown();
  const observer = new MutationObserver(() => {
    if (!seen && isShown()) seen = true;
  });
  observer.observe(document.body, { childList: true, subtree: true, characterData: true });
  onTestFinished(() => {
    observer.disconnect();
  });
  return () => {
    // Changes not yet delivered to the observer count too.
    if (!seen && observer.takeRecords().length > 0 && isShown()) seen = true;
    return seen;
  };
}
