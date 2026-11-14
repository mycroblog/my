// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import type { ComposeTarget } from "@my/app-features";
import { renderToStaticMarkup } from "react-dom/server";
import { useCompose } from "./compose";

/** The targets a part sees as it opens the composer for each of `targets` in turn, then closes it. */
function opened(targets: ComposeTarget[]) {
  const seen: (ComposeTarget | undefined)[] = [];
  let next = 0;
  function Part() {
    const { target, open, close } = useCompose();
    seen.push(target);
    const to = targets[next];
    next += 1;
    if (to !== undefined) open(to);
    else if (target !== undefined) close();
    return null;
  }
  // No router: opening the composer is no navigation, so it runs no View Transition.
  renderToStaticMarkup(<Part />);
  return seen;
}

test("New post, `n`, Reply and Quote open the composer by the shell's state, with no router", () => {
  const reply = { kind: "reply", id: "1" } as const;
  const quote = { kind: "quote", id: "1" } as const;
  expect(opened([{ kind: "new" }, reply, quote])).toEqual([undefined, { kind: "new" }, reply, quote, undefined]);
});
