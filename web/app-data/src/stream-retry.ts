// SPDX-License-Identifier: AGPL-3.0-only
// What starts the session's stream again once the server refused it (stream.ts), apart from the stream so that the
// timelines it fills can ask for it when the reader pulls one, without importing what imports them.
import type { AppData } from "./app-data";

const retries = new WeakMap<AppData, () => void>();

/** Makes retry what retryStream calls for `data`, until the function it returns is called. */
export function onStreamRetry(data: AppData, retry: () => void): () => void {
  retries.set(data, retry);
  return () => {
    if (retries.get(data) === retry) retries.delete(data);
  };
}

/**
 * Starts the stream of `data` again when it is `stopped` (the server refused it): its state goes `connecting`, then
 * `open`, or `stopped` again if the server refuses it again. In any other state, or without a stream, it does nothing.
 */
export function retryStream(data: AppData): void {
  retries.get(data)?.();
}
