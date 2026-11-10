// SPDX-License-Identifier: AGPL-3.0-only
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import type { DraftRequest, DraftVersion } from "../../compose";
import { draftFixture } from "../../../test/fixtures";
import { ArticleAutosaver, AutosaveConflictError, autosaveDelay, type DraftWriter } from "./autosaver";

/** A draft request of the body `text`. */
function body(text: string): DraftRequest {
  return { text, visibility: "public", article: { title: "A title", summary: "", tags: [] } };
}

class Conflict extends Error {}

/** A server of one draft: each save is recorded; `conflictOn` answers a 412 to the update of that ETag. */
function writer(conflictOn: string | null = null) {
  const writes: string[] = [];
  let version = 0;
  let next = 0;
  const answer = (id: string, request: DraftRequest): DraftVersion => {
    version += 1;
    return { draft: draftFixture(id, request), etag: `"v${String(version)}"` };
  };
  const draftWriter: DraftWriter = {
    create: (request) => {
      next += 1;
      writes.push(`POST ${request.text ?? ""}`);
      return Promise.resolve(answer(`d${String(next)}`, request));
    },
    update: (id, etag, request) => {
      writes.push(`PUT ${id} ${etag} ${request.text ?? ""}`);
      if (etag === conflictOn) return Promise.reject(new Conflict("412"));
      return Promise.resolve(answer(id, request));
    },
    isConflict: (error) => error instanceof Conflict,
  };
  return { writes, draftWriter };
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("the autosave of an article's draft", () => {
  test("saves the last change once the author pauses, making the draft first and replacing it next", async () => {
    const { writes, draftWriter } = writer();
    const saver = new ArticleAutosaver(draftWriter, null);
    saver.change(body("H"));
    saver.change(body("Hello"));
    await vi.advanceTimersByTimeAsync(autosaveDelay);
    saver.change(body("Hello, world"));
    await vi.advanceTimersByTimeAsync(autosaveDelay);

    expect(writes).toEqual(["POST Hello", 'PUT d1 "v1" Hello, world']);
    expect(saver.getSnapshot()).toEqual({
      status: { kind: "saved" },
      link: { id: "d1", etag: '"v2"', scheduledAt: null },
    });
  });

  test("stops on a conflict without overwriting the other device's save, and saves the text as a new draft", async () => {
    const { writes, draftWriter } = writer('"v0"');
    const saver = new ArticleAutosaver(draftWriter, { id: "d0", etag: '"v0"', scheduledAt: null });
    saver.change(body("Mine"));
    await vi.advanceTimersByTimeAsync(autosaveDelay);

    expect(saver.getSnapshot().status).toEqual({ kind: "conflict" });
    // Further changes wait for the author: nothing is sent behind their back.
    saver.change(body("Mine, still"));
    await vi.advanceTimersByTimeAsync(autosaveDelay * 2);
    await expect(saver.flush(body("Mine, still"))).rejects.toBeInstanceOf(AutosaveConflictError);
    expect(writes).toEqual(['PUT d0 "v0" Mine']);

    const link = await saver.saveAsNew(body("Mine, still"));
    expect(link.id).toBe("d1");
    expect(writes).toEqual(['PUT d0 "v0" Mine', "POST Mine, still"]);
    expect(saver.getSnapshot().status).toEqual({ kind: "saved" });
  });

  test("keeps a change that failed to save and saves it with the next one", async () => {
    const { writes, draftWriter } = writer();
    let failing = true;
    const saver = new ArticleAutosaver(
      {
        ...draftWriter,
        create: (request) => (failing ? Promise.reject(new Error("offline")) : draftWriter.create(request)),
      },
      null,
    );
    saver.change(body("First"));
    await vi.advanceTimersByTimeAsync(autosaveDelay);
    expect(saver.getSnapshot().status.kind).toBe("failed");

    failing = false;
    const link = await saver.flush(body("First, again"));
    expect(link.id).toBe("d1");
    expect(writes).toEqual(["POST First, again"]);
  });

  test("saves the last change at once when the editor closes", async () => {
    const { writes, draftWriter } = writer();
    const saver = new ArticleAutosaver(draftWriter, null);
    saver.change(body("Typed just now"));
    saver.dispose();
    await vi.advanceTimersByTimeAsync(0);

    expect(writes).toEqual(["POST Typed just now"]);
  });
});
