// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import type { AppData } from "@my/app-data";
import { scenario } from "@my/e2e/fake";
import { cleanup, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { PostActions } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
import { ListIndex, ListTimeline, loadLists, loadListTimeline } from "./lists";

/**
 * A server where the reader follows sam, hana and layla, and keeps the list Bread and coffee of sam and hana and
 * 翻訳の仲間 of layla; sam posted once, unless `posts` is false. `l1` and `l2` are the lists' IDs.
 */
function server({ lists = true, posts = true } = {}) {
  const { fake, data, requests } = fakeData({
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
  return { data, requests, l1, l2 };
}

/** `node` on `data`, alone in the document, for a reader of `locale`. */
function shown(data: AppData, node: ReactNode, locale = "en") {
  cleanup();
  renderFeature(data, node, locale);
}

const timeline = (data: AppData, id: string, locale = "en") => {
  shown(
    data,
    <PostActions onCompose={() => undefined}>
      <ListTimeline id={id} onBack={() => undefined} />
    </PostActions>,
    locale,
  );
};

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
