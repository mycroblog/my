// SPDX-License-Identifier: AGPL-3.0-only
// The draft of a new post and the outbox of posts being sent, kept outside React so that the side panel,
// the sheet and the reply buttons of other features share one draft (read with useSyncExternalStore).
//
// The store is saved to sessionStorage under the account's ID (lib/storage's composeKey): a reload, or a crash of the tab, keeps the
// draft, and a post that was being sent stays in the outbox so it can be sent again. sessionStorage
// belongs to this tab only and ends with it; another account signing in on the same tab reads its own
// key, and the end of the session removes it (it is the user's own data). Files picked but not yet uploaded live only in memory (their object URLs end with the page).
import type { StorageKey, StoredCompose } from "../../../lib/storage";
import type { CreateStatusRequest, QuotePolicy, Visibility } from "../api/compose";
import type { PollDraft } from "./poll";

/** One image of the draft. */
export type AttachmentDraft = {
  /** A client-side ID, the React key. */
  key: string;
  /** The uploaded file's ID; null until the upload answers. */
  mediaId: string | null;
  /** The alternative text the user typed. */
  description: string;
  /** An object URL of the prepared file, for the preview; null after a reload (the server's thumbnail is used). */
  previewUrl: string | null;
  /** Why the upload failed (localized), or null. */
  error: string | null;
};

export type Draft = {
  text: string;
  hasWarning: boolean;
  spoilerText: string;
  sensitive: boolean;
  /** The chosen visibility; null follows the default (the preference, narrowed for a reply). */
  visibility: Visibility | null;
  /** The chosen language; null follows the preference. */
  language: string | null;
  replyToId: string | null;
  /** The post quoted (FR-114), or null for none. */
  quoteId: string | null;
  /** Who may quote the post; null follows the preference. */
  quotePolicy: QuotePolicy | null;
  attachments: readonly AttachmentDraft[];
  /** The poll of the post (FR-112), or null for none. A post with a poll attaches no media. */
  poll: PollDraft | null;
  /**
   * The Idempotency-Key of the last attempt to post this draft. It is kept while the draft is unchanged,
   * so sending it again cannot post it twice, and dropped by any edit.
   */
  idempotencyKey: string | null;
};

/** A post that was handed to the server and has not been confirmed yet: what is sent, and the draft it came from. */
export type OutboxItem = { idempotencyKey: string; body: CreateStatusRequest; draft: Draft };

export type ComposeState = {
  draft: Draft;
  outbox: readonly OutboxItem[];
  /** The localized reason the last attempt to post the draft failed; cleared by any edit. Not saved. */
  error: string | null;
};

const emptyDraft: Draft = {
  text: "",
  hasWarning: false,
  spoilerText: "",
  sensitive: false,
  visibility: null,
  language: null,
  replyToId: null,
  quoteId: null,
  quotePolicy: null,
  attachments: [],
  poll: null,
  idempotencyKey: null,
};

/** Whether the draft has nothing the user would miss. */
export function isEmptyDraft(draft: Draft): boolean {
  return (
    draft.text.trim() === "" &&
    draft.spoilerText.trim() === "" &&
    draft.attachments.length === 0 &&
    draft.poll === null &&
    draft.replyToId === null &&
    draft.quoteId === null
  );
}

/** The part of the state that is saved: uploads in progress and failed ones cannot be resumed. */
function savedState(state: ComposeState) {
  const savedDraft = (draft: Draft): Draft => ({
    ...draft,
    attachments: draft.attachments
      .filter((a) => a.mediaId !== null)
      .map((a) => ({ ...a, previewUrl: null, error: null })),
  });
  return {
    draft: savedDraft(state.draft),
    outbox: state.outbox.map((item) => ({ ...item, draft: savedDraft(item.draft) })),
  };
}

/** Reads a saved state; anything unreadable starts empty. */
function loadState(key: StorageKey<StoredCompose | null>): ComposeState {
  const saved = key.read();
  if (saved === null) return { draft: emptyDraft, outbox: [], error: null };
  // The saved state is what savedState wrote: lib/storage checks its shape, and the fields of the draft fill in from the empty one.
  return {
    draft: { ...emptyDraft, ...(saved.draft as Partial<Draft>) },
    outbox: saved.outbox as unknown as OutboxItem[],
    error: null,
  };
}

/**
 * The compose state of one account. Every change makes a new immutable snapshot (useSyncExternalStore
 * compares snapshots by identity) and saves it.
 */
export class DraftStore {
  #state: ComposeState;
  readonly #listeners = new Set<() => void>();
  readonly #key: StorageKey<StoredCompose | null>;

  /** `key` is lib/storage's composeKey of the account. */
  constructor(key: StorageKey<StoredCompose | null>) {
    this.#key = key;
    this.#state = loadState(key);
  }

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => {
      this.#listeners.delete(listener);
    };
  };

  readonly getSnapshot = (): ComposeState => this.#state;

  #set(next: ComposeState) {
    this.#state = next;
    this.#key.write(savedState(next));
    for (const listener of this.#listeners) listener();
  }

  /** Edits the draft. An edit drops the idempotency key and the error of the last attempt. */
  edit(change: Partial<Omit<Draft, "idempotencyKey">>) {
    this.#set({ ...this.#state, draft: { ...this.#state.draft, ...change, idempotencyKey: null }, error: null });
  }

  /** Changes one attachment, found by its key; nothing happens when it was removed meanwhile. */
  editAttachment(key: string, change: Partial<Omit<AttachmentDraft, "key">>) {
    const { attachments } = this.#state.draft;
    if (!attachments.some((a) => a.key === key)) return;
    this.edit({ attachments: attachments.map((a) => (a.key === key ? { ...a, ...change } : a)) });
  }

  /**
   * Moves the draft into the outbox under `idempotencyKey`, with the request `body` made from it, and starts an empty draft that keeps the
   * chosen visibility, language and quote policy.
   */
  send(idempotencyKey: string, body: CreateStatusRequest): OutboxItem {
    const item: OutboxItem = { idempotencyKey, body, draft: { ...this.#state.draft, idempotencyKey } };
    const { visibility, language, quotePolicy } = this.#state.draft;
    this.#set({
      draft: { ...emptyDraft, visibility: item.draft.replyToId === null ? visibility : null, language, quotePolicy },
      outbox: [...this.#state.outbox.filter((o) => o.idempotencyKey !== idempotencyKey), item],
      error: null,
    });
    return item;
  }

  /** Drops a post from the outbox: it was posted, or the user discarded it. */
  remove(idempotencyKey: string) {
    this.#set({ ...this.#state, outbox: this.#state.outbox.filter((o) => o.idempotencyKey !== idempotencyKey) });
  }

  /**
   * Puts a post of the outbox back into the form with its idempotency key, when the form is empty, and
   * returns whether it did. A non-empty draft is never overwritten.
   */
  restore(idempotencyKey: string, error: string | null): boolean {
    const item = this.#state.outbox.find((o) => o.idempotencyKey === idempotencyKey);
    if (item === undefined || !isEmptyDraft(this.#state.draft)) return false;
    this.#set({ draft: item.draft, outbox: this.#state.outbox.filter((o) => o !== item), error });
    return true;
  }
}
