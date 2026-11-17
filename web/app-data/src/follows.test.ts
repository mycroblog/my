// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { scenario, type FakeOptions, type FakePerson } from "@my/e2e/fake";
import { useRelationship } from "./accounts";
import type { AppData } from "./app-data";
import { accountFollowsQuery, useAccountFollows, type FollowsList } from "./follows";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";

/** The people who follow hana and whom hana follows, the first the oldest follow: one more than a page. */
const fans = Array.from({ length: 21 }, (_, i) => `fan${String(i + 1)}`);
/** The fans the reader follows. */
const followed = ["fan3", "fan21"];

/**
 * A server where each of `fans` and hana follow each other, the reader follows those of `followed`, and marco follows
 * and is followed by nobody.
 */
function server(options: FakeOptions = {}) {
  const [model] = scenario.people;
  if (model === undefined) throw new Error("test: the scenario has no people");
  const people: FakePerson[] = fans.map((username) => ({ ...model, username, displayName: username }));
  return fakeData({
    seed: {
      people: [...scenario.people, ...people],
      follows: [
        ...fans.flatMap((fan) => [
          { follower: fan, followee: "hana" },
          { follower: "hana", followee: fan },
        ]),
        ...followed.map((followee) => ({ follower: "yuki", followee })),
      ],
    },
    ...options,
  });
}

/**
 * Fetches the list once, which fails, and keeps its failure for the probe as a screen that fetched it holds it: a
 * render alone would ask again (retryOnMount) and show the list pending.
 */
async function failed(data: AppData, account: string, list: FollowsList) {
  const query = accountFollowsQuery(data, account, list);
  data.queryClient.setQueryDefaults(query.queryKey, { retryOnMount: false });
  await data.queryClient.infiniteQuery(query).catch(() => undefined);
}

/** The value of each `name` of the query of a request. */
const asked = (request: string, name: string) =>
  new URL(request.slice("GET ".length), "https://demo.test").searchParams.getAll(name);

for (const list of ["followers", "following"] as const satisfies FollowsList[])
  describe(`the ${list} of an account`, () => {
    test("reads the list's pages in order, and the viewer's relationships with each page's accounts in one request", async () => {
      const { fake, data, requests } = server();
      const hana = fake.accountId("hana");
      // The server lists the newest follow first.
      const ids = fans.toReversed().map(fake.accountId);
      await data.queryClient.infiniteQuery({ ...accountFollowsQuery(data, hana, list), pages: 2 });
      const shown = probe(data, () => useAccountFollows(hana, list));
      expect(shown.items.map((a) => a.id)).toEqual(ids);
      expect(shown.hasNextPage).toBe(false);
      expect(shown.hidden).toBe(false);
      const lists = requests.filter((r) => r.startsWith(`GET /accounts/${hana}/${list}`));
      expect(lists.map((r) => asked(r, "cursor"))).toEqual([[], [ids[19] ?? ""]]);
      const relationships = requests.filter((r) => r.startsWith("GET /accounts/relationships"));
      expect(relationships.map((r) => asked(r, "id"))).toEqual([ids.slice(0, 20), ids.slice(20)]);
      // Each row's follow shows from the page's relationships, with no request of its own.
      const before = requests.length;
      const following = (fan: string) => probe(data, () => useRelationship(fake.accountId(fan)))?.following;
      expect(["fan1", "fan3", "fan21"].map(following)).toEqual([false, true, true]);
      expect(requests).toHaveLength(before);
    });

    test("says there is a next page until the last one is read", async () => {
      const { fake, data } = server();
      const hana = fake.accountId("hana");
      await data.queryClient.infiniteQuery(accountFollowsQuery(data, hana, list));
      const shown = probe(data, () => useAccountFollows(hana, list));
      expect(shown.items.map((a) => a.id)).toEqual(fans.toReversed().slice(0, 20).map(fake.accountId));
      expect(shown.hasNextPage).toBe(true);
    });

    test("an empty list shows no account, and is neither hidden nor failed", async () => {
      const { fake, data, requests } = server();
      const marco = fake.accountId("marco");
      await data.queryClient.infiniteQuery(accountFollowsQuery(data, marco, list));
      const shown = probe(data, () => useAccountFollows(marco, list));
      expect(shown.items).toEqual([]);
      expect(shown.status).toBe("success");
      expect(shown.hidden).toBe(false);
      // No account, so no relationship to read.
      expect(requests.filter((r) => r.startsWith("GET /accounts/relationships"))).toEqual([]);
    });

    test("a list its owner keeps from the viewer is hidden, with no account", async () => {
      // kai shows their follows to nobody.
      const { fake, data } = server();
      const kai = fake.accountId("kai");
      await failed(data, kai, list);
      const shown = probe(data, () => useAccountFollows(kai, list));
      expect(shown.hidden).toBe(true);
      expect(shown.items).toEqual([]);
    });

    test("another failure is a failure, not a hidden list", async () => {
      const { fake, data } = server({ followsLists: "fails" });
      const hana = fake.accountId("hana");
      await failed(data, hana, list);
      const shown = probe(data, () => useAccountFollows(hana, list));
      expect(shown.hidden).toBe(false);
      expect(shown.status).toBe("error");
      expect(shown.error).not.toBeNull();
    });
  });

test("the two lists of an account are kept apart", () => {
  const { fake, data } = server();
  const hana = fake.accountId("hana");
  expect(accountFollowsQuery(data, hana, "followers").queryKey).not.toEqual(
    accountFollowsQuery(data, hana, "following").queryKey,
  );
});
