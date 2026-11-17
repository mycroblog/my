// SPDX-License-Identifier: AGPL-3.0-only

// The profile of an account that moved (ADR-0089 §3, §4): its main action opens the new account, and a line names the
// new account with a link to it. On the fake's server Sora's account on social.example moved to photos.example.

import { expect, test } from "bun:test";
import { screen, within } from "@testing-library/react";
import { main, renderApp, words } from "./test/app";

const sora = "Sora そら";
const old = "sora@social.example";
const now = "sora@photos.example";

/** The screen's heading of the profile of Sora's account `handle`: the display name and the full handle. */
const heading = (handle: string) => main().findByRole("heading", { level: 1, name: `${sora} (@${handle})` });
/** The profile's card. */
const card = () => within(screen.getByRole("region", { name: sora }));
/** The line of the card that names the new account, "Moved to @username@host". */
const line = () => {
  const [found, ...more] = card()
    .getAllByRole("paragraph")
    .filter((paragraph) => new RegExp(`^Moved to ⁨?@${now}⁩?$`).test(words(paragraph)));
  if (found === undefined || more.length > 0) throw new Error("test: no one line says where the account moved to");
  return found;
};

test("a person opens a moved account, follows its link to the new one, and follows that", async () => {
  const app = await renderApp(`/@${old}`);
  expect(await heading(old)).toBeDefined();
  expect(line()).toBeDefined();
  // A moved account offers no follow.
  expect(card().queryAllByRole("button", { name: /^(Follow|Follow back|Request to follow|Following)$/ })).toEqual([]);

  await app.user.click(card().getByRole("link", { name: "Go to new account" }));

  expect(await heading(now)).toBeDefined();
  expect(app.path()).toBe(`/@${now}`);
  await app.user.click(await card().findByRole("button", { name: /^Follow( back)?$/ }));
  expect(await card().findByRole("button", { name: "Following" })).toBeDefined();
});

test("the line of a moved account links to the new one", async () => {
  const app = await renderApp(`/@${old}`);
  await heading(old);

  await app.user.click(within(line()).getByRole("link", { name: `@${now}` }));

  expect(await heading(now)).toBeDefined();
});
