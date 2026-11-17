// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { accountByIdQuery, accountStatusesQuery, homeTimelineQuery } from "@my/app-data";
import { scenario, type FakeOptions } from "@my/e2e/fake";
import { PostActions, type ComposeRequest } from "../post-actions";
import { press } from "../test/actions";
import { fakeData, markupOf } from "../test/feature";
import { loadProfile, Profile } from "./profile";

const sora = "sora@photos.example";

/** The scenario, where sora's profile on another server names a website, and the reader follows sora back. */
const followingSora: FakeOptions = {
  seed: {
    ...scenario,
    people: scenario.people.map((p) =>
      p.username === sora ? { ...p, fields: [["Website", "https://sora.photos.example"]] } : p,
    ),
    follows: [...scenario.follows, { follower: "yuki", followee: sora }],
  },
};

/** A server where nobody follows anybody, and the reader mutes sam, whom they follow, when `muted`. */
const strangers = (muted = false): FakeOptions => ({
  seed: {
    people: scenario.people,
    follows: muted ? [{ follower: "yuki", followee: "sam" }] : [],
    restrictions: muted ? [{ kind: "mute", by: "yuki", target: "sam" }] : [],
  },
});

/** The profile of `acct` on a server of `options` (the scenario by default), as its screen reads it. */
async function shown(acct: string, options: FakeOptions = {}) {
  const { fake, data, requests } = fakeData(options);
  const id = fake.accountId(acct);
  await loadProfile(data, acct);
  await data.queryClient.query(
    data.queries.queryOptions("get", "/accounts/relationships", { params: { query: { id: [id] } } }),
  );
  return { html: markupOf(data, <Profile acct={acct} />), requests };
}

test("the loader reads the account of the handle and the viewer's own", async () => {
  const { requests } = await shown(sora);
  expect(requests.slice(0, 2)).toEqual([
    "GET /accounts/lookup?acct=sora%40photos.example&resolve=true",
    "GET /accounts/verify_credentials",
  ]);
});

test("the profile of a post's author shows at once from the post's copy, and the counts the account's fetch brings", async () => {
  // The reader follows hana alone, whose one post is their home timeline.
  const { fake, data, requests } = fakeData({
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "hana" }],
      posts: [{ key: "post", author: "hana", text: "Welcome.", visibility: "public", language: "en" }],
    },
  });
  const hana = fake.accountId("hana");
  const home = homeTimelineQuery(data).queryKey;
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  // The home timeline was fetched a minute ago: its copy of hana is stale, and others followed hana since.
  data.queryClient.setQueryData(home, (pages) => pages, { updatedAt: Date.now() - 60_000 });
  fake.script("GET /accounts/{id}", { change: (account) => ({ ...account, followers_count: 30 }) });
  requests.length = 0;
  await loadProfile(data, "hana");
  expect(requests).toEqual(["GET /accounts/verify_credentials"]);
  expect(markupOf(data, <Profile acct="hana" />)).toMatch(/Followers <strong[^>]*>1<\/strong>/);
  await data.queryClient.query(accountByIdQuery(data, hana));
  expect(requests).toEqual(["GET /accounts/verify_credentials", `GET /accounts/${hana}`]);
  expect(markupOf(data, <Profile acct="hana" />)).toMatch(/Followers <strong[^>]*>30<\/strong>/);
});

test("a remote account the reader follows: Following, Follows you, its counts and the line of its server", async () => {
  const { html } = await shown(sora, followingSora);
  expect(html).toContain(">Following<");
  expect(html).toContain("Follows you");
  expect(html).toContain("Film photographer on another server.");
  expect(html).toContain("Website");
  expect(html).toMatch(/Followers <strong[^>]*>1,285<\/strong>/);
  // After a full sentence, no dot before the action.
  expect(html).toContain("Information from photos.example may be incomplete. <span");
  expect(html).toMatch(/aria-label="Posts of the account" role="tablist"/);
});

test("an account that follows the reader asks to follow back, in the primary color", async () => {
  // mio follows the reader, who does not follow mio.
  const { html } = await shown("mio");
  expect(html).toMatch(/bg-accent[^"]*"[^>]*>(<span[^>]*>)?Follow back/);
});

test("a locked account asks to request to follow, and says that its followers alone see its other posts", async () => {
  const { html } = await shown("ren", strangers());
  expect(html).toContain("Request to follow");
  expect(html).toContain('aria-label="Follow requests need approval"');
});

test("a blocked account shows its head and no posts, and offers to unblock", async () => {
  // The reader blocks quickdeals.
  const { html } = await shown("quickdeals");
  expect(html).toContain("You blocked this account.");
  expect(html).toContain("Its posts are hidden while you block it.");
  expect(html).toContain(">Unblock<");
  expect(html).not.toContain('role="tablist"');
});

test("a muted account says so, with an unmute", async () => {
  const { html } = await shown("sam", strangers(true));
  // After a full sentence, no dot before the action.
  expect(html).toContain("You muted this account. Its posts stay out of your timelines. <span");
  expect(html).toContain("Unmute");
});

test("the reader's own profile offers no follow", async () => {
  const { html } = await shown("yuki");
  expect(html).not.toContain(">Follow<");
  expect(html).not.toContain('aria-label="More"');
});

test("a post and a pinned one answer their actions: a favourite reaches the server, Reply opens the composer", async () => {
  // hana pinned her welcome, and posted her slides later.
  const { fake, data, requests } = fakeData();
  const hana = fake.accountId("hana");
  const [pinned, later] = ["hana-welcome", "hana-slides"].map(fake.idOf) as [string, string];
  const favourite = fake.script("POST /statuses/{id}/favourite");
  await loadProfile(data, "hana");
  await data.queryClient.infiniteQuery(accountStatusesQuery(data, hana, "posts"));
  await data.queryClient.infiniteQuery(accountStatusesQuery(data, hana, "pinned"));
  const composed: ComposeRequest[] = [];
  const html = markupOf(
    data,
    <PostActions onCompose={(to) => composed.push(to)}>
      <Profile acct="hana" />
    </PostActions>,
  );
  expect(html).toContain("Pinned");
  press({ type: "favourite", id: later });
  press({ type: "reply", id: pinned });
  await favourite.arrived();
  expect(requests).toContain(`POST /statuses/${later}/favourite`);
  expect(composed).toEqual([{ kind: "reply", id: pinned }]);
});

test("a vote in the poll of a post on the profile reaches the server", async () => {
  // sam asks which loaf to bake, in a poll the reader has not voted in.
  const { fake, data } = fakeData();
  const sam = fake.accountId("sam");
  const id = fake.idOf("sam-poll");
  const sent = fake.script("POST /statuses/{id}/poll/votes");
  await loadProfile(data, "sam");
  const posts = await data.queryClient.infiniteQuery(accountStatusesQuery(data, sam, "posts"));
  const poll = posts.pages.flatMap((p) => p.items).find((s) => s.id === id)?.poll?.id ?? "";
  markupOf(
    data,
    <PostActions onCompose={() => undefined}>
      <Profile acct="sam" />
    </PostActions>,
  );
  press({ type: "vote", id, poll, choices: [1] });
  expect((await sent.arrived()).body).toEqual({ poll_id: poll, choices: [1] });
});
