// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { createElement, Suspense, type ComponentType } from "react";
import { prerender } from "react-dom/static";
import { retryingLazy } from "./chunk";

/** The markup of `component` once every part it waits for came, or failed (a failed one is left to the browser). */
async function drawn(component: ComponentType): Promise<string> {
  const { prelude } = await prerender(<Suspense fallback={null}>{createElement(component)}</Suspense>, {
    onError: () => undefined,
  });
  return new Response(prelude).text();
}

test("a lazy part whose fetch failed keeps failing until tried again, which fetches it again", async () => {
  let fetches = 0;
  const part = retryingLazy(() => {
    fetches += 1;
    return fetches === 1
      ? Promise.reject(new Error("the chunk could not be fetched"))
      : Promise.resolve(() => <p>drawn</p>);
  });
  const first = part.get();
  expect(await drawn(first)).not.toContain("drawn");
  // Drawn again before its boundary shows the failure, it is the same failed part: no fetch without end.
  expect(part.get()).toBe(first);
  expect(fetches).toBe(1);
  // Tried again, it is another lazy part (React.lazy keeps its rejection), which fetches again.
  part.retry();
  const second = part.get();
  expect(second).not.toBe(first);
  expect(await drawn(second)).toContain("drawn");
  expect(fetches).toBe(2);
  // Once it came, a retry keeps it.
  part.retry();
  expect(part.get()).toBe(second);
});
