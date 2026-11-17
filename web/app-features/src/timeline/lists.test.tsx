// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import type { AppData } from "@my/app-data";
import { scenario } from "@my/e2e/fake";
import { PostActions } from "../post-actions";
import { fakeData, markupOf } from "../test/feature";
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

/** The markup without classes and icons, to read the structure. */
const bare = (html: string) => html.replace(/ class="[^"]*"/g, "").replace(/<svg.*?<\/svg>/g, "");

const timeline = (data: AppData, id: string, locale = "en") =>
  markupOf(
    data,
    <PostActions onCompose={() => undefined}>
      <ListTimeline id={id} onBack={() => undefined} />
    </PostActions>,
    locale,
  );

test("the index lists the reader's lists, each a link to its timeline with how many members it has", async () => {
  const { data, requests, l1, l2 } = server();
  await loadLists(data);
  expect(requests).toEqual(["GET /lists"]);
  const en = bare(markupOf(data, <ListIndex />));
  expect(en).toMatch(/<ul aria-label="Your lists">/);
  expect(en).toMatch(new RegExp(`<a [^>]*href="/lists/${l1}"[^>]*>.*Bread and coffee.*2 members.*?</a>`));
  expect(en).toMatch(new RegExp(`<a [^>]*href="/lists/${l2}"[^>]*>.*翻訳の仲間.*1 member<.*?</a>`));
  const ja = bare(markupOf(data, <ListIndex />, "ja"));
  expect(ja).toMatch(/<ul aria-label="自分のリスト">/);
  expect(ja).toContain("2 人");
});

test("with no lists, the index says so, in en and ja", async () => {
  const { data } = server({ lists: false });
  await loadLists(data);
  expect(markupOf(data, <ListIndex />)).toContain(
    "No lists yet. Your lists show here, each with the posts of its members.",
  );
  expect(markupOf(data, <ListIndex />, "ja")).toContain(
    "まだリストはありません。リストはここに表示され、それぞれのメンバーの投稿を読めます。",
  );
});

test("the loader reads the list and its timeline; the screen shows its name, members and posts", async () => {
  const { data, requests, l1 } = server();
  await loadListTimeline(data, l1);
  expect(requests).toContain(`GET /lists/${l1}`);
  expect(requests).toContain(`GET /timelines/list/${l1}`);
  const html = bare(timeline(data, l1));
  expect(html).toMatch(
    />Lists<\/button><\/p><div><h2><span><\/span><bdi>Bread and coffee<\/bdi> <span>2 members<\/span><\/h2>/,
  );
  expect(html).toMatch(/role="feed" aria-label="Posts from the list Bread and coffee"/);
  expect(html).toContain("The first loaf.");
  // A list has nothing to follow: no action in its head.
  expect(html).not.toContain(">Follow<");
});

test("a list whose members have not posted says so, in en and ja", async () => {
  const { data, l1 } = server({ posts: false });
  await loadListTimeline(data, l1);
  expect(timeline(data, l1)).toContain(
    "No posts from this list&#x27;s members yet. When they post, their posts show here.",
  );
  expect(timeline(data, l1, "ja")).toContain(
    "まだこのリストのメンバーの投稿はありません。メンバーが投稿すると、ここに表示されます。",
  );
});
