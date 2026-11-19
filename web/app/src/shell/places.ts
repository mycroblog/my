// SPDX-License-Identifier: AGPL-3.0-only

// The places of the navigation (ADR-0110 §11, ADR-0058 §2, ADR-0070 §1, ADR-0152 §2): one list in one order, the
// timelines the instance offers, notifications, the direct messages and search, then the reader's own places (profile,
// bookmarks, settings), then apart the places of their rights (moderation, admin, finance). The tab bar holds the first
// part but the direct messages, and Menu; the menu's sheet holds the rest, the direct messages first, and the
// announcements. A visitor without a session has the timelines the instance shows everyone, and search.

import { costsPagePath, supportersPagePath } from "@my/app-components";
import type { Instance, Rights } from "@my/app-data";
import type { IconName } from "@my/design-system";
import { inNativeShell } from "@my/platform";
import type { MessageId } from "../messages.gen";

/** A place of the navigation; `count` names what its badge counts. */
export type Place = {
  readonly href: string;
  readonly icon: IconName;
  readonly label: MessageId;
  readonly count?: "notifications" | "direct" | "moderation";
};

const home: Place = { href: "/", icon: "home", label: "app.home" };
const local: Place = { href: "/local", icon: "local", label: "app.local" };
const federated: Place = { href: "/public", icon: "federated", label: "app.federated" };
const notifications: Place = {
  href: "/notifications",
  icon: "notifications",
  label: "app.notifications",
  count: "notifications",
};
const search: Place = { href: "/search", icon: "search", label: "app.search" };

/** The direct messages, after notifications in the rail and the sidebar, and first in the menu's sheet on a phone. */
const directMessages: Place = {
  href: "/conversations",
  icon: "directMessages",
  label: "app.direct_messages",
  count: "direct",
};

/** The settings: their menu, whose entries open screens under it. */
const settings: Place = { href: "/settings", icon: "settings", label: "app.settings" };

/** The reader's own places, after the tab bar's in the rail and the sidebar, and in the menu's sheet on a phone. */
const ownPlaces: readonly Place[] = [
  { href: "/profile", icon: "profile", label: "app.profile" },
  { href: "/bookmarks", icon: "bookmark", label: "app.bookmarks" },
  settings,
];

/**
 * Whether `place` is where the reader at `pathname` is: at its own address, or, for the settings, at a screen under
 * their menu, which the navigation marks as the settings too.
 */
export const isPlaceAt = (place: Place, pathname: string): boolean =>
  place.href === pathname || (place === settings && pathname.startsWith(`${settings.href}/`));

/**
 * The reader's lists, which the rail and the sidebar leave to the menu (ADR-0110 §11 names their places): a group of
 * its own at the top of their menu, and after the reader's own places, with them, in the phone's sheet.
 */
export const listsPlace: Place = { href: "/lists", icon: "list", label: "app.lists" };

const moderation: Place = { href: "/moderation", icon: "moderation", label: "app.moderation", count: "moderation" };
const admin: Place = { href: "/admin", icon: "admin", label: "app.admin" };
const finance: Place = { href: "/finance", icon: "finance", label: "app.finance" };

// What is around the reader, whose sections the column beside the main one shows from `wide`: the menu leads to their
// screens at every width (ADR-0125 §4).
const announcements: Place = { href: "/announcements", icon: "announcements", label: "app.announcements" };
const voiceRooms: Place = { href: "/voice", icon: "room", label: "app.voice_rooms" };
const events: Place = { href: "/events", icon: "events", label: "app.events_place" };
/** The funding meter's place: the costs page, whose month's costs the meter says are covered (FR-905). */
const funding: Place = { href: costsPagePath, icon: "funding", label: "app.funding" };
const supporters: Place = { href: supportersPagePath, icon: "funding", label: "app.supporters" };

/**
 * Whether the supporters' page has something for the reader: the plans or the tips, out of the native shell, which
 * shows no purchase (ADR-0072 §14, ADR-0162 §3), or the supporters list the instance shows.
 */
export function supportersPageShows(instance: Instance | undefined, nativeShell = inNativeShell()): boolean {
  if (instance === undefined) return false;
  const offers = !nativeShell && (instance.supporters_enabled || instance.tips_enabled);
  const list = instance.supporters_enabled && instance.supporter_list_visibility !== "off";
  return offers || list;
}

/** The sign-in at `href`, in a visitor's menu. */
const signInPlace = (href: string): Place => ({ href, icon: "signIn", label: "app.sign_in_action" });

/** Every place of the navigation, which a navigation between two of replaces the screen (transitions.ts). */
export const navigationPaths: ReadonlySet<string> = new Set(
  [
    home,
    local,
    federated,
    notifications,
    directMessages,
    search,
    ...ownPlaces,
    listsPlace,
    moderation,
    admin,
    finance,
    announcements,
    voiceRooms,
    events,
  ].map((p) => p.href),
);

/**
 * The places of what is around the reader, which the menu holds: the announcements, the voice rooms while they are
 * on (FR-920: none of them shows while they are off, nor before the instance says), the events, the supporters' page
 * while it has something for the reader, and the costs page while the instance shows its funding meter. A visitor
 * has the sign-in (at `signIn`) alone: the API answers the voice rooms and the events only with a session.
 */
export function aroundPlacesFor(signedIn: boolean, instance: Instance | undefined, signIn: string): readonly Place[] {
  if (!signedIn) return [signInPlace(signIn)];
  return [
    announcements,
    ...(instance?.voice_enabled === true ? [voiceRooms] : []),
    events,
    ...(supportersPageShows(instance) ? [supporters] : []),
    ...(instance?.funding_meter === undefined ? [] : [funding]),
  ];
}

/**
 * The places of a reader: the tab bar's; the rail's and the sidebar's (the tab bar's with the direct messages after
 * notifications, then their own); their own that the tab bar has not, as the menu's sheet lists them (the direct
 * messages first); and those of their rights.
 */
export type Places = {
  readonly tabs: readonly Place[];
  readonly side: readonly Place[];
  readonly own: readonly Place[];
  readonly rights: readonly Place[];
};

/**
 * The places for a reader signed in or not, by the timelines the instance offers (all of them until it answered) and
 * the reader's rights.
 */
export function placesFor(signedIn: boolean, instance: Instance | undefined, rights: Rights): Places {
  const offers = (audience: "off" | "signed_in" | "everyone" | undefined) =>
    audience === undefined ? true : signedIn ? audience !== "off" : audience === "everyone";
  const timelines = [
    ...(offers(instance?.timelines.local) ? [local] : []),
    ...(offers(instance?.timelines.federated) ? [federated] : []),
  ];
  if (!signedIn) return { tabs: [...timelines, search], side: [...timelines, search], own: [], rights: [] };
  return {
    tabs: [home, ...timelines, notifications, search],
    side: [home, ...timelines, notifications, directMessages, search, ...ownPlaces],
    own: [directMessages, ...ownPlaces],
    rights: [
      ...(rights.moderation ? [moderation] : []),
      ...(rights.admin ? [admin] : []),
      ...(rights.finance ? [finance] : []),
    ],
  };
}

/**
 * Whether the screen at `pathname` is a timeline (a hashtag's and a list's too) or a profile, where a new post floats
 * on a phone.
 */
export const composesAt = (pathname: string) =>
  ["/", "/local", "/public", "/profile"].includes(pathname) ||
  pathname.startsWith("/tags/") ||
  /^\/lists\/[^/]+$/.test(pathname) ||
  (pathname.startsWith("/@") && !pathname.includes("/status/"));
