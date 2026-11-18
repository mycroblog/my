// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: happy-dom gives the focus to any element that is asked to take it, so where Relationships (#3202) puts the
// focus once a row's button is gone, and that a failed undo leaves it on the button, are decided here (ADR-0171 §4).
// What an undo sends and says is relationships.dom.test.tsx.

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { fakeData } from "../test/feature";
import { first, lists, plain, read, relationships, rows, said, section, some } from "../test/relationships";

/** The entry on the scenario's server, once its lists are read. */
async function entry() {
  const on = fakeData(some);
  const drawn = await relationships(on);
  await screen.findAllByRole("region");
  await read(on);
  return { ...on, ...drawn };
}

/** What has the focus, in words: for the error of a test that waits for it. */
const focused = () => {
  const has = document.activeElement;
  return `<${has?.tagName.toLowerCase() ?? "nothing"}> "${plain(has?.getAttribute("aria-label") ?? has?.textContent)}"`;
};

for (const kind of lists) {
  const { group, said: sentence, failed } = first[kind.list];

  test(`after ${kind.undo} of ${kind.name.toLowerCase()}, the focus is on what the row says was done, in the place of the button that is gone`, async () => {
    const { user } = await entry();
    const part = section(kind.name);

    await user.click(within(within(part).getByRole("group", { name: group })).getByRole("button", { name: kind.undo }));

    await waitFor(() => {
      if (!plain(document.activeElement?.textContent).endsWith(sentence))
        throw new Error(`test: the focus is not on "${sentence}" but on ${focused()}`);
    });
    // It is the row's own: the first row, where the button was.
    expect(rows(section(kind.name))[0]?.contains(document.activeElement)).toBe(true);
    expect(plain(document.activeElement?.textContent)).toBe(`${kind.done}${sentence}`.replace(/\s+/g, " "));
    // The next Tab goes on from there, to what follows the row.
    await user.tab();
    expect(rows(section(kind.name))[0]?.contains(document.activeElement)).toBe(false);
    expect(document.activeElement === document.body).toBe(false);
  });

  test(`after ${kind.undo} of ${kind.name.toLowerCase()} that fails, the focus stays on the button`, async () => {
    const on = await entry();
    on.fake.script(kind.route, { times: 1, fail: 500 });
    const button = () =>
      within(within(section(kind.name)).getByRole("group", { name: group })).getByRole("button", { name: kind.undo });

    await on.user.click(button());

    await waitFor(() => {
      expect(said(section(kind.name))).toEqual([failed]);
    });
    expect(`${focused()} ${String(document.activeElement === button())}`).toBe(`<button> "${kind.undo}" true`);
  });
}
