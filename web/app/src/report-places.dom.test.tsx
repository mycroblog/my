// SPDX-License-Identifier: AGPL-3.0-only

// A reader reports from each of the five places that offer it (FR-701, #3259): a post's menu, a profile's, a
// notification's, a voice chat room's and an event's. Each report says why (a break of the server's rules), which rule
// and a comment, and reaches the server with what the place points at: the post, the room or the event, and the
// account behind it. Once resolved, the reader's notification says so and nothing of how. What the panel shows in each
// state is the stories' (app-components report.stories.tsx); what it offers once sent, the feature's
// (app-features report.dom.test.tsx).

import { expect, test } from "bun:test";
import { fakeOrigin, scenario, type FakeApi } from "@my/e2e/fake";
import { screen, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { findPost, openMore } from "./test/posts";
import { fakeApp } from "./test/server";

const comment = "Shouting at the guests, again.";

/** The ID of the server's rule whose text is `text`, as the instance lists it. */
async function ruleId(fake: FakeApi, text: string): Promise<string> {
  const answer = await fake.serve(new Request(`${fakeOrigin}/api/v1/instance`));
  const instance = (await answer.json()) as { rules: { id: string; text: string }[] };
  const rule = instance.rules.find((r) => r.text === text);
  if (rule === undefined) throw new Error(`test: the server has no rule ${text}`);
  return rule.id;
}

/**
 * Fills the open panel titled `title` as a break of the rule "Be kind to each other." with the comment, sends it, and
 * waits for the panel that says it was sent; returns what the server was sent.
 */
async function fileFrom(app: TestApp, title: RegExp) {
  const sent = app.fake.script("POST /reports");
  const panel = within(await screen.findByRole("dialog", { name: title }));
  await app.user.click(
    within(panel.getByRole("radiogroup", { name: "Why are you reporting this?" })).getByRole("radio", {
      name: /^Breaks the server's rules/,
    }),
  );
  await app.user.click(
    within(await panel.findByRole("group", { name: "Which rules does this break?" })).getByRole("checkbox", {
      name: /^Be kind to each other\./,
    }),
  );
  await app.user.click(panel.getByRole("textbox", { name: "Comment for the moderators" }));
  await app.user.paste(comment);
  await app.user.click(panel.getByRole("button", { name: "Send report" }));
  const { body } = await sent.arrived();
  await screen.findByRole("dialog", { name: "Report sent" });
  return { body, rule: await ruleId(app.fake, "Be kind to each other.") };
}

/** Chooses Report… of the menu that is open (Report the room…, Report the event… for a place). */
async function chooseReport(app: TestApp, menu: HTMLElement, name = "Report…") {
  await app.user.click(within(menu).getByRole("menuitem", { name }));
}

test("from a post's menu, the report names its author and the post, with the category, the rule and the comment", async () => {
  const app = await renderApp("/", {
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: [{ key: "loaf", author: "sam", text: "The first loaf.", visibility: "public", language: "en" }],
    },
  });
  await chooseReport(app, await openMore(app, await findPost("Sam Rivera", "The first loaf.")));

  const { body, rule } = await fileFrom(app, /^Report .*@sam/);

  expect(body).toMatchObject({
    account_id: app.fake.accountId("sam"),
    status_ids: [app.fake.idOf("loaf")],
    category: "violation",
    rule_ids: [rule],
    comment,
  });
  expect(body.voice_room_id).toBeUndefined();
  expect(body.event_id).toBeUndefined();
});

test("from a profile's menu, the report names the account and no post until one is chosen", async () => {
  const app = await renderApp("/@sam");
  const card = within(await screen.findByRole("region", { name: "Sam Rivera" }));
  await app.user.click(await card.findByRole("button", { name: "More" }));
  await chooseReport(app, await screen.findByRole("menu", { name: "More" }));

  const { body, rule } = await fileFrom(app, /^Report .*@sam/);

  expect(body).toMatchObject({
    account_id: app.fake.accountId("sam"),
    category: "violation",
    rule_ids: [rule],
    comment,
  });
  expect(body.status_ids ?? []).toEqual([]);
});

test("from a notification's menu, the report names the account that acted, and not the reader's own post it is about", async () => {
  const app = await renderApp("/notifications", {
    seed: {
      people: scenario.people,
      posts: [{ key: "ridge", author: "yuki", text: "The ridge at dawn.", visibility: "public", language: "en" }],
      interactions: [{ kind: "reaction", by: "layla", post: "ridge", emoji: "🌄" }],
    },
  });
  const row = within(await main().findByRole("article", { name: /Layla reacted to your post$/ }));
  await app.user.click(row.getByRole("button", { name: "More actions for this notification" }));
  await chooseReport(app, await screen.findByRole("menu", { name: "More actions for this notification" }));

  const { body, rule } = await fileFrom(app, /^Report .*@layla/);

  expect(body).toMatchObject({
    account_id: app.fake.accountId("layla"),
    category: "violation",
    rule_ids: [rule],
    comment,
  });
  expect(body.status_ids ?? []).toEqual([]);
});

test("from a voice chat room's menu, the report names the room and its host, and no post", async () => {
  const on = fakeApp();
  const room = on.fake.voice.roomId(0);
  const app = await renderApp(`/voice/${room}`, { on });
  await app.user.click(await main().findByRole("button", { name: "More" }));
  await chooseReport(app, await screen.findByRole("menu", { name: "More" }), "Report the room…");

  const { body, rule } = await fileFrom(app, /^Report the room .*Morning coffee chat/);

  expect(body).toMatchObject({
    account_id: app.fake.accountId("sam"),
    voice_room_id: room,
    category: "violation",
    rule_ids: [rule],
    comment,
  });
  expect(body.status_ids ?? []).toEqual([]);
  expect(body.event_id).toBeUndefined();
});

test("from an event's menu, the report names the event and its organizer, and no post", async () => {
  const on = fakeApp();
  const event = on.fake.events.id("book-swap");
  const app = await renderApp(`/events/${event}`, { on });
  await app.user.click(await main().findByRole("button", { name: "More" }));
  await chooseReport(app, await screen.findByRole("menu", { name: "More" }), "Report the event…");

  const { body, rule } = await fileFrom(app, /^Report the event .*Book swap/);

  expect(body).toMatchObject({
    account_id: app.fake.accountId("mio"),
    event_id: event,
    category: "violation",
    rule_ids: [rule],
    comment,
  });
  expect(body.status_ids ?? []).toEqual([]);
  expect(body.voice_room_id).toBeUndefined();
});

test("the reader is told that a report of theirs was resolved, and nothing of how, why or about whom", async () => {
  const on = fakeApp({ everyKind: "notices" });
  on.fake.script("GET /notifications", {
    change: (page) => ({
      ...page,
      items: page.items.map((n) =>
        n.type === "report_resolved" && n.report !== undefined
          ? { ...n, report: { ...n.report, category: "spam" as const, comment: "Cheap followers in every reply." } }
          : n,
      ),
    }),
  });
  await renderApp("/notifications", { on });

  const row = await main().findByRole("article", { name: /^A report you filed on demo\.test has been resolved$/ });

  const said = words(row);
  expect(said).toMatch(/A report you filed on demo\.test has been resolved/);
  expect(said).not.toMatch(/Cheap followers|[Ss]pam|action|dismiss|@/);
  const leads = within(row)
    .queryAllByRole("link")
    .map((link) => link.getAttribute("href") ?? "");
  expect(leads.filter((href) => href.startsWith("/moderation"))).toEqual([]);
});
