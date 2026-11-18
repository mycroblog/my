// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { ProblemError } from "@mycroblog/mykit";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { domainMutesQuery, muteDomain, useMutedDomain, useMutedDomains } from "./domains";
import { notificationsQuery } from "./notifications";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import {
  bookmarksQuery,
  homeTimelineQuery,
  listTimelineQuery,
  publicTimelineQuery,
  tagTimelineQuery,
} from "./timeline";

/** The largest page of the server's list of muted domains, which a domain of a test comes after. */
const filler = Array.from({ length: 40 }, (_, i) => `server-${String(i + 1)}.example`);

const mutedOf = (data: AppData, domain: string) => probe(data, () => useMutedDomain(domain));
const writes = (requests: readonly string[]) => requests.filter((r) => !r.startsWith("GET "));

describe("useMutedDomain", () => {
  test("names the muted domain a server falls under: itself or a parent, on any page of the list", async () => {
    const { data, requests } = fakeData({ domainMutes: ["a.example", ...filler, "photos.example"] });
    await data.queryClient.query(domainMutesQuery(data));
    const cursors = requests.map((r) =>
      new URL(r.slice("GET ".length), "https://demo.test").searchParams.get("cursor"),
    );
    expect(cursors).toEqual([null, filler.at(-2) ?? ""]);
    expect(mutedOf(data, "photos.example")).toBe("photos.example");
    expect(mutedOf(data, "cdn.photos.example")).toBe("photos.example");
    expect(mutedOf(data, "a.example")).toBe("a.example");
  });

  test("names none for a server not under a muted domain, though its name ends alike", async () => {
    const { data } = fakeData({ domainMutes: ["photos.example"] });
    await data.queryClient.query(domainMutesQuery(data));
    expect(mutedOf(data, "otherphotos.example")).toBeUndefined();
    expect(mutedOf(data, "example")).toBeUndefined();
    expect(mutedOf(data, "social.example")).toBeUndefined();
  });

  test("names none until the list is read, and for an account of this server", () => {
    const { data } = fakeData({ domainMutes: ["photos.example"] });
    expect(mutedOf(data, "photos.example")).toBeUndefined();
    expect(probe(data, () => useMutedDomain(undefined))).toBeUndefined();
  });
});

// The settings list the servers the reader mutes (#3202).
describe("useMutedDomains", () => {
  test("is empty and pending until the list is read, then holds the muted domains of every page, in the server's order, with no page left to ask for", async () => {
    const muted = ["a.example", ...filler, "photos.example"];
    const { data } = fakeData({ domainMutes: muted });
    expect(probe(data, useMutedDomains)).toMatchObject({ items: [], status: "pending" });
    await data.queryClient.query(domainMutesQuery(data));
    const list = probe(data, useMutedDomains);
    expect(list.status).toBe("success");
    expect(list.items).toEqual(muted);
    expect(list.hasNextPage).toBe(false);
  });

  test("reports an error when the list could not be read", async () => {
    const { fake, data } = fakeData({ domainMutes: ["photos.example"] });
    fake.script("GET /domain_mutes", { fail: 500 });
    await data.queryClient.query(domainMutesQuery(data)).catch(() => undefined);
    const queries = data.queryClient.getDefaultOptions().queries;
    data.queryClient.setDefaultOptions({ queries: { ...queries, retryOnMount: false } });
    expect(probe(data, useMutedDomains)).toMatchObject({ items: [], status: "error" });
  });

  test("no longer holds a server once its unmute resolved, and holds one once its mute did", async () => {
    const { data } = fakeData({ domainMutes: ["photos.example", "social.example"] });
    await data.queryClient.query(domainMutesQuery(data));
    await muteDomain(data, "photos.example", false);
    expect(probe(data, useMutedDomains).items).toEqual(["social.example"]);
    await muteDomain(data, "ads.example", true);
    expect([...probe(data, useMutedDomains).items].sort()).toEqual(["ads.example", "social.example"]);
  });
});

describe("muteDomain", () => {
  test("mutes a server, and the cache holds it once the write resolves", async () => {
    const { fake, data, requests } = fakeData();
    const sent = fake.script("POST /domain_mutes");
    await data.queryClient.query(domainMutesQuery(data));
    await muteDomain(data, "photos.example", true);
    expect(writes(requests)).toEqual(["POST /domain_mutes"]);
    expect(sent.seen.map((s) => s.body)).toEqual([{ domain: "photos.example" }]);
    expect(mutedOf(data, "photos.example")).toBe("photos.example");
    expect(mutedOf(data, "cdn.photos.example")).toBe("photos.example");
  });

  test("stops muting a server, and the cache no longer holds it once the write resolves", async () => {
    const { data, requests } = fakeData({ domainMutes: ["photos.example", "social.example"] });
    await data.queryClient.query(domainMutesQuery(data));
    await muteDomain(data, "photos.example", false);
    expect(writes(requests)).toEqual(["DELETE /domain_mutes/photos.example"]);
    expect(mutedOf(data, "photos.example")).toBeUndefined();
    expect(mutedOf(data, "social.example")).toBe("social.example");
  });

  test("rejects with the server's problem, and the cache keeps the list as it was", async () => {
    const { data } = fakeData({ domainMutes: ["photos.example"] });
    await data.queryClient.query(domainMutesQuery(data));
    // The server mutes no domain of its own.
    const refused = await muteDomain(data, "demo.test", true).then(
      () => undefined,
      (error: unknown) => error,
    );
    expect(refused).toBeInstanceOf(ProblemError);
    expect(mutedOf(data, "demo.test")).toBeUndefined();
    expect(mutedOf(data, "photos.example")).toBe("photos.example");
  });

  // ADR-0013 §30: a muted server's posts leave the timelines other than home, its boosts leave home, its accounts
  // notify no more. The server filters; the client reads them again.
  /** Reads the timelines, the notifications, the bookmarks and the muted domains, and returns the keys a mute marks. */
  async function cached(data: AppData, list: string) {
    const home = homeTimelineQuery(data);
    const listed = listTimelineQuery(data, list);
    const federated = publicTimelineQuery(data, "federated");
    const tagged = tagTimelineQuery(data, "baking");
    const notifications = notificationsQuery(data, "all");
    await data.queryClient.infiniteQuery(home);
    await data.queryClient.infiniteQuery(listed);
    await data.queryClient.infiniteQuery(federated);
    await data.queryClient.infiniteQuery(tagged);
    await data.queryClient.infiniteQuery(notifications);
    await data.queryClient.infiniteQuery(bookmarksQuery(data));
    await data.queryClient.query(domainMutesQuery(data));
    return [home.queryKey, listed.queryKey, federated.queryKey, tagged.queryKey, notifications.queryKey];
  }

  for (const on of [true, false])
    for (const marking of ["shown", "later"] as const)
      test(`${on ? "a mute" : "an unmute"} (${marking}) marks the timelines and the notifications, and nothing else`, async () => {
        const { fake, data } = fakeData({ domainMutes: on ? [] : ["photos.example"] });
        const changed = await cached(data, fake.listPath(0).replace("/lists/", ""));
        await muteDomain(data, "photos.example", on, marking);
        const marked = (key: readonly unknown[]) => data.queryClient.getQueryState(key)?.isInvalidated;
        for (const key of changed) expect([key, marked(key)]).toEqual([key, true]);
        expect(marked(bookmarksQuery(data).queryKey)).toBe(false);
      });

  test("shown, the home timeline on the screen is read again before the write resolves; later, it is not", async () => {
    for (const marking of ["shown", "later"] as const) {
      const { data, requests } = fakeData();
      await data.queryClient.query(domainMutesQuery(data));
      await data.queryClient.infiniteQuery(homeTimelineQuery(data));
      const observer = new InfiniteQueryObserver(data.queryClient, homeTimelineQuery(data));
      const unsubscribe = observer.subscribe(() => undefined);
      const reads = () => requests.filter((r) => r === "GET /timelines/home").length;
      const before = reads();
      await muteDomain(data, "photos.example", true, marking);
      expect([marking, reads() - before]).toEqual([marking, marking === "shown" ? 1 : 0]);
      unsubscribe();
    }
  });
});
