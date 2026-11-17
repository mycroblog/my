// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { viewerQuery } from "./accounts";
import { moderationCountsQuery, useModerationCount, useRights } from "./rights";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";

/**
 * The data of a person of the scenario, once the server answered who they are and what awaits the moderators: hana
 * administers the server, marco moderates it, yuki does neither. For marco the server counts 3 open reports, 2
 * registrations to approve and 1 appeal to decide.
 */
async function viewer(username: "hana" | "marco" | "yuki", reportsOnly = false) {
  const { fake, data } = fakeData({ viewer: username });
  // A moderator whose role manages the reports alone: the server leaves the other counts out.
  if (reportsOnly) {
    fake.script("GET /accounts/verify_credentials", {
      change: (account) => ({ ...account, permissions: ["manage_reports" as const] }),
    });
    fake.script("GET /admin/counts", {
      change: ({ open_reports }) => (open_reports === undefined ? {} : { open_reports }),
    });
  }
  await data.queryClient.query(viewerQuery(data));
  await data.queryClient.query(moderationCountsQuery(data));
  return data;
}

test("the viewer's rights are those of their permissions, and none for a reader without a session", async () => {
  const data = await viewer("hana");
  expect(probe(data, () => useRights(true))).toEqual({ moderation: true, admin: true, finance: true });
  expect(probe(data, () => useRights(false))).toEqual({ moderation: false, admin: false, finance: false });
  expect(probe(await viewer("marco"), () => useRights(true))).toEqual({
    moderation: true,
    admin: false,
    finance: false,
  });
});

test("Moderation counts the open reports, the registrations to approve and the appeals to decide together", async () => {
  expect(probe(await viewer("marco"), () => useModerationCount(true))).toBe(6);
});

test("a count the moderator's permissions do not show is left out of the sum", async () => {
  expect(probe(await viewer("marco", true), () => useModerationCount(true))).toBe(3);
});

test("a reader without the rights of moderation, or without a session, has no count", async () => {
  expect(probe(await viewer("yuki"), () => useModerationCount(true))).toBeUndefined();
  expect(probe(await viewer("marco"), () => useModerationCount(false))).toBeUndefined();
});
