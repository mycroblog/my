// SPDX-License-Identifier: AGPL-3.0-only

// A list's members as a feature (ADR-0013 §13–§15): the members, each with the Remove that takes it out, under the
// field that finds an account the reader follows to add; an account added, refused at the server's limit of members,
// and removed. That the list then reads the member's posts is the E2E tests' (web/e2e/specs/list-management.spec.ts).

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { ListMembers, loadListMembers } from "./list-members";

/**
 * A server where the reader follows sam, hana and layla, and keeps the list Bread and coffee of sam and hana; it takes
 * no more than `limit` members in a list. The list's members are loaded and drawn.
 */
async function shown(limit?: number) {
  const made = fakeData({
    ...(limit === undefined ? {} : { listLimits: { members: limit } }),
    seed: {
      people: scenario.people,
      follows: ["sam", "hana", "layla"].map((followee) => ({ follower: "yuki", followee })),
      lists: [{ key: "l1", owner: "yuki", name: "Bread and coffee", members: ["sam", "hana"] }],
    },
  });
  const id = made.fake.listPath(0).replace("/lists/", "");
  await loadListMembers(made.data, id);
  const user = renderFeature(made.data, <ListMembers id={id} onBack={() => undefined} />);
  return { ...made, id, user };
}

/** The members, each by the name of the link to its profile. */
const members = () =>
  within(screen.getByRole("list", { name: "Members" }))
    .getAllByRole("link")
    .map((link) => link.textContent);
/** The title: Members, and how many. */
const title = () => screen.getByRole("heading", { level: 2 }).textContent;

/** Finds `q` among the accounts the reader follows: what was found, under the field. */
async function find(user: Awaited<ReturnType<typeof shown>>["user"], q: string) {
  await user.type(screen.getByRole("searchbox", { name: "Add an account you follow" }), q);
  return within(await screen.findByRole("group", { name: "Add an account you follow" }));
}

test("the members stand under the field that adds, each with a Remove named by whom it takes out", async () => {
  await shown();

  expect(title()).toMatch(/^Members\s*2 members$/);
  expect(members()).toEqual(["Hana 花", "Sam Rivera"]);
  expect(
    within(screen.getByRole("list", { name: "Members" }))
      .getAllByRole("button")
      .map((button) => button.getAttribute("aria-label")),
  ).toEqual(["Remove Hana 花", "Remove Sam Rivera"]);
});

test("an account found among those the reader follows is added: the server has it, Added says so where it was found, and it stands first among the members, counted", async () => {
  const { fake, user, id } = await shown();
  const sent = fake.script("POST /lists/{id}/accounts/{account_id}");
  const found = await find(user, "lay");

  await user.click(await found.findByRole("button", { name: "Add" }));

  expect((await sent.arrived()).url.pathname).toEndWith(`/lists/${id}/accounts/${fake.accountId("layla")}`);
  expect(await found.findByText("Added")).not.toBeNull();
  expect(found.queryAllByRole("button", { name: "Add" })).toHaveLength(0);
  await waitFor(() => {
    if (members().length !== 3) throw new Error(`the list shows ${String(members().length)} members`);
  });
  expect(members()).toEqual(["ليلى Layla", "Hana 花", "Sam Rivera"]);
  expect(title()).toMatch(/^Members\s*3 members$/);
});

test("an account the server refuses at its limit of members says so under the field, and Add waits for a change", async () => {
  // The list holds two accounts on a server that takes no more than two.
  const { user } = await shown(2);
  const found = await find(user, "lay");

  await user.click(await found.findByRole("button", { name: "Add" }));

  expect(await screen.findByText(/^A list can have at most [\d,]+ members\./)).not.toBeNull();
  expect(found.getByRole<HTMLButtonElement>("button", { name: "Add" }).disabled).toBe(true);
  expect(members()).toEqual(["Hana 花", "Sam Rivera"]);
  expect(title()).toMatch(/^Members\s*2 members$/);
});

test("Remove takes a member out: the server has it, the member is gone, the count is down and the others stay", async () => {
  const { fake, user, id } = await shown();
  const sent = fake.script("DELETE /lists/{id}/accounts/{account_id}");

  await user.click(screen.getByRole("button", { name: "Remove Sam Rivera" }));

  expect((await sent.arrived()).url.pathname).toEndWith(`/lists/${id}/accounts/${fake.accountId("sam")}`);
  await waitFor(() => {
    if (members().length !== 1) throw new Error(`the list shows ${String(members().length)} members`);
  });
  expect(members()).toEqual(["Hana 花"]);
  expect(title()).toMatch(/^Members\s*1 member$/);
});
