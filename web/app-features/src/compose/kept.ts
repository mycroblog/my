// SPDX-License-Identifier: AGPL-3.0-only

// The drafts left by accident (layer.tsx), by their target's key: kept on this page at once, and on the server (ADR-0156)
// one write after another. A draft left, opened again and left before the server answered is saved over the server
// draft the first save made, and one forgotten, posted or kept meanwhile leaves no server draft behind: each write
// waits for the one before, and takes the server draft it saved when it is of the same post (its key). No React here.

import type { ComposeSettings, ComposeWrites, SavedDraft } from "@my/app-data";
import { boardOf, onServer, targetKey, toDraft, type Draft } from "./model";

/** The writes the drafts left by accident need. */
export type KeptWrites = Pick<ComposeWrites, "saveDraft" | "deleteDraft">;

/** What the last write of a target left on the server: the server draft of the post of `key`, or none. */
type Written = { readonly key: string; readonly saved: SavedDraft } | undefined;

export class KeptDrafts {
  readonly #drafts = new Map<string, Draft>();
  readonly #writing = new Map<string, Promise<Written>>();

  /** The draft left for a target's key. */
  get(key: string): Draft | undefined {
    return this.#drafts.get(key);
  }

  /** Keeps a draft on this page alone (an edit left by accident, a discarded post brought back). */
  set(key: string, d: Draft): void {
    this.#drafts.set(key, d);
  }

  /** Forgets the draft of a key on this page alone (a draft opened from the server stays there as saved). */
  delete(key: string): void {
    this.#drafts.delete(key);
  }

  /** Writes after the last write of `key`, which `write` hears. */
  #after(key: string, write: (last: Written) => Promise<Written>): void {
    const done = (this.#writing.get(key) ?? Promise.resolve(undefined)).then(write);
    this.#writing.set(key, done);
    void done.finally(() => {
      if (this.#writing.get(key) === done) this.#writing.delete(key);
    });
  }

  /** The server draft of `d`: the one its last write saved, else the one it was opened with. */
  static #savedOf(d: Draft, last: Written): SavedDraft | undefined {
    return last?.key === d.key ? last.saved : d.saved;
  }

  /** Saves a draft left by accident: kept here at once, and on the server; the server's answer is kept with it. */
  keep(d: Draft, writes: KeptWrites, settings: ComposeSettings): void {
    const key = targetKey(d.target);
    // An image still uploading, or failed, is not kept: the server has no file of it (but a draft's it could not read).
    const draft = { ...d, attachments: d.attachments.filter(onServer) };
    this.#drafts.set(key, draft);
    // An edit is no new post, and a post on an event's board would leave its board as a draft: the server's drafts
    // hold neither.
    if (d.target.kind === "edit" || boardOf(d.target) !== undefined) return;
    this.#after(key, async (last) => {
      const before = KeptDrafts.#savedOf(draft, last);
      try {
        const saved = await writes.saveDraft(toDraft(draft, settings), before);
        if (this.#drafts.get(key) === draft) this.#drafts.set(key, { ...draft, saved });
        return { key: draft.key, saved };
      } catch {
        // Not saved on the server: the page still keeps it.
        return before && { key: draft.key, saved: before };
      }
    });
  }

  /** Forgets a draft: here, and on the server, once what was being saved of it is saved. */
  forget(d: Draft, writes: KeptWrites): void {
    const key = targetKey(d.target);
    this.#drafts.delete(key);
    this.#after(key, async (last) => {
      const saved = KeptDrafts.#savedOf(d, last);
      if (saved !== undefined) await writes.deleteDraft(saved.id).catch(() => undefined);
      return undefined;
    });
  }

  /**
   * Forgets a draft that went out from the composer (posted, or kept as a draft): a server draft saved of it while it
   * was open is deleted, but the one it went out from, which the server deleted or holds.
   */
  settle(d: Draft, writes: KeptWrites): void {
    const key = targetKey(d.target);
    this.#drafts.delete(key);
    if (d.target.kind === "edit" || boardOf(d.target) !== undefined) return;
    this.#after(key, async (last) => {
      if (last?.key === d.key && last.saved.id !== d.saved?.id)
        await writes.deleteDraft(last.saved.id).catch(() => undefined);
      return undefined;
    });
  }
}
