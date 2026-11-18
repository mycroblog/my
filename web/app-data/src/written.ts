// SPDX-License-Identifier: AGPL-3.0-only
// What a write of the settings does to the cache once the server answered it: it puts what it changed into what the
// cache holds of a read, after cancelling the reads of it on their way, since a read sent before the write would put
// back what the server held then; and a removal of something the server no longer has is done all the same.
import { ProblemError } from "@mycroblog/mykit";
import type { QueryKey } from "@tanstack/react-query";
import type { AppData } from "./app-data";

/**
 * Puts what a write changed (`change`, of what the cache holds now) into the cache under `queryKey`, the reads of it
 * on their way cancelled first. Where the cache holds nothing of it, nothing is put there, and it is read again, by
 * the screens that show it at once: a first read cancelled here would otherwise never end.
 */
export async function writeCached<T>(data: AppData, queryKey: QueryKey, change: (now: T) => T): Promise<void> {
  const { queryClient } = data;
  await queryClient.cancelQueries({ queryKey });
  if (queryClient.getQueryData<T>(queryKey) === undefined) {
    void queryClient.invalidateQueries({ queryKey });
    return;
  }
  queryClient.setQueryData<T>(queryKey, (now) => (now === undefined ? now : change(now)));
}

/** Whether `error` is the server's 404 of a removal: what it names is gone already. */
function goneAlready(error: unknown): boolean {
  return error instanceof ProblemError && error.status === 404;
}

/**
 * Waits for a removal the server was asked (`request`) and takes what it removed from the list the cache holds under
 * `queryKey` (`change`, by `writeCached`). A 404 says the server no longer has what the removal names: the reader's
 * intent is met, so it resolves as a removal done does, and the list, which held what the server does not, is read
 * again. Any other failure rejects with the server's problem, and the cache stays as it was.
 */
export async function removeCached<T>(
  data: AppData,
  queryKey: QueryKey,
  request: Promise<unknown>,
  change: (now: T) => T,
): Promise<void> {
  const gone = await request.then(
    () => false,
    (error: unknown) => {
      if (goneAlready(error)) return true;
      throw error;
    },
  );
  await writeCached(data, queryKey, change);
  if (gone) void data.queryClient.invalidateQueries({ queryKey });
}
