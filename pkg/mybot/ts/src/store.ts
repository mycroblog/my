// SPDX-License-Identifier: MIT

/**
 * Keeps what a bot must remember across restarts (ADR-0094 §6): the notifications it processed, the position of the
 * catch-up, the last runs of the scheduled jobs, and the rotated refresh token. MemoryStore and FileStore are the
 * implementations of the library; implement Store to keep the state elsewhere, such as in a database. One Store
 * serves one bot process: it needs no locking between processes. Times are milliseconds since the Unix epoch.
 */
export type Store = {
  /** Whether the notification id was recorded by markProcessed (and not forgotten since). */
  isProcessed(id: string): Promise<boolean>;
  /** Records that the notification id was processed at at. */
  markProcessed(id: string, at: number): Promise<void>;
  /** Drops the records of the notifications processed before before. */
  forgetProcessed(before: number): Promise<void>;
  /**
   * The ID of the newest notification up to which every notification was processed, null when there is none yet. The
   * catch-up reads the notifications back to it.
   */
  position(): Promise<string | null>;
  /** Sets the position. */
  setPosition(id: string): Promise<void>;
  /** When the scheduled job name last ran, null when it never did. */
  lastRun(name: string): Promise<number | null>;
  /** Records when the scheduled job name ran. */
  setLastRun(name: string, at: number): Promise<void>;
  /** The refresh token saved by setRefreshToken, null when there is none. */
  refreshToken(): Promise<string | null>;
  /** Saves the refresh token that a renewal returned. The previous one no longer works. */
  setRefreshToken(token: string): Promise<void>;
};

/**
 * The state of a MemoryStore and the JSON document of a FileStore: the same document as the FileStore of the Go
 * library, with the times as RFC 3339 strings, so that a bot can move between the two.
 */
export type StoreState = {
  processed: Map<string, number>;
  position: string | null;
  lastRuns: Map<string, number>;
  refreshToken: string | null;
};

/** An empty state. */
export function emptyState(): StoreState {
  return { processed: new Map(), position: null, lastRuns: new Map(), refreshToken: null };
}

/** The state as the JSON document of a FileStore. */
export function encodeState(s: StoreState): string {
  const doc: Record<string, unknown> = {
    processed: Object.fromEntries([...s.processed].map(([id, at]) => [id, new Date(at).toISOString()])),
  };
  if (s.position !== null && s.position !== "") doc["position"] = s.position;
  if (s.lastRuns.size > 0)
    doc["last_runs"] = Object.fromEntries([...s.lastRuns].map(([name, at]) => [name, new Date(at).toISOString()]));
  if (s.refreshToken !== null && s.refreshToken !== "") doc["refresh_token"] = s.refreshToken;
  return JSON.stringify(doc, null, 2) + "\n";
}

/** Reads a map of RFC 3339 times. */
function decodeTimes(value: unknown, field: string): Map<string, number> {
  const out = new Map<string, number>();
  if (value === undefined || value === null) return out;
  if (typeof value !== "object") throw new Error(`mybot: the field ${field} of the state is no object`);
  for (const [k, v] of Object.entries(value)) {
    // The Go library writes nanoseconds, which not every runtime's Date.parse reads: milliseconds are enough.
    const at = typeof v === "string" ? Date.parse(v.replace(/(\.\d{3})\d+/, "$1")) : Number.NaN;
    if (Number.isNaN(at))
      throw new Error(`mybot: the field ${field} of the state has a time that is no RFC 3339 string`);
    out.set(k, at);
  }
  return out;
}

/** Reads the JSON document of a FileStore. */
export function decodeState(text: string): StoreState {
  const doc: unknown = JSON.parse(text);
  if (typeof doc !== "object" || doc === null || Array.isArray(doc))
    throw new Error("mybot: the state is no JSON object");
  const d = doc as Record<string, unknown>;
  const str = (v: unknown, field: string): string | null => {
    if (v === undefined || v === null || v === "") return null;
    if (typeof v !== "string") throw new Error(`mybot: the field ${field} of the state is no string`);
    return v;
  };
  return {
    processed: decodeTimes(d["processed"], "processed"),
    position: str(d["position"], "position"),
    lastRuns: decodeTimes(d["last_runs"], "last_runs"),
    refreshToken: str(d["refresh_token"], "refresh_token"),
  };
}

/** A Store in memory that calls save after every change; the base of MemoryStore and FileStore. */
export abstract class StateStore implements Store {
  /** Resolves to the state, once it has been read. */
  protected abstract load(): Promise<StoreState>;
  /** Persists the state after a change. */
  protected abstract save(state: StoreState): Promise<void>;

  /** The changes in order: each starts from the state that the one before left. */
  #changes: Promise<void> = Promise.resolve();

  /**
   * Applies f to a copy of the state and saves it, then keeps the copy. When the save fails, the state stays as it
   * was, so that it never holds what the file does not.
   */
  #change(f: (s: StoreState) => void): Promise<void> {
    const change = this.#changes.then(async () => {
      const s = await this.load();
      const next: StoreState = { ...s, processed: new Map(s.processed), lastRuns: new Map(s.lastRuns) };
      f(next);
      await this.save(next);
      Object.assign(s, next);
    });
    // A failed change fails its caller, not the next change.
    this.#changes = change.catch(() => undefined);
    return change;
  }

  async isProcessed(id: string): Promise<boolean> {
    return (await this.load()).processed.has(id);
  }

  markProcessed(id: string, at: number): Promise<void> {
    return this.#change((s) => s.processed.set(id, at));
  }

  async forgetProcessed(before: number): Promise<void> {
    const s = await this.load();
    if (![...s.processed.values()].some((at) => at < before)) return;
    await this.#change((st) => {
      for (const [id, at] of st.processed) if (at < before) st.processed.delete(id);
    });
  }

  async position(): Promise<string | null> {
    return (await this.load()).position;
  }

  setPosition(id: string): Promise<void> {
    return this.#change((s) => {
      s.position = id;
    });
  }

  async lastRun(name: string): Promise<number | null> {
    return (await this.load()).lastRuns.get(name) ?? null;
  }

  setLastRun(name: string, at: number): Promise<void> {
    return this.#change((s) => s.lastRuns.set(name, at));
  }

  async refreshToken(): Promise<string | null> {
    return (await this.load()).refreshToken;
  }

  setRefreshToken(token: string): Promise<void> {
    return this.#change((s) => {
      s.refreshToken = token;
    });
  }
}

/**
 * A Store in memory, for tests. Everything is lost when the process ends, the rotated refresh token too: the next
 * start uses the refresh token of the options again, which the server has seen used and so revokes. A bot that runs
 * for real, all the time or started again and again, uses a FileStore or a Store of its own.
 */
export class MemoryStore extends StateStore {
  readonly #state = emptyState();

  protected load(): Promise<StoreState> {
    return Promise.resolve(this.#state);
  }

  protected save(): Promise<void> {
    return Promise.resolve();
  }
}
