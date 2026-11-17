// SPDX-License-Identifier: AGPL-3.0-only

import { expect, jest, test } from "bun:test";
import { scenario, type FakeOptions, type FakeSeed } from "@my/e2e/fake";
import { NetworkError, type components } from "@mycroblog/mykit";
import type { AppData } from "./app-data";
import { choiceOf, postStatus, refusal, refusedField, ruleOf, saveDraft, uploadAttachment } from "./compose";
import { cachedStatus } from "./shadow";
import { fakeData } from "./test/data";
import { statusContextQuery, type StatusContext } from "./thread";
import { homeTimelineQuery } from "./timeline";

const ids = (data: AppData) =>
  data.queryClient.getQueryData(homeTimelineQuery(data).queryKey)?.pages[0]?.items.map((s) => s.id);

const by = (author: string, key: string, replyTo?: string): FakeSeed["posts"][number] => ({
  key,
  author,
  text: `Post ${key}.`,
  visibility: "public",
  language: "en",
  ...(replyTo === undefined ? {} : { replyTo }),
});

/** A server where the reader follows sam, who posted `parent` and then `other`. */
const followed: FakeOptions = {
  seed: {
    people: scenario.people,
    follows: [{ follower: "yuki", followee: "sam" }],
    posts: [by("sam", "parent"), by("sam", "other")],
  },
};

test("a posted status tops the home timeline at once, and its parent counts one more reply", async () => {
  const { fake, data } = fakeData(followed);
  const parent = fake.idOf("parent");
  const sent = fake.script("POST /statuses");
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  expect(cachedStatus(data, parent)?.replies_count).toBe(0);
  const posted = await postStatus(data, { text: "hi", visibility: "public", in_reply_to_id: parent }, "key-1");
  expect(sent.seen.map((s) => s.headers.get("Idempotency-Key"))).toEqual(["key-1"]);
  expect(ids(data)).toEqual([posted.id, fake.idOf("other"), parent]);
  expect(cachedStatus(data, parent)?.replies_count).toBe(1);
});

test("a parent the server answered with the reply counted already does not count it again", async () => {
  const { fake, data } = fakeData(followed);
  const parent = fake.idOf("parent");
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  // The timeline is read again while the reply is posted, and the server answers with the reply counted.
  const again = fake.script("GET /timelines/home", { hold: "request" });
  const refetch = data.queryClient.refetchQueries({ queryKey: homeTimelineQuery(data).queryKey });
  await again.arrived();
  await postStatus(data, { text: "hi", visibility: "public", in_reply_to_id: parent }, "key-1");
  expect(cachedStatus(data, parent)?.replies_count).toBe(1);
  again.release();
  await refetch;
  expect(cachedStatus(data, parent)?.replies_count).toBe(1);
});

test("a posted reply shows in a cached thread of its parent, after the replies already under it", async () => {
  const { fake, data } = fakeData({
    seed: {
      people: scenario.people,
      posts: [by("sam", "root"), by("sam", "1", "root"), by("sam", "2", "1"), by("sam", "3", "root")],
    },
  });
  const context = statusContextQuery(data, fake.idOf("root"));
  await data.queryClient.query(context);
  const posted = await postStatus(data, { text: "hi", visibility: "public", in_reply_to_id: fake.idOf("1") }, "key-1");
  expect(data.queryClient.getQueryData<StatusContext>(context.queryKey)?.descendants.map((s) => s.id)).toEqual([
    fake.idOf("1"),
    fake.idOf("2"),
    posted.id,
    fake.idOf("3"),
  ]);
});

type Problem = components["schemas"]["Problem"];

/** The failure of a post the server refuses with `problem`. */
async function refused(problem: Problem) {
  const { fake, data } = fakeData();
  fake.script("POST /statuses", { fail: problem });
  return postStatus(data, { text: "hi", visibility: "public" }, "k").catch((e: unknown) => e);
}

test("a refused post says the server's words; one without an answer says nothing of its own", async () => {
  const error = await refused({
    type: "/api/v1/problems/status-rejected",
    title: "Refused",
    status: 422,
    detail: "Too long.",
  });
  expect(refusal(error)).toBe("Too long.");
  expect(refusal(new NetworkError("POST", "/statuses", new Error("offline")))).toBeUndefined();
});

test("a 422 that names a member gives the member, what is wrong with its value, and the problem's title", async () => {
  const invalid: Problem = { type: "about:blank", title: "This could not be done.", status: 422 };
  const detail = "This value is not valid. Check what you entered.";
  expect(refusedField(await refused({ ...invalid, detail, field: "poll.options[1]" }))).toEqual({
    member: "poll.options[1]",
    detail,
    title: "This could not be done.",
  });
  // The server's words are the form's alone: no member named, an empty one, or nothing said of its value.
  expect(refusedField(await refused({ ...invalid, detail }))).toBeUndefined();
  expect(refusedField(await refused({ ...invalid, detail, field: "" }))).toBeUndefined();
  expect(refusedField(await refused({ ...invalid, field: "text" }))).toBeUndefined();
  expect(refusedField(await refused({ ...invalid, detail: "", field: "text" }))).toBeUndefined();
  // Only a 422 is about the value of a member.
  expect(refusedField(await refused({ ...invalid, status: 400, detail, field: "text" }))).toBeUndefined();
  expect(refusedField(new NetworkError("POST", "/statuses", new Error("offline")))).toBeUndefined();
  expect(refusedField(new Error("text"))).toBeUndefined();
});

/**
 * What `upload` settles with, with the test's clock in place of the second the app waits before it looks at a file
 * the server still processes.
 */
async function processed<T>(upload: () => Promise<T>): Promise<T | Error> {
  jest.useFakeTimers();
  try {
    let outcome: { value: T | Error } | undefined;
    upload().then(
      (value) => (outcome = { value }),
      (error: unknown) => (outcome = { value: error instanceof Error ? error : new Error(String(error)) }),
    );
    for (let turn = 0; turn < 100 && outcome === undefined; turn += 1) {
      await new Promise((next) => setImmediate(next));
      jest.advanceTimersByTime(250);
    }
    if (outcome === undefined) throw new Error("test: the upload did not settle");
    return outcome.value;
  } finally {
    jest.useRealTimers();
  }
}

test("an upload waits until the server has processed the file, and a file it could not process fails", async () => {
  const { data } = fakeData({ uploads: "processing" });
  let processing = false;
  const media = await processed(() =>
    uploadAttachment(data, new File(["x"], "a.png", { type: "image/png" }), {
      onProcessing: () => {
        processing = true;
      },
    }),
  );
  expect(processing).toBe(true);
  expect(media).toMatchObject({ state: "ready" });

  const failing = fakeData({ uploads: "processing" });
  failing.fake.script("GET /media/{id}", { change: (file) => ({ ...file, state: "failed" as const }) });
  const error = await processed(() => uploadAttachment(failing.data, new File(["x"], "a.png")));
  expect((error as Error).name).toBe("MediaFailedError");
});

test("a draft saves over the one saved before, and as a new one when another device saved over it", async () => {
  const { fake, data } = fakeData();
  const draft = fake.idOf("yuki-draft-river");
  const saves = [fake.script("PUT /drafts/{id}"), fake.script("POST /drafts")] as const;
  // The version this device read is not the one the server holds any more.
  const saved = await saveDraft(data, { visibility: "public" }, { id: draft, etag: '"1"' });
  expect(saves[0].seen.map((s) => s.headers.get("If-Match"))).toEqual(['"1"']);
  expect(saves[1].seen).toHaveLength(1);
  expect(saved.id).not.toBe(draft);
  expect(saved.etag).toMatch(/^"\d+"$/);
});

test("a choice with approval lets everyone else ask; without it, or for everyone, the rule has no manual audience", () => {
  expect(ruleOf({ audience: "followers", approval: true })).toEqual({ automatic: "followers", manual: "everyone" });
  expect(ruleOf({ audience: "nobody", approval: true })).toEqual({ automatic: "nobody", manual: "everyone" });
  expect(ruleOf({ audience: "mentioned", approval: false })).toEqual({ automatic: "mentioned" });
  expect(ruleOf({ audience: "everyone", approval: true })).toEqual({ automatic: "everyone" });
});

test("a rule reads as a choice, any manual audience as approval, and a choice's rule reads back as the choice", () => {
  expect(choiceOf({ automatic: "followers", manual: "everyone" })).toEqual({ audience: "followers", approval: true });
  expect(choiceOf({ automatic: "nobody", manual: "followers" })).toEqual({ audience: "nobody", approval: true });
  expect(choiceOf({ automatic: "everyone" })).toEqual({ audience: "everyone", approval: false });
  for (const rule of [{ automatic: "mentioned", manual: "everyone" }, { automatic: "nobody" }] as const)
    expect(ruleOf(choiceOf(rule))).toEqual(rule);
});

test("a posted reply that waits for approval does not count on its parent; one admitted at once does", async () => {
  // layla approves by hand the replies of all but those her post mentions (the fake's `approvals`).
  const { fake, data } = fakeData({ approvals: true });
  const asks = fake.idOf("layla-asks");
  const admits = fake.idOf("layla-jokes");
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  const [before, counted] = [cachedStatus(data, asks)?.replies_count, cachedStatus(data, admits)?.replies_count];
  expect([before, counted]).toEqual([0, 0]);
  const waits = await postStatus(data, { text: "hi", visibility: "public", in_reply_to_id: asks }, "key-1");
  expect(waits.approval).toBe("pending");
  expect(cachedStatus(data, asks)?.replies_count).toBe(0);
  await postStatus(data, { text: "hi", visibility: "public", in_reply_to_id: admits }, "key-2");
  expect(cachedStatus(data, admits)?.replies_count).toBe(1);
});
