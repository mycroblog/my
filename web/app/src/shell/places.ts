// SPDX-License-Identifier: AGPL-3.0-only

// The places of the navigation (ADR-0110 §11, ADR-0058 §2, ADR-0070 §1, ADR-0152 §2): one list in one order, the
// timelines the instance offers, notifications and search, then the reader's own places (profile, bookmarks,
// settings), then apart the places of their rights (moderation, admin, finance). The tab bar holds the first part and
// Menu; the menu's sheet holds the rest, and the announcements. A visitor without a session has the timelines the
// instance shows everyone, and search.

import type { Instance, Rights } from "@my/app-data";
import type { IconName } from "@my/design-system";
import type { MessageId } from "../messages.gen";

/** A place of the navigation; `count` names what its badge counts. */
export type Place = {
  readonly href: string;
  readonly icon: IconName;
  readonly label: MessageId;
  readonly count?: "notifications" | "moderation";
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

/** The reader's own places, after the tab bar's in the rail and the sidebar, and in the menu's sheet on a phone. */
const ownPlaces: readonly Place[] = [
  { href: "/profile", icon: "profile", label: "app.profile" },
  { href: "/bookmarks", icon: "bookmark", label: "app.bookmarks" },
  { href: "/settings", icon: "settings", label: "app.settings" },
];

const moderation: Place = { href: "/moderation", icon: "moderation", label: "app.moderation", count: "moderation" };
const admin: Place = { href: "/admin", icon: "admin", label: "app.admin" };
const finance: Place = { href: "/finance", icon: "finance", label: "app.finance" };

// What is around the reader, whose sections the column beside the main one shows from `wide`: the menu leads to their
// screens at every width (ADR-0125 §4).
const announcements: Place = { href: "/announcements", icon: "announcements", label: "app.announcements" };
const voiceRooms: Place = { href: "/voice", icon: "room", label: "app.voice_rooms" };
const events: Place = { href: "/events", icon: "events", label: "app.events_place" };
const funding: Place = { href: "/about#funding", icon: "funding", label: "app.funding" };
/** The sign-in at `href`, in a visitor's menu. */
const signInPlace = (href: string): Place => ({ href, icon: "signIn", label: "app.sign_in_action" });

/** Every place of the navigation, which a navigation between two of replaces the screen (transitions.ts). */
export const navigationPaths: ReadonlySet<string> = new Set(
  [
    home,
    local,
    federated,
    notifications,
    search,
    ...ownPlaces,
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
 * on (FR-920: none of them shows while they are off, nor before the instance says), the events, and the funding while
 * the instance shows its meter. A visitor has the sign-in (at `signIn`) alone: the API answers the voice rooms and the
 * events only with a session.
 */
export function aroundPlacesFor(signedIn: boolean, instance: Instance | undefined, signIn: string): readonly Place[] {
  if (!signedIn) return [signInPlace(signIn)];
  return [
    announcements,
    ...(instance?.voice_enabled === true ? [voiceRooms] : []),
    events,
    ...(instance?.funding_meter === undefined ? [] : [funding]),
  ];
}

/** The places of a reader: the tab bar's, their own, and those of their rights. */
export type Places = {
  readonly tabs: readonly Place[];
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
  if (!signedIn) return { tabs: [...timelines, search], own: [], rights: [] };
  return {
    tabs: [home, ...timelines, notifications, search],
    own: ownPlaces,
    rights: [
      ...(rights.moderation ? [moderation] : []),
      ...(rights.admin ? [admin] : []),
      ...(rights.finance ? [finance] : []),
    ],
  };
}

/** Whether the screen at `pathname` is a timeline or a profile, where a new post floats on a phone. */
export const composesAt = (pathname: string) =>
  ["/", "/local", "/public", "/profile"].includes(pathname) ||
  (pathname.startsWith("/@") && !pathname.includes("/status/"));
