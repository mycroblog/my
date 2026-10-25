// SPDX-License-Identifier: AGPL-3.0-only
// The closure of the service (FR-808, FR-809, ADR-0088) as GET /instance states it: absent while none is announced,
// `announced` with the closing date and the export deadline during the notice, `closed` (and later
// `delivery_completed`) once the server has closed. The server decides every request; the app shows the notice
// and, once closed, the screen of the export.
import { useQuery } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import { useApi } from "./auth/SessionProvider";
import { shellQueryReader } from "./query";

/** The closure as the instance states it. */
export type Closure = components["schemas"]["InstanceClosure"];

/** Whether the closure is only announced: the server still works as usual. */
export function isAnnounced(closure: Closure | undefined): closure is Closure & { state: "announced" } {
  return closure?.state === "announced";
}

/** Whether the server has closed: users only sign in and take their data away. */
export function hasClosed(closure: Closure | undefined): closure is Closure & { state: "closed" | "delivery_completed" } {
  return closure?.state === "closed" || closure?.state === "delivery_completed";
}

/** The closure from GET /instance; undefined while none is announced, and until the instance loads. */
export function useClosure(): Closure | undefined {
  const api = useApi();
  // The shell reads GET /instance on every page and handles its failures; this only reads its answer.
  const instance = useQuery({ ...api.query.queryOptions("get", "/instance"), ...shellQueryReader });
  return instance.data?.closure;
}
