// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Runs `run` once the page is idle (within 5 seconds where the browser tells it, else after 2), for what a screen may
 * want soon and need not wait for (a module fetched ahead); returns what cancels it.
 */
export function whenIdle(run: () => void): () => void {
  if (typeof window.requestIdleCallback === "function") {
    const handle = window.requestIdleCallback(run, { timeout: 5000 });
    return () => {
      window.cancelIdleCallback(handle);
    };
  }
  const timer = setTimeout(run, 2000);
  return () => {
    clearTimeout(timer);
  };
}
