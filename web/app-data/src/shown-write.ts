// SPDX-License-Identifier: AGPL-3.0-only
// A write that shows what the reader chose from the press on (an answer to an event, a cancellation of a subscription):
// the writes of one thing go one at a time, in the order made (`inOrder`), and once none is on its way the cache holds
// what the server last said of it; after a refusal it is read again, since what made the server refuse may have
// changed it.
import { ProblemError } from "@mycroblog/mykit";
import type { QueryClient, QueryKey } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { inOrder } from "./in-order";

/**
 * What was on its way of a key of each written thing, what the server said of it last, and whether the server refused
 * the last of them: what it holds then may have changed since (an event that filled up, a subscription that ended), so
 * it is read again rather than put back.
 */
type Writing = { count: number; server: unknown; refused: boolean };

const writings = new WeakMap<QueryClient, Map<string, Writing>>();

/**
 * Runs `write`, a write of the thing named `key`, once its writes made before have ended (`inOrder`), with what the
 * reader chose shown from now on (`show`); once no write of it is on its way, the reads of `keys` on their way are
 * cancelled, and the cache holds what the server last said of it (`put`, given what `read` held before the first of
 * them where every one failed for want of an answer), or reads it again (`refresh`) where the server refused the last
 * one, where it failed with no answer and the write may have been done (`unsure`), or the cache held nothing before. It resolves or rejects as the write does.
 */
export async function shownWrite<S, T>(
  data: AppData,
  key: string,
  w: {
    readonly keys: readonly QueryKey[];
    readonly read: () => S | undefined;
    readonly show: () => void;
    readonly write: () => Promise<T>;
    readonly server: (answer: T) => S;
    readonly put: (server: S) => void;
    readonly refresh: () => void;
    /**
     * The write may have been done without its answer reaching the reader (a change of a subscription that charged
     * them): a failure with no answer reads the thing again too, rather than putting back what was there before.
     */
    readonly unsure?: boolean;
  },
): Promise<T> {
  let all = writings.get(data.queryClient);
  if (all === undefined) {
    all = new Map();
    writings.set(data.queryClient, all);
  }
  const writing = all.get(key) ?? { count: 0, server: w.read(), refused: false };
  all.set(key, writing);
  writing.count += 1;
  w.show();
  try {
    const answer = await inOrder(data, key, w.write);
    writing.server = w.server(answer);
    writing.refused = false;
    return answer;
  } catch (error) {
    if (error instanceof ProblemError || w.unsure === true) writing.refused = true;
    throw error;
  } finally {
    writing.count -= 1;
    // Read anew after the wait: a write begun meanwhile joins this one, and puts what comes of them once it ends.
    const last = () => writing.count === 0;
    if (last()) {
      // A read of the thing started before the write answered would put back what the server held before it.
      await Promise.all(w.keys.map((queryKey) => data.queryClient.cancelQueries({ queryKey })));
      if (last()) {
        all.delete(key);
        if (writing.refused || writing.server === undefined) w.refresh();
        else w.put(writing.server as S);
      }
    }
  }
}

/** The body of an answer, which the client sets whenever the request resolves. */
export function answered<T>({ data }: { data?: T }, what: string): T {
  if (data === undefined) throw new Error(`app-data: ${what} answered without a body`);
  return data;
}
