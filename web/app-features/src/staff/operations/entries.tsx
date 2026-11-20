// SPDX-License-Identifier: AGPL-3.0-only

// The entries of the server's operations among the staff's screens (entries.tsx; ADR-0004, ADR-0058 §1, ADR-0103 §8,
// ADR-0167 §3): under Operations, the job queue (`manage_jobs`), the errors reported and the UX measurements
// (`manage_errors`); under Connections, the rules of the link card crawler (`manage_settings`), which say what the
// server fetches from other sites.

import type { AppData } from "@my/app-data";
import { loadStaffEntry, type StaffEntry, type StaffSearch } from "../entries";
import { ErrorGroupsScreen, loadErrorList } from "./errors";
import { JobQueueScreen, loadJobQueue } from "./jobs";
import { LinkCardRulesScreen, loadLinkCardDomains } from "./link-cards";
import { loadUXScreen, UXMeasurementsScreen } from "./ux";

/** The group these entries add to the root of the administration, after the others. */
export const operationGroups = [
  { id: "operations", area: "admin", heading: "staff_operations.group.operations" },
] as const;

/** The entries of the operations, in the root's order within their groups. */
export const operationEntries: readonly StaffEntry[] = [
  {
    // What the crawler of link cards does not fetch, per domain (ADR-0062).
    path: "link-card-domains",
    area: "admin",
    group: "connections",
    permission: "manage_settings",
    icon: "link",
    name: "staff_operations.entry.link_card_domains",
    hint: "staff_operations.entry.link_card_domains.hint",
    Screen: LinkCardRulesScreen,
    load: (data) => loadLinkCardDomains(data),
  },
  {
    // The job queue: its state, its jobs, and a failed job run again (ADR-0004).
    path: "jobs",
    area: "admin",
    group: "operations",
    permission: "manage_jobs",
    icon: "jobs",
    name: "staff_operations.entry.jobs",
    hint: "staff_operations.entry.jobs.hint",
    Screen: JobQueueScreen,
    load: (data) => loadJobQueue(data),
  },
  {
    // The errors the server and the web client reported, resolved and muted (ADR-0103 §8).
    path: "errors",
    area: "admin",
    group: "operations",
    permission: "manage_errors",
    icon: "errors",
    name: "staff_operations.entry.error_groups",
    hint: "staff_operations.entry.error_groups.hint",
    Screen: ErrorGroupsScreen,
    load: (data, search) => loadErrorList(data, search),
  },
  {
    // How fast the web client responds to its readers, and whether it is measured (ADR-0167 §3); its route is its own
    // (admin.ux-measurements.tsx), which holds the reader back while the form has changes not saved.
    path: "ux-measurements",
    area: "admin",
    group: "operations",
    permission: "manage_errors",
    icon: "measurements",
    name: "staff_operations.entry.ux_measurements",
    hint: "staff_operations.entry.ux_measurements.hint",
    Screen: ({ search, onSearch }) => <UXMeasurementsScreen search={search} onSearch={onSearch} />,
    load: (data, search) => loadUXScreen(data, search),
  },
];

/**
 * Reads what a screen under the entry of the administration at `path` shows (a job, an error group), with `load`, once
 * the reader's permissions open the entry, for its route's loader.
 */
export function loadUnderOperations(
  data: AppData,
  path: string,
  load: (data: AppData) => Promise<void>,
  search: StaffSearch = {},
): Promise<void> {
  return loadStaffEntry(data, "admin", path, search, load);
}
