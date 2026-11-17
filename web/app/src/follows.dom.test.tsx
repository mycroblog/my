// SPDX-License-Identifier: AGPL-3.0-only

// A person's lists of follows and followers, opened from the counts of their profile: a tab replaces the address, and
// a list its owner keeps from the reader says so (FR-311). On the fake's server Mio shows her lists to her followers
// only and Kai to nobody; the reader follows neither. That a follow from a row stays is the E2E tests'
// (web/e2e/specs/follows.spec.ts).

import { expect, test } from "bun:test";
import { screen, within } from "@testing-library/react";
import { main, renderApp, words } from "./test/app";

/** The screen's heading of the profile of `name` (a display name): the name and the full handle. */
const heading = (name: string) =>
  main().findByRole("heading", { level: 1, name: new RegExp(`^${name} \\(@[^)]+\\)$`) });
/** The link of the profile's card that opens its list `list`, named by the list and its count ("Followers 12"). */
const countLink = (name: string, list: "Following" | "Followers") =>
  within(screen.getByRole("region", { name })).findByRole("link", { name: new RegExp(`^${list} [\\d,]+$`) });
/** The tab of the list `list`, named by the list and its count. */
const tab = (list: "Following" | "Followers") =>
  within(main().getByRole("tablist", { name: "Following and followers" })).getByRole("tab", {
    name: new RegExp(`^${list} [\\d,]+$`),
  });
/** The open list. */
const panel = () => within(main().getByRole("tabpanel"));

test("a tab of the lists replaces the address, and Back returns to the profile", async () => {
  const app = await renderApp("/@sam", { viewer: "marco" });
  expect(await heading("Sam Rivera")).toBeDefined();

  await app.user.click(await countLink("Sam Rivera", "Followers"));
  await main().findByRole("tablist", { name: "Following and followers" });
  expect(app.path()).toBe("/@sam/followers");
  expect(tab("Followers").getAttribute("aria-selected")).toBe("true");
  await app.user.click(tab("Following"));
  expect(await panel().findByRole("link", { name: "Yuki 雪" })).toBeDefined();
  expect(app.path()).toBe("/@sam/following");

  // The tab replaced the list's address, so the browser's back leaves the lists for the profile.
  await app.back();
  expect(await heading("Sam Rivera")).toBeDefined();
  expect(app.path()).toBe("/@sam");
  await app.user.click(await countLink("Sam Rivera", "Following"));
  expect(await panel().findByRole("link", { name: "Yuki 雪" })).toBeDefined();
  await app.user.click(main().getByRole("button", { name: "Back to the profile" }));
  expect(await heading("Sam Rivera")).toBeDefined();
});

for (const [username, name, shown] of [
  ["mio", "Mio 澪", "their followers"],
  ["kai", "Kai", "nobody"],
] as const)
  test(`a list its owner shows to ${shown} says so to another`, async () => {
    const app = await renderApp(`/@${username}/followers`, { viewer: "sam" });
    const refusal = new RegExp(`^⁨?${name}⁩? does not show this list to you\\.$`);

    expect(words(await panel().findByRole("status"))).toMatch(refusal);
    expect(panel().queryAllByRole("link")).toEqual([]);

    await app.user.click(tab("Following"));
    expect(app.path()).toBe(`/@${username}/following`);
    expect(words(await panel().findByRole("status"))).toMatch(refusal);
    expect(panel().queryAllByRole("link")).toEqual([]);
  });
