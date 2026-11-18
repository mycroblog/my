// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { components } from "@mycroblog/mykit";
import type { AppData } from "./app-data";
import { postStatus } from "./compose";
import {
  appealHookDecision,
  appealModerationAction,
  appealRefusalOf,
  hookAppealQuery,
  hookRefusalOf,
  ownModerationActionsQuery,
  useHookAppeal,
  useOwnModerationActions,
  type ModerationAction,
} from "./own-moderation";
import type { Pages } from "./paged";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { refusedWith } from "./test/refused";

/** Reads the reader's actions, all and in effect, as two screens would. */
async function read(data: AppData) {
  await data.queryClient.infiniteQuery(ownModerationActionsQuery(data));
  await data.queryClient.infiniteQuery(ownModerationActionsQuery(data, true));
}

/** The actions the cache holds for a query, without reading the server. */
const cached = (data: AppData, active: boolean): readonly ModerationAction[] =>
  (
    data.queryClient.getQueryData(ownModerationActionsQuery(data, active).queryKey) as Pages<ModerationAction>
  ).pages.flatMap((p) => p.items);

describe("the reader's moderation actions", () => {
  test("list every action, newest first, and the ones in effect apart", async () => {
    const { data } = fakeData();
    await read(data);
    expect(probe(data, () => useOwnModerationActions()).items.map((a) => [a.kind, a.state])).toEqual([
      ["limit", "active"],
      ["mark_statuses_sensitive", "lifted"],
    ]);
    expect(probe(data, () => useOwnModerationActions(true)).items.map((a) => a.kind)).toEqual(["limit"]);
  });

  test("tell what can be appealed, and the appeal made, on each action", async () => {
    const { data } = fakeData();
    await read(data);
    const [limit, lifted] = probe(data, () => useOwnModerationActions()).items;
    expect([limit?.appealable, limit?.appeal]).toEqual([true, undefined]);
    expect(lifted?.appealable).toBe(false);
    expect(lifted?.appeal).toMatchObject({ state: "overturned" });
  });

  test("are none for a reader who was not acted on", async () => {
    const { data } = fakeData({ settings: { moderation: [] } });
    await read(data);
    expect(probe(data, () => useOwnModerationActions()).items).toEqual([]);
  });
});

describe("appealModerationAction", () => {
  test("sends the text, returns the action as appealed, and replaces it in the cached pages of both lists", async () => {
    const { fake, data, requests } = fakeData();
    await read(data);
    const limit = cached(data, false)[0];
    const sent = fake.script("POST /moderation/actions/{id}/appeal");
    const appealed = await appealModerationAction(data, limit?.id ?? "", "It was a misunderstanding.");
    expect(sent.seen.map((s) => s.body)).toEqual([{ text: "It was a misunderstanding." }]);
    expect(appealed).toMatchObject({ id: limit?.id, appealable: false, appeal: { state: "pending" } });
    expect(requests.filter((r) => !r.startsWith("GET "))).toEqual([`POST /moderation/actions/${limit?.id}/appeal`]);
    const readsBefore = requests.filter((r) => r.startsWith("GET ")).length;
    for (const active of [false, true]) {
      const [first] = cached(data, active);
      expect([active, first?.id, first?.appealable, first?.appeal?.text]).toEqual([
        active,
        limit?.id,
        false,
        "It was a misunderstanding.",
      ]);
    }
    // The other action stays as it was, and nothing was read again to show the change.
    expect(cached(data, false)[1]?.appeal?.state).toBe("overturned");
    expect(requests.filter((r) => r.startsWith("GET ")).length).toBe(readsBefore);
  });

  test("an action that cannot be appealed again is refused with the problem that says so", async () => {
    const { data } = fakeData();
    await read(data);
    const [limit, lifted] = cached(data, false);
    expect((await refusedWith(appealModerationAction(data, lifted?.id ?? "", "Again."))).type).toBe(
      "/api/v1/problems/not-appealable",
    );
    await appealModerationAction(data, limit?.id ?? "", "Once.");
    const second = await refusedWith(appealModerationAction(data, limit?.id ?? "", "Twice."));
    expect([second.status, second.type]).toEqual([409, "/api/v1/problems/not-appealable"]);
  });

  test("a text of blanks is refused with a 422, and the action stays appealable", async () => {
    const { data } = fakeData();
    await read(data);
    const limit = cached(data, false)[0];
    expect((await refusedWith(appealModerationAction(data, limit?.id ?? "", "   "))).status).toBe(422);
    expect(cached(data, false)[0]?.appealable).toBe(true);
    expect(cached(data, true)[0]?.appealable).toBe(true);
  });

  test("an action the reader does not have is refused as not found", async () => {
    const { data } = fakeData();
    await read(data);
    expect((await refusedWith(appealModerationAction(data, "gone", "Why?"))).status).toBe(404);
  });
});

/** What `write` was rejected with, whatever it is; fails the test when it resolved. */
async function rejection(write: Promise<unknown>): Promise<unknown> {
  const outcome = await write.then(
    () => ({ resolved: true }) as const,
    (error: unknown) => ({ error }),
  );
  if ("resolved" in outcome) throw new Error("the write was not rejected");
  return outcome.error;
}

/** A problem of the server of `type`, as it refuses an appeal. */
const refusal = (type: string, status: number) =>
  ({ type: `/api/v1/problems/${type}`, title: "This could not be done.", status }) as const;

const appealRoute = "POST /moderation/actions/{id}/appeal";

describe("what the rejection of an appeal means for the reader (appealRefusalOf)", () => {
  // The problem's type alone decides: the specification answers these with a 422, and a server may answer a 409.
  const told = [
    ["already-appealed", 422, "appealed"],
    ["appeal-window-closed", 422, "closed"],
    ["not-appealable", 422, "not_appealable"],
    ["not-appealable", 409, "not_appealable"],
    ["already-appealed", 409, "appealed"],
  ] as const;
  for (const [type, status, means] of told)
    test(`${type} (${String(status)}) is "${means}"`, async () => {
      const { fake, data } = fakeData();
      await read(data);
      fake.script(appealRoute, { fail: refusal(type, status) });
      const error = await rejection(appealModerationAction(data, cached(data, false)[0]?.id ?? "", "Why?"));
      expect(appealRefusalOf(error)).toBe(means);
    });

  test('an action the reader does not have (a 404) is "gone"', async () => {
    const { data } = fakeData();
    await read(data);
    expect(appealRefusalOf(await rejection(appealModerationAction(data, "gone", "Why?")))).toBe("gone");
  });

  test("a text the server refuses, a server that fails, and one that is not reached say nothing of the action", async () => {
    const { fake, data } = fakeData();
    await read(data);
    const id = cached(data, false)[0]?.id ?? "";
    // A 422 that names the text, of no type the client knows.
    expect(appealRefusalOf(await rejection(appealModerationAction(data, id, "   ")))).toBeUndefined();
    for (const fail of [500, "network"] as const) {
      const failing = fake.script(appealRoute, { fail });
      expect(appealRefusalOf(await rejection(appealModerationAction(data, id, "Why?")))).toBeUndefined();
      failing.end();
    }
    expect(appealRefusalOf(new Error("anything else"))).toBeUndefined();
  });
});

describe("an appeal the server refuses, in the cache", () => {
  /** The requests that read the reader's actions. */
  const reads = (requests: readonly string[]) => requests.filter((r) => r.startsWith("GET /moderation/actions")).length;

  // The server says that the action cannot be appealed any more: the lists say so at once, and the count of what
  // waits for the reader drops, without a read.
  const ended = [
    ["the time to appeal has passed", { settings: { appealSave: "closed" } }, undefined],
    ["the action can no longer be appealed", {}, refusal("not-appealable", 422)],
  ] as const;
  for (const [why, options, fail] of ended)
    test(`${why}: the action stays cached, no longer appealable and with no appeal, in both lists, and nothing is read again`, async () => {
      const { fake, data, requests } = fakeData(options);
      await read(data);
      const limit = cached(data, false)[0];
      expect(limit?.appealable).toBe(true);
      if (fail !== undefined) fake.script(appealRoute, { fail });
      const before = reads(requests);

      await rejection(appealModerationAction(data, limit?.id ?? "", "Why?"));

      for (const active of [false, true]) {
        const [first] = cached(data, active);
        expect([active, first?.id, first?.appealable, first?.appeal]).toEqual([active, limit?.id, false, undefined]);
      }
      // The other action stays as it was.
      expect(cached(data, false)[1]?.appeal?.state).toBe("overturned");
      await read(data);
      expect(reads(requests)).toBe(before);
    });

  // Anything else says nothing the lists can show by themselves: they are left as they are. The screen that sent the
  // appeal reads them again where the server may hold more than they do (app-features, account-status-action).
  const others = [
    ["the action was appealed already", refusal("already-appealed", 422)],
    ["the action is no longer the reader's", 404],
    ["the server fails", 500],
    ["the server is not reached", "network"],
  ] as const;
  for (const [why, fail] of others)
    test(`${why}: the cached lists are left as they were`, async () => {
      const { fake, data } = fakeData();
      await read(data);
      const before = [cached(data, false), cached(data, true)];
      fake.script(appealRoute, { fail });

      await rejection(appealModerationAction(data, before[0]?.[0]?.id ?? "", "Why?"));

      expect([cached(data, false), cached(data, true)]).toEqual(before);
      expect(cached(data, true)[0]?.appealable).toBe(true);
    });
});

describe("a post the server's automated review refused (hookRefusalOf)", () => {
  /** The problem of such a post, with the members given besides. */
  const rejected = (more: object): components["schemas"]["StatusRejectedProblem"] => ({
    type: "/api/v1/problems/status-rejected",
    title: "This post was not published.",
    status: 422,
    detail: "This server's automated review did not accept this post.",
    ...more,
  });
  const post = (data: AppData) => postStatus(data, { text: "Just a recipe.", visibility: "public" }, "key");

  test("names the decision the author may appeal, and the last moment to appeal it", async () => {
    const { fake, data } = fakeData();
    fake.script("POST /statuses", {
      fail: rejected({ hook_decision_id: "decision", appeal_deadline: "2031-03-09T12:00:00.000Z" }),
    });
    expect(hookRefusalOf(await rejection(post(data)))).toEqual({
      decision: "decision",
      deadline: "2031-03-09T12:00:00.000Z",
    });
  });

  test("is none for a refusal that names no decision, for another problem, and for anything else", async () => {
    const { fake, data } = fakeData();
    const unnamed = fake.script("POST /statuses", { fail: rejected({}) });
    expect(hookRefusalOf(await rejection(post(data)))).toBeUndefined();
    unnamed.end();
    // Another problem that happens to carry the member is no refusal of the review.
    const other = fake.script("POST /statuses", {
      fail: { ...rejected({ hook_decision_id: "decision" }), type: "/api/v1/problems/posting-restricted", status: 403 },
    });
    expect(hookRefusalOf(await rejection(post(data)))).toBeUndefined();
    other.end();
    expect(hookRefusalOf(new Error("anything else"))).toBeUndefined();
  });
});

describe("the appeal of a hook's decision", () => {
  test("is null when none was made (a 404), and undefined until it is read", async () => {
    const { data } = fakeData();
    expect(probe(data, () => useHookAppeal("decision"))).toBeUndefined();
    await data.queryClient.query(hookAppealQuery(data, "decision"));
    expect(probe(data, () => useHookAppeal("decision"))).toBeNull();
  });

  test("an appeal is made with the post's text, and the cache holds it for the decision", async () => {
    const { fake, data } = fakeData();
    await data.queryClient.query(hookAppealQuery(data, "decision"));
    const sent = fake.script("POST /hook_decisions/{id}/appeal");
    const made = await appealHookDecision(data, "decision", { status_text: "Just a recipe.", comment: "Please look." });
    expect(sent.seen.map((s) => s.body)).toEqual([{ status_text: "Just a recipe.", comment: "Please look." }]);
    expect(made).toMatchObject({ hook_decision_id: "decision", state: "pending" });
    expect(probe(data, () => useHookAppeal("decision"))).toEqual(made);
    expect(probe(data, () => useHookAppeal("another"))).toBeUndefined();
  });

  test("a read after the appeal finds it on the server", async () => {
    const { data } = fakeData();
    const made = await appealHookDecision(data, "decision", { status_text: "Just a recipe." });
    expect(await data.queryClient.query({ ...hookAppealQuery(data, "decision"), staleTime: 0 })).toEqual(made);
  });

  test("a decision appealed already is refused with the problem that says so, and a blank text with a 422", async () => {
    const { data } = fakeData();
    expect((await refusedWith(appealHookDecision(data, "decision", { status_text: " " }))).status).toBe(422);
    await appealHookDecision(data, "decision", { status_text: "Just a recipe." });
    const again = await refusedWith(appealHookDecision(data, "decision", { status_text: "Just a recipe." }));
    expect([again.status, again.type]).toEqual([409, "/api/v1/problems/already-appealed"]);
  });

  test("an error other than a 404 is not read as no appeal", async () => {
    const { fake, data } = fakeData();
    fake.script("GET /hook_decisions/{id}/appeal", { fail: 500 });
    await data.queryClient.query(hookAppealQuery(data, "decision")).catch(() => undefined);
    expect(probe(data, () => useHookAppeal("decision"))).toBeUndefined();
  });
});
