// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { voteInPoll } from "./polls";
import { shadowed } from "./shadow";
import type { Status } from "./statuses";
import { fakeAppData, json, status, type Handler } from "./test/fake";
import { homeTimelineQuery } from "./timeline";

type Poll = NonNullable<Status["poll"]>;

const open = {
  id: "p1",
  expires_at: "2026-11-20T00:00:00Z",
  expired: false,
  multiple: false,
  votes_count: 3,
  voters_count: 3,
  options: [
    { title: "Sourdough", votes_count: 2 },
    { title: "Rye", votes_count: 1 },
  ],
  voted: false,
  own_votes: [],
  emojis: [],
} as Poll;

const post = (poll: Poll = open) => status("1", { poll, favourites_count: 0, boosts_count: 0, reactions: [] });

/** The poll as the server counts it after a vote for `choices`. */
const counted = (choices: number[], over: Partial<Poll> = {}): Poll => ({
  ...open,
  votes_count: open.votes_count + choices.length,
  voters_count: (open.voters_count ?? 0) + 1,
  options: open.options.map((o, i) => ({ ...o, votes_count: (o.votes_count ?? 0) + (choices.includes(i) ? 1 : 0) })),
  voted: true,
  own_votes: choices,
  ...over,
});

/**
 * AppData whose server answers handler, with the home timeline and the thread of post 0 holding post 1, and the poll
 * each copy shows.
 */
function cached(handler: Handler, poll: Poll = open) {
  const { data } = fakeAppData(handler);
  const timeline = homeTimelineQuery(data).queryKey;
  const context = data.queries.queryOptions("get", "/statuses/{id}/context", {
    params: { path: { id: "0" } },
  }).queryKey;
  data.queryClient.setQueryData(timeline, { pages: [{ items: [post(poll)] }], pageParams: [undefined] });
  data.queryClient.setQueryData(context, { ancestors: [], descendants: [post(poll)] });
  const shown = (copy: Status | undefined) => (copy && shadowed(data, copy))?.poll;
  const read = () => ({
    timeline: shown(data.queryClient.getQueryData(timeline)?.pages[0]?.items[0]),
    thread: shown(data.queryClient.getQueryData<{ descendants: Status[] }>(context)?.descendants[0]),
  });
  return { data, read, timeline };
}

type Vote = { poll_id: string; choices: number[] };

/** A server that records each vote and answers it with `answer`, the status as the vote left it. */
function votes(answer: (body: Vote) => Response | Promise<Response>) {
  const sent: { method: string; path: string; body: unknown }[] = [];
  const handler: Handler = async (request) => {
    const body = (await request.json()) as Vote;
    sent.push({ method: request.method, path: new URL(request.url).pathname, body });
    return answer(body);
  };
  return { sent, handler };
}

test("a vote of one choice reaches the server with the poll's ID, and every copy shows the poll it answered", async () => {
  const { sent, handler } = votes((body) => json(200, post(counted(body.choices))));
  const { data, read } = cached(handler);
  await voteInPoll(data, "1", { poll: "p1", choices: [1] });
  expect(sent).toEqual([
    { method: "POST", path: "/api/v1/statuses/1/poll/votes", body: { poll_id: "p1", choices: [1] } },
  ]);
  expect(read().timeline).toMatchObject({ voted: true, own_votes: [1], votes_count: 4, voters_count: 4 });
  expect(read().thread).toMatchObject({ voted: true, own_votes: [1] });
  expect(read().thread?.options.map((o) => o.votes_count)).toEqual([2, 2]);
});

test("a vote of several choices sends them all", async () => {
  const multiple = { ...open, multiple: true };
  const { sent, handler } = votes((body) => json(200, post(counted(body.choices, { multiple: true }))));
  const { data, read } = cached(handler, multiple);
  await voteInPoll(data, "1", { poll: "p1", choices: [0, 1] });
  expect(sent.map((s) => s.body)).toEqual([{ poll_id: "p1", choices: [0, 1] }]);
  expect(read().timeline).toMatchObject({ voted: true, own_votes: [0, 1], votes_count: 5, voters_count: 4 });
});

test("in flight, no copy guesses the counts: the poll shows as it was until the server answers", async () => {
  const { promise: answered, resolve: answer } = Promise.withResolvers<Response>();
  const { handler } = votes(() => answered);
  const { data, read } = cached(handler);
  const done = voteInPoll(data, "1", { poll: "p1", choices: [0] });
  expect(read().timeline).toEqual(open);
  expect(read().thread).toEqual(open);
  answer(json(200, post(counted([0]))));
  await done;
  expect(read().timeline).toMatchObject({ voted: true, own_votes: [0] });
});

test("a poll whose counts are hidden until it ends shows what the server answered, still without counts", async () => {
  const hidden: Poll = { ...open, options: open.options.map(({ title }) => ({ title })) };
  const { handler } = votes(() => json(200, post({ ...hidden, voted: true, own_votes: [1], votes_count: 4 })));
  const { data, read } = cached(handler, hidden);
  await voteInPoll(data, "1", { poll: "p1", choices: [1] });
  expect(read().timeline).toMatchObject({ voted: true, own_votes: [1] });
  expect(read().timeline?.options).toEqual([{ title: "Sourdough" }, { title: "Rye" }]);
});

test("a fetch begun before the vote and answered after it does not undo the vote", async () => {
  const { promise: page, resolve: answerPage } = Promise.withResolvers<Response>();
  const handler: Handler = (request) => (request.method === "GET" ? page : json(200, post(counted([1]))));
  const { data, read, timeline } = cached(handler);
  const refetch = data.queryClient.refetchQueries({ queryKey: timeline });
  await voteInPoll(data, "1", { poll: "p1", choices: [1] });
  answerPage(json(200, { items: [post()] }));
  await refetch;
  expect(read().timeline).toMatchObject({ voted: true, own_votes: [1] });
});

test("a failed vote rejects, and every copy shows the poll as it was", async () => {
  for (const failure of [
    () => json(422, { type: "about:blank", title: "Unprocessable", status: 422, detail: "Already voted." }),
    () => Promise.reject(new TypeError("Failed to fetch")),
  ]) {
    const { handler } = votes(failure);
    const { data, read } = cached(handler);
    const rejected = await voteInPoll(data, "1", { poll: "p1", choices: [0] }).then(
      () => false,
      () => true,
    );
    expect(rejected).toBe(true);
    expect(read().timeline).toEqual(open);
    expect(read().thread).toEqual(open);
  }
});
