// SPDX-License-Identifier: AGPL-3.0-only

// Whether the device has a network (ADR-0088 §15): the browser's own word, which says when the device has none, never
// that a server answers. The app tells the reader offline from the server down by asking the server's health
// (@my/app-data); this only says when to ask again.

type Target = Pick<EventTarget, "addEventListener" | "removeEventListener">;

/** Whether the browser says the device has a network; true where it cannot say. */
export function isOnline(): boolean {
  return typeof navigator === "undefined" || navigator.onLine;
}

/**
 * Calls `onChange` with whether the device has a network whenever the browser says it changed, until the function it
 * returns is called.
 */
export function watchOnline(
  onChange: (online: boolean) => void,
  target: Target | undefined = typeof window === "undefined" ? undefined : window,
): () => void {
  if (target === undefined) return () => undefined;
  const online = () => {
    onChange(true);
  };
  const offline = () => {
    onChange(false);
  };
  target.addEventListener("online", online);
  target.addEventListener("offline", offline);
  return () => {
    target.removeEventListener("online", online);
    target.removeEventListener("offline", offline);
  };
}
