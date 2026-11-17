// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario, type FakeSeed } from "@my/e2e/fake";
import { voteInPoll } from "./polls";
import { shadowed } from "./shadow";
import type { Status } from "./statuses";
import { fakeData } from "./test/data";
import { statusQuery } from "./thread";
import { homeTimelineQuery } from "./timeline";

type Poll = NonNullable<Status["poll"]>;

/**
 * AppData on a server where sam, whom the reader follows, asks which loaf to bake, in a poll of one choice or of
 * `multiple`: hana and marco voted for the first, layla for the second. The home timeline and the post alone, as its
 * thread's screen reads it, are in the cache; `read()` is the poll each copy shows, `open` the poll as it was read.
 */
async function cached(multiple = false, more: Partial<NonNullable<FakeSeed["posts"][number]["poll"]>> = {}) {
  const { fake, data, requests } = fakeData({
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: [
        {
          key: "post",
          author: "sam",
          text: "Which loaf should I bake?",
          visibility: "public",
          language: "en",
          poll: { options: ["Sourdough", "Rye"], days: 3, multiple, ...more },
        },
      ],
      interactions: [
        { kind: "vote", by: "hana", post: "post", choices: [0] },
        { kind: "vote", by: "marco", post: "post", choices: [0] },
        { kind: "vote", by: "layla", post: "post", choices: [1] },
      ],
    },
  });
  const id = fake.idOf("post");
  const timeline = homeTimelineQuery(data).queryKey;
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  const open = (await data.queryClient.query(statusQuery(data, id))).poll;
  if (open === undefined) throw new Error("test: the post has no poll");
  requests.length = 0;
  const shown = (copy: Status | undefined) => (copy && shadowed(data, copy))?.poll;
  const read = () => ({
    timeline: shown(data.queryClient.getQueryData(timeline)?.pages[0]?.items[0]),
    thread: shown(data.queryClient.getQueryData(statusQuery(data, id).queryKey)),
  });
  return { fake, data, requests, read, timeline, id, open };
}

const votes = (poll: Poll | undefined) => poll?.options.map((o) => o.votes_count);

test("a vote of one choice reaches the server with the poll's ID, and every copy shows the poll it answered", async () => {
  const { fake, data, requests, read, id, open } = await cached();
  const sent = fake.script("POST /statuses/{id}/poll/votes");
  expect(open).toMatchObject({ voted: false, own_votes: [], votes_count: 3, voters_count: 3 });
  await voteInPoll(data, id, { poll: open.id, choices: [1] });
  expect(requests).toEqual([`POST /statuses/${id}/poll/votes`]);
  expect(sent.seen.map((s) => s.body)).toEqual([{ poll_id: open.id, choices: [1] }]);
  expect(read().timeline).toMatchObject({ voted: true, own_votes: [1], votes_count: 4, voters_count: 4 });
  expect(read().thread).toMatchObject({ voted: true, own_votes: [1] });
  expect(votes(read().thread)).toEqual([2, 2]);
});

test("a vote of several choices sends them all", async () => {
  const { fake, data, read, id, open } = await cached(true);
  const sent = fake.script("POST /statuses/{id}/poll/votes");
  await voteInPoll(data, id, { poll: open.id, choices: [0, 1] });
  expect(sent.seen.map((s) => s.body)).toEqual([{ poll_id: open.id, choices: [0, 1] }]);
  expect(read().timeline).toMatchObject({ voted: true, own_votes: [0, 1], votes_count: 5, voters_count: 4 });
});

test("in flight, no copy guesses the counts: the poll shows as it was until the server answers", async () => {
  const { fake, data, read, id, open } = await cached();
  const write = fake.script("POST /statuses/{id}/poll/votes", { hold: "response" });
  const done = voteInPoll(data, id, { poll: open.id, choices: [0] });
  expect(read().timeline).toEqual(open);
  expect(read().thread).toEqual(open);
  write.release();
  await done;
  expect(read().timeline).toMatchObject({ voted: true, own_votes: [0] });
});

test("a poll whose counts are hidden until it ends shows what the server answered, still without counts", async () => {
  const { fake, data, read, id, open } = await cached();
  // The poll hides its counts until it ends: the server leaves them out of every answer.
  const hidden = (poll: Poll): Poll => ({ ...poll, options: poll.options.map(({ title }) => ({ title })) });
  fake.script("POST /statuses/{id}/poll/votes", {
    change: (status) => ({ ...status, ...(status.poll === undefined ? {} : { poll: hidden(status.poll) }) }),
  });
  await voteInPoll(data, id, { poll: open.id, choices: [1] });
  expect(read().timeline).toMatchObject({ voted: true, own_votes: [1] });
  expect(read().timeline?.options).toEqual([{ title: "Sourdough" }, { title: "Rye" }]);
});

test("a fetch begun before the vote and answered after it does not undo the vote", async () => {
  const { fake, data, read, timeline, id, open } = await cached();
  // The timeline's read reaches the server before the vote, and its answer comes after the vote's.
  const page = fake.script("GET /timelines/home", { hold: "response" });
  const refetch = data.queryClient.refetchQueries({ queryKey: timeline });
  await page.served();
  await voteInPoll(data, id, { poll: open.id, choices: [1] });
  page.release();
  await refetch;
  expect(read().timeline).toMatchObject({ voted: true, own_votes: [1] });
});

test("a failed vote rejects, and every copy shows the poll as it was", async () => {
  for (const fail of [
    { type: "about:blank", title: "Unprocessable", status: 422, detail: "Already voted." },
    "network",
  ] as const) {
    const { fake, data, read, id, open } = await cached();
    fake.script("POST /statuses/{id}/poll/votes", { fail });
    const rejected = await voteInPoll(data, id, { poll: open.id, choices: [0] }).then(
      () => false,
      () => true,
    );
    expect(rejected).toBe(true);
    expect(read().timeline).toEqual(open);
    expect(read().thread).toEqual(open);
  }
});
