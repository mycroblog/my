// SPDX-License-Identifier: AGPL-3.0-only
import { beforeEach, describe, expect, test } from "vitest";
import { composeKey } from "../../../lib/storage";
import { DraftStore } from "./draftStore";

const key = composeKey("account-1");
const body = { text: "Hi", visibility: "public" } as const;

beforeEach(() => {
  sessionStorage.clear();
});

describe("DraftStore", () => {
  test("saves the draft for the next store of the same account, but not files still uploading", () => {
    const store = new DraftStore(key);
    store.edit({
      text: "Draft",
      attachments: [
        { key: "a", mediaId: "media-1", description: "A cat", previewUrl: "blob:a", error: null },
        { key: "b", mediaId: null, description: "", previewUrl: "blob:b", error: null },
      ],
    });

    const next = new DraftStore(key).getSnapshot();
    expect(next.draft.text).toBe("Draft");
    expect(next.draft.attachments).toEqual([
      { key: "a", mediaId: "media-1", description: "A cat", previewUrl: null, error: null },
    ]);
    expect(new DraftStore(composeKey("account-2")).getSnapshot().draft.text).toBe("");
  });

  test("sending moves the draft into the outbox and keeps the visibility and language for the next one", () => {
    const store = new DraftStore(key);
    store.edit({ text: "Hi", visibility: "private", language: "ja" });
    store.send("key-1", body);

    const { draft, outbox } = store.getSnapshot();
    expect(draft).toMatchObject({ text: "", visibility: "private", language: "ja" });
    expect(outbox).toEqual([
      {
        idempotencyKey: "key-1",
        body,
        draft: expect.objectContaining({ text: "Hi", idempotencyKey: "key-1" }) as unknown,
      },
    ]);
    expect(new DraftStore(key).getSnapshot().outbox).toHaveLength(1);
  });

  test("restores a failed post with its idempotency key only into an empty form; an edit drops the key", () => {
    const store = new DraftStore(key);
    store.edit({ text: "Hi" });
    store.send("key-1", body);
    store.edit({ text: "Another" });

    expect(store.restore("key-1", "failed")).toBe(false);
    expect(store.getSnapshot().draft.text).toBe("Another");

    store.edit({ text: "" });
    expect(store.restore("key-1", "failed")).toBe(true);
    expect(store.getSnapshot()).toMatchObject({
      draft: { text: "Hi", idempotencyKey: "key-1" },
      outbox: [],
      error: "failed",
    });

    store.edit({ text: "Hi!" });
    expect(store.getSnapshot()).toMatchObject({ draft: { idempotencyKey: null }, error: null });
  });

  test("starts empty from an unreadable saved state", () => {
    sessionStorage.setItem(key.name, "{not json");
    expect(new DraftStore(key).getSnapshot().draft.text).toBe("");
  });
});
