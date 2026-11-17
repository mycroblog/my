// SPDX-License-Identifier: AGPL-3.0-only

// A profile as a feature: what its loader reads, what its head says of the account and of the reader's relationship
// with it, and what a press on one of its posts asks of the server or of the screen.

import { expect, test } from "bun:test";
import { homeTimelineQuery } from "@my/app-data";
import { scenario, type FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { PostActions, type ComposeRequest } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
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

/**
 * The profile of `acct` on a server of `options` (the scenario by default), loaded and drawn: its head, once it says
 * `settled`, which only the reader's relationship with the account brings.
 */
async function shown(acct: string, options: FakeOptions, settled: () => HTMLElement) {
  const made = fakeData(options);
  await loadProfile(made.data, acct);
  const composed: ComposeRequest[] = [];
  const user = renderFeature(
    made.data,
    <PostActions onCompose={(to) => composed.push(to)}>
      <Profile acct={acct} />
    </PostActions>,
  );
  await waitFor(settled);
  const head = within(screen.getByRole("region"));
  return { ...made, user, composed, head, buttons: () => head.queryAllByRole("button").map(nameOf) };
}

const nameOf = (element: HTMLElement) => element.getAttribute("aria-label") ?? element.textContent;
const button = (name: string) => () => screen.getByRole("button", { name });
/** A count of the head, by its label. */
const count = (label: string) =>
  screen
    .getByRole("link", { name: new RegExp(`^${label}`) })
    .textContent.replace(label, "")
    .trim();

test("the loader reads the account of the handle and the viewer's own", async () => {
  const { data, requests } = fakeData();

  await loadProfile(data, sora);

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
  const home = homeTimelineQuery(data).queryKey;
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  // The home timeline was fetched a minute ago: its copy of hana is stale, and others followed hana since.
  data.queryClient.setQueryData(home, (pages) => pages, { updatedAt: Date.now() - 60_000 });
  fake.script("GET /accounts/{id}", { change: (account) => ({ ...account, followers_count: 30 }) });
  const relationship = fake.script("GET /accounts/relationships", { hold: "request" });
  requests.length = 0;

  await loadProfile(data, "hana");
  expect(requests).toEqual(["GET /accounts/verify_credentials"]);
  renderFeature(data, <Profile acct="hana" />);

  // No wait: the head is the post's copy of the account, with its note and its fields; what the reader's
  // relationship adds is still asked for.
  const head = within(screen.getByRole("region", { name: "Hana 花" }));
  expect(head.getByText(scenario.people.find((p) => p.username === "hana")?.note ?? "")).not.toBeNull();
  expect(head.getAllByRole("term").map((term) => term.textContent)).toContain("役割");
  expect(count("Followers")).toBe("1");
  await relationship.arrived();
  expect(head.queryAllByRole("button", { name: "Following" })).toHaveLength(0);
  relationship.release();
  expect(await head.findByRole("button", { name: "Following" })).not.toBeNull();
  await waitFor(() => {
    if (count("Followers") !== "30") throw new Error(`the head counts ${count("Followers")} followers`);
  });
  expect(requests).toContain(`GET /accounts/${fake.accountId("hana")}`);
});

test("a remote account the reader follows: Following, Follows you, its note, its fields, its counts and the line of its server", async () => {
  const { head } = await shown(sora, followingSora, button("Following"));

  expect(head.getByText("Follows you")).not.toBeNull();
  expect(head.getByText(/^Film photographer on another server\./)).not.toBeNull();
  expect(head.getAllByRole("term").map((term) => term.textContent)).toContain("Website");
  expect(count("Followers")).toBe("1,285");
  // After a full sentence, the action follows with no dot between them.
  const line = head.getByText("Information from photos.example may be incomplete.");
  expect(line.parentElement?.textContent).toMatch(
    /^Information from photos\.example may be incomplete\.\s?Open original page$/,
  );
  expect(head.getByRole("button", { name: "Open original page" })).not.toBeNull();
  expect(screen.getByRole("tablist", { name: "Posts of the account" })).not.toBeNull();
});

test("an account that follows the reader asks to follow back", async () => {
  // mio follows the reader, who does not follow mio.
  const { buttons } = await shown("mio", {}, button("Follow back"));

  expect(buttons()).not.toContain("Follow");
});

test("a locked account asks to request to follow, and says that its follow requests need approval", async () => {
  const { head } = await shown("ren", strangers(), button("Request to follow"));

  expect(head.getByRole("img", { name: "Follow requests need approval" })).not.toBeNull();
});

test("a blocked account shows its head and no posts, and offers to unblock", async () => {
  // The reader blocks quickdeals.
  const { head } = await shown("quickdeals", {}, button("Unblock"));

  expect(head.getByText("You blocked this account.")).not.toBeNull();
  expect(screen.getByRole("status").textContent).toBe("Its posts are hidden while you block it.");
  expect(screen.queryAllByRole("tablist")).toHaveLength(0);
  expect(screen.queryAllByRole("feed")).toHaveLength(0);
});

test("a muted account says so, with an unmute", async () => {
  const { head } = await shown("sam", strangers(true), button("Unmute"));

  // After a full sentence, the action follows with no dot between them.
  const line = head.getByText("You muted this account. Its posts stay out of your timelines.");
  expect(line.parentElement?.textContent).toMatch(
    /^You muted this account\. Its posts stay out of your timelines\.\s?Unmute$/,
  );
});

test("the reader's own profile offers no follow and no menu of what to do with the account", async () => {
  const { buttons } = await shown("yuki", {}, () => screen.getByRole("feed", { name: "Posts of the account" }));

  expect(buttons().filter((name) => /Follow|More|Unblock|Unmute/.test(name))).toEqual([]);
});

test("a post and a pinned one answer their actions: a favourite reaches the server, Reply opens the composer", async () => {
  // hana pinned her welcome, and posted her slides later.
  const { fake, user, composed } = await shown("hana", {}, () => screen.getByText("Pinned"));
  const [pinned, later] = ["hana-welcome", "hana-slides"].map(fake.idOf) as [string, string];
  const favourite = fake.script("POST /statuses/{id}/favourite");
  const posts = within(screen.getByRole("feed", { name: "Posts of the account" })).getAllByRole("article");
  const first = posts.find((post) => within(post).queryAllByText("Pinned").length > 0);
  const slides = posts.find((post) => within(post).queryAllByRole("link", { name: /^slides\.key/ }).length > 0);
  if (first === undefined || slides === undefined)
    throw new Error("test: the profile shows no pinned post or no slides");

  expect(posts[0] === first).toBe(true);
  await user.click(within(slides).getByRole("button", { name: /^Favourite/ }));
  expect((await favourite.arrived()).url.pathname).toEndWith(`/statuses/${later}/favourite`);
  await user.click(within(first).getByRole("button", { name: /^Reply/ }));

  expect(composed).toEqual([{ kind: "reply", id: pinned }]);
});

test("a vote in the poll of a post on the profile reaches the server", async () => {
  // sam asks which loaf to bake, in a poll the reader has not voted in.
  const { fake, user } = await shown("sam", {}, () => screen.getByRole("group", { name: "Poll" }));
  const sent = fake.script("POST /statuses/{id}/poll/votes");
  const poll = within(screen.getByRole("group", { name: "Poll" }));
  const [, second] = poll.getAllByRole("radio");
  if (second === undefined) throw new Error("test: the poll has no second option");

  await user.click(second);
  await user.click(poll.getByRole("button", { name: "Vote" }));

  const { url, body } = await sent.arrived();
  expect(url.pathname).toEndWith(`/statuses/${fake.idOf("sam-poll")}/poll/votes`);
  expect(body.choices).toEqual([1]);
  await waitFor(() => {
    if (poll.queryAllByText(/your vote/).length !== 1) throw new Error("the poll shows no vote of the reader's");
  });
});

test("a moved account says where it moved to, links to the new one, and offers no follow", async () => {
  const { head, buttons } = await shown("sora@social.example", {}, () =>
    screen.getByRole("link", { name: "Go to new account" }),
  );

  expect(head.getByRole("link", { name: "Go to new account" }).getAttribute("href")).toBe("/@sora@photos.example");
  const line = head
    .getAllByRole("paragraph")
    .find((paragraph) => /^Moved to\s*⁨?@sora@photos\.example⁩?$/.test(paragraph.textContent.trim()));
  if (line === undefined) throw new Error("test: no line says where the account moved to");
  expect(within(line).getByRole("link", { name: "@sora@photos.example" }).getAttribute("href")).toBe(
    "/@sora@photos.example",
  );
  expect(buttons().filter((name) => /^(Follow|Follow back|Request to follow|Following)$/.test(name))).toEqual([]);
});

test("the account it moved to can be followed from its profile", async () => {
  const { fake, user, head } = await shown("sora@photos.example", {}, button("Follow back"));
  const followed = fake.script("POST /accounts/{id}/follow");

  await user.click(head.getByRole("button", { name: "Follow back" }));

  expect((await followed.arrived()).url.pathname).toEndWith(
    `/accounts/${fake.accountId("sora@photos.example")}/follow`,
  );
  expect(await head.findByRole("button", { name: "Following" })).not.toBeNull();
});
