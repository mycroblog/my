// SPDX-License-Identifier: AGPL-3.0-only
// What starts the streams of a tab again once the server refused them (stream.ts), apart from the streams so that the
// timelines they fill can ask for it when the reader pulls one, without importing what imports them.
import type { AppData } from "./app-data";

const retries = new WeakMap<AppData, Set<() => void>>();

/** Adds retry to what retryStream calls for `data`, until the function it returns is called. */
export function onStreamRetry(data: AppData, retry: () => void): () => void {
  let all = retries.get(data);
  if (!all) {
    all = new Set();
    retries.set(data, all);
  }
  const mine = () => {
    retry();
  };
  all.add(mine);
  return () => {
    all.delete(mine);
  };
}

/**
 * Starts each stream of `data` again that is `stopped` (the server refused it): the session's, and those of the public
 * timelines it shows. Its state goes `connecting`, then `open`, or `stopped` again if the server refuses it again. A
 * stream in any other state is left as it is, and without a stream it does nothing.
 */
export function retryStream(data: AppData): void {
  for (const retry of retries.get(data) ?? []) retry();
}
