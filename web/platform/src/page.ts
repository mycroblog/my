// SPDX-License-Identifier: AGPL-3.0-only

// The page going away (ADR-0101 §7): what outlives every screen of the app but not the page (a call) hears that the
// page is hidden for good or unloaded (`pagehide`), and holds an unload the reader did not ask for (a reload, a link
// to a page outside the app) behind the browser's own question (`beforeunload`).

type Target = Pick<EventTarget, "addEventListener" | "removeEventListener">;

const pageTarget = (): Target | undefined => (typeof window === "undefined" ? undefined : window);

/** Calls `listener` when the page goes away (pagehide), until the function it returns is called. */
export function watchPageHide(listener: () => void, target: Target | undefined = pageTarget()): () => void {
  if (target === undefined) return () => undefined;
  target.addEventListener("pagehide", listener);
  return () => {
    target.removeEventListener("pagehide", listener);
  };
}

/**
 * Has the browser ask the reader before the page unloads, until the function it returns is called: a reload, a link
 * to a page outside the app, closing the tab.
 */
export function holdUnload(target: Target | undefined = pageTarget()): () => void {
  if (target === undefined) return () => undefined;
  const ask = (event: Event) => {
    event.preventDefault();
  };
  target.addEventListener("beforeunload", ask);
  return () => {
    target.removeEventListener("beforeunload", ask);
  };
}
