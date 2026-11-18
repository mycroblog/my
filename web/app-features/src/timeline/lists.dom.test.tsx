// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { preferencesQuery, receiveListStatuses, type AppData } from "@my/app-data";
import { scenario, type FakeOptions } from "@my/e2e/fake";
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { PostActions } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
import { ListIndex, ListTimeline, loadLists, loadListTimeline } from "./lists";

/**
 * A server where the reader follows sam, hana and layla, and keeps the list Bread and coffee of sam and hana and
 * 翻訳の仲間 of layla; sam posted once, unless `posts` is false. `l1` and `l2` are the lists' IDs.
 */
function server({
  lists = true,
  posts = true,
  limit,
  preferences,
}: { lists?: boolean; posts?: boolean; limit?: number; preferences?: FakeOptions["preferences"] } = {}) {
  const { fake, data, requests } = fakeData({
    ...(limit === undefined ? {} : { listLimits: { lists: limit } }),
    ...(preferences === undefined ? {} : { preferences }),
    seed: {
      people: scenario.people,
      follows: ["sam", "hana", "layla"].map((followee) => ({ follower: "yuki", followee })),
      posts: posts ? [{ key: "p", author: "sam", text: "The first loaf.", visibility: "public", language: "en" }] : [],
      lists: lists
        ? [
            { key: "l1", owner: "yuki", name: "Bread and coffee", members: ["sam", "hana"] },
            { key: "l2", owner: "yuki", name: "翻訳の仲間", members: ["layla"] },
          ]
        : [],
    },
  });
  const [l1, l2] = [0, 1].map((i) => fake.listPath(i).replace("/lists/", "")) as [string, string];
  return { fake, data, requests, l1, l2 };
}

/** `node` on `data`, alone in the document, for a reader of `locale`: the reader's hands. */
function shown(data: AppData, node: ReactNode, locale = "en") {
  cleanup();
  return renderFeature(data, node, locale);
}

const timeline = (data: AppData, id: string, locale = "en", onBack: () => void = () => undefined) =>
  shown(
    data,
    <PostActions onCompose={() => undefined}>
      <ListTimeline id={id} onBack={onBack} />
    </PostActions>,
    locale,
  );

test("the index lists the reader's lists, each a link to its timeline with how many members it has", async () => {
  const { data, requests, l1, l2 } = server();
  await loadLists(data);
  expect(requests).toEqual(["GET /lists"]);

  shown(data, <ListIndex />);
  const links = within(screen.getByRole("list", { name: "Your lists" })).getAllByRole("link");
  expect(links.map((link) => link.getAttribute("href"))).toEqual([`/lists/${l1}`, `/lists/${l2}`]);
  expect(links[0]?.textContent).toMatch(/^Bread and coffee\s*2 members$/);
  expect(links[1]?.textContent).toMatch(/^翻訳の仲間\s*1 member$/);

  shown(data, <ListIndex />, "ja");
  const [first] = within(screen.getByRole("list", { name: "自分のリスト" })).getAllByRole("link");
  expect(first?.textContent).toMatch(/^Bread and coffee\s*2 人$/);
});

test("with no lists, the index says so, in en and ja", async () => {
  const { data } = server({ lists: false });
  await loadLists(data);

  shown(data, <ListIndex />);
  expect(screen.getByText("No lists yet. Your lists show here, each with the posts of its members.")).not.toBeNull();
  shown(data, <ListIndex />, "ja");
  expect(
    screen.getByText("まだリストはありません。リストはここに表示され、それぞれのメンバーの投稿を読めます。"),
  ).not.toBeNull();
});

test("the loader reads the list and its timeline; the screen shows its name, members and posts", async () => {
  const { data, requests, l1 } = server();
  await loadListTimeline(data, l1);
  expect(requests).toContain(`GET /lists/${l1}`);
  expect(requests).toContain(`GET /timelines/list/${l1}`);

  timeline(data, l1);
  const back = screen.getByRole("button", { name: "Lists" });
  const heading = screen.getByRole("heading", { level: 2 });
  expect(heading.textContent).toMatch(/^Bread and coffee\s*2 members$/);
  expect(back.compareDocumentPosition(heading) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  const feed = screen.getByRole("feed", { name: "Posts from the list Bread and coffee" });
  expect(within(feed).getByText("The first loaf.")).not.toBeNull();
  // A list has nothing to follow: no action in its head.
  expect(screen.queryAllByRole("button", { name: "Follow" })).toHaveLength(0);
});

test("a list's new posts wait behind the button for a reader who chose to hold them back, and go in by themselves otherwise", async () => {
  /** The list's feed for a reader who set `timeline_auto_insert` to `autoInsert`, after a post arrived on the stream. */
  async function afterAPost(autoInsert: boolean) {
    const { fake, data, l1 } = server({ preferences: { timeline_auto_insert: autoInsert } });
    await data.queryClient.query(preferencesQuery(data));
    await loadListTimeline(data, l1);
    const user = timeline(data, l1);
    const feed = within(screen.getByRole("feed", { name: "Posts from the list Bread and coffee" }));
    expect(feed.getAllByRole("article")).toHaveLength(1);
    const arrived = fake.newPosts(1);
    act(() => {
      receiveListStatuses(data, l1, arrived);
    });
    return { user, feed };
  }
  const inTheFeed = (feed: Awaited<ReturnType<typeof afterAPost>>["feed"], count: number) =>
    waitFor(() => {
      if (feed.getAllByRole("article").length !== count)
        throw new Error(`the feed does not show ${String(count)} posts`);
    });

  const held = await afterAPost(false);
  const button = await screen.findByRole("button", { name: "1 new post" });
  expect(held.feed.getAllByRole("article")).toHaveLength(1);
  await held.user.click(button);
  await inTheFeed(held.feed, 2);

  const inserted = await afterAPost(true);
  await inTheFeed(inserted.feed, 2);
  expect(screen.queryAllByRole("button", { name: /new posts?$/ })).toHaveLength(0);
});

test("a list whose members have not posted says so, in en and ja", async () => {
  const { data, l1 } = server({ posts: false });
  await loadListTimeline(data, l1);

  timeline(data, l1);
  expect(
    screen.getByText("No posts from this list's members yet. When they post, their posts show here."),
  ).not.toBeNull();
  timeline(data, l1, "ja");
  expect(
    screen.getByText("まだこのリストのメンバーの投稿はありません。メンバーが投稿すると、ここに表示されます。"),
  ).not.toBeNull();
});

test("a list that no longer exists says so", async () => {
  const { data } = server();
  // An ID of the server's form that no list has.
  const id = "01890a5d-ac96-774b-bcce-b302099a8057";
  await loadListTimeline(data, id);

  timeline(data, id);

  expect(await screen.findByText("This list no longer exists.")).not.toBeNull();
});

// Managing the lists (ADR-0013 §11–§15). That the server keeps a list made, renamed and deleted is the E2E tests'
// (web/e2e/specs/list-management.spec.ts).

test("New list puts the name's field in the button's place, with the focus on it; opened to create, the index shows the field at once", async () => {
  const { data } = server();
  await loadLists(data);
  const user = shown(data, <ListIndex />);

  await user.click(screen.getByRole("button", { name: "New list" }));

  const name = await screen.findByRole("textbox", { name: "List name" });
  await waitFor(() => {
    if (document.activeElement !== name) throw new Error("the name's field does not have the focus");
  });
  expect(screen.queryAllByRole("button", { name: "New list" })).toHaveLength(0);
  expect(screen.getAllByRole("button").map((button) => button.textContent)).toEqual(["Cancel", "Create"]);

  // As Add to list's New list opens the index.
  shown(data, <ListIndex create />);
  expect(screen.getByRole("textbox", { name: "List name" })).not.toBeNull();
  expect(screen.queryAllByRole("button", { name: "New list" })).toHaveLength(0);
});

test("a new list the server refuses at its limit of lists says so under the name, which stands invalid, and Create waits for a change", async () => {
  // The reader has two lists on a server that takes no more than two.
  const { fake, data } = server({ limit: 2 });
  await loadLists(data);
  const sent = fake.script("POST /lists");
  const user = shown(data, <ListIndex create />);
  const name = screen.getByRole("textbox", { name: "List name" });

  await user.type(name, "Friends");
  await user.click(screen.getByRole("button", { name: "Create" }));

  expect((await sent.arrived()).body.name).toBe("Friends");
  const said = await screen.findByText(/^You can have at most [\d,]+ lists\. Delete one first\.$/);
  expect(name.getAttribute("aria-invalid")).toBe("true");
  expect((name.getAttribute("aria-describedby") ?? "").split(" ").map((id) => document.getElementById(id))).toContain(
    said.closest("[id]"),
  );
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Create" }).disabled).toBe(true);
  expect(within(screen.getByRole("list", { name: "Your lists" })).getAllByRole("link")).toHaveLength(2);
});

test("Rename from the list's menu puts the name in a field in the title's place; saved, the server has the new name, the title says it and the field is gone", async () => {
  const { fake, data, l1 } = server();
  await loadListTimeline(data, l1);
  const sent = fake.script("PATCH /lists/{id}");
  const user = timeline(data, l1);

  await user.click(screen.getByRole("button", { name: "List options" }));
  await user.click(await screen.findByRole("menuitem", { name: "Rename" }));

  const name = await screen.findByRole<HTMLInputElement>("textbox", { name: "List name" });
  expect(name.value).toBe("Bread and coffee");
  expect(screen.queryAllByRole("heading", { level: 2 })).toHaveLength(0);
  await user.clear(name);
  await user.type(name, "Bakers");
  await user.click(screen.getByRole("button", { name: "Save" }));

  const { url, body } = await sent.arrived();
  expect(url.pathname).toEndWith(`/lists/${l1}`);
  expect(body.name).toBe("Bakers");
  expect((await screen.findByRole("heading", { level: 2 })).textContent).toMatch(/^Bakers\s*2 members$/);
  expect(screen.queryAllByRole("textbox", { name: "List name" })).toHaveLength(0);
});

test("Delete list… asks before it deletes, naming the list; confirmed, the server deletes it and the screen goes back", async () => {
  const { fake, data, l1 } = server();
  await loadListTimeline(data, l1);
  const sent = fake.script("DELETE /lists/{id}");
  const left: string[] = [];
  const user = timeline(data, l1, "en", () => left.push("back"));

  await user.click(screen.getByRole("button", { name: "List options" }));
  await user.click(await screen.findByRole("menuitem", { name: "Delete list…" }));

  const confirmation = await screen.findByRole("alertdialog", { name: "Delete the list Bread and coffee?" });
  expect(sent.seen).toHaveLength(0);
  await user.click(within(confirmation).getByRole("button", { name: "Delete" }));

  expect((await sent.served()).url.pathname).toEndWith(`/lists/${l1}`);
  await waitFor(() => {
    if (left.length === 0) throw new Error("the screen did not go back");
  });
});
