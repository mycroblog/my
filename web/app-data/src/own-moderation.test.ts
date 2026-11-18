// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { AppData } from "./app-data";
import {
  appealHookDecision,
  appealModerationAction,
  hookAppealQuery,
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
