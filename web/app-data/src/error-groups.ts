// SPDX-License-Identifier: AGPL-3.0-only
// The errors the server and the web client reported, as a holder of `manage_errors` reads them (ADR-0103 §8): the
// groups of one state and source, the most recently seen first, a page at a time by the server's cursor; a group with
// its latest occurrence, its hourly counts and the changes of its state; and resolving, muting and unmuting one. A
// write puts the group the server answered into the group's read and reads again the lists and what awaits the staff.
import type { components } from "@mycroblog/mykit";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { oneOf, pageOf, staffChanged, type StaffOne, type StaffPage } from "./staff-reads";
import { writeCached } from "./written";

/** A group of reported errors, as the list gives it. */
export type ErrorGroup = components["schemas"]["ErrorGroup"];
/** A group with its latest occurrence, its hourly counts and the changes of its state. */
export type ErrorGroupDetail = components["schemas"]["ErrorGroupDetail"];
/** The state of an error group. */
export type ErrorGroupState = components["schemas"]["ErrorGroupState"];
/** What reported the errors of a group. */
export type ErrorGroupSource = components["schemas"]["ErrorGroupSource"];
/** A change of the state of a group. */
export type ErrorGroupEvent = components["schemas"]["ErrorGroupEvent"];

/** The states of a group, in the server's order. */
export const errorGroupStates = ["open", "resolved", "muted"] as const satisfies readonly ErrorGroupState[];
/** The sources of a group, in the server's order. */
export const errorGroupSources = ["server", "web"] as const satisfies readonly ErrorGroupSource[];

/** What the list of groups shows: the groups of `state` and `source`, each of every kind where left out. */
export type ErrorGroupQuery = {
  readonly state?: ErrorGroupState | undefined;
  readonly source?: ErrorGroupSource | undefined;
  readonly cursor?: string | undefined;
};

/** A page of the groups of `q`. */
function groupsQuery({ queries }: AppData, q: ErrorGroupQuery) {
  return queries.queryOptions("get", "/admin/error_groups", {
    params: {
      query: {
        ...(q.state === undefined ? {} : { state: q.state }),
        ...(q.source === undefined ? {} : { source: q.source }),
        ...(q.cursor === undefined ? {} : { cursor: q.cursor }),
      },
    },
  });
}

/** Reads a page of the groups of `q`, for a route's loader. */
export async function loadErrorGroups(data: AppData, q: ErrorGroupQuery): Promise<void> {
  await data.queryClient.query(groupsQuery(data, q)).catch(() => undefined);
}

/** The page of the groups of `q`, the page shown before staying while the one asked for is on its way. */
export function useErrorGroups(q: ErrorGroupQuery): StaffPage<ErrorGroup> {
  return pageOf(useQuery({ ...groupsQuery(useAppData(), q), placeholderData: keepPreviousData }));
}

/** The group of `id`. */
function groupQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/error_groups/{id}", { params: { path: { id } } });
}

/** Reads the group of `id`, for a route's loader. */
export async function loadErrorGroup(data: AppData, id: string): Promise<void> {
  await data.queryClient.query(groupQuery(data, id)).catch(() => undefined);
}

/** The group of `id`, with its latest occurrence; `missing` once the server says it has none. */
export function useErrorGroup(id: string): StaffOne<ErrorGroupDetail> {
  return oneOf(useQuery(groupQuery(useAppData(), id)));
}

/** The writes of the state of a group; each resolves with the group as the server answered, else rejects. */
export type ErrorGroupWrites = {
  /** Marks a group resolved with the version of its last occurrence. */
  readonly resolve: (id: string) => Promise<ErrorGroup>;
  /** Mutes a group: it notifies nobody, and is still counted. */
  readonly mute: (id: string) => Promise<ErrorGroup>;
  /** Opens a muted group again. */
  readonly unmute: (id: string) => Promise<ErrorGroup>;
};

/**
 * Changes the state of the group of `id` by `act`: the group the server answered goes into the group's read (its
 * events read again with it), and the lists and the counts of what awaits the staff are read again.
 */
export async function changeErrorGroup(
  data: AppData,
  id: string,
  act: "resolve" | "mute" | "unmute",
): Promise<ErrorGroup> {
  const init = { params: { path: { id } } };
  const { data: group } =
    act === "resolve"
      ? await data.api.POST("/admin/error_groups/{id}/resolve", init)
      : act === "mute"
        ? await data.api.POST("/admin/error_groups/{id}/mute", init)
        : await data.api.POST("/admin/error_groups/{id}/unmute", init);
  if (group === undefined) throw new Error("app-data: a change of an error group answered without the group");
  await writeCached<ErrorGroupDetail>(data, groupQuery(data, id).queryKey, (now) => ({ ...now, ...group }));
  staffChanged(data, ["/admin/error_groups"]);
  return group;
}

/** The writes of the state of a group, for a screen (`changeErrorGroup`). */
export function useErrorGroupWrites(): ErrorGroupWrites {
  const data = useAppData();
  return {
    resolve: (id) => changeErrorGroup(data, id, "resolve"),
    mute: (id) => changeErrorGroup(data, id, "mute"),
    unmute: (id) => changeErrorGroup(data, id, "unmute"),
  };
}
