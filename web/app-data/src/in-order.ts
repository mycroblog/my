// SPDX-License-Identifier: AGPL-3.0-only
// Writes of one document whose every answer is the whole of it (the notification settings, the policy, the push
// subscription), sent one at a time in the order they were made: two on their way at once could be answered crossed,
// the older answer putting back in the cache what the newer one changed.
import type { QueryClient } from "@tanstack/react-query";
import type { AppData } from "./app-data";

/** The end of the last write of each document, which the next one waits for. */
const tails = new WeakMap<QueryClient, Map<string, Promise<void>>>();

/**
 * Runs `write`, a write of the document named `document`, once the writes of it made before have ended, whatever came
 * of them; what it resolves or rejects with is the write's own.
 */
export function inOrder<T>({ queryClient }: AppData, document: string, write: () => Promise<T>): Promise<T> {
  let ends = tails.get(queryClient);
  if (ends === undefined) {
    ends = new Map();
    tails.set(queryClient, ends);
  }
  const answered = (ends.get(document) ?? Promise.resolve()).then(write);
  ends.set(
    document,
    answered.then(
      () => undefined,
      () => undefined,
    ),
  );
  return answered;
}
