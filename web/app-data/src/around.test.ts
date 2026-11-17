// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario, type FakeSeed } from "@my/e2e/fake";
import { openVoiceRoomsQuery, upcomingEventsQuery, useOpenVoiceRooms, useUpcomingEvents, type FirstOf } from "./around";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";

/** The people whose voice rooms the fake opens while its seed has them, the reader first. */
const hosts = ["yuki", "sam", "hana", "layla"];

const event = (n: number): FakeSeed["events"][number] => ({
  key: `event-${String(n)}`,
  organizer: "yuki",
  title: `Meetup ${String(n)}`,
  description: "An evening of talks.",
  timeZone: "UTC",
  inDays: n,
  hour: 19,
  hours: 2,
});

/**
 * A server with `count` open voice rooms and as many upcoming events (4 at most: the fake opens a room per host); with
 * `short`, its page ends at 3 and says there are more, as a server does that leaves out what the reader may not see.
 * Both lists are read; `ids` are those the server lists first.
 */
async function server(count: number, short = false) {
  const { fake, data } = fakeData({
    seed: {
      people: scenario.people.filter(
        (p) => !hosts.includes(p.username) || hosts.indexOf(p.username) < Math.max(count, 1),
      ),
      events: Array.from({ length: count }, (_, i) => event(i + 1)),
    },
    around: count === 0 ? "none" : "all",
  });
  if (short) {
    fake.script("GET /voice/rooms", {
      change: ({ items }) => ({ items: items.slice(0, 3), next_cursor: items[2]?.id ?? "" }),
    });
    fake.script("GET /events", {
      change: ({ items }) => ({ items: items.slice(0, 3), next_cursor: items[2]?.id ?? "" }),
    });
  }
  const { data: rooms } = await data.api.GET("/voice/rooms");
  const { data: events } = await data.api.GET("/events");
  await data.queryClient.query(openVoiceRoomsQuery(data));
  await data.queryClient.query(upcomingEventsQuery(data));
  const first = (page: { items: { id: string }[] } | undefined) => (page?.items ?? []).slice(0, 3).map((x) => x.id);
  return { data, ids: { rooms: first(rooms), events: first(events) } };
}

const ids = (first: FirstOf<{ id: string }> | undefined) =>
  first === undefined ? undefined : { items: first.items.map((x) => x.id), more: first.more };

// The column beside the timeline lists up to 3 rooms and 3 events, with a link to all of them when there are more.
for (const [count, short, shown, more] of [
  [0, false, 0, false],
  [2, false, 2, false],
  [3, false, 3, false],
  [4, false, 3, true],
  // The server's page ends at 3 and says there are more.
  [4, true, 3, true],
] as const)
  test(`of ${String(count)} on the server${short ? " (a page of 3)" : ""}, it shows ${String(shown)} and ${more ? "a link to all" : "no link to all"}`, async () => {
    const { data, ids: listed } = await server(count, short);
    expect([listed.rooms.length, listed.events.length]).toEqual([shown, shown]);
    expect(ids(probe(data, () => useOpenVoiceRooms(true)))).toEqual({ items: listed.rooms, more });
    expect(ids(probe(data, () => useUpcomingEvents(true)))).toEqual({ items: listed.events, more });
  });
