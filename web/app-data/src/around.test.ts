// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { openVoiceRoomsQuery, upcomingEventsQuery, useOpenVoiceRooms, useUpcomingEvents, type FirstOf } from "./around";
import { fakeAppData, json } from "./test/fake";
import { probe } from "./test/probe";

/**
 * A server whose open voice rooms and upcoming events are `count` of each, a page of `page` at a time, at most the
 * `limit` a request asks.
 */
async function server(count: number, page = 20) {
  const { data } = fakeAppData((request) => {
    const url = new URL(request.url);
    const limit = Number(url.searchParams.get("limit") ?? page);
    const size = Math.min(limit, page);
    const items = Array.from({ length: Math.min(count, size) }, (_, i) => ({ id: String(i + 1) }));
    return json(200, { items, ...(count > size ? { next_cursor: String(size) } : {}) });
  });
  await data.queryClient.query(openVoiceRoomsQuery(data));
  await data.queryClient.query(upcomingEventsQuery(data));
  return data;
}

const ids = (first: FirstOf<{ id: string }> | undefined) =>
  first === undefined ? undefined : { items: first.items.map((x) => x.id), more: first.more };

// The column beside the timeline lists up to 3 rooms and 3 events, with a link to all of them when there are more.
for (const [count, page, items, more] of [
  [0, 20, [], false],
  [2, 20, ["1", "2"], false],
  [3, 20, ["1", "2", "3"], false],
  [5, 20, ["1", "2", "3"], true],
  // The server's page ends at 3 and says there are more.
  [5, 3, ["1", "2", "3"], true],
] as const)
  test(`of ${String(count)} on the server (pages of ${String(page)}), it shows ${String(items.length)} and ${more ? "a link to all" : "no link to all"}`, async () => {
    const data = await server(count, page);
    expect(ids(probe(data, () => useOpenVoiceRooms(true)))).toEqual({ items: [...items], more });
    expect(ids(probe(data, () => useUpcomingEvents(true)))).toEqual({ items: [...items], more });
  });
