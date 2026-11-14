// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { announcementsQuery, markAnnouncementRead, useAnnouncements, type Announcement } from "./announcements";
import { fakeAppData, json } from "./test/fake";
import { probe } from "./test/probe";

/** An announcement as the server sends it; `n` orders them, the latest published first. */
const announcement = (n: number, read: boolean): Announcement => ({
  id: String(n),
  title: `Announcement ${String(n)}`,
  content: `<p>Announcement ${String(n)}</p>`,
  published_at: `2026-11-${String(n).padStart(2, "0")}T00:00:00Z`,
  revised_at: `2026-11-${String(n).padStart(2, "0")}T00:00:00Z`,
  read,
  ended: false,
});

/** A server with the announcements `reads` says, the latest published first, which marks one read when asked. */
async function server(reads: Record<number, boolean>) {
  const list = Object.entries(reads)
    .map(([n, read]) => announcement(Number(n), read))
    .sort((a, b) => b.published_at.localeCompare(a.published_at));
  const marked: string[] = [];
  const { data } = fakeAppData((request) => {
    const read = /^\/api\/v1\/announcements\/([^/]+)\/read$/.exec(new URL(request.url).pathname);
    if (read?.[1] !== undefined && request.method === "POST") {
      const id = read[1];
      marked.push(id);
      const i = list.findIndex((a) => a.id === id);
      const found = list[i];
      if (found === undefined) return json(404, { status: 404, title: "Not Found" });
      list[i] = { ...found, read: true };
      return json(200, list[i]);
    }
    return json(200, { items: list });
  });
  await data.queryClient.query(announcementsQuery(data));
  return { data, marked };
}

const shown = (items: readonly Announcement[] | undefined) => items?.map((a) => [a.id, a.read]);

test("the unread announcements come first, each part in the server's order, the latest first", async () => {
  const { data } = await server({ 1: false, 2: true, 3: false, 4: true, 5: false });
  expect(shown(probe(data, () => useAnnouncements(true)))).toEqual([
    ["5", false],
    ["3", false],
    ["1", false],
    ["4", true],
    ["2", true],
  ]);
});

test("a reader without a session has no announcements", async () => {
  const { data } = await server({ 1: false });
  expect(probe(data, () => useAnnouncements(false))).toEqual([]);
});

test("an announcement opened is marked read on the server, and the list shows it read, after the unread", async () => {
  const { data, marked } = await server({ 1: false, 2: false });
  await markAnnouncementRead(data, "2");
  expect(marked).toEqual(["2"]);
  expect(shown(probe(data, () => useAnnouncements(true)))).toEqual([
    ["1", false],
    ["2", true],
  ]);
});
