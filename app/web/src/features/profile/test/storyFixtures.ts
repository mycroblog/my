// SPDX-License-Identifier: AGPL-3.0-only
// The data of the profile screen stories (profile.stories.tsx, followLists.stories.tsx): accounts with long and varied
// names, profile text and fields, and a few posts, answered by handlers that override the feature's defaults.
import { http, HttpResponse } from "msw";
import {
  accountFixture,
  longDomainAccountFixture,
  longNameAccountFixture,
  otherAccountFixture,
  relationshipFixture,
  rtlNameAccountFixture,
  type Schemas,
  statusPage,
  timelineStatusFixture,
} from "../../../test/fixtures";
import { apiBase, problem } from "../../../test/msw/http";

/** Bob with profile text, three fields, a badge and the counts of an active account. */
export const richBob: Schemas["Account"] = {
  ...otherAccountFixture,
  display_name: "Bob the Baker of Sourdough and Long Display Names",
  note: "<p>I bake bread at dawn and write about it after. Rye, spelt, and the occasional disaster.</p><p>Opinions are my starter's own.</p>",
  fields: [
    { name: "Website", value: '<a href="https://bakery.example/" rel="me nofollow noopener">bakery.example</a>' },
    { name: "Pronouns", value: "he/him" },
    { name: "Favourite flour", value: "Stone-ground whole wheat from the mill down the road" },
  ],
  badges: [{ role_id: "0192f0c4-7a8e-7000-8000-0000000000b1", label: "Moderator", color: "blue" }],
  followers_count: 1284,
  following_count: 311,
};

/** A few posts of `account`, varied in length. */
export function postsOf(account: Schemas["Account"]): Schemas["Status"][] {
  const texts = [
    "<p>The first loaf of the week came out of the oven with an ear you could hang a coat on.</p>",
    "<p>Short one.</p>",
    "<p>A longer thought about hydration: at 80% the dough is sticky, at 85% it is a puddle, and somewhere in between there is a loaf that makes every failed attempt worth it. Write down what you did, every time, even when it goes well.</p>",
  ];
  return texts.map((content, i) => ({
    ...timelineStatusFixture,
    id: `0192f0c4-7a8e-7000-8000-0000000005${String(i).padStart(2, "0")}`,
    uri: `https://${account.domain}/ap/statuses/${String(i)}`,
    url: `https://${account.domain}/@${account.username}/status/${String(i)}`,
    account,
    content,
    created_at: `2026-10-${String(16 - i).padStart(2, "0")}T09:00:00Z`,
    replies_count: i * 3,
    boosts_count: i * 7,
    favourites_count: i * 12,
  }));
}

/** GET /accounts/lookup finding `account` for its acct, and nothing else. */
export function lookingUp(account: Schemas["Account"]) {
  return http.get(`${apiBase}/accounts/lookup`, ({ request }) =>
    (new URL(request.url).searchParams.get("acct") ?? "").replace(/^@/, "").toLowerCase() === account.acct
      ? HttpResponse.json(account)
      : problem(404, "about:blank", "Not Found"),
  );
}

/** The posts of every account are `posts`; it pinned the first of them. */
export function posting(posts: Schemas["Status"][]) {
  return http.get(`${apiBase}/accounts/:id/statuses`, ({ request }) =>
    HttpResponse.json(
      statusPage(new URL(request.url).searchParams.get("pinned") === "true" ? posts.slice(0, 1) : posts),
    ),
  );
}

/** The viewer's relationship with every account is `overrides`. */
export function relatedAs(overrides: Partial<Schemas["Relationship"]>) {
  return http.get(`${apiBase}/accounts/relationships`, ({ request }) =>
    HttpResponse.json(new URL(request.url).searchParams.getAll("id").map((id) => relationshipFixture(id, overrides))),
  );
}

/** Twelve accounts of a follow list: long names, right-to-left names, long domains and plain ones. */
export const manyAccounts: Schemas["Account"][] = [
  longNameAccountFixture,
  rtlNameAccountFixture,
  longDomainAccountFixture,
  ...["Dana", "Eiji", "Fatima", "Gustav", "Hanna", "Ines", "Jun", "Kofi", "Lena"].map((name, i) =>
    accountFixture(40 + i, name, i % 3 === 0 ? "my.example" : "social.example"),
  ),
].map((account, i) => ({
  ...account,
  note: i % 2 === 0 ? "<p>Bread, books, and the long walk home.</p>" : "",
}));
