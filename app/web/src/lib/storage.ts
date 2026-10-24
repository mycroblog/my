// SPDX-License-Identifier: AGPL-3.0-only
// Everything the app keeps in the browser's storage (localStorage and sessionStorage), declared in one place
// (web-client/README.md S11). Each key says where it is kept, the type of its value and how a stored value is checked,
// the version of that format, the value that stands for "nothing kept", and whether it is the user's own data,
// which is removed when the session ends. Nothing else reads or writes the storage: Oxlint rejects
// `localStorage` and `sessionStorage` outside this module and the tests (restrict/storage).
//
// A value is stored as the JSON `{"v":<version>,"value":<value>}`. A value of another version, one that is not
// JSON or one that fails its check reads as the default, as does a key that nothing was written to; writing the
// default removes the key. Where the storage cannot be used (a browser that blocks it, a sandboxed frame, a full
// quota), a write is kept in memory for the rest of the page, so the app behaves the same until it is reloaded.
//
// Reading is an external store (react.dev: useSyncExternalStore): a key returns the same value until its stored
// text changes, a write here notifies the key's subscribers, and a write in another tab of the origin reaches
// them through the storage event (https://developer.mozilla.org/docs/Web/API/Window/storage_event).
import { useSyncExternalStore } from "react";
import { z } from "zod";

/** localStorage outlives the tab and is shared by the origin's tabs; sessionStorage belongs to one tab. */
type Area = "local" | "session";

type Declaration<T> = {
  area: Area;
  /** The version of the stored format; a stored value of another version reads as the default. */
  version: number;
  /** The value when nothing (valid) is kept. Writing it removes the key. */
  defaultValue: T;
  /** Checks a stored value and returns it typed, or undefined when it is not one. */
  parse: (value: unknown) => T | undefined;
  /** Whether the value is the user's own data (what they typed), removed when the session ends (lib/auth/userData). */
  userData: boolean;
};

/** The stored form of every value. */
const envelope = z.object({ v: z.number(), value: z.unknown() });

/** The values written where the storage could not be used, by key name; null is a removal. */
const unsaved = new Map<string, string | null>();
/** The subscribers of each key name. */
const listeners = new Map<string, Set<() => void>>();

function storageOf(area: Area): Storage | null {
  try {
    return area === "local" ? globalThis.localStorage : globalThis.sessionStorage;
  } catch {
    // The browser refuses the storage (blocked site data, a sandboxed frame).
    return null;
  }
}

function notify(name: string): void {
  for (const listener of listeners.get(name) ?? []) listener();
}

/** Follows a write made in another tab; a null key is a clear() of the whole storage. */
function onStorage(event: StorageEvent): void {
  if (event.key === null) for (const name of listeners.keys()) notify(name);
  else notify(event.key);
}

/** One declared key. Instances are made by the declarations of this module only. */
class StorageKey<T> {
  readonly name: string;
  readonly #declaration: Declaration<T>;
  /** The last stored text read and its value, so that a snapshot keeps its identity while the text is the same. */
  #cache: { raw: string | null; value: T } | undefined;

  constructor(name: string, declaration: Declaration<T>) {
    this.name = name;
    this.#declaration = declaration;
  }

  /** The version of the stored format. */
  get version(): number {
    return this.#declaration.version;
  }

  /** The kept value, or the default when nothing valid is kept or the storage cannot be read. */
  read(): T {
    const raw = this.#readRaw();
    if (this.#cache === undefined || this.#cache.raw !== raw) this.#cache = { raw, value: this.#decode(raw) };
    return this.#cache.value;
  }

  /** Keeps `value`, or removes the key when it is the default, and notifies the subscribers. */
  write(value: T): void {
    if (Object.is(value, this.#declaration.defaultValue)) {
      this.remove();
      return;
    }
    this.#writeRaw(JSON.stringify({ v: this.#declaration.version, value }));
  }

  /** Removes the key, so that it reads as the default, and notifies the subscribers. */
  remove(): void {
    this.#writeRaw(null);
  }

  /** Calls `listener` whenever the value may have changed, here or in another tab (useSyncExternalStore's subscribe). */
  readonly subscribe = (listener: () => void): (() => void) => {
    if (listeners.size === 0) window.addEventListener("storage", onStorage);
    let set = listeners.get(this.name);
    if (set === undefined) {
      set = new Set();
      listeners.set(this.name, set);
    }
    set.add(listener);
    return () => {
      set.delete(listener);
      if (set.size === 0) listeners.delete(this.name);
      if (listeners.size === 0) window.removeEventListener("storage", onStorage);
    };
  };

  /** useSyncExternalStore's getSnapshot. */
  readonly getSnapshot = (): T => this.read();

  #readRaw(): string | null {
    const kept = unsaved.get(this.name);
    if (kept !== undefined) return kept;
    try {
      return storageOf(this.#declaration.area)?.getItem(this.name) ?? null;
    } catch {
      return null;
    }
  }

  #writeRaw(raw: string | null): void {
    const storage = storageOf(this.#declaration.area);
    try {
      if (storage === null) throw new Error("no storage");
      if (raw === null) storage.removeItem(this.name);
      else storage.setItem(this.name, raw);
      unsaved.delete(this.name);
    } catch {
      // The storage is blocked or full: keep the value for this page.
      unsaved.set(this.name, raw);
    }
    notify(this.name);
  }

  #decode(raw: string | null): T {
    const { version, defaultValue, parse } = this.#declaration;
    if (raw === null) return defaultValue;
    try {
      const stored = envelope.safeParse(JSON.parse(raw));
      if (!stored.success || stored.data.v !== version) return defaultValue;
      return parse(stored.data.value) ?? defaultValue;
    } catch {
      return defaultValue;
    }
  }
}

export type { StorageKey };

/** Every declaration, for removing the user's data; a family is matched by its prefix. */
const declared: { prefix: string; exact: boolean; area: Area; userData: boolean }[] = [];

function declareKey<T>(name: string, declaration: Declaration<T>): StorageKey<T> {
  declared.push({ prefix: name, exact: true, area: declaration.area, userData: declaration.userData });
  return new StorageKey(name, declaration);
}

/** A key per ID under `prefix` (one per account, say). The same ID returns the same key. */
function declareKeyFamily<T>(prefix: string, declaration: Declaration<T>): (id: string) => StorageKey<T> {
  declared.push({ prefix, exact: false, area: declaration.area, userData: declaration.userData });
  const keys = new Map<string, StorageKey<T>>();
  return (id) => {
    let key = keys.get(id);
    if (key === undefined) {
      key = new StorageKey(prefix + id, declaration);
      keys.set(id, key);
    }
    return key;
  };
}

/** A check that accepts the values of `schema`. */
function schemaParser<T>(schema: z.ZodType<T>): (value: unknown) => T | undefined {
  return (value) => {
    const result = schema.safeParse(value);
    return result.success ? result.data : undefined;
  };
}

/** Returns the value of `key`, rendering again when it changes, here or in another tab. */
export function useStorage<T>(key: StorageKey<T>): T {
  return useSyncExternalStore(key.subscribe, key.getSnapshot);
}

/** Removes every key that holds the user's own data, in both storages (lib/auth/userData, at the end of a session). */
export function clearUserStorage(): void {
  const isUserData = (area: Area, name: string) =>
    declared.some((d) => d.userData && d.area === area && (d.exact ? name === d.prefix : name.startsWith(d.prefix)));
  const removed = new Set<string>();
  for (const area of ["local", "session"] as const) {
    const storage = storageOf(area);
    if (storage === null) continue;
    try {
      const names = Array.from({ length: storage.length }, (_, i) => storage.key(i));
      for (const name of names) {
        if (name === null || !isUserData(area, name)) continue;
        storage.removeItem(name);
        removed.add(name);
      }
    } catch {
      // The storage cannot be read: nothing was saved there.
    }
  }
  for (const name of unsaved.keys()) {
    if (isUserData("local", name) || isUserData("session", name)) {
      unsaved.delete(name);
      removed.add(name);
    }
  }
  for (const name of removed) notify(name);
}

/** Forgets the values kept in memory where the storage could not be used. For the test setup only. */
export function forgetUnsavedStorage(): void {
  unsaved.clear();
}

// The keys. The names are listed in web-client/10-api-client-and-auth.md §10–§12; public/theme.js reads `my:display:theme` itself.

/** The color theme chosen on this browser (lib/theme). Kept across sessions: it is how this browser shows the app. */
export const themeKey = declareKey<"system" | "light" | "dark">("my:display:theme", {
  area: "local",
  version: 1,
  defaultValue: "system",
  parse: schemaParser(z.enum(["light", "dark"])),
  userData: false,
});

/** The locale the app was last displayed in on this browser (lib/locale); null starts in the instance's locale. */
export const localeKey = declareKey<string | null>("my:display:locale", {
  area: "local",
  version: 1,
  defaultValue: null,
  parse: schemaParser(z.string().min(1)),
  userData: false,
});

/** Whether animated images play by themselves on this browser (lib/autoplay, ADR-0054 §3). */
export const autoplayKey = declareKey<boolean>("my:display:autoplay", {
  area: "local",
  version: 1,
  defaultValue: true,
  parse: schemaParser(z.boolean()),
  userData: false,
});

/** Which public timelines the instance served when GET /instance last answered (features/timeline). */
/** Who may read a public timeline (the instance settings `local_timeline_access` and `federated_timeline_access`). */
const timelineAccess = z.enum(["off", "logged_in", "everyone"]);

export type StoredTimelines = { local: z.infer<typeof timelineAccess>; federated: z.infer<typeof timelineAccess> };

/** The instance's public timelines, kept for a start without the network; an instance setting, not the user's data. */
export const timelinesKey = declareKey<StoredTimelines | null>("my:instance:timelines", {
  area: "local",
  version: 2,
  defaultValue: null,
  parse: schemaParser(z.object({ local: timelineAccess, federated: timelineAccess })),
  userData: false,
});

/** The saved compose state of one account (features/compose): its draft and outbox, checked further by the draft store. */
export type StoredCompose = { draft: Record<string, unknown>; outbox: Record<string, unknown>[] };

/** The compose state of each account, for this tab only; the user's own data. */
export const composeKey = declareKeyFamily<StoredCompose | null>("my:user:compose:", {
  area: "session",
  version: 1,
  defaultValue: null,
  parse: schemaParser(z.object({ draft: z.record(z.string(), z.unknown()), outbox: z.array(z.record(z.string(), z.unknown())) })),
  userData: true,
});

/**
 * The newest posts of the home timeline as last loaded (features/timeline), without the followers-only and direct
 * ones: when they were loaded, in epoch milliseconds, the posts (the client API's Status, checked further by the
 * feature) and the cursor after them.
 */
export type StoredHomeTimeline = { loadedAt: number; items: Record<string, unknown>[]; nextCursor: string | null };

/**
 * The home timeline kept for reading offline (FR-604, web-client/11-pwa.md §11): the user's data, removed when the
 * session ends and replaced when another user signs in (lib/auth/userData). Its version changes with the
 * stored shape, the Status of the client API included.
 */
export const homeTimelineKey = declareKey<StoredHomeTimeline | null>("my:user:home-timeline", {
  area: "local",
  version: 1,
  defaultValue: null,
  parse: schemaParser(
    z.object({
      loadedAt: z.number(),
      items: z.array(z.record(z.string(), z.unknown())).max(100),
      nextCursor: z.string().nullable(),
    }),
  ),
  userData: true,
});

/** A sign-in between leaving for the authorization screen and its callback (lib/auth/signIn). */
export type StoredSignIn = { state: string; codeVerifier: string; returnTo: string };

/** The sign-in in progress in this tab; deleted when the callback reads it. */
export const signInKey = declareKey<StoredSignIn | null>("my:oauth-pending", {
  area: "session",
  version: 1,
  defaultValue: null,
  parse: schemaParser(z.object({ state: z.string(), codeVerifier: z.string(), returnTo: z.string() })),
  userData: false,
});

/** When this tab last reloaded after a chunk failed to load (lib/pwa), in epoch milliseconds. */
export const preloadReloadKey = declareKey<number | null>("my:preload-error-reloaded-at", {
  area: "session",
  version: 1,
  defaultValue: null,
  parse: schemaParser(z.number()),
  userData: false,
});

/**
 * One emoji used on this browser: a Unicode emoji without its skin tone, or `:shortcode:` for a custom emoji; how often
 * it was used, decayed by age as of `at`; and when it was used last (milliseconds since the epoch).
 */
export type StoredEmojiUse = { emoji: string; score: number; at: number };

/**
 * The emoji used often on this browser, in the order the picker shows them (features/emoji, ADR-0091 §3). Version 1 held
 * the shortcodes of the custom emoji picked recently; it is not converted and reads as the default.
 */
export const frequentEmojisKey = declareKey<readonly StoredEmojiUse[]>("my:emoji:recent", {
  area: "local",
  version: 2,
  defaultValue: [],
  parse: schemaParser(
    z.array(z.object({ emoji: z.string().min(1).max(64), score: z.number().nonnegative(), at: z.number().nonnegative() })).max(64),
  ),
  userData: false,
});

/** The skin tones of the emoji picker: the default yellow and the five Fitzpatrick modifiers (@my/ui `SkinTone`). */
const skinTone = z.enum(["none", "light", "medium-light", "medium", "medium-dark", "dark"]);

/** The skin tone chosen in the emoji picker on this browser (features/emoji, ADR-0091 §3). */
export const skinToneKey = declareKey<z.infer<typeof skinTone>>("my:emoji:skin-tone", {
  area: "local",
  version: 1,
  defaultValue: "none",
  parse: schemaParser(skinTone),
  userData: false,
});

/** How many hashtags `recentTagsKey` keeps (ADR-0092 §1). */
export const maxRecentTags = 16;

/**
 * The hashtags used lately in posts sent from this browser, the latest first, as typed without `#` (features/compose,
 * ADR-0092 §1): the completion after "#" ranks them first. They are what the user posted, so they go when the session ends.
 */
export const recentTagsKey = declareKey<readonly string[]>("my:compose:recent-tags", {
  area: "local",
  version: 1,
  defaultValue: [],
  parse: schemaParser(z.array(z.string().min(1).max(100)).max(maxRecentTags)),
  userData: true,
});

/** The order of the navigation's items and the ones the user hid, by their IDs; checked further by features/settings. */
export type StoredNavigation = { order: readonly string[]; hidden: readonly string[] };

/** The navigation's items as the user arranged them on this browser (features/settings); null is the default arrangement. */
export const navigationKey = declareKey<StoredNavigation | null>("my:display:navigation", {
  area: "local",
  version: 1,
  defaultValue: null,
  parse: schemaParser(z.object({ order: z.array(z.string()).max(64), hidden: z.array(z.string()).max(64) })),
  userData: false,
});
