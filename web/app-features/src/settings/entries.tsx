// SPDX-License-Identifier: AGPL-3.0-only

// The entries of the settings (ADR-0110 §5), declared in one place: each is a row of the menu, in a group, and a screen
// at /settings/<path>. The menu (menu.tsx), the screen at an entry's address (screen.tsx) and the route's loader read
// this list alone, so an entry is added here and nowhere else: its declaration in `settingsEntries`, at its place in
// the menu's order, with its screen. A group with no entry to show is not drawn.
//
// What an entry's conditions read of the reader is one hook's (`useSettingsFacts`). A fact no screen of the app has
// read before the menu paints (whether the reader may create invites) is read by the settings' loader
// (`loadSettingsFacts`), so that the menu comes with every entry it has instead of gaining one a moment later; on the
// app's first load, whose loader does not wait, the menu paints its entries once that fact is known
// (`useSettingsFactsKnown`), or once it has waited long enough for it (menu.tsx).

import {
  loadInviteRight,
  useInstance,
  useInviteRightKnown,
  useMayInvite,
  usePermissions,
  type AppData,
  type Permission,
} from "@my/app-data";
import type { IconName } from "@my/design-system";
import { inNativeShell } from "@my/platform";
import type { ComponentType, ReactNode } from "react";
import type { MessageId } from "../messages.gen";
import { AccountSettings, loadAccountSettings, useAppealableActions } from "./account";
import { AppsSettings, loadAppsSettings } from "./apps/apps";
import { CustomEmojiSettings, loadCustomEmojiSettings } from "./custom-emojis";
import { DisplaySettings, loadDisplaySettings } from "./display";
import { EventSettings, loadEventSettings } from "./events";
import { FilterSettings, loadFilterSettings } from "./filters";
import { InvitesSettings, loadInvitesSettings } from "./invites/invites";
import { loadNotificationSettings, NotificationSettings } from "./notifications";
import { loadProfileSettings, ProfileSettings } from "./profile";
import { loadRelationshipSettings, RelationshipSettings } from "./relationships";
import { loadSecuritySettings, SecuritySettings } from "./security/security";
import { loadSupporterSettings, SupporterSettings } from "./supporter";
import { loadVoiceSettings, VoiceSettings } from "./voice";

/** The groups of the menu, in its order, each under its heading. */
export const settingsGroups = [
  { id: "general", heading: "settings.group.general" },
  { id: "people", heading: "settings.group.people" },
  { id: "access", heading: "settings.group.access" },
  { id: "account", heading: "settings.group.account" },
] as const satisfies readonly { id: string; heading: MessageId }[];
export type SettingsGroupId = (typeof settingsGroups)[number]["id"];

/**
 * What a screen of the settings reads of its address and of the app around it, for what leaves the app and comes
 * back: going to a login provider to link an account or to confirm an operation (confirmation.tsx).
 */
export type SettingsAddress = {
  /**
   * The fragment of the screen's address, without its `#`: what a login provider's return leaves there
   * (`linked=…`, `external_confirmation=…`, `external_error=…`), empty otherwise.
   */
  fragment: string;
  /** Takes the fragment off the address, once the screen has read it, so that a reload does not read it again. */
  onFragmentRead: () => void;
  /**
   * The query of the screen's address, with its `?`: what a payment provider's return leaves there
   * (`?checkout=success`, `?tip=…&checkout=…`), empty otherwise; none where the screen is given no address.
   */
  search?: string | undefined;
  /** Takes the query off the address, once the screen has read it, as `onFragmentRead` takes the fragment. */
  onSearchRead?: (() => void) | undefined;
  /** Leaves the app for `href`, a page that loads anew (a login provider's, a payment provider's). */
  leave: (href: string) => void;
};

/** What an entry's screen is given by the screen that places it. */
export type SettingsEntryProps = SettingsAddress & {
  /** The way back to the menu (a `BackButton`), for the head of the entry's `SettingsScreen`. */
  back: ReactNode;
  /** The languages the app can be shown in, as BCP 47 tags. */
  languages: readonly string[];
};

/** What a screen under an entry (changing the password) is given by the route that places it. */
export type SettingsSubProps = SettingsAddress & {
  /** Goes back to the screen this one is under, its entry's for most: its head's back, and the end of what it does. */
  onBack: () => void;
};

/** What a screen under an entry that may end the browser's session is given beside, by the route that places it. */
export type SettingsSessionProps = {
  /**
   * The server ended this browser's session with what the reader just did on the screen (revoking the server's own
   * web client among the authorized apps): removes what the session left in this browser and leaves the app, as
   * after signing out.
   */
  onSessionEnded: () => void;
};

/**
 * What the menu reads of the reader, for the conditions and the counts of the entries: one hook reads it
 * (`useSettingsFacts`), where a new fact is added.
 */
export type SettingsFacts = {
  /** The permissions of the reader's account; none until the server said them. */
  readonly permissions: readonly Permission[];
  /** How many moderators' actions in effect on the reader's account they can still appeal; none until they are read. */
  readonly appealable: number;
  /** Whether the server is one person's (ADR-0152), which has no invites; false until the server said so. */
  readonly singleUser: boolean;
  /**
   * Whether the reader may create invites now (ADR-0106 §14): by a permission of their account or because the server
   * lets everyone, which only the list of their invites says; false until the server said so.
   */
  readonly mayInvite: boolean;
  /** Whether the server has voice chat (ADR-0100 §2); false until the server said so. */
  readonly voice: boolean;
  /**
   * Whether the server takes the readers' support here: its supporter program or its tips are on, and the app is no
   * native shell, which shows no purchase (ADR-0072 §14, ADR-0162 §3); false until the server said so.
   */
  readonly supporters: boolean;
};

/** An entry of the settings. */
export type SettingsEntry = {
  /** The last part of its address, /settings/<path>. */
  readonly path: string;
  readonly group: SettingsGroupId;
  readonly icon: IconName;
  /** Its name, in the menu and as the title of its screen (settings.entry.<path>). */
  readonly name: MessageId;
  /** Whether the menu lists it and its screen opens for this reader; for every reader without it. */
  readonly shown?: (facts: SettingsFacts) => boolean;
  /**
   * Whether the menu lists it, for an entry whose screen still opens at its address for a reader the menu does not
   * offer it to (the invites of a reader who may not create any now); wherever it is shown without it.
   */
  readonly listed?: (facts: SettingsFacts) => boolean;
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
    Screen: ProfileSettings,
    load: loadProfileSettings,
  },
  {
    path: "display",
    group: "general",
    icon: "display",
    name: "settings.entry.display",
    Screen: DisplaySettings,
    load: loadDisplaySettings,
  },
  {
    path: "notifications",
    group: "general",
    icon: "notifications",
    name: "settings.entry.notifications",
    Screen: NotificationSettings,
    load: loadNotificationSettings,
  },
  {
    // The rooms the reader hosted (ADR-0100 §13), while the server has voice chat.
    path: "voice",
    group: "general",
    icon: "room",
    name: "settings.entry.voice",
    shown: (facts) => facts.voice,
    Screen: VoiceSettings,
    load: loadVoiceSettings,
  },
  {
    // The reminders of the events the reader goes to (ADR-0161 §12).
    path: "events",
    group: "general",
    icon: "events",
    name: "settings.entry.events",
    Screen: EventSettings,
    load: loadEventSettings,
  },
  {
    // The custom emoji the reader asks for and made (FR-915, ADR-0070 §2): for a holder of `create_custom_emojis`,
    // unless they manage the emoji, which they register from the administration with no request and no limit.
    path: "custom-emojis",
    group: "general",
    icon: "emojiCustom",
    name: "settings.entry.custom_emojis",
    shown: ({ permissions }) =>
      permissions.includes("create_custom_emojis") && !permissions.includes("manage_custom_emojis"),
    Screen: CustomEmojiSettings,
    load: loadCustomEmojiSettings,
  },
  {
    // The reader's subscription, the plans and the tips, while the server takes them, never in the native shell.
    path: "supporter",
    group: "general",
    icon: "funding",
    name: "settings.entry.supporter",
    shown: (facts) => facts.supporters,
    Screen: SupporterSettings,
    load: loadSupporterSettings,
  },
  {
    path: "relationships",
    group: "people",
    icon: "members",
    name: "settings.entry.relationships",
    Screen: RelationshipSettings,
    load: loadRelationshipSettings,
  },
  {
    path: "filters",
    group: "people",
    icon: "filter",
    name: "settings.entry.filters",
    Screen: FilterSettings,
    load: loadFilterSettings,
  },
  {
    path: "security",
    group: "access",
    icon: "security",
    name: "settings.entry.security",
    Screen: SecuritySettings,
    load: loadSecuritySettings,
  },
  {
    path: "apps",
    group: "access",
    icon: "apps",
    name: "settings.entry.apps",
    Screen: AppsSettings,
    load: loadAppsSettings,
  },
  {
    // A one-person server has no invites (ADR-0152 §4). Elsewhere the menu offers them to a reader who may create
    // one (ADR-0106 §18), and the screen says to any other who opens its address why they may not.
    path: "invites",
    group: "access",
    icon: "invite",
    name: "settings.entry.invites",
    shown: (facts) => !facts.singleUser,
    listed: (facts) => facts.mayInvite,
    Screen: InvitesSettings,
    load: loadInvitesSettings,
  },
  {
    path: "account",
    group: "account",
    icon: "accountData",
    name: "settings.entry.account",
    count: ({ appealable }) => appealable,
    Screen: AccountSettings,
    load: loadAccountSettings,
  },
];

/** What the conditions and the counts of the entries read of the reader. */
function useSettingsFacts(): SettingsFacts {
  return {
    permissions: usePermissions(),
    appealable: useAppealableActions(),
    singleUser: useInstance()?.single_user === true,
    mayInvite: useMayInvite(),
    voice: useInstance()?.voice_enabled === true,
    supporters: useSupportersOffered(),
  };
}

/** Whether the server takes the readers' support in this app: its program or its tips are on, out of the native shell. */
function useSupportersOffered(): boolean {
  const instance = useInstance();
  return !inNativeShell() && (instance?.supporters_enabled === true || instance?.tips_enabled === true);
}

/**
 * Reads what the entries' conditions read that the app has not read by the time the settings open (whether the reader
 * may create invites), for the settings' loader to wait for.
 */
export async function loadSettingsFacts(data: AppData): Promise<void> {
  await loadInviteRight(data);
}

/**
 * Whether the server has answered every fact the entries' conditions read (whether the reader may create invites),
 * for the menu to paint its entries once, with every one it has: on the app's first load the loader does not wait.
 */
export function useSettingsFactsKnown(): boolean {
  return useInviteRightKnown();
}

/** An entry as the settings show it to this reader. */
export type ShownEntry = SettingsEntry & {
  readonly waiting: number | undefined;
  /** Whether the menu lists it; its screen opens at its address either way. */
  readonly inMenu: boolean;
};

/**
 * The entries whose screens open for the reader, in the menu's order, each with how many things wait on its screen
 * and whether the menu lists it.
 */
export function useShownEntries(): readonly ShownEntry[] {
  const facts = useSettingsFacts();
  return settingsEntries
    .filter((entry) => entry.shown?.(facts) ?? true)
    .map((entry) => ({ ...entry, waiting: entry.count?.(facts), inMenu: entry.listed?.(facts) ?? true }));
}

/** Reads what the screen of the entry at `path` shows; nothing for an address that names no entry. */
export async function loadSettingsEntry(data: AppData, path: string): Promise<void> {
  await settingsEntries.find((entry) => entry.path === path)?.load?.(data);
}
