// SPDX-License-Identifier: AGPL-3.0-only
// The job queue as a holder of `manage_jobs` reads it (ADR-0004, ADR-0058 §1): the queue's state (its jobs by state and
// type, how long each type has kept a job waiting, and whether the queue lists its pending jobs at all); the jobs of one
// state and type, a page at a time by the server's cursor; a job, pending or dead, with the runs that did not succeed;
// and running a dead job again. The retry reads again everything the job is in: the queue, the lists, the job and its
// attempts.
import type { components } from "@mycroblog/mykit";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";
import { oneOf, pageOf, staffChanged, type StaffOne, type StaffPage } from "./staff-reads";

/** The state of the job queue. */
export type AdminQueue = components["schemas"]["AdminQueue"];
/** A job of the queue, pending or dead. */
export type AdminJob = components["schemas"]["AdminJob"];
/** The state of a job. */
export type AdminJobState = components["schemas"]["AdminJobState"];
/** A run of a job that did not succeed. */
export type AdminJobAttempt = components["schemas"]["AdminJobAttempt"];

/** The states of a job, in the order a job goes through them. */
export const jobStates = ["queued", "running", "retrying", "dead"] as const satisfies readonly AdminJobState[];

/** What the list of jobs shows: the jobs in `state` (the pending ones where left out) of `type` (every type). */
export type AdminJobQuery = {
  readonly state?: AdminJobState | undefined;
  readonly type?: string | undefined;
  readonly cursor?: string | undefined;
};

/** The state of the queue. */
function queueQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/queue");
}

/** Reads the state of the queue, for a route's loader. */
export async function loadAdminQueue(data: AppData): Promise<void> {
  await data.queryClient.query(queueQuery(data)).catch(() => undefined);
}

/** The state of the queue, and where its read is. */
export function useAdminQueue(): StaffOne<AdminQueue> {
  return oneOf(useQuery(queueQuery(useAppData())));
}

/** A page of the jobs of `q`. */
function jobsQuery({ queries }: AppData, q: AdminJobQuery) {
  return queries.queryOptions("get", "/admin/jobs", {
    params: {
      query: {
        ...(q.state === undefined ? {} : { state: q.state }),
        ...(q.type === undefined ? {} : { type: q.type }),
        ...(q.cursor === undefined ? {} : { cursor: q.cursor }),
      },
    },
  });
}

/** Reads a page of the jobs of `q`, for a route's loader. */
export async function loadAdminJobs(data: AppData, q: AdminJobQuery): Promise<void> {
  await data.queryClient.query(jobsQuery(data, q)).catch(() => undefined);
}

/** The page of the jobs of `q`, the page shown before staying while the one asked for is on its way. */
export function useAdminJobs(q: AdminJobQuery): StaffPage<AdminJob> {
  return pageOf(useQuery({ ...jobsQuery(useAppData(), q), placeholderData: keepPreviousData }));
}

/** The job of `id`. */
function jobQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/jobs/{id}", { params: { path: { id } } });
}

/** The attempts of the job of `id` that did not succeed, the newest first, a page at a time. */
function attemptsQuery({ api, queries }: AppData, id: string) {
  return pagedQuery<AdminJobAttempt>(
    queries.queryOptions("get", "/admin/jobs/{id}/attempts", { params: { path: { id } } }).queryKey,
    async (cursor, signal) =>
      (
        await api.GET("/admin/jobs/{id}/attempts", {
          params: { path: { id }, query: cursor === undefined ? {} : { cursor } },
          signal,
        })
      ).data,
  );
}

/** Reads the job of `id` and its newest attempts, for a route's loader. */
export async function loadAdminJob(data: AppData, id: string): Promise<void> {
  await Promise.all([
    data.queryClient.query(jobQuery(data, id)).catch(() => undefined),
    data.queryClient.infiniteQuery(attemptsQuery(data, id)).catch(() => undefined),
  ]);
}

/** The job of `id`; `missing` once the server says it has none (a job that completed, or was deleted). */
export function useAdminJob(id: string): StaffOne<AdminJob> {
  return oneOf(useQuery(jobQuery(useAppData(), id)));
}

/** The attempts of the job of `id` that did not succeed, the newest first, as many pages as the reader asked for. */
export function useAdminJobAttempts(id: string): PagedList<AdminJobAttempt> {
  return usePagedList(attemptsQuery(useAppData(), id));
}

/**
 * Runs the dead job of `id` again, a new count of attempts from 1; the queue, the lists of jobs, the job and its
 * attempts are read again, after a refusal too. Rejects with the server's problem: a 404 for a job that is not dead
 * (retried or deleted meanwhile).
 */
export async function retryAdminJob(data: AppData, id: string): Promise<void> {
  try {
    await data.api.POST("/admin/jobs/{id}/retry", { params: { path: { id } } });
  } finally {
    // A refusal too says the job is not what the screen shows (retried or deleted meanwhile): it is read again.
    staffChanged(data, ["/admin/jobs", "/admin/queue"]);
  }
}

/** Runs a dead job again, for a screen (`retryAdminJob`). */
export function useRetryAdminJob(): (id: string) => Promise<void> {
  const data = useAppData();
  return (id) => retryAdminJob(data, id);
}
