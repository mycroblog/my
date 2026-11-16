// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import type { AppData } from "@my/app-data";
import { PostActions } from "../post-actions";
import { render, server, status } from "../test/server";
import { ListIndex, ListTimeline, loadLists, loadListTimeline } from "./lists";

const list = (id: string, name: string, members: number) => ({
  id,
  name,
  members_count: members,
  created_at: "2026-11-01T00:00:00Z",
});

/** The markup without classes and icons, to read the structure. */
const bare = (html: string) => html.replace(/ class="[^"]*"/g, "").replace(/<svg.*?<\/svg>/g, "");

const timeline = (data: AppData, id: string, locale = "en") =>
  render(
    data,
    <PostActions onCompose={() => undefined}>
      <ListTimeline id={id} onBack={() => undefined} />
    </PostActions>,
    locale,
  );

test("the index lists the reader's lists, each a link to its timeline with how many members it has", async () => {
  const { data, requests } = server({
    "/lists": { items: [list("l1", "Bread and coffee", 2), list("l2", "翻訳の仲間", 1)] },
  });
  await loadLists(data);
  expect(requests).toEqual(["GET /lists"]);
  const en = bare(render(data, <ListIndex />));
  expect(en).toMatch(/<nav aria-label="Your lists">/);
  expect(en).toMatch(/<a [^>]*href="\/lists\/l1"[^>]*>.*Bread and coffee.*2 members.*?<\/a>/);
  expect(en).toMatch(/<a [^>]*href="\/lists\/l2"[^>]*>.*翻訳の仲間.*1 member<.*?<\/a>/);
  const ja = bare(render(data, <ListIndex />, "ja"));
  expect(ja).toMatch(/<nav aria-label="自分のリスト">/);
  expect(ja).toContain("2 人");
});

test("with no lists, the index says so, in en and ja", async () => {
  const { data } = server({ "/lists": { items: [] } });
  await loadLists(data);
  expect(render(data, <ListIndex />)).toContain(
    "No lists yet. Your lists show here, each with the posts of its members.",
  );
  expect(render(data, <ListIndex />, "ja")).toContain(
    "まだリストはありません。リストはここに表示され、それぞれのメンバーの投稿を読めます。",
  );
});

test("the loader reads the list and its timeline; the screen shows its name, members and posts", async () => {
  const { data, requests } = server({
    "/lists/l1": list("l1", "Bread and coffee", 2),
    "/timelines/list/l1": { items: [status("p", "sam")] },
  });
  await loadListTimeline(data, "l1");
  expect(requests).toContain("GET /lists/l1");
  expect(requests).toContain("GET /timelines/list/l1");
  const html = bare(timeline(data, "l1"));
  expect(html).toMatch(
    />Lists<\/button><\/p><div><h2><span><\/span><bdi>Bread and coffee<\/bdi> <span>2 members<\/span><\/h2>/,
  );
  expect(html).toMatch(/role="feed" aria-label="Posts from the list Bread and coffee"/);
  expect(html).toContain("Post p");
  // A list has nothing to follow: no action in its head.
  expect(html).not.toContain(">Follow<");
});

test("a list whose members have not posted says so, in en and ja", async () => {
  const { data } = server({
    "/lists/l1": list("l1", "Bread and coffee", 2),
    "/timelines/list/l1": { items: [] },
  });
  await loadListTimeline(data, "l1");
  expect(timeline(data, "l1")).toContain(
    "No posts from this list&#x27;s members yet. When they post, their posts show here.",
  );
  expect(timeline(data, "l1", "ja")).toContain(
    "まだこのリストのメンバーの投稿はありません。メンバーが投稿すると、ここに表示されます。",
  );
});
