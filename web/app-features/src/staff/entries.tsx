// SPDX-License-Identifier: AGPL-3.0-only

// The staff's screens (ADR-0058 §1, §2), declared in one place: each is an entry of the root of its area, /moderation
// or /admin, in a group, and a screen at /<area>/<path> that the permission it names opens. The root (root.tsx), the
// screen at an entry's address (screen.tsx) and the routes' loaders read this list alone, so a screen is added here
// and nowhere else: its declaration in `staffEntries`, at its place in its group, with its screen. A screen under an
// entry (a report, /moderation/reports/{id}) is a route of its own that `useStaffEntry` tells is open to the reader.
//
// Which permission belongs to which area is the server's (`internal/domain/identity`), which the API does not say: the
// navigation's places read the client's copy of it (app-data's rights.ts), and an entry names the permission that opens
// its screen.

import {
  instanceQuery,
  loadReportQueue,
  moderationCountsQuery,
  useAccountCounts,
  useInstance,
  useOpenReportCount,
  usePermissions,
  useViewer,
  viewerQuery,
  type AppData,
  type Permission,
} from "@my/app-data";
import type { IconName } from "@my/design-system";
import type { ComponentType } from "react";
import type { MessageId } from "../messages.gen";
import { queueOf, ReportQueueScreen } from "./reports/queue";
import { serverEntries } from "./server/entries";
import { accountEntries, accountGroups } from "./accounts/entries";

/** The two areas of the staff's screens, each a tree of routes under its root. */
const staffAreas = ["moderation", "admin"] as const;
export type StaffArea = (typeof staffAreas)[number];

/** The groups of an area's root, in its order, each under its heading. */
export const staffGroups = [
  { id: "queues", area: "moderation", heading: "staff.group.queues" },
  // The server's own settings, texts and rules (#3262).
  { id: "server", area: "admin", heading: "staff.group.server" },
  // What the server connects to and lets connect to it (#3262).
  { id: "connections", area: "admin", heading: "staff.group.connections" },
  ...accountGroups,
] as const satisfies readonly { id: string; area: StaffArea; heading: MessageId }[];
type StaffGroupId = (typeof staffGroups)[number]["id"];

/** The query of a screen's address, member by member, as a screen of the staff reads it. */
export type StaffSearch = Readonly<Record<string, string | undefined>>;

/** What a staff screen reads of its address, and how it changes it. */
export type StaffAddress = {
  /** The query of the screen's address (its filters, its page). */
  readonly search: StaffSearch;
  /** Shows the screen with `search` in place of the query, as a new entry of the history. */
  readonly onSearch: (search: StaffSearch) => void;
  /** Shows another address of the app, as a new entry of the history (a server the reader names, a thing made). */
  readonly go: (href: string) => void;
};

/** What an entry's screen is given by the route that places it. */
export type StaffEntryProps = StaffAddress;

/** What the root reads of the reader, for the entries it lists and their counts: one hook reads it (`useStaffFacts`). */
export type StaffFacts = {
  /** The permissions of the reader's account; none until the server said them. */
  readonly permissions: readonly Permission[];
  /** The reports that are open; undefined until the server said, or without `manage_reports`. */
  readonly openReports: number | undefined;
  /**
   * The registrations that wait for approval and the appeals that wait for a decision; undefined until the server
   * said, or without `manage_accounts`.
   */
  readonly pendingRegistrations: number | undefined;
  readonly pendingAppeals: number | undefined;
  /** The server is a one-person server (ADR-0152); undefined until the server said. */
  readonly singleUser: boolean | undefined;
};

/** A screen of the staff's. */
export type StaffEntry = {
  /** The last part of its address, /<area>/<path>. */
  readonly path: string;
  readonly area: StaffArea;
  readonly group: StaffGroupId;
  /** The permission that opens it, which the server checks of each of its operations too. */
  readonly permission: Permission;
  readonly icon: IconName;
  /** Its name, in the root and as the title of its screen (staff.entry.<path>). */
  readonly name: MessageId;
  /** What its screen holds, the line under its name in the root (staff.entry.<path>.hint). */
  readonly hint: MessageId;
  /** How many things wait there for the reader, the badge at the row's end, and its words; none without it. */
  readonly count?: { readonly of: (facts: StaffFacts) => number | undefined; readonly words: MessageId };
  /** Whether the server offers it at all (none on a one-person server); offered where left out. */
  readonly shown?: (facts: StaffFacts) => boolean;
  readonly Screen: ComponentType<StaffEntryProps>;
  /** Reads what its screen shows at the query `search`, for the route's loader to wait for. */
  readonly load?: (data: AppData, search: StaffSearch) => Promise<void>;
};

/** Every entry, in the root's order within its group. */
export const staffEntries: readonly StaffEntry[] = [
  {
    // The moderators' queue (FR-709–FR-712): the open reports first, which the badge of Moderation counts with the rest.
    path: "reports",
    area: "moderation",
    group: "queues",
    permission: "manage_reports",
    icon: "report",
    name: "staff.entry.reports",
    hint: "staff.entry.reports.hint",
    count: { of: (facts) => facts.openReports, words: "staff.entry.reports.count" },
    Screen: ReportQueueScreen,
    load: (data, search) => loadReportQueue(data, queueOf(search)),
  },
  // Roles, custom emoji, announcements and webhooks (#3263).
  ...serverEntries,
  ...accountEntries,
];

/**
 * What the root reads of the reader (`StaffFacts`), and whether the server said who they are yet; nothing asked while
 * `enabled` is false (a reader without a session).
 */
export function useStaffFacts(enabled = true): StaffFacts & { readonly known: boolean } {
  const known = useViewer(enabled) !== undefined;
  const permissions = usePermissions(enabled);
  const openReports = useOpenReportCount(enabled);
  const counts = useAccountCounts(enabled);
  const singleUser = useInstance()?.single_user;
  return {
    permissions,
    openReports,
    pendingRegistrations: counts.registrations,
    pendingAppeals: counts.appeals,
    singleUser,
    known,
  };
}

/**
 * How many things wait for the reader in `area`: the sum of the counts of the entries its root lists them, which the
 * navigation's badge shows, so that the two always agree; undefined while none of them counts anything yet.
 */
export function useStaffWaiting(area: StaffArea, enabled = true): number | undefined {
  const facts = useStaffFacts(enabled);
  const counts = staffEntries
    .filter((e) => e.area === area && opens(e, facts))
    .flatMap((e) => {
      const n = e.count?.of(facts);
      return n === undefined ? [] : [n];
    });
  return counts.length === 0 ? undefined : counts.reduce((a, b) => a + b, 0);
}

/** Whether `entry` is open to the reader of `facts`: their permissions open it, and the server offers it. */
const opens = (entry: StaffEntry, facts: StaffFacts): boolean =>
  facts.permissions.includes(entry.permission) && (entry.shown?.(facts) ?? true);

/** The entries of `area` open to the reader, in their order; none until the server said who they are. */
export function useStaffEntries(area: StaffArea): readonly StaffEntry[] {
  const facts = useStaffFacts();
  return staffEntries.filter((e) => e.area === area && opens(e, facts));
}

/** The entry of `area` at `path` open to the reader; undefined for none, or one their permissions do not open. */
export function useStaffEntry(area: StaffArea, path: string): StaffEntry | undefined {
  return useStaffEntries(area).find((e) => e.path === path);
}

/** The address of `pathname` with `search` as its query, the members it leaves out left out. */
export function staffHref(pathname: string, search: StaffSearch): string {
  const query = new URLSearchParams(
    Object.entries(search).flatMap(([k, v]) => (v === undefined ? [] : [[k, v] as [string, string]])),
  ).toString();
  return query === "" ? pathname : `${pathname}?${query}`;
}

/** The address of the root of `area`. */
export const staffRootPath = (area: StaffArea): string => `/${area}`;

/** The address of the screen of the entry of `area` at `path`. */
export const staffEntryPath = (area: StaffArea, path: string): string => `/${area}/${path}`;

/** Reads who the reader is and what waits for them, which the root lists, for the loader of an area's routes. */
export async function loadStaffFacts(data: AppData): Promise<void> {
  const viewer = await readViewer(data);
  if (viewer?.permissions.some((p) => p === "manage_reports" || p === "manage_accounts") === true)
    await data.queryClient.query(moderationCountsQuery(data)).catch(() => undefined);
}

/** The reader's account, from the cache unless a write marked it stale; undefined when it could not be read. */
const readViewer = (data: AppData) =>
  data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined);

/** Reads what the screen of the entry of `area` at `path` shows at `search`, once the reader's permissions open it. */
export async function loadStaffEntry(
  data: AppData,
  area: StaffArea,
  path: string,
  search: StaffSearch,
  /** What to read in place of the entry's own screen (a screen under it); the entry's `load` where left out. */
  load?: (data: AppData) => Promise<void>,
): Promise<void> {
  const viewer = await readViewer(data);
  const entry = staffEntries.find((e) => e.area === area && e.path === path);
  if (entry === undefined || viewer?.permissions.includes(entry.permission) !== true) return;
  // An entry the server does not offer (registrations on a one-person server) asks it nothing: its screen says that
  // the address names none, as the root lists none.
  if (entry.shown !== undefined) {
    const instance = await data.queryClient
      .query({ ...instanceQuery(data), staleTime: Infinity })
      .catch(() => undefined);
    const facts: StaffFacts = {
      permissions: viewer.permissions,
      openReports: undefined,
      pendingRegistrations: undefined,
      pendingAppeals: undefined,
      singleUser: instance?.single_user,
    };
    if (!entry.shown(facts)) return;
  }
  await (load === undefined ? entry.load?.(data, search) : load(data));
}
