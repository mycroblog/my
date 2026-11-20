// SPDX-License-Identifier: AGPL-3.0-only

// What ending a subscription and retiring a plan read again: besides the lists, everything that counts the members of
// the plans (the supporters list, and the income's totals of the administrators, of a holder of `view_finance` and of
// the public), so that no screen shows the members of before.

import { expect, test } from "bun:test";
import { endSubscription, membersRead, planActions } from "./supporters-admin";
import { fakeData } from "./test/data";

/** Puts a read of each path that counts the members in the cache, and tells which of them a write marked stale. */
function seeded(data: ReturnType<typeof fakeData>["data"]) {
  for (const path of membersRead) data.queryClient.setQueryData(["get", path, {}], { seeded: path });
  return () => membersRead.filter((path) => data.queryClient.getQueryState(["get", path, {}])?.isInvalidated === true);
}

test("ending a subscription reads again what counts the members of the plans", async () => {
  const { fake, data } = fakeData({ viewer: "hana", supportersAdmin: {} });
  const stale = seeded(data);
  await endSubscription(data, fake.supportersAdmin.subscriptionId("sam"));
  expect(stale()).toEqual([...membersRead]);
});

test("retiring a plan reads again what counts the members of the plans", async () => {
  const { fake, data } = fakeData({ viewer: "hana", supportersAdmin: {} });
  const stale = seeded(data);
  await planActions(data).retire(fake.supportersAdmin.planId("patron"));
  expect(stale()).toEqual([...membersRead]);
});
