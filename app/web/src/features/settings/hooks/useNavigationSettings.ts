// SPDX-License-Identifier: AGPL-3.0-only
// The navigation's items as the user arranges them (web-client/09-app-shell.md §9): their order and the ones they hid, kept on
// this browser (lib/storage's navigationKey) like the theme, so that it applies at once and outlives the session.
// The stored value is checked against the items that exist: an unknown ID is dropped, a missing item takes its
// place in the default order, and Settings is never hidden, so that the user can always come back to undo a change.
import { useAnnouncer } from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { navigationKey, type StoredNavigation, useStorage } from "../../../lib/storage";

/** The items the user arranges, in their default order. The staff's areas (ADR-0058) follow them and are not arranged. */
const navigationEntryIds = [
  "home",
  "local",
  "public",
  "notifications",
  "search",
  "profile",
  "bookmarks",
  "settings",
] as const;

export type NavigationEntryId = (typeof navigationEntryIds)[number];

/** The item that is always shown: the way back to these settings. */
export const alwaysShownEntry: NavigationEntryId = "settings";

/**
 * How many of the shown items the phones' tab bar holds, before its Menu button (ADR-0039: six items fit a 320 px
 * screen in every locale); the others are in the menu's sheet.
 */
export const tabBarLimit = 5;

/** The label of each item, the same in the navigation and in its settings. */
export const navigationEntryLabels: Record<NavigationEntryId, ClientMessageKey> = {
  home: "client:shell.nav.home",
  local: "client:shell.nav.local",
  public: "client:shell.nav.public",
  notifications: "client:shell.nav.notifications",
  search: "client:shell.nav.search",
  profile: "client:shell.nav.profile",
  bookmarks: "client:shell.nav.bookmarks",
  settings: "client:shell.nav.settings",
};

/** An item in the user's order, and whether they show it. */
export type NavigationEntry = { id: NavigationEntryId; shown: boolean };

function isEntryId(id: string): id is NavigationEntryId {
  return (navigationEntryIds as readonly string[]).includes(id);
}

/** The items in the stored order, the default one for what is not stored. */
function navigationEntriesOf(stored: StoredNavigation | null): NavigationEntry[] {
  const order = [...new Set([...(stored?.order ?? []).filter(isEntryId), ...navigationEntryIds])];
  const hidden = new Set(stored?.hidden ?? []);
  return order.map((id) => ({ id, shown: id === alwaysShownEntry || !hidden.has(id) }));
}

/** Keeps `entries`, or forgets them when they are the default arrangement. */
function save(entries: readonly NavigationEntry[]): void {
  const isDefault = entries.every((entry, index) => entry.id === navigationEntryIds[index] && entry.shown);
  if (isDefault) {
    navigationKey.remove();
    return;
  }
  navigationKey.write({
    order: entries.map((entry) => entry.id),
    hidden: entries.filter((entry) => !entry.shown).map((entry) => entry.id),
  });
}

/** The navigation's items in the user's order, rendering again when they change here or in another tab. */
export function useNavigationEntries(): NavigationEntry[] {
  return navigationEntriesOf(useStorage(navigationKey));
}

export type NavigationSettings = {
  entries: NavigationEntry[];
  /** Whether the items are in the default order, all shown. */
  isDefault: boolean;
  /**
   * Moves an item one place up (-1) or down (1), and announces the move ("Search moved to position 3 of 8").
   */
  move: (id: NavigationEntryId, step: -1 | 1) => void;
  setShown: (id: NavigationEntryId, shown: boolean) => void;
  /** Puts back the default order with every item shown. */
  reset: () => void;
};

/** The arrangement of the navigation's items, for its settings. */
export function useNavigationSettings(): NavigationSettings {
  const t = useT();
  const announce = useAnnouncer();
  const stored = useStorage(navigationKey);
  const entries = navigationEntriesOf(stored);
  return {
    entries,
    isDefault: stored === null,
    move: (id, step) => {
      const from = entries.findIndex((entry) => entry.id === id);
      const to = from + step;
      const moved = entries[from];
      if (moved === undefined || to < 0 || to >= entries.length) return;
      const next = entries.toSpliced(from, 1).toSpliced(to, 0, moved);
      save(next);
      announce(
        t("client:settings.navigation.moved", {
          name: t(navigationEntryLabels[id]),
          position: to + 1,
          count: next.length,
        }),
      );
    },
    setShown: (id, shown) => {
      if (id === alwaysShownEntry) return;
      save(entries.map((entry) => (entry.id === id ? { ...entry, shown } : entry)));
    },
    reset: () => {
      navigationKey.remove();
      announce(t("client:settings.navigation.reset_done"));
    },
  };
}
