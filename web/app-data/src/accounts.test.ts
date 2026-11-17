// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { fakeOrigin, scenario, type FakeOptions } from "@my/e2e/fake";
import { ProblemError } from "@mycroblog/mykit";
import { QueryObserver } from "@tanstack/react-query";
import {
  accountByIdQuery,
  accountQuery,
  accountStatusesQuery,
  cachedAccount,
  changeRelationship,
  loadAccount,
  accountNoteLimit,
  relationshipQuery,
  useAccount,
  useRelationship,
  writeAccountNote,
  type Account,
  type Relationship,
} from "./accounts";
import type { AppData } from "./app-data";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { homeTimelineQuery } from "./timeline";

/** The requests of `requests` that are writes. */
const writes = (requests: readonly string[]) => requests.filter((r) => !r.startsWith("GET "));
/** The requests of `requests` without their queries. */
const paths = (requests: readonly string[]) => requests.map((r) => r.split("?")[0]);

test("a follow puts the relationship the server answered into the cache", async () => {
  const { fake, data, requests } = fakeData();
  const marco = fake.accountId("marco");
  const following = () =>
    data.queryClient.getQueryData<{ items: Relationship[] }>(relationshipQuery(data, marco).queryKey)?.items[0]
      ?.following;
  await data.queryClient.query(relationshipQuery(data, marco));
  expect(following()).toBe(false);
  await changeRelationship(data, marco, "follow");
  expect(requests.at(-1)).toBe(`POST /accounts/${marco}/follow`);
  expect(following()).toBe(true);
  await changeRelationship(data, marco, "unfollow");
  expect(following()).toBe(false);
});

test("a change of a relationship marks the account's counts, the home timeline and the account's posts", async () => {
  const { fake, data } = fakeData();
  const { queryClient } = data;
  const marco = fake.accountId("marco");
  const hana = fake.accountId("hana");
  await queryClient.query(accountQuery(data, "marco"));
  await queryClient.query(accountByIdQuery(data, marco));
  await queryClient.infiniteQuery(homeTimelineQuery(data));
  await queryClient.infiniteQuery(accountStatusesQuery(data, marco, "posts"));
  await queryClient.infiniteQuery(accountStatusesQuery(data, hana, "posts"));
  await changeRelationship(data, marco, "follow");
  const marked = (key: readonly unknown[]) => queryClient.getQueryState(key)?.isInvalidated;
  expect(marked(accountQuery(data, "marco").queryKey)).toBe(true);
  expect(marked(accountByIdQuery(data, marco).queryKey)).toBe(true);
  expect(marked(homeTimelineQuery(data).queryKey)).toBe(true);
  expect(marked(accountStatusesQuery(data, marco, "posts").queryKey)).toBe(true);
  expect(marked(accountStatusesQuery(data, hana, "posts").queryKey)).toBe(false);
});

/** A server where the reader follows `author` alone, whose one post is the reader's home timeline. */
const followed = (author = "sam"): FakeOptions => ({
  seed: {
    people: scenario.people,
    follows: [{ follower: "yuki", followee: author }],
    posts: [{ key: "post", author, text: "The crumb is finally open.", visibility: "public", language: "en" }],
  },
});

/** Reads the home timeline into the cache, as fetched at `at`. */
async function cacheHome(data: AppData, at: number) {
  const home = await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  data.queryClient.setQueryData(homeTimelineQuery(data).queryKey, home, { updatedAt: at });
}

test("a post's author shows at once, and its account is fetched in the background once the copy is stale", async () => {
  const { fake, data, requests } = fakeData(followed());
  const sam = fake.accountId("sam");
  await cacheHome(data, Date.now() - 60_000);
  // Since the timeline was read, the account's followers grew.
  fake.script("GET /accounts/{id}", { change: (account) => ({ ...account, followers_count: 9 }) });
  expect((await loadAccount(data, "sam")).followers_count).toBe(1);
  expect(requests).toEqual(["GET /timelines/home"]);
  const observer = new QueryObserver(data.queryClient, accountByIdQuery(data, sam));
  const shown: (number | undefined)[] = [];
  const done = new Promise<void>((resolve) => {
    observer.subscribe((result) => {
      shown.push(result.data?.followers_count);
      if (result.isSuccess && !result.isFetching) resolve();
    });
  });
  expect(observer.getCurrentResult().data?.followers_count).toBe(1);
  await done;
  expect(requests.slice(1)).toEqual([`GET /accounts/${sam}`]);
  expect(shown.at(-1)).toBe(9);
});

test("a fresh copy is not fetched again", async () => {
  const { fake, data, requests } = fakeData(followed());
  const sam = fake.accountId("sam");
  await cacheHome(data, Date.now());
  const observer = new QueryObserver(data.queryClient, accountByIdQuery(data, sam));
  const unsubscribe = observer.subscribe(() => undefined);
  expect(observer.getCurrentResult().data?.id).toBe(sam);
  expect(requests).toEqual(["GET /timelines/home"]);
  unsubscribe();
});

test("the newest copy is the first data", async () => {
  // The reader follows hana, who boosted a post of sam's: the home timeline holds sam's account inside the boost.
  const { fake, data } = fakeData({
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "hana" }],
      posts: [{ key: "post", author: "sam", text: "The crumb is finally open.", visibility: "public", language: "en" }],
      interactions: [{ kind: "boost", by: "hana", post: "post" }],
    },
  });
  const sam = fake.accountId("sam");
  const asked = fake.script("GET /accounts/{id}");
  // sam's posts were read a minute ago, when nobody followed sam.
  const posts = accountStatusesQuery(data, sam, "posts");
  const read = await data.queryClient.infiniteQuery(posts);
  data.queryClient.setQueryData(posts.queryKey, read, { updatedAt: Date.now() - 60_000 });
  // Since then the reader followed sam from another device, and the home timeline is read now.
  await fake.serve(new Request(`${fakeOrigin}/api/v1/accounts/${sam}/follow`, { method: "POST" }));
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  const account = await data.queryClient.query({ ...accountByIdQuery(data, sam), staleTime: Infinity });
  expect(account.followers_count).toBe(1);
  expect(cachedAccount(data, "SAM@demo.test")?.followers_count).toBe(1);
  expect(asked.seen).toHaveLength(0);
});

test("without a copy, the loader waits for the server", async () => {
  const { fake, data, requests } = fakeData();
  const marco = fake.accountId("marco");
  expect((await loadAccount(data, "sam")).id).toBe(fake.accountId("sam"));
  expect(paths(requests)).toEqual(["GET /accounts/lookup"]);
  const other = await data.queryClient.query({ ...accountByIdQuery(data, marco), staleTime: Infinity });
  expect(other.acct).toBe("marco");
  expect(requests.at(-1)).toBe(`GET /accounts/${marco}`);
});

test("an older copy in a timeline does not replace the account the query holds", async () => {
  const { fake, data } = fakeData(followed());
  const sam = fake.accountId("sam");
  fake.script("GET /accounts/{id}", { change: (account) => ({ ...account, followers_count: 5 }) });
  await data.queryClient.query(accountByIdQuery(data, sam));
  await cacheHome(data, Date.now() - 60_000);
  const account = await data.queryClient.query({ ...accountByIdQuery(data, sam), staleTime: Infinity });
  expect(account.followers_count).toBe(5);
  expect((await loadAccount(data, "sam")).followers_count).toBe(5);
});

test("the media of an account are its posts without replies, keyed apart from them", () => {
  const { fake, data } = fakeData();
  const sam = fake.accountId("sam");
  const posts = accountStatusesQuery(data, sam, "posts").queryKey;
  const media = accountStatusesQuery(data, sam, "media").queryKey;
  expect(posts).not.toEqual(media);
  expect(posts[2]).toEqual(media[2]);
});

describe("cachedAccount", () => {
  const remote = "sora@photos.example";

  test("finds a local account by its acct or its username@domain, in any case", async () => {
    const { fake, data } = fakeData(followed());
    await cacheHome(data, Date.now());
    for (const handle of ["sam", "SAM", "sam@demo.test", "Sam@Demo.TEST"])
      expect(cachedAccount(data, handle)?.id).toBe(fake.accountId("sam"));
  });

  test("finds a remote account by its acct in any case, and not by its username alone", async () => {
    const { fake, data } = fakeData(followed(remote));
    await cacheHome(data, Date.now());
    for (const handle of [remote, "SORA@Photos.Example"])
      expect(cachedAccount(data, handle)?.id).toBe(fake.accountId(remote));
    expect(cachedAccount(data, "sora")).toBeUndefined();
  });

  test("does not take an account of another server, or one the cache does not hold", async () => {
    const { data } = fakeData(followed(remote));
    await cacheHome(data, Date.now());
    expect(cachedAccount(data, "sora@social.example")).toBeUndefined();
    expect(cachedAccount(data, "sam")).toBeUndefined();
  });

  test("finds the author of a quoted post", async () => {
    // The reader follows hana alone, who quotes a post of sora's: only the quote carries sora's account.
    const { fake, data } = fakeData({
      seed: {
        people: scenario.people,
        follows: [{ follower: "yuki", followee: "hana" }],
        posts: [
          { key: "quoted", author: remote, text: "The grain came out just right.", visibility: "public", language: "en" },
          { key: "quoting", author: "hana", quote: "quoted", text: "Look at this.", visibility: "public", language: "en" },
        ],
      },
    });
    await cacheHome(data, Date.now());
    expect(cachedAccount(data, remote)?.id).toBe(fake.accountId(remote));
  });

  test("finds an account the cache holds from its lookup, or from its own query", async () => {
    const looked = fakeData();
    const sora = looked.fake.accountId(remote);
    expect(cachedAccount(looked.data, remote)).toBeUndefined();
    await looked.data.queryClient.query(accountQuery(looked.data, remote));
    expect(cachedAccount(looked.data, remote)?.id).toBe(sora);
    const fetched = fakeData();
    await fetched.data.queryClient.query(accountByIdQuery(fetched.data, sora));
    expect(cachedAccount(fetched.data, remote)?.id).toBe(sora);
  });
});

// In the scenario the reader (yuki) and sam follow each other.
describe("the viewer's relationship with an account", () => {
  const shown = (data: AppData, id: string) => probe(data, () => useRelationship(id));

  for (const [change, method, operation, field, value] of [
    ["subscribe", "POST", "subscribe", "notify", true],
    ["unsubscribe", "DELETE", "subscribe", "notify", false],
    ["remove-follower", "DELETE", "follower", "followed_by", false],
  ] as const)
    test(`${change} sends ${method} /accounts/{id}/${operation}, and the cache holds the relationship the server answered`, async () => {
      const { fake, data, requests } = fakeData({ subscriptions: change === "unsubscribe" ? ["sam"] : [] });
      const sam = fake.accountId("sam");
      await data.queryClient.query(relationshipQuery(data, sam));
      expect(shown(data, sam)?.[field]).toBe(!value);
      await changeRelationship(data, sam, change);
      expect(writes(requests)).toEqual([`${method} /accounts/${sam}/${operation}`]);
      expect(shown(data, sam)?.[field]).toBe(value);
    });

  test("a private note is written, and the cache holds it once the write resolves", async () => {
    const { fake, data, requests } = fakeData();
    const sam = fake.accountId("sam");
    const written = fake.script("PUT /accounts/{id}/note");
    await data.queryClient.query(relationshipQuery(data, sam));
    await writeAccountNote(data, sam, "Met at the bakery.");
    expect(writes(requests)).toEqual([`PUT /accounts/${sam}/note`]);
    expect(written.seen.map((s) => s.body)).toEqual([{ note: "Met at the bakery." }]);
    expect(shown(data, sam)?.note).toBe("Met at the bakery.");
  });

  test("an empty note deletes the note", async () => {
    const { fake, data, requests } = fakeData({ notes: { sam: "Met at the bakery." } });
    const sam = fake.accountId("sam");
    await data.queryClient.query(relationshipQuery(data, sam));
    expect(shown(data, sam)?.note).toBe("Met at the bakery.");
    await writeAccountNote(data, sam, "");
    expect(writes(requests)).toEqual([`DELETE /accounts/${sam}/note`]);
    expect(shown(data, sam)?.note).toBe("");
  });

  test("a note of the limit's characters (code points) is written; one more is refused, and the note stays", async () => {
    expect(accountNoteLimit).toBe(2000);
    const { fake, data } = fakeData({ notes: { sam: "before" } });
    const sam = fake.accountId("sam");
    await data.queryClient.query(relationshipQuery(data, sam));
    const full = "🍞".repeat(accountNoteLimit);
    await writeAccountNote(data, sam, full);
    expect(shown(data, sam)?.note).toBe(full);
    const refused = await writeAccountNote(data, sam, `${full}!`).then(
      () => undefined,
      (error: unknown) => error,
    );
    expect(refused).toBeInstanceOf(ProblemError);
    expect(shown(data, sam)?.note).toBe(full);
  });
});

describe("useAccount", () => {
  test("shows a post's author from the cache at once, with no request", async () => {
    const { fake, data, requests } = fakeData(followed());
    await cacheHome(data, Date.now() - 60_000);
    expect(probe(data, () => useAccount("sam")).account?.id).toBe(fake.accountId("sam"));
    expect(requests).toEqual(["GET /timelines/home"]);
  });

  test("without a handle, has nothing and asks nothing", () => {
    const { data, requests } = fakeData();
    expect(probe(data, () => useAccount(undefined))).toEqual({ account: undefined, error: null, missing: false });
    expect(requests).toHaveLength(0);
  });

  test("without a copy, moves from the lookup to the account's query without another request", async () => {
    const { fake, data, requests } = fakeData();
    const sam = fake.accountId("sam");
    expect(probe(data, () => useAccount("sam")).account).toBeUndefined();
    const looked = await loadAccount(data, "sam");
    expect(probe(data, () => useAccount("sam")).account).toEqual(looked);
    // The account's query starts from the lookup's answer, as fetched when it was: fresh, so a mount fetches nothing.
    const byId = data.queryClient.getQueryState<Account>(accountByIdQuery(data, sam).queryKey);
    const lookup = data.queryClient.getQueryState(accountQuery(data, "sam").queryKey);
    expect(byId?.data).toEqual(looked);
    expect(byId?.dataUpdatedAt).toBe(lookup?.dataUpdatedAt ?? Number.NaN);
    expect(byId?.isInvalidated).toBe(false);
    expect(paths(requests)).toEqual(["GET /accounts/lookup"]);
  });

  /**
   * The account's state as a screen that read it holds it: a render alone would ask again (retryOnMount) and show the
   * account pending.
   */
  function asRead({ queryClient }: AppData) {
    const queries = queryClient.getDefaultOptions().queries;
    queryClient.setDefaultOptions({ queries: { ...queries, retryOnMount: false } });
  }

  // nao deactivated their account, which the server answers as an unknown one: a 404.
  test("an account the server answers 404 for is missing", async () => {
    const { data } = fakeData();
    asRead(data);
    await loadAccount(data, "nao").catch(() => undefined);
    const shown = probe(data, () => useAccount("nao"));
    expect(shown.missing).toBe(true);
    expect(shown.account).toBeUndefined();
  });

  test("an account that failed otherwise is not missing: its failure shows, to try again", async () => {
    const { fake, data } = fakeData();
    asRead(data);
    fake.script("GET /accounts/lookup", { fail: 500 });
    await loadAccount(data, "sam").catch(() => undefined);
    const shown = probe(data, () => useAccount("sam"));
    expect(shown.missing).toBe(false);
    expect(shown.error).toBeInstanceOf(ProblemError);
  });

  test("an account is missing once the server says so, over the copy a post carried", async () => {
    const { fake, data } = fakeData(followed());
    asRead(data);
    const sam = fake.accountId("sam");
    await cacheHome(data, Date.now() - 60_000);
    expect(probe(data, () => useAccount("sam")).missing).toBe(false);
    fake.script("GET /accounts/{id}", { fail: 404 });
    await data.queryClient.query({ ...accountByIdQuery(data, sam), staleTime: 0 }).catch(() => undefined);
    expect(probe(data, () => useAccount("sam")).missing).toBe(true);
  });
});
