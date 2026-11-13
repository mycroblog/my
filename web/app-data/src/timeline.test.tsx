// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { ProblemError } from "@mycroblog/mykit";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { AppDataProvider } from "./provider";
import { fakeAppData, json, page } from "./test/fake";
import { homeTimelineQuery, useHomeTimeline } from "./timeline";
import type { AppData } from "./app-data";

/** A server whose home timeline has the pages given, keyed by the cursor that asks for them ("" for the first). */
function timeline(pages: Record<string, unknown>) {
  return fakeAppData((request) => {
    const url = new URL(request.url);
    if (url.pathname !== "/api/v1/timelines/home") return json(404, { type: "about:blank", status: 404 });
    const body = pages[url.searchParams.get("cursor") ?? ""];
    return body === undefined ? json(400, { type: "about:blank", status: 400 }) : json(200, body);
  });
}

/** What a loader reads (ADR-0028 §6): the cache when it holds the query and it is not marked, else a fetch. */
const load = (data: AppData) => data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity });

describe("homeTimelineQuery", () => {
  test("fetches the first page without a cursor", async () => {
    const { data, requests } = timeline({ "": page(["3", "2"], "c2") });
    const result = await load(data);
    expect(result.pages.map((p) => p.items.map((s) => s.id))).toEqual([["3", "2"]]);
    expect(result.pageParams).toEqual([undefined]);
    expect(requests.map((u) => u.pathname + u.search)).toEqual(["/api/v1/timelines/home"]);
  });

  test("a loader reads the cache until the query is marked", async () => {
    const { data, requests } = timeline({ "": page(["1"]) });
    await load(data);
    await load(data);
    expect(requests).toHaveLength(1);
    await data.queryClient.invalidateQueries({ queryKey: homeTimelineQuery(data).queryKey });
    await load(data);
    expect(requests).toHaveLength(2);
  });

  test("its key is the one openapi-react-query gives the operation", () => {
    const { data } = timeline({});
    expect([...homeTimelineQuery(data).queryKey]).toEqual(["get", "/timelines/home"]);
  });

  test("follows next_cursor until the last page", async () => {
    const { data, requests } = timeline({ "": page(["4", "3"], "c2"), c2: page(["2", "1"], "") });
    await load(data);
    const observer = new InfiniteQueryObserver(data.queryClient, homeTimelineQuery(data));
    expect(observer.getCurrentResult().hasNextPage).toBe(true);
    const second = await observer.fetchNextPage();
    expect(second.data?.pages.flatMap((p) => p.items.map((s) => s.id))).toEqual(["4", "3", "2", "1"]);
    expect(second.hasNextPage).toBe(false);
    expect(requests.map((u) => u.search)).toEqual(["", "?cursor=c2"]);
  });

  test("rejects with the problem the server answered, without retrying", async () => {
    const { data, requests } = fakeAppData(() => json(500, { type: "about:blank", title: "Internal", status: 500 }));
    const error: unknown = await load(data).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ProblemError);
    expect(requests).toHaveLength(1);
  });
});

function Column() {
  const home = useHomeTimeline();
  return (
    <ol data-status={home.status} data-more={String(home.hasNextPage)}>
      {home.statuses.map((s) => (
        <li key={s.id}>{s.id}</li>
      ))}
    </ol>
  );
}

const render = (data: AppData) =>
  renderToStaticMarkup(
    <AppDataProvider data={data}>
      <Column />
    </AppDataProvider>,
  );

describe("useHomeTimeline", () => {
  test("shows the statuses of the cached pages, newest first", async () => {
    const { data } = timeline({ "": page(["2", "1"], "c2") });
    await load(data);
    expect(render(data)).toBe('<ol data-status="success" data-more="true"><li>2</li><li>1</li></ol>');
  });

  test("is pending with no statuses before the first page", () => {
    const { data } = timeline({});
    expect(render(data)).toBe('<ol data-status="pending" data-more="false"></ol>');
  });

  test("after its first page failed, refetch fetches it again", async () => {
    let fail = true;
    const { data, requests } = fakeAppData(() =>
      fail ? json(500, { type: "about:blank", status: 500 }) : json(200, page(["1"])),
    );
    await load(data).catch(() => undefined);
    let refetch: () => void = () => undefined;
    function Failed() {
      refetch = useHomeTimeline().refetch;
      return null;
    }
    renderToStaticMarkup(
      <AppDataProvider data={data}>
        <Failed />
      </AppDataProvider>,
    );
    expect(data.queryClient.getQueryState(homeTimelineQuery(data).queryKey)?.status).toBe("error");
    fail = false;
    refetch();
    await data.queryClient.getQueryCache().find({ queryKey: homeTimelineQuery(data).queryKey })?.promise;
    expect(requests).toHaveLength(2);
    expect(render(data)).toBe('<ol data-status="success" data-more="false"><li>1</li></ol>');
  });

  test("fails outside AppDataProvider", () => {
    expect(() => renderToStaticMarkup(<Column />)).toThrow(/outside AppDataProvider/);
  });
});
