// SPDX-License-Identifier: AGPL-3.0-only
// A post and its thread (FR-106): the status, and its context as the API gives it, the ancestors oldest first and the
// replies depth first in posting order.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import type { Status } from "./statuses";

/** The statuses around a status: what it replies to and what replies to it. */
export type StatusContext = components["schemas"]["StatusContext"];

/** The status of `id`. */
export function statusQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/statuses/{id}", { params: { path: { id } } });
}

/** The thread around the status of `id`. */
export function statusContextQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/statuses/{id}/context", { params: { path: { id } } });
}

/** What a screen shows of a post and its thread. */
export type Thread = {
  /** The status; undefined until the server answered, or when it failed. */
  readonly status: Status | undefined;
  /** Its thread; undefined until the server answered. */
  readonly context: StatusContext | undefined;
  /** The failure of the status (a ProblemError of mykit: a 404 for one the reader may not see), or null. */
  readonly error: Error | null;
  /** Reads the status and its thread again, after they failed. */
  readonly refetch: () => void;
};

/** The status of `id` and its thread. */
export function useThread(id: string): Thread {
  const data = useAppData();
  const status = useQuery(statusQuery(data, id));
  const context = useQuery(statusContextQuery(data, id));
  return {
    status: status.data,
    context: context.data,
    // The client rejects with a ProblemError or a NetworkError, which openapi-react-query types as the problem's body.
    error: status.error as Error | null,
    refetch: () => {
      void status.refetch();
      void context.refetch();
    },
  };
}
