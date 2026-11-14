// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { cachedStatus, marked, markStatus } from "./interactions";
import type { Status } from "./statuses";
import { fakeAppData, json, status } from "./test/fake";
import { homeTimelineQuery } from "./timeline";

const post = (fields: Partial<Status> = {}) =>
  status("1", { favourites_count: 0, boosts_count: 0, reactions: [], ...fields });

/** AppData whose server answers handler, with the home timeline and the thread of post 0 holding `items`. */
function cached(items: Status[], handler: (request: Request) => Response | Promise<Response>) {
  const { data, requests } = fakeAppData(handler);
  const timeline = homeTimelineQuery(data).queryKey;
  const context = data.queries.queryOptions("get", "/statuses/{id}/context", {
    params: { path: { id: "0" } },
  }).queryKey;
  data.queryClient.setQueryData(timeline, { pages: [{ items }], pageParams: [undefined] });
  data.queryClient.setQueryData(context, { ancestors: [], descendants: items });
  const read = () => ({
    timeline: data.queryClient.getQueryData(timeline)?.pages[0]?.items[0],
    thread: data.queryClient.getQueryData<{ descendants: Status[] }>(context)?.descendants[0],
  });
  return { data, requests, read };
}

test("a favourite shows at once in every cached copy, then as the server answered", async () => {
  const { promise: answered, resolve: answer } = Promise.withResolvers<Response>();
  const { data, requests, read } = cached([post()], () => answered);
  const done = markStatus(data, "1", { type: "favourite", on: true });
  expect(read().timeline?.favourited).toBe(true);
  expect(read().thread?.favourites_count).toBe(1);
  answer(json(200, post({ favourited: true, favourites_count: 5 })));
  await done;
  expect(requests.map((u) => u.pathname)).toEqual(["/api/v1/statuses/1/favourite"]);
  expect(read().timeline?.favourites_count).toBe(5);
  expect(read().thread?.favourites_count).toBe(5);
});

test("a failed write puts back what every copy showed, and rejects", async () => {
  const { data, read } = cached([post({ boosted: true, boosts_count: 3 })], () =>
    json(422, { type: "about:blank", title: "Unprocessable", status: 422, detail: "Not boostable." }),
  );
  const rejected = await markStatus(data, "1", { type: "boost", on: false }).then(
    () => false,
    () => true,
  );
  expect(rejected).toBe(true);
  expect(read().timeline?.boosted).toBe(true);
  expect(read().thread?.boosts_count).toBe(3);
});

test("each mark sends its own request, set or taken back", async () => {
  const sent: string[] = [];
  const { data } = cached([post()], (request) => {
    sent.push(`${request.method} ${new URL(request.url).pathname.replace("/api/v1", "")}`);
    return json(200, post());
  });
  await markStatus(data, "1", { type: "boost", on: true });
  await markStatus(data, "1", { type: "bookmark", on: false });
  await markStatus(data, "1", { type: "reaction", name: "🎉", on: true });
  await markStatus(data, "1", { type: "reaction", name: "🎉", on: false });
  expect(sent).toEqual([
    "POST /statuses/1/boost",
    "DELETE /statuses/1/bookmark",
    "PUT /statuses/1/reaction",
    "DELETE /statuses/1/reaction",
  ]);
});

test("a reaction takes back the favourite and the reader's other reaction, as the server does", () => {
  const before = post({
    favourited: true,
    favourites_count: 2,
    reactions: [
      { name: "👍", count: 1, me: true },
      { name: "🎉", count: 2, me: false },
    ],
  });
  const after = marked(before, { type: "reaction", name: "🎉", on: true });
  expect(after.favourited).toBe(false);
  expect(after.favourites_count).toBe(1);
  expect(after.reactions).toEqual([{ name: "🎉", count: 3, me: true }]);
  const favourited = marked(after, { type: "favourite", on: true });
  expect(favourited.reactions).toEqual([{ name: "🎉", count: 2, me: false }]);
});

test("the cached status is found wherever a query holds it, and none when none does", () => {
  const { data } = cached([post({ bookmarked: true })], () => json(500, {}));
  expect(cachedStatus(data, "1")?.bookmarked).toBe(true);
  expect(cachedStatus(data, "2")).toBeUndefined();
});
