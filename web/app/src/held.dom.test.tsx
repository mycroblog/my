// SPDX-License-Identifier: AGPL-3.0-only

// The notifications the reader's notification policy holds back (ADR-0158 §7), on a server that holds back two of kiri
// and one of kobo (the fake's `held`): the screen of the accounts that sent them, where an answer takes the place of
// its buttons with the focus, and the screen of one account's, which says so when its request could not be read or is
// no longer there. That the answer's text can take a script's focus is the design system's (list.jsdom.test.tsx).

import { expect, test } from "bun:test";
import { waitFor, within } from "@testing-library/react";
import { main, renderApp, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

const held = { held: true } as const;

/** The buttons of the screen in place that are named `name`. */
const buttons = (name: "Accept" | "Dismiss" | "Try again") => main().queryAllByRole("button", { name });

/** The screen of the accounts whose notifications are held back, once it shows their rows. */
async function accounts(): Promise<TestApp> {
  const app = await renderApp("/notifications/requests", held);
  await main().findByRole("link", { name: "Bakery Kobo" });
  return app;
}

/** The screen of the held-back notifications of the person of `username`, on a server `script` prepared. */
async function accountOf(username: string, script: (on: ReturnType<typeof fakeApp>) => void = () => undefined) {
  const on = fakeApp(held);
  script(on);
  return renderApp(on.fake.heldPath(username), { on });
}

for (const [button, answer] of [
  ["Accept", "Accepted"],
  ["Dismiss", "Dismissed"],
] as const) {
  test(`${button} on an account's held-back notifications puts ${answer} in place of its buttons, with the focus, and leaves the other account waiting`, async () => {
    const app = await accounts();
    expect(buttons("Accept")).toHaveLength(2);
    expect(buttons("Dismiss")).toHaveLength(2);
    // Kiri's row is the first: its notifications are the newest.
    const [first] = buttons(button);
    if (first === undefined) throw new Error(`test: no ${button}`);

    await app.user.click(first);

    const said = await main().findByText(answer, { exact: true });
    await waitFor(() => {
      if (app.focused() !== said) throw new Error(`test: ${answer} does not have the focus`);
    });
    expect(buttons("Accept")).toHaveLength(1);
    expect(buttons("Dismiss")).toHaveLength(1);
    // The row that still waits is Kobo's: its buttons come after the answer.
    const [waiting] = buttons("Accept");
    if (waiting === undefined) throw new Error("test: no account waits");
    expect(said.compareDocumentPosition(waiting) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  });
}

test("one account's screen is headed by the account, with its answer's buttons, over its held-back notifications", async () => {
  await accountOf("kiri@far.example");

  const feed = await main().findByRole("feed", { name: "Held-back notifications" });
  expect(main().getByRole("heading", { level: 2, name: "Kiri 霧" })).toBeDefined();
  expect(within(feed).getAllByRole("article")).toHaveLength(2);
  expect(within(feed).getByRole("article", { name: "Kiri 霧 favourited your post" })).toBeDefined();
  expect(within(feed).getByRole("article", { name: "Kiri 霧 boosted your post" })).toBeDefined();
  expect(buttons("Accept")).toHaveLength(1);
  expect(buttons("Dismiss")).toHaveLength(1);
});

test("one account's screen whose request could not be read says so with Try again, not that it holds none", async () => {
  await accountOf("kiri@far.example", (on) => on.fake.script("GET /notifications/requests/{id}", { fail: 500 }));

  const failure = await main().findByRole("alert");
  expect(failure.textContent).toContain("Held-back notifications could not be loaded.");
  expect(within(failure).getByRole("button", { name: "Try again" })).toBeDefined();
  expect(main().queryAllByText("This person has no held-back notifications now.")).toHaveLength(0);
  expect(buttons("Accept")).toHaveLength(0);
});

test("one account's screen whose request the server no longer lists says that it holds none, with nothing to try again", async () => {
  // Sam sent nothing the policy held back.
  await accountOf("sam");

  expect(await main().findByText("This person has no held-back notifications now.")).toBeDefined();
  expect(buttons("Try again")).toHaveLength(0);
  expect(buttons("Accept")).toHaveLength(0);
});
