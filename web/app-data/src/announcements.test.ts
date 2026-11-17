// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { announcementsQuery, markAnnouncementRead, useAnnouncements, type Announcement } from "./announcements";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";

/**
 * A server with the announcements `reads` says the reader read or not, by number, a higher one published later, which
 * marks one read when asked; the announcements are read. `id(n)` is the ID of the announcement of a number.
 */
async function server(reads: Record<number, boolean>) {
  const { fake, data, requests } = fakeData({
    seed: {
      people: scenario.people,
      announcements: Object.entries(reads).map(([n, read]) => ({
        key: n,
        title: `Announcement ${n}`,
        text: `Announcement ${n}.`,
        ...(read ? { readBy: ["yuki"] } : {}),
      })),
    },
  });
  await data.queryClient.query(announcementsQuery(data));
  const id = (n: number) => fake.announcementPath(String(n)).replace("/announcements/", "");
  return { data, requests, id };
}

const shown = (items: readonly Announcement[] | undefined) => items?.map((a) => [a.title, a.read]);

test("the unread announcements come first, each part in the server's order, the latest first", async () => {
  const { data } = await server({ 1: false, 2: true, 3: false, 4: true, 5: false });
  expect(shown(probe(data, () => useAnnouncements(true)))).toEqual([
    ["Announcement 5", false],
    ["Announcement 3", false],
    ["Announcement 1", false],
    ["Announcement 4", true],
    ["Announcement 2", true],
  ]);
});

test("a reader without a session has no announcements", async () => {
  const { data } = await server({ 1: false });
  expect(probe(data, () => useAnnouncements(false))).toEqual([]);
});

test("an announcement opened is marked read on the server, and the list shows it read, after the unread", async () => {
  const { data, requests, id } = await server({ 1: false, 2: false });
  await markAnnouncementRead(data, id(2));
  expect(requests).toContain(`POST /announcements/${id(2)}/read`);
  expect(shown(probe(data, () => useAnnouncements(true)))).toEqual([
    ["Announcement 1", false],
    ["Announcement 2", true],
  ]);
});
