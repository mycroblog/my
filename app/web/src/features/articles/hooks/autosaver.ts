// SPDX-License-Identifier: AGPL-3.0-only
// The autosave of an article's draft on the server (ADR-0156, web-client/screens/article-editor.md §4), outside React:
// the editor tells it every change, and it saves the latest one once the author pauses, one save at a time. The first
// save makes the draft (POST /drafts); the next ones replace the version they read (PUT /drafts/{id} with If-Match).
//
// A save that another device's save overtook (412) overwrites nothing: the autosave stops in `conflict`, keeping the
// link and the change, until the author saves the text as a new draft (`saveAsNew`). The editor keeps its text in
// every case, so a conflict never loses what was typed; nothing is saved behind the author's back either.
import type { DraftRequest, DraftVersion } from "../../compose";

/** How long the author pauses before the change is saved, in ms. */
export const autosaveDelay = 1500;

/** The draft on the server that the next save replaces, as its last save answered it. */
export type DraftLink = { id: string; etag: string; scheduledAt: string | null };

/** Where the autosave is: nothing to save yet, a save on its way, the last change kept, a failure, or a conflict. */
export type AutosaveStatus =
  { kind: "idle" } | { kind: "saving" } | { kind: "saved" } | { kind: "failed"; error: unknown } | { kind: "conflict" };

export type AutosaveSnapshot = { status: AutosaveStatus; link: DraftLink | null };

/** The writes of the drafts, and what follows a save that kept something. */
export type DraftWriter = {
  create: (body: DraftRequest) => Promise<DraftVersion>;
  update: (id: string, etag: string, body: DraftRequest) => Promise<DraftVersion>;
  /** Whether `error` says that another save changed the draft since it was read (a 412). */
  isConflict: (error: unknown) => boolean;
  /** Called after every save that kept the draft (the drafts page's lists are stale then). */
  onSaved?: () => void;
};

/** The error of a save that the autosave's conflict holds back. */
export class AutosaveConflictError extends Error {
  override name = "AutosaveConflictError";
}

/** Saves the editor's article as a draft while the author writes. */
export class ArticleAutosaver {
  private snapshot: AutosaveSnapshot;
  private readonly listeners = new Set<() => void>();
  /** The change not saved yet, or null when the server has the last one. */
  private pending: DraftRequest | null = null;
  private timer: ReturnType<typeof setTimeout> | undefined;
  /** The save on its way; the next one waits for it. */
  private inFlight: Promise<void> | null = null;

  private readonly writer: DraftWriter;
  private readonly delay: number;

  /** Saves through `writer`, replacing the draft `link` (null makes a new one), `delay` ms after the last change. */
  constructor(writer: DraftWriter, link: DraftLink | null, delay = autosaveDelay) {
    this.writer = writer;
    this.delay = delay;
    this.snapshot = { status: { kind: "idle" }, link };
  }

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  getSnapshot = (): AutosaveSnapshot => this.snapshot;

  private set(next: Partial<AutosaveSnapshot>): void {
    this.snapshot = { ...this.snapshot, ...next };
    for (const listener of this.listeners) listener();
  }

  /** Takes a change of the article: it is saved once the author pauses, unless a conflict holds the autosave. */
  change(body: DraftRequest): void {
    this.pending = body;
    clearTimeout(this.timer);
    if (this.snapshot.status.kind === "conflict") return;
    // "Saved" no longer holds for the article as it is now.
    if (this.snapshot.status.kind === "saved") this.set({ status: { kind: "idle" } });
    this.timer = setTimeout(() => {
      this.timer = undefined;
      void this.run().catch(() => {
        // The status says why; the next change tries again.
      });
    }, this.delay);
  }

  /** Saves one change after another until none is left; rejects with the failure of a save. */
  private async run(): Promise<void> {
    while (this.inFlight !== null) await this.inFlight.catch(() => undefined);
    const body = this.pending;
    if (body === null) return;
    if (this.snapshot.status.kind === "conflict") throw new AutosaveConflictError();
    this.pending = null;
    this.set({ status: { kind: "saving" } });
    const save = this.save(body);
    this.inFlight = save;
    try {
      await save;
    } finally {
      this.inFlight = null;
    }
    // A change that came while the save was on its way is the next save, without waiting again.
    if (this.hasPending() && this.timer === undefined) await this.run();
  }

  /** Whether a change is not saved yet (a change may come while a save is on its way). */
  private hasPending(): boolean {
    return this.pending !== null;
  }

  private async save(body: DraftRequest): Promise<void> {
    const { link } = this.snapshot;
    try {
      const version =
        link === null ? await this.writer.create(body) : await this.writer.update(link.id, link.etag, body);
      this.set({
        status: { kind: this.pending === null ? "saved" : "saving" },
        link: { id: version.draft.id, etag: version.etag, scheduledAt: version.draft.scheduled_at ?? null },
      });
      this.writer.onSaved?.();
    } catch (error) {
      // The change is not kept: it stays the one to save, unless a newer one came meanwhile.
      this.pending ??= body;
      if (this.writer.isConflict(error)) {
        clearTimeout(this.timer);
        this.timer = undefined;
        this.set({ status: { kind: "conflict" } });
        throw new AutosaveConflictError();
      }
      this.set({ status: { kind: "failed", error } });
      throw error;
    }
  }

  /**
   * Saves `body` now, after the save on its way, and resolves to the draft that holds it: for publishing and
   * scheduling, which act on the saved draft. Rejects with the failure of the save (`AutosaveConflictError` while a
   * conflict holds the autosave).
   */
  async flush(body: DraftRequest): Promise<DraftLink> {
    clearTimeout(this.timer);
    this.timer = undefined;
    this.pending = body;
    if (this.snapshot.status.kind === "conflict") throw new AutosaveConflictError();
    await this.run();
    const { link } = this.snapshot;
    if (link === null) throw new Error("the draft was not saved");
    return link;
  }

  /** After a conflict: saves `body` as a new draft, leaving the other device's draft as it is, and autosaves on. */
  async saveAsNew(body: DraftRequest): Promise<DraftLink> {
    this.set({ status: { kind: "idle" }, link: null });
    return this.flush(body);
  }

  /** Stops waiting: a change not saved yet is saved now (the editor is closing), unless a conflict holds it. */
  dispose(): void {
    clearTimeout(this.timer);
    this.timer = undefined;
    if (this.pending !== null && this.snapshot.status.kind !== "conflict") {
      void this.run().catch(() => undefined);
    }
  }
}
