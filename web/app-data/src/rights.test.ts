// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { viewerQuery } from "./accounts";
import { moderationCountsQuery, useModerationCount, useRights, type Permission } from "./rights";
import { fakeAppData, json } from "./test/fake";
import { probe } from "./test/probe";

/** The data of a viewer with `permissions`, once the server answered who they are and what awaits the moderators. */
async function viewer(permissions: Permission[], counts: Record<string, number>) {
  const { data, requests } = fakeAppData((request) => {
    const path = new URL(request.url).pathname;
    if (path === "/api/v1/accounts/verify_credentials")
      return json(200, { id: "1", username: "hana", acct: "hana", permissions });
    if (path === "/api/v1/admin/counts") return json(200, counts);
    return json(404, { status: 404, title: "Not Found" });
  });
  await data.queryClient.query(viewerQuery(data));
  await data.queryClient.query(moderationCountsQuery(data));
  return { data, requests };
}

test("the viewer's rights are those of their permissions, and none for a reader without a session", async () => {
  const { data } = await viewer(["manage_reports", "manage_settings", "view_finance"], {});
  expect(probe(data, () => useRights(true))).toEqual({ moderation: true, admin: true, finance: true });
  expect(probe(data, () => useRights(false))).toEqual({ moderation: false, admin: false, finance: false });
});

test("Moderation counts the open reports, the registrations to approve and the appeals to decide together", async () => {
  const { data } = await viewer(["manage_reports", "manage_accounts"], {
    open_reports: 2,
    pending_registrations: 3,
    pending_appeals: 4,
  });
  expect(probe(data, () => useModerationCount(true))).toBe(9);
});

test("a count the moderator's permissions do not show is left out of the sum", async () => {
  const { data } = await viewer(["manage_reports"], { open_reports: 5 });
  expect(probe(data, () => useModerationCount(true))).toBe(5);
});

test("a reader without the rights of moderation, or without a session, has no count", async () => {
  const { data } = await viewer(["manage_settings"], { open_reports: 5 });
  expect(probe(data, () => useModerationCount(true))).toBeUndefined();
  const { data: moderator } = await viewer(["manage_reports"], { open_reports: 5 });
  expect(probe(moderator, () => useModerationCount(false))).toBeUndefined();
});
