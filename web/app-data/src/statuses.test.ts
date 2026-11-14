// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { removeCachedStatus, updateCachedStatuses, type Status } from "./statuses";
import { fakeAppData, json, page, status } from "./test/fake";
import { homeTimelineQuery } from "./timeline";

const favourite = (s: Status): Status => ({ ...s, favourited: true, favourites_count: 1 });

function cached(items: Status[]) {
  const { data } = fakeAppData(() => new Response(null, { status: 500 }));
  const key = homeTimelineQuery(data).queryKey;
  data.queryClient.setQueryData(key, { pages: [{ items }], pageParams: [undefined] });
  const read = () => data.queryClient.getQueryData(key)?.pages[0]?.items;
  return { data, read };
}

describe("updateCachedStatuses", () => {
  test("writes the change into the status on a timeline", () => {
    const { data, read } = cached([status("1"), status("2")]);
    updateCachedStatuses(data.queryClient, "2", favourite);
    expect(read()?.map((s) => s.favourited ?? false)).toEqual([false, true]);
  });

  test("writes the change into a boost and a quote of the status", () => {
    const { data, read } = cached([
      status("b", { boost: status("1") }),
      status("q", { quote: { state: "accepted", quoted_status: status("1") } }),
    ]);
    updateCachedStatuses(data.queryClient, "1", favourite);
    const [boost, quote] = read() ?? [];
    expect(boost?.boost?.favourited).toBe(true);
    expect(quote?.quote?.quoted_status?.favourited).toBe(true);
    expect(boost?.favourited).toBeUndefined();
  });

  test("keeps the data of a timeline without the status as it was", () => {
    const { data, read } = cached([status("1")]);
    const before = read();
    updateCachedStatuses(data.queryClient, "9", favourite);
    expect(read()).toBe(before);
  });
});

describe("removeCachedStatus", () => {
  test("takes the status and the boosts of it out of a timeline, and shows a quote of it deleted", () => {
    const { data, read } = cached([
      status("2"),
      status("1"),
      status("b", { boost: status("1") }),
      status("q", { quote: { state: "accepted", quoted_status: status("1") } }),
      status("bq", { boost: status("q2", { quote: { state: "accepted", quoted_status: status("1") } }) }),
    ]);
    removeCachedStatus(data.queryClient, "1");
    const items = read() ?? [];
    expect(items.map((s) => s.id)).toEqual(["2", "q", "bq"]);
    expect(items[1]?.quote).toEqual({ state: "deleted" });
    expect(items[2]?.boost?.quote).toEqual({ state: "deleted" });
  });

  test("keeps the data of a timeline without the status as it was", () => {
    const { data, read } = cached([status("1"), status("b", { boost: status("2") })]);
    const before = read();
    removeCachedStatus(data.queryClient, "9");
    expect(read()).toBe(before);
  });

  test("takes the status out again once a fetch that was reading the timeline ends", async () => {
    let answer: (response: Response) => void = () => {};
    const { data } = fakeAppData(
      () =>
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
    );
    const query = homeTimelineQuery(data);
    const fetched = data.queryClient.infiniteQuery(query);
    await Bun.sleep(0);
    removeCachedStatus(data.queryClient, "1");
    answer(json(200, page(["2", "1"])));
    await fetched;
    await Bun.sleep(0);
    expect(data.queryClient.getQueryData(query.queryKey)?.pages[0]?.items.map((s) => s.id)).toEqual(["2"]);
  });
});
