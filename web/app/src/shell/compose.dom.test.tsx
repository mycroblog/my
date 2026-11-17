// SPDX-License-Identifier: AGPL-3.0-only

// What opens the composer: the shell's own state, which New post, `n`, Reply and Quote set, and no navigation.

import { expect, test } from "bun:test";
import type { ComposeOpening as ComposeTarget } from "@my/app-features";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useCompose } from "./compose";

const reply = { kind: "reply", id: "1" } as const;
const quote = { kind: "quote", id: "1" } as const;

/** A part of the shell that opens the composer for each target and closes it, and says what it is open for. */
function Part() {
  const { target, open, close } = useCompose();
  const targets: [string, ComposeTarget][] = [
    ["New post", { kind: "new" }],
    ["Reply", reply],
    ["Quote", quote],
  ];
  return (
    <>
      <output>{target === undefined ? "closed" : JSON.stringify(target)}</output>
      {targets.map(([name, to]) => (
        <button
          key={name}
          type="button"
          onClick={() => {
            open(to);
          }}
        >
          {name}
        </button>
      ))}
      <button type="button" onClick={close}>
        Close
      </button>
    </>
  );
}

test("New post, Reply and Quote open the composer by the shell's state, with no router, and close closes it", async () => {
  const user = userEvent.setup();
  const address = location.href;
  // No router is around the part: opening the composer is no navigation, so it runs no View Transition.
  render(<Part />);
  const seen = [screen.getByRole("status").textContent];

  for (const name of ["New post", "Reply", "Quote", "Close"]) {
    await user.click(screen.getByRole("button", { name }));
    seen.push(screen.getByRole("status").textContent);
  }

  expect(seen).toEqual([
    "closed",
    JSON.stringify({ kind: "new" }),
    JSON.stringify(reply),
    JSON.stringify(quote),
    "closed",
  ]);
  expect(location.href).toBe(address);
});
