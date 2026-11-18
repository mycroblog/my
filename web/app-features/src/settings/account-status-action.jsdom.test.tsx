// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: happy-dom gives the focus to any element that is asked to take it, so where the appeal of a moderation
// action (#3204) puts the focus after Send appeal is decided here (ADR-0171 §4): on the field when nothing was sent
// for what it holds, and on the status that says what became of the appeal once the form is gone, which no control
// stands in. What the form sends and says is account-status-action.dom.test.tsx.

import { expect, test } from "bun:test";
import { waitFor } from "@testing-library/react";
import {
  acted,
  answered,
  appealField,
  drawnAction,
  main,
  noPosting,
  sendButton,
  wordsOf,
  write,
  type Drawn,
} from "../test/account-status";

/** Waits until `element` has the focus; the page's own description of what has it otherwise. */
const focused = (element: () => Element | undefined, name: string) =>
  waitFor(() => {
    if (document.activeElement !== element() || element() === undefined) {
      const has = document.activeElement;
      throw new Error(
        `test: the focus is not on ${name} but on <${has?.tagName.toLowerCase() ?? "nothing"}> "${has?.getAttribute("aria-label") ?? has?.textContent ?? ""}"`,
      );
    }
  });

/** The status of the screen that says `words`. */
const status = (words: string) => () =>
  main()
    .queryAllByRole("status")
    .find((region) => wordsOf(region) === words);

/** The writes the screen sent. */
const writes = ({ requests }: Drawn) => requests.filter((request) => !request.startsWith("GET "));

/** The action drawn once the server answered. */
async function action(...given: Parameters<typeof drawnAction>): Promise<Drawn> {
  const on = await drawnAction(...given);
  await answered(on);
  return on;
}

for (const [what, written] of [
  ["nothing written", ""],
  ["more than 2,000 characters", "a".repeat(2001)],
] as const)
  test(`Send appeal with ${what} sends nothing and moves the focus from the button to the field`, async () => {
    const on = await action(acted(noPosting));
    if (written !== "") await write(on.user, written);

    await on.user.click(sendButton());

    await focused(appealField, "Your appeal");
    expect(writes(on)).toEqual([]);
  });

test("once the appeal is sent, the status that says so has the focus: the button that had it is gone", async () => {
  const on = await action(acted(noPosting));
  await write(on.user, "Please look again.");

  await on.user.click(sendButton());

  await focused(status("Your appeal was sent."), "the status that says the appeal was sent");
  expect(writes(on)).toHaveLength(1);
});

test("once the server refused the appeal for good, the status that says why has the focus", async () => {
  const on = await action({ settings: { moderation: [noPosting], appealSave: "closed" } });
  await write(on.user, "Please look again.");

  await on.user.click(sendButton());

  await focused(
    status(
      "The time to appeal this action has passed, so your appeal was not sent. To reach the moderators, see the contact on this server's About page.",
    ),
    "the status that says why the appeal was not sent",
  );
});
