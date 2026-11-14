// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { account, render, server, status } from "../test/server";
import { loadProfile, Profile } from "./profile";

const alice = account("alice", {
  acct: "alice@example.social",
  domain: "example.social",
  note: "<p>Runs a small server.</p>",
  fields: [{ name: "Website", value: "<p>alice.example.social</p>" }],
  followers_count: 2910,
  following_count: 380,
});

/** The profile of `who` as `relationship` makes it, with a post and a pinned one. */
async function shown(who: Record<string, unknown>, relationship: Record<string, unknown>) {
  const id = who["id"] as string;
  const { data, requests } = server({
    "/accounts/lookup": who,
    "/accounts/verify_credentials": account("kumi"),
    "/accounts/relationships": { items: [{ id, ...relationship }] },
    [`/accounts/${id}/statuses`]: { items: [status("p", id)] },
    [`/accounts/${id}/featured_tags`]: { items: [{ name: "books", url: "", statuses_count: 64 }] },
  });
  await loadProfile(data, who["acct"] as string);
  await data.queryClient.query(
    data.queries.queryOptions("get", "/accounts/relationships", { params: { query: { id: [id] } } }),
  );
  return { html: render(data, <Profile acct={who["acct"] as string} />), requests };
}

test("the loader reads the account of the handle and the viewer's own", async () => {
  const { requests } = await shown(alice, {});
  expect(requests.slice(0, 2)).toEqual([
    "GET /accounts/lookup?acct=alice%40example.social&resolve=true",
    "GET /accounts/verify_credentials",
  ]);
});

test("a remote account the reader follows: Following, Follows you, its counts and the line of its server", async () => {
  const { html } = await shown(alice, { following: true, followed_by: true });
  expect(html).toContain(">Following<");
  expect(html).toContain("Follows you");
  expect(html).toContain("Runs a small server.");
  expect(html).toContain("Website");
  expect(html).toMatch(/Followers <strong[^>]*>2,910<\/strong>/);
  expect(html).toContain("Information from example.social may be incomplete.");
  expect(html).toMatch(/aria-label="Posts of the account" role="tablist"/);
});

test("an account that follows the reader asks to follow back, in the primary color", async () => {
  const { html } = await shown(account("hana"), { followed_by: true });
  expect(html).toMatch(/bg-accent[^"]*"[^>]*>(<span[^>]*>)?Follow back/);
});

test("a locked account asks to request to follow, and says that its followers alone see its other posts", async () => {
  const { html } = await shown(account("takuya", { locked: true }), {});
  expect(html).toContain("Request to follow");
  expect(html).toContain('aria-label="Follow requests need approval"');
});

test("a blocked account shows its head and no posts, and offers to unblock", async () => {
  const { html } = await shown(account("ravi"), { blocking: true });
  expect(html).toContain("You blocked this account.");
  expect(html).toContain("Its posts are hidden while you block it.");
  expect(html).toContain(">Unblock<");
  expect(html).not.toContain('role="tablist"');
});

test("a muted account says so, with an unmute", async () => {
  const { html } = await shown(account("lena"), { muting: true, following: true });
  expect(html).toContain("You muted this account. Its posts stay out of your timelines.");
  expect(html).toContain("Unmute");
});

test("the reader's own profile offers no follow", async () => {
  const { html } = await shown(account("kumi"), {});
  expect(html).not.toContain(">Follow<");
  expect(html).not.toContain('aria-label="More"');
});
