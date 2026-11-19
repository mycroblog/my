// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { reportQueueQuery } from "./moderation-reports";
import { fileReport } from "./reports";
import { moderationCountsQuery } from "./rights";
import { fakeData } from "./test/data";

test("a report a moderator files marks the queue of reports and the count of what awaits them, so both show it", async () => {
  const { fake, data, requests } = fakeData({ viewer: "marco" });
  const { queryClient } = data;
  const queue = reportQueueQuery(data, { state: "open" });
  const counts = moderationCountsQuery(data);
  await queryClient.query(queue);
  await queryClient.query(counts);
  const open = () => queryClient.getQueryData<{ open_reports?: number }>(counts.queryKey)?.open_reports ?? 0;
  const before = open();

  await fileReport(data, { account_id: fake.accountId("hana"), category: "spam", forward: false });

  expect(requests).toContain("POST /reports");
  const marked = (key: readonly unknown[]) => queryClient.getQueryState(key)?.isInvalidated;
  expect(marked(queue.queryKey)).toBe(true);
  expect(marked(counts.queryKey)).toBe(true);
  await queryClient.refetchQueries({ queryKey: counts.queryKey });
  expect(open()).toBe(before + 1);
});
