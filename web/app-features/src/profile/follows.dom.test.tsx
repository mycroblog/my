// SPDX-License-Identifier: AGPL-3.0-only

// An account's lists of follows and followers as a feature: which of its two tabs is chosen, a row per account with
// the reader's follow of it, and what stands in a list's place when it could not be read or holds no one. A list its
// owner keeps from the reader is in profile.dom.test.tsx.

import { expect, test } from "bun:test";
import { scenario, type FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { Follows, loadFollows } from "./follows";

type List = "following" | "followers";

const drawn = (list: List) => <Follows acct="sam" list={list} onList={() => undefined} onBack={() => undefined} />;

/** The list `list` of sam on a server of `options` (the scenario by default), loaded and drawn as yuki reads it. */
async function shown(list: List, options: FakeOptions = {}) {
  const { data } = fakeData(options);
  await loadFollows(data, "sam", list);
  renderFeature(data, drawn(list));
  return { panel: within(screen.getByRole("tabpanel")) };
}

/** The two tabs, each by its name without its count, with whether it is the one chosen. */
const tabs = () =>
  within(screen.getByRole("tablist", { name: "Following and followers" }))
    .getAllByRole("tab")
    .map((tab) => [tab.textContent.replace(/\s*[\d,]+$/, ""), tab.getAttribute("aria-selected")]);

test("the followers of an account: their tab chosen of the two, a row per follower that leads to its profile, and Follow for the one the reader does not follow alone", async () => {
  // marco, layla, yuki and hana follow sam; yuki, who reads, follows layla and hana.
  const { panel } = await shown("followers");

  const rows = await panel.findAllByRole("link");
  expect(tabs()).toEqual([
    ["Following", "false"],
    ["Followers", "true"],
  ]);
  expect(rows.map((row) => [row.textContent, row.getAttribute("href")])).toEqual([
    ["Marco", "/@marco"],
    ["ليلى Layla", "/@layla"],
    ["Yuki 雪", "/@yuki"],
    ["Hana 花", "/@hana"],
  ]);
  expect(panel.getAllByRole("button").map((button) => button.textContent)).toEqual(["Follow"]);
});

test("the accounts an account follows: their tab chosen of the two, and a row per account", async () => {
  const { panel } = await shown("following");

  const rows = await panel.findAllByRole("link");
  expect(tabs()).toEqual([
    ["Following", "true"],
    ["Followers", "false"],
  ]);
  expect(rows.map((row) => row.getAttribute("href"))).toContain("/@yuki");
});

test("a list that could not be read says so in its tab's place, with Try again", async () => {
  // The screen reads the list itself, which its route could not load.
  renderFeature(fakeData({ followsLists: "fails" }).data, drawn("followers"));

  // One wait from the first draw on: the panel is a stop of Tab while nothing in it is one, and stops being one a
  // turn after Try again shows.
  const alert = await waitFor(() => {
    const panel = screen.getByRole("tabpanel");
    if (panel.hasAttribute("tabindex")) throw new Error("the panel is still a stop of Tab");
    return within(panel).getByRole("alert");
  });
  expect(alert.textContent).toMatch(/^The followers could not be loaded\.\s?Try again$/);
  expect(within(alert).getByRole<HTMLButtonElement>("button", { name: "Try again" }).disabled).toBe(false);
  expect(tabs().map(([, chosen]) => chosen)).toEqual(["false", "true"]);
  expect(screen.queryAllByRole("link")).toHaveLength(0);
});

test("a list with no account says so, and shows no row", async () => {
  // A server where nobody follows anybody.
  const { panel } = await shown("followers", { seed: { people: scenario.people } });

  expect((await panel.findByRole("status")).textContent).toBe("No followers yet.");
  expect(panel.queryAllByRole("link")).toHaveLength(0);
});
