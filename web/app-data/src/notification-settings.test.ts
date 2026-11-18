// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  changeNotificationPolicy,
  changeNotificationSettings,
  notificationPolicyQuery,
  useNotificationPolicy,
  type NotificationPolicy,
  type NotificationSettings,
} from "./notification-settings";
import { notificationRequestsQuery } from "./notification-requests";
import { notificationRequestSummaryQuery, notificationsQuery } from "./notifications";
import { notificationSettingsQuery } from "./push";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { refusedWith } from "./test/refused";

const cellOf = (data: ReturnType<typeof fakeData>["data"], group: string) => {
  const settings = data.queryClient.getQueryData<NotificationSettings>(notificationSettingsQuery(data).queryKey);
  const row = settings?.rows.find((r) => r.group === group);
  return { in_app: row?.in_app.enabled, email: row?.email?.enabled };
};

describe("changeNotificationSettings", () => {
  test("sends the cells that changed, and the cache holds the rows the server answered", async () => {
    const { fake, data, requests } = fakeData();
    const sent = fake.script("PATCH /preferences/notifications");
    await data.queryClient.query(notificationSettingsQuery(data));
    expect(cellOf(data, "mention")).toEqual({ in_app: true, email: false });
    await changeNotificationSettings(data, [
      { group: "mention", channel: "email", enabled: true },
      { group: "boost", channel: "in_app", enabled: false },
    ]);
    expect(requests.filter((r) => !r.startsWith("GET "))).toEqual(["PATCH /preferences/notifications"]);
    expect(sent.seen.map((s) => s.body)).toEqual([
      {
        changes: [
          { group: "mention", channel: "email", enabled: true },
          { group: "boost", channel: "in_app", enabled: false },
        ],
      },
    ]);
    expect(cellOf(data, "mention")).toEqual({ in_app: true, email: true });
    expect(cellOf(data, "boost")).toEqual({ in_app: false, email: false });
    expect(cellOf(data, "follow")).toEqual({ in_app: true, email: false });
  });

  test("a change written before the rows were read still shows when they are", async () => {
    const { data } = fakeData();
    await changeNotificationSettings(data, [{ group: "favourite", channel: "email", enabled: true }]);
    expect(cellOf(data, "favourite").email).toBe(true);
  });

  test("a row the reader does not get is refused with a 422, and the cache keeps the rows", async () => {
    const { data } = fakeData();
    await data.queryClient.query(notificationSettingsQuery(data));
    const refused = await refusedWith(
      changeNotificationSettings(data, [
        { group: "mention", channel: "email", enabled: true },
        // The specification has more groups than a reader's rows hold.
        { group: "no-such-group" as "mention", channel: "email", enabled: true },
      ]),
    );
    expect(refused.status).toBe(422);
    expect(cellOf(data, "mention")).toEqual({ in_app: true, email: false });
  });
});

describe("the notification policy", () => {
  test("is undefined until it is read, then the server's", async () => {
    const { data } = fakeData();
    expect(probe(data, useNotificationPolicy)).toBeUndefined();
    await data.queryClient.query(notificationPolicyQuery(data));
    expect(probe(data, useNotificationPolicy)).toEqual({
      not_following: "accept",
      not_follower: "accept",
      new_account: "filter",
      unsolicited_private_mention: "filter",
      limited_account: "filter",
    });
  });

  test("a change sends the members given only, and the cache holds the whole policy the server answered", async () => {
    const { fake, data } = fakeData();
    const sent = fake.script("PATCH /preferences/notification_policy");
    await data.queryClient.query(notificationPolicyQuery(data));
    await changeNotificationPolicy(data, { not_following: "filter", new_account: "accept" });
    expect(sent.seen.map((s) => s.body)).toEqual([{ not_following: "filter", new_account: "accept" }]);
    const expected: NotificationPolicy = {
      not_following: "filter",
      not_follower: "accept",
      new_account: "accept",
      unsolicited_private_mention: "filter",
      limited_account: "filter",
    };
    expect(probe(data, useNotificationPolicy)).toEqual(expected);
  });

  test("marks the notifications and the held-back requests stale, so that a screen shows the new policy's effect", async () => {
    const { data } = fakeData({ held: true });
    await data.queryClient.query(notificationPolicyQuery(data));
    await data.queryClient.query(notificationRequestSummaryQuery(data));
    await data.queryClient.infiniteQuery(notificationsQuery(data, "all"));
    await data.queryClient.infiniteQuery(notificationRequestsQuery(data));
    await changeNotificationPolicy(data, { not_following: "drop" });
    const marked = (key: readonly unknown[]) => data.queryClient.getQueryState(key)?.isInvalidated;
    expect(marked(notificationsQuery(data, "all").queryKey)).toBe(true);
    expect(marked(notificationRequestsQuery(data).queryKey)).toBe(true);
    expect(marked(notificationRequestSummaryQuery(data).queryKey)).toBe(true);
  });

  test("a change the server failed rejects, and the cache keeps the policy", async () => {
    const { fake, data } = fakeData();
    const before = await data.queryClient.query(notificationPolicyQuery(data));
    fake.script("PATCH /preferences/notification_policy", { fail: 500 });
    await refusedWith(changeNotificationPolicy(data, { not_following: "filter" }));
    expect(probe(data, useNotificationPolicy)).toEqual(before);
  });
});
