// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario, type FakeOptions, type FakeSeed } from "@my/e2e/fake";
import { PostActions, type ComposeRequest } from "../post-actions";
import { press } from "../test/actions";
import { fakeData, markupOf } from "../test/feature";
import { loadNotifications, Notifications } from "./notifications";

type Post = FakeSeed["posts"][number];

/** marco's post that mentions the reader, with `more`. */
const mention = (more: Partial<Post> = {}): Post => ({
  key: "m",
  author: "marco",
  text: "@ren Are you coming on Saturday?",
  visibility: "public",
  language: "en",
  ...more,
});

/**
 * A server as ren reads it, whose follows need their approval: sam asked to follow them, marco mentioned them, and
 * hana favourited their post, the newest; they have not read the last two. The server's policy holds back three
 * notifications of two people, and the list ends with a notice of the moderators and a notification of a kind a
 * newer server sends, which this client does not know.
 */
const answers = (posts: Post[] = [mention()]): FakeOptions => ({
  viewer: "ren",
  seed: {
    people: scenario.people,
    follows: [{ follower: "sam", followee: "ren" }],
    posts: [{ key: "y", author: "ren", text: "Bread tomorrow.", visibility: "public", language: "en" }, ...posts],
    interactions: [{ kind: "favourite", by: "hana", post: "y" }],
  },
  held: true,
  unreadNotifications: 2,
});

function server(options = answers()) {
  const made = fakeData(options);
  made.fake.script("GET /notifications", {
    change: (page) => ({
      ...page,
      items: [
        ...page.items,
        { id: "00000000-0000-7000-8000-000000000002", type: "moderation_notice", created_at: "2026-11-13T01:00:00Z" },
        {
          id: "00000000-0000-7000-8000-000000000001",
          // No kind of the specification: one a server newer than this client sends.
          type: "some_new_kind" as never,
          created_at: "2026-11-12T01:00:00Z",
        },
      ],
    }),
  });
  return made;
}

async function shown(locale = "en") {
  const { data, requests } = server();
  await loadNotifications(data);
  return { html: markupOf(data, <Notifications instance="my.example" />, locale), requests };
}

test("the loader reads the notifications, the read marker, the count of the requests, what waits for approval and the follow requests, then what the prompt of Web Push reads", async () => {
  const { requests } = await shown();
  expect(requests.map((r) => r.split("?")[0])).toEqual([
    "GET /notifications",
    "GET /notifications/marker",
    "GET /notifications/requests/summary",
    "GET /interaction_requests",
    "GET /follow_requests",
    "GET /push/subscription",
    "GET /preferences/notifications",
  ]);
});

test("each notification newer than the marker carries its own unread mark, the others none", async () => {
  const { html } = await shown();
  expect(html.match(/aria-label="Unread"/g)).toHaveLength(2);
  const favourite = html.indexOf("favourited your post");
  const mentioned = html.indexOf("mentioned you");
  const requested = html.indexOf("requested to follow you");
  expect(favourite).toBeGreaterThan(-1);
  expect([favourite < mentioned, mentioned < requested]).toEqual([true, true]);
  // The favourite's mark stands by its time, before its sentence; the mention's with its post, after its sentence.
  expect(html.slice(0, favourite).match(/aria-label="Unread"/g)).toHaveLength(1);
  expect(html.slice(mentioned, requested).match(/aria-label="Unread"/g)).toHaveLength(1);
  expect(html.slice(requested)).not.toContain('aria-label="Unread"');
});

test("a sentence names the account strong, in the server's words, and a kind the client does not know says there is one", async () => {
  const { html } = await shown();
  expect(html).toMatch(/<bdi class="[^"]*font-strong[^"]*">Marco<\/bdi> mentioned you/);
  expect(html).toContain("The moderators of my.example sent you a notice");
  expect(html).toContain("You have a new notification on my.example");
  const ja = (await shown("ja")).html;
  expect(ja).toContain("さんがあなたをメンションしました");
});

test("over the list, the tabs, mark-all-read, and the row of the requests", async () => {
  const { html } = await shown();
  expect(html).toMatch(/aria-label="Show" role="tablist"/);
  expect(html).toContain("Mark all as read");
  expect(html).toContain("3 held-back notifications · from 2 people");
});

test("a row over the list counts the follow requests and opens their screen; a request's notification has no buttons", async () => {
  const { fake, data } = server();
  await loadNotifications(data);
  await data.queryClient.query(
    data.queries.queryOptions("get", "/accounts/relationships", {
      params: { query: { id: [fake.accountId("sam")] } },
    }),
  );
  const html = markupOf(data, <Notifications instance="my.example" />);
  expect(html).toMatch(/href="\/notifications\/follow_requests"[^>]*>.*1 follow request/);
  expect(html).not.toContain(">Decline<");
});

test("over the list, a row counts what waits for the reader's approval; a request's notification has no buttons", async () => {
  // A post of yuki's waits for their answers to marco's reply and boost and mio's quote (the fake's `approvals`).
  const { data } = fakeData({ approvals: true });
  await loadNotifications(data);
  const html = markupOf(data, <Notifications instance="my.example" />);
  expect(html).toContain("3 waiting for your approval");
  const asked = html.indexOf("</bdi> asked to reply to your post");
  expect(html.indexOf("3 waiting for your approval")).toBeLessThan(asked);
  // Up to the next notification's sentence.
  expect(html.slice(asked, html.indexOf("<bdi", asked + 1))).not.toContain("<button");
  const ja = markupOf(data, <Notifications instance="my.example" />, "ja");
  expect(ja).toContain("承認待ち 3件");
});

test("without a request waiting, no row counts them", async () => {
  const { html } = await shown();
  expect(html).not.toContain("waiting for your approval");
});

test("the post of a mention answers its actions: a favourite reaches the server, Reply opens the composer", async () => {
  const { fake, data, requests } = server();
  const id = fake.idOf("m");
  const favourite = fake.script("POST /statuses/{id}/favourite");
  await loadNotifications(data);
  const composed: ComposeRequest[] = [];
  markupOf(
    data,
    <PostActions onCompose={(to) => composed.push(to)}>
      <Notifications instance="my.example" />
    </PostActions>,
  );
  press({ type: "favourite", id });
  press({ type: "reply", id });
  await favourite.arrived();
  expect(requests).toContain(`POST /statuses/${id}/favourite`);
  expect(composed).toEqual([{ kind: "reply", id }]);
});

test("a vote in the poll of a mentioning post reaches the server", async () => {
  const { fake, data } = server(answers([mention({ poll: { options: ["Yes", "No"], days: 1 } })]));
  const id = fake.idOf("m");
  const sent = fake.script("POST /statuses/{id}/poll/votes");
  await loadNotifications(data);
  const { data: post } = await data.api.GET("/statuses/{id}", { params: { path: { id } } });
  const poll = post?.poll?.id ?? "";
  markupOf(
    data,
    <PostActions onCompose={() => undefined}>
      <Notifications instance="my.example" />
    </PostActions>,
  );
  press({ type: "vote", id, poll, choices: [1] });
  expect((await sent.arrived()).body).toEqual({ poll_id: poll, choices: [1] });
});
