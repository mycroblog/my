// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { ProblemError } from "@mycroblog/mykit";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { domainMutesQuery, muteDomain, useMutedDomain } from "./domains";
import { notificationsQuery } from "./notifications";
import { fakeAppData, json, page } from "./test/fake";
import { probe } from "./test/probe";
import {
  bookmarksQuery,
  homeTimelineQuery,
  listTimelineQuery,
  publicTimelineQuery,
  tagTimelineQuery,
} from "./timeline";

/**
 * A server that keeps the viewer's muted domains, `muted` at the start, and lists them `size` a page; it refuses a mute
 * of this server's own domain (example.com) as the server does, and answers every other read with a page of one post.
 */
function server(muted: string[] = [], size = 2) {
  const domains = [...muted];
  const writes: { method: string; path: string; body: unknown }[] = [];
  const fake = fakeAppData(async (request) => {
    const url = new URL(request.url);
    const path = url.pathname;
    if (path === "/api/v1/domain_mutes" && request.method === "GET") {
      const at = Number(url.searchParams.get("cursor") ?? "0");
      const items = domains.slice(at, at + size).map((domain) => ({ domain, created_at: "2026-11-16T00:00:00Z" }));
      return json(200, { items, ...(at + size < domains.length ? { next_cursor: String(at + size) } : {}) });
    }
    if (path === "/api/v1/domain_mutes" && request.method === "POST") {
      const body = (await request.json()) as { domain: string };
      writes.push({ method: "POST", path, body });
      if (body.domain === "example.com") return json(422, { type: "about:blank", title: "Unprocessable", status: 422 });
      if (!domains.includes(body.domain)) domains.push(body.domain);
      return json(200, { domain: body.domain, created_at: "2026-11-16T00:00:00Z" });
    }
    if (path.startsWith("/api/v1/domain_mutes/") && request.method === "DELETE") {
      writes.push({ method: "DELETE", path, body: undefined });
      const domain = decodeURIComponent(path.slice("/api/v1/domain_mutes/".length));
      const at = domains.indexOf(domain);
      if (at >= 0) domains.splice(at, 1);
      return new Response(null, { status: 204 });
    }
    return json(200, page(["1"]));
  });
  return { ...fake, writes };
}

const mutedOf = (data: AppData, domain: string) => probe(data, () => useMutedDomain(domain));

describe("useMutedDomain", () => {
  test("names the muted domain a server falls under: itself or a parent, on any page of the list", async () => {
    const { data, requests } = server(["a.example", "b.example", "photos.example"]);
    await data.queryClient.query(domainMutesQuery(data));
    expect(requests.map((r) => r.searchParams.get("cursor"))).toEqual([null, "2"]);
    expect(mutedOf(data, "photos.example")).toBe("photos.example");
    expect(mutedOf(data, "cdn.photos.example")).toBe("photos.example");
    expect(mutedOf(data, "a.example")).toBe("a.example");
  });

  test("names none for a server not under a muted domain, though its name ends alike", async () => {
    const { data } = server(["photos.example"]);
    await data.queryClient.query(domainMutesQuery(data));
    expect(mutedOf(data, "otherphotos.example")).toBeUndefined();
    expect(mutedOf(data, "example")).toBeUndefined();
    expect(mutedOf(data, "social.example")).toBeUndefined();
  });

  test("names none until the list is read, and for an account of this server", () => {
    const { data } = server(["photos.example"]);
    expect(mutedOf(data, "photos.example")).toBeUndefined();
    expect(probe(data, () => useMutedDomain(undefined))).toBeUndefined();
  });
});

describe("muteDomain", () => {
  test("mutes a server, and the cache holds it once the write resolves", async () => {
    const { data, writes } = server();
    await data.queryClient.query(domainMutesQuery(data));
    await muteDomain(data, "photos.example", true);
    expect(writes).toEqual([{ method: "POST", path: "/api/v1/domain_mutes", body: { domain: "photos.example" } }]);
    expect(mutedOf(data, "photos.example")).toBe("photos.example");
    expect(mutedOf(data, "cdn.photos.example")).toBe("photos.example");
  });

  test("stops muting a server, and the cache no longer holds it once the write resolves", async () => {
    const { data, writes } = server(["photos.example", "social.example"]);
    await data.queryClient.query(domainMutesQuery(data));
    await muteDomain(data, "photos.example", false);
    expect(writes).toEqual([{ method: "DELETE", path: "/api/v1/domain_mutes/photos.example", body: undefined }]);
    expect(mutedOf(data, "photos.example")).toBeUndefined();
    expect(mutedOf(data, "social.example")).toBe("social.example");
  });

  test("rejects with the server's problem, and the cache keeps the list as it was", async () => {
    const { data } = server(["photos.example"]);
    await data.queryClient.query(domainMutesQuery(data));
    const refused = await muteDomain(data, "example.com", true).then(
      () => undefined,
      (error: unknown) => error,
    );
    expect(refused).toBeInstanceOf(ProblemError);
    expect(mutedOf(data, "example.com")).toBeUndefined();
    expect(mutedOf(data, "photos.example")).toBe("photos.example");
  });

  // ADR-0013 §30: a muted server's posts leave the timelines other than home, its boosts leave home, its accounts
  // notify no more. The server filters; the client reads them again.
  const changed = (data: AppData) => [
    homeTimelineQuery(data).queryKey,
    listTimelineQuery(data, "l").queryKey,
    publicTimelineQuery(data, "federated").queryKey,
    tagTimelineQuery(data, "film").queryKey,
    notificationsQuery(data, "all").queryKey,
  ];

  async function cached(data: AppData) {
    const keys: (readonly unknown[])[] = [...changed(data), bookmarksQuery(data).queryKey];
    for (const key of keys) data.queryClient.setQueryData(key, { pages: [page(["1"])], pageParams: [undefined] });
    await data.queryClient.query(domainMutesQuery(data));
  }

  for (const on of [true, false])
    for (const marking of ["shown", "later"] as const)
      test(`${on ? "a mute" : "an unmute"} (${marking}) marks the timelines and the notifications, and nothing else`, async () => {
        const { data } = server(on ? [] : ["photos.example"]);
        await cached(data);
        await muteDomain(data, "photos.example", on, marking);
        const marked = (key: readonly unknown[]) => data.queryClient.getQueryState(key)?.isInvalidated;
        for (const key of changed(data)) expect([key, marked(key)]).toEqual([key, true]);
        expect(marked(bookmarksQuery(data).queryKey)).toBe(false);
      });

  test("shown, the home timeline on the screen is read again before the write resolves; later, it is not", async () => {
    for (const marking of ["shown", "later"] as const) {
      const { data, requests } = server();
      await data.queryClient.query(domainMutesQuery(data));
      await data.queryClient.infiniteQuery(homeTimelineQuery(data));
      const observer = new InfiniteQueryObserver(data.queryClient, homeTimelineQuery(data));
      const unsubscribe = observer.subscribe(() => undefined);
      const reads = () => requests.filter((r) => r.pathname === "/api/v1/timelines/home").length;
      const before = reads();
      await muteDomain(data, "photos.example", true, marking);
      expect([marking, reads() - before]).toEqual([marking, marking === "shown" ? 1 : 0]);
      unsubscribe();
    }
  });
});
