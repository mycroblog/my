// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import type { PostAction } from "@my/app-components";
import { homeTimelineQuery, type Status } from "@my/app-data";
import { markOf, PostActions, usePostActions, type ComposeRequest } from "./post-actions";
import { render, server, status } from "./test/server";

const post = (fields: Record<string, unknown> = {}) => status("1", "lena", fields) as unknown as Status;

test("each action asks for the mark the post does not have, or takes back the one it has", () => {
  const plain = post({ reactions: [{ name: "🎉", count: 1, me: true }] });
  const marked = post({ favourited: true, boosted: true, bookmarked: true });
  expect(markOf({ type: "favourite", id: "1" }, plain)).toEqual({ type: "favourite", on: true });
  expect(markOf({ type: "favourite", id: "1" }, marked)).toEqual({ type: "favourite", on: false });
  expect(markOf({ type: "boost", id: "1" }, marked)).toEqual({ type: "boost", on: false });
  expect(markOf({ type: "bookmark", id: "1" }, plain)).toEqual({ type: "bookmark", on: true });
  expect(markOf({ type: "reaction", id: "1", name: "🎉" }, plain)).toEqual({ type: "reaction", name: "🎉", on: false });
  expect(markOf({ type: "reaction", id: "1", name: "👍" }, plain)).toEqual({ type: "reaction", name: "👍", on: true });
  expect(markOf({ type: "reply", id: "1" }, plain)).toBeUndefined();
  expect(markOf({ type: "favourite", id: "1" }, undefined)).toBeUndefined();
});

/** The handler as a screen under PostActions gets it, with the home timeline holding post 1. */
function handler(answers: Record<string, unknown>) {
  const { data, requests } = server(answers);
  const key = homeTimelineQuery(data).queryKey;
  data.queryClient.setQueryData(key, { pages: [{ items: [post()] }], pageParams: [undefined] });
  const composed: ComposeRequest[] = [];
  let act: (action: PostAction) => void = () => undefined;
  function Probe() {
    act = usePostActions();
    return null;
  }
  render(
    data,
    <PostActions onCompose={(to) => composed.push(to)}>
      <Probe />
    </PostActions>,
  );
  return { act, composed, requests, home: () => data.queryClient.getQueryData(key)?.pages[0]?.items[0] };
}

test("Reply and Quote open the composer for the post, and write nothing", () => {
  const { act, composed, requests } = handler({});
  act({ type: "reply", id: "1" });
  act({ type: "quote", id: "1" });
  expect(composed).toEqual([
    { kind: "reply", id: "1" },
    { kind: "quote", id: "1" },
  ]);
  expect(requests).toEqual([]);
});

test("favourite reaches the server, and the cache shows it", async () => {
  const { act, requests, home } = handler({
    "/statuses/1/favourite": status("1", "lena", { favourited: true, favourites_count: 1 }),
  });
  act({ type: "favourite", id: "1" });
  expect(home()?.favourited).toBe(true);
  await Bun.sleep(0);
  expect(requests).toEqual(["POST /statuses/1/favourite"]);
});

test("an action with no mark, or on a post not in the cache, writes nothing", () => {
  const { act, requests } = handler({});
  act({ type: "translate", id: "1" });
  act({ type: "favourite", id: "2" });
  expect(requests).toEqual([]);
});
