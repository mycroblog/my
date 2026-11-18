// SPDX-License-Identifier: AGPL-3.0-only

// The entries of the settings (ADR-0110 §5), declared in one place: each is a row of the menu, in a group, and a screen
// at /settings/<path>. The menu (menu.tsx), the screen at an entry's address (screen.tsx) and the route's loader read
// this list alone, so an entry is added here and nowhere else: its declaration in `settingsEntries`, at its place in
// the menu's order, with its screen. A group with no entry to show is not drawn.

import { usePermissions, type AppData, type Permission } from "@my/app-data";
import type { IconName } from "@my/design-system";
import type { ComponentType, ReactNode } from "react";
import type { MessageId } from "../messages.gen";
import { DisplaySettings, loadDisplaySettings } from "./display";
import { loadNotificationSettings, NotificationSettings } from "./notifications";
import { loadProfileSettings, ProfileSettings } from "./profile";

/** The groups of the menu, in its order, each under its heading. */
export const settingsGroups = [
  { id: "general", heading: "settings.group.general" },
  { id: "people", heading: "settings.group.people" },
  { id: "access", heading: "settings.group.access" },
  { id: "account", heading: "settings.group.account" },
] as const satisfies readonly { id: string; heading: MessageId }[];
export type SettingsGroupId = (typeof settingsGroups)[number]["id"];

/** What an entry's screen is given by the screen that places it. */
export type SettingsEntryProps = {
  /** The way back to the menu (a `BackButton`), for the head of the entry's `SettingsScreen`. */
  back: ReactNode;
  /** The languages the app can be shown in, as BCP 47 tags. */
  languages: readonly string[];
};

/**
 * What the menu reads of the reader, for the conditions and the counts of the entries: one hook reads it
 * (`useSettingsFacts`), where a new fact is added.
 */
export type SettingsFacts = {
  /** The permissions of the reader's account; none until the server said them. */
  readonly permissions: readonly Permission[];
};

/** An entry of the settings. */
export type SettingsEntry = {
  /** The last part of its address, /settings/<path>. */
  readonly path: string;
  readonly group: SettingsGroupId;
  readonly icon: IconName;
  /** Its name, in the menu and as the title of its screen (settings.entry.<path>). */
  readonly name: MessageId;
  /** What its screen holds, the line under its name in the menu at /settings (settings.entry.<path>.hint). */
  readonly hint: MessageId;
  /** Whether the menu lists it and its screen opens for this reader; for every reader without it. */
  readonly shown?: (facts: SettingsFacts) => boolean;
  /** How many things on its screen wait for the reader, the badge at the row's end; none without it, or at zero. */
  readonly count?: (facts: SettingsFacts) => number | undefined;
  /** Its screen: a `SettingsScreen` of the design system, headed by the entry's glyph and name. */
  readonly Screen: ComponentType<SettingsEntryProps>;
  /** Reads what its screen shows, for the route's loader to wait for. */
  readonly load?: (data: AppData) => Promise<void>;
};

/** Every entry, in the menu's order within its group. */
export const settingsEntries: readonly SettingsEntry[] = [
  {
    path: "profile",
    group: "general",
    icon: "profile",
    name: "settings.entry.profile",
    hint: "settings.entry.profile.hint",
    Screen: ProfileSettings,
    load: loadProfileSettings,
  },
  {
    path: "display",
    group: "general",
    icon: "display",
    name: "settings.entry.display",
    hint: "settings.entry.display.hint",
    Screen: DisplaySettings,
    load: loadDisplaySettings,
  },
  {
    path: "notifications",
    group: "general",
    icon: "notifications",
    name: "settings.entry.notifications",
    hint: "settings.entry.notifications.hint",
    Screen: NotificationSettings,
    load: loadNotificationSettings,
  },
];

/** What the conditions and the counts of the entries read of the reader. */
function useSettingsFacts(): SettingsFacts {
  return { permissions: usePermissions() };
}

/** An entry as the menu shows it to this reader. */
export type ShownEntry = SettingsEntry & { readonly waiting: number | undefined };

/** The entries the reader is shown, in the menu's order, each with how many things wait on its screen. */
export function useShownEntries(): readonly ShownEntry[] {
  const facts = useSettingsFacts();
  return settingsEntries
    .filter((entry) => entry.shown?.(facts) ?? true)
    .map((entry) => ({ ...entry, waiting: entry.count?.(facts) }));
}

/** Reads what the screen of the entry at `path` shows; nothing for an address that names no entry. */
export async function loadSettingsEntry(data: AppData, path: string): Promise<void> {
  await settingsEntries.find((entry) => entry.path === path)?.load?.(data);
}
