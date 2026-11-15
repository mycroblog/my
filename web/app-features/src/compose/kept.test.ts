// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { defaultComposeSettings, type SavedDraft } from "@my/app-data";
import { KeptDrafts, type KeptWrites } from "./kept";
import { emptyDraft, type Draft } from "./model";

/** Writes whose answers the test gives, one by one, and what they were asked as `save <id or new>` or `delete <id>`. */
function server() {
  const asked: string[] = [];
  const answers: ((saved: SavedDraft) => void)[] = [];
  let next = 0;
  const writes: KeptWrites = {
    saveDraft: (_body, saved) => {
      asked.push(`save ${saved?.id ?? "new"}`);
      return new Promise((resolve) => answers.push(resolve));
    },
    deleteDraft: (id) => {
      asked.push(`delete ${id}`);
      return Promise.resolve();
    },
  };
  /** Answers the oldest save waiting, with a new server draft or the one it was saved over. */
  const answer = async (id = `d${String((next += 1))}`) => {
    // A write starts after the one before it, a turn later.
    await Bun.sleep(0);
    answers.shift()?.({ id, etag: '"1"' });
    await Bun.sleep(0);
  };
  return { writes, asked, answer };
}

const written = (key: string, text: string): Draft => ({
  ...emptyDraft({ kind: "new" }, defaultComposeSettings, key),
  text,
});

test("a draft left, opened again and left before the server answered is saved over the same server draft", async () => {
  const { writes, asked, answer } = server();
  const kept = new KeptDrafts();
  kept.keep(written("k1", "first"), writes, defaultComposeSettings);
  // Opened again from what the page keeps, before the first save answered, and left again.
  const reopened = kept.get("new");
  expect(reopened?.saved).toBeUndefined();
  kept.keep({ ...written("k1", "first, then more") }, writes, defaultComposeSettings);
  await Bun.sleep(0);
  // The second save waits for the first.
  expect(asked).toEqual(["save new"]);
  await answer();
  await answer("d1");
  expect(asked).toEqual(["save new", "save d1"]);
  expect(kept.get("new")?.saved?.id).toBe("d1");
});

test("a draft discarded while its save is in flight is deleted once saved, leaving none", async () => {
  const { writes, asked, answer } = server();
  const kept = new KeptDrafts();
  kept.keep(written("k1", "left"), writes, defaultComposeSettings);
  kept.forget(written("k1", "left"), writes);
  expect(kept.get("new")).toBeUndefined();
  await answer();
  expect(asked).toEqual(["save new", "delete d1"]);
});

test("a draft posted from the composer while its save was in flight deletes the server draft that save made", async () => {
  const { writes, asked, answer } = server();
  const kept = new KeptDrafts();
  kept.keep(written("k1", "left"), writes, defaultComposeSettings);
  // Posted from the composer opened again, which had no server draft yet.
  kept.settle(written("k1", "left"), writes);
  await answer();
  expect(asked).toEqual(["save new", "delete d1"]);
});

test("a draft posted from its own server draft leaves it to the server", async () => {
  const { writes, asked, answer } = server();
  const kept = new KeptDrafts();
  kept.keep(written("k1", "left"), writes, defaultComposeSettings);
  await answer();
  const reopened = kept.get("new");
  expect(reopened?.saved?.id).toBe("d1");
  if (reopened) kept.settle(reopened, writes);
  await Bun.sleep(0);
  expect(asked).toEqual(["save new"]);
});

test("another post of the same target is not saved over the server draft of the one before", async () => {
  const { writes, asked, answer } = server();
  const kept = new KeptDrafts();
  kept.keep(written("k1", "one post"), writes, defaultComposeSettings);
  kept.keep(written("k2", "another post"), writes, defaultComposeSettings);
  await answer();
  await answer();
  expect(asked).toEqual(["save new", "save new"]);
});
