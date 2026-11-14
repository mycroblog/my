// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { PostActions, type ComposeRequest } from "../post-actions";
import { press } from "../test/actions";
import { account, render, server, status } from "../test/server";
import { loadNotifications, Notifications } from "./notifications";

/** An ID as the server's: a later one sorts after an earlier one. */
const id = (n: number) => `0190a5d0-0000-7000-8000-${n.toString().padStart(12, "0")}`;

const notifications = [
  { id: id(6), type: "mention", created_at: "2026-11-14T03:00:00Z", account: account("ken"), status: status("m") },
  { id: id(5), type: "follow_request", created_at: "2026-11-14T02:00:00Z", account: account("takuya") },
  { id: id(4), type: "favourite", created_at: "2026-11-14T01:00:00Z", account: account("alice"), status: status("y") },
  { id: id(3), type: "moderation_notice", created_at: "2026-11-13T01:00:00Z" },
  { id: id(2), type: "some_new_kind", created_at: "2026-11-12T01:00:00Z" },
];

const answers = {
  "/notifications": { items: notifications },
  "/notifications/marker": { last_read_id: id(4), unread_count: 2 },
  "/notifications/requests/summary": { accounts_count: 3, notifications_count: 5 },
  "/accounts/relationships": { items: [{ id: "takuya", requested_by: true }] },
};

async function shown(locale = "en") {
  const { data, requests } = server(answers);
  await loadNotifications(data);
  return { html: render(data, <Notifications instance="my.example" />, locale), requests };
}

test("the loader reads the notifications, the read marker and the count of the requests", async () => {
  const { requests } = await shown();
  expect(requests).toEqual(["GET /notifications", "GET /notifications/marker", "GET /notifications/requests/summary"]);
});

test("each notification newer than the marker carries its own unread mark, the others none", async () => {
  const { html } = await shown();
  expect(html.match(/aria-label="Unread"/g)).toHaveLength(2);
  const mention = html.indexOf("mentioned you");
  const favourite = html.indexOf("favourited your post");
  expect(html.slice(mention, favourite)).toContain('aria-label="Unread"');
  expect(html.slice(favourite)).not.toContain('aria-label="Unread"');
});

test("a sentence names the account strong, in the server's words, and a kind the client does not know says there is one", async () => {
  const { html } = await shown();
  expect(html).toMatch(/<bdi class="[^"]*font-strong[^"]*">Ken<\/bdi> mentioned you/);
  expect(html).toContain("The moderators of my.example sent you a notice");
  expect(html).toContain("You have a new notification on my.example");
  const ja = (await shown("ja")).html;
  expect(ja).toContain("さんがあなたをメンションしました");
});

test("over the list, the tabs, mark-all-read, and the row of the requests", async () => {
  const { html } = await shown();
  expect(html).toMatch(/aria-label="Show" role="tablist"/);
  expect(html).toContain("Mark all as read");
  expect(html).toContain("Requests from 3 people · 5 notifications");
});

test("a follow request asks to accept, its refusal beside it", async () => {
  const { data } = server(answers);
  await loadNotifications(data);
  await data.queryClient.query(
    data.queries.queryOptions("get", "/accounts/relationships", { params: { query: { id: ["takuya"] } } }),
  );
  const html = render(data, <Notifications instance="my.example" />);
  expect(html.indexOf(">Reject<")).toBeGreaterThan(0);
  expect(html.indexOf(">Reject<")).toBeLessThan(html.indexOf(">Accept<"));
});

test("the post of a mention answers its actions: a favourite reaches the server, Reply opens the composer", async () => {
  const { data, requests } = server({ ...answers, "/statuses/m/favourite": status("m", "lena", { favourited: true }) });
  await loadNotifications(data);
  const composed: ComposeRequest[] = [];
  render(
    data,
    <PostActions onCompose={(to) => composed.push(to)}>
      <Notifications instance="my.example" />
    </PostActions>,
  );
  press({ type: "favourite", id: "m" });
  press({ type: "reply", id: "m" });
  await Bun.sleep(0);
  expect(requests).toContain("POST /statuses/m/favourite");
  expect(composed).toEqual([{ kind: "reply", id: "m" }]);
});
