// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the queue dashboard (ADR-0064, web-client/README.md S1): the state of the job queue, its
// jobs by state, the records of a job's runs that did not succeed, and running a dead job again. The lists follow
// the API's keyset pagination as TanStack Query infinite queries, keyed like the client's own `[method, path]` with
// "pages" at the end (web-client/10-api-client-and-auth.md §10).
import { infiniteQueryOptions, type QueryClient, queryOptions } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { components } from "@mycroblog/mykit";
import { pageQuery } from "../../moderation";

export type Job = components["schemas"]["AdminJob"];
export type JobState = components["schemas"]["AdminJobState"];
export type JobAttempt = components["schemas"]["AdminJobAttempt"];
export type JobQueue = components["schemas"]["AdminQueue"];

/** The filters of the job list, in the order of its tabs: the pending jobs of every state, then each state. */
export const jobFilters = ["pending", "queued", "running", "retrying", "dead"] as const;

export type JobFilter = (typeof jobFilters)[number];

/** The states of the jobs, in the order the dashboard counts them. */
export const jobStates = ["queued", "running", "retrying", "dead"] as const satisfies readonly JobState[];

/** The state of the queue: its jobs by state and type, the oldest waiting job, and what it can list (GET /admin/queue). */
export function jobQueueQueryOptions(api: Api) {
  return queryOptions({
    queryKey: ["get", "/admin/queue"] as const,
    queryFn: async ({ signal }) => {
      const { data, error } = await api.fetch.GET("/admin/queue", { signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  });
}

/** The jobs under `filter` (GET /admin/jobs): pending ones the earliest due first, dead ones the most recently failed first. */
export function jobsQueryOptions(api: Api, filter: JobFilter) {
  const state = filter === "pending" ? undefined : filter;
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/jobs", { state }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = { ...pageQuery(pageParam), ...(state === undefined ? {} : { state }) };
      const { data, error } = await api.fetch.GET("/admin/jobs", { params: { query }, signal });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The records of the runs of the job `id` that did not succeed, the most recently finished first (GET /admin/jobs/{id}/attempts). */
export function jobAttemptsQueryOptions(api: Api, id: string) {
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/jobs/{id}/attempts", { id }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/admin/jobs/{id}/attempts", { params: { path: { id }, query: pageQuery(pageParam) }, signal });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** Runs a dead job again (POST /admin/jobs/{id}/retry), as the options of useMutation. */
export function retryJobMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/jobs/{id}/retry"] as const,
    mutationFn: async (id: string): Promise<void> => {
      // The middleware throws for every error status; a 204 has no body to read.
      await api.fetch.POST("/admin/jobs/{id}/retry", { params: { path: { id } } });
    },
  };
}

/**
 * Takes the dead job `id`, enqueued again, off the cached list of dead jobs (ADR-0045) and returns it as the list had
 * it. The job is pending now, so the counts of the queue and the lists of pending jobs are marked stale (S1).
 */
export function cacheRetriedJob(api: Api, queryClient: QueryClient, id: string): Job | undefined {
  const listKey = jobsQueryOptions(api, "dead").queryKey;
  const job = queryClient.getQueryData(listKey)?.pages.flatMap((page) => page.items).find((item) => item.id === id);
  queryClient.setQueryData(listKey, (data) => data && { ...data, pages: data.pages.map((page) => ({ ...page, items: page.items.filter((item) => item.id !== id) })) });
  void queryClient.invalidateQueries({ queryKey: jobQueueQueryOptions(api).queryKey });
  void queryClient.invalidateQueries({
    predicate: (query) => query.queryKey[1] === "/admin/jobs" && !isDeadList(query.queryKey[2]),
  });
  return job;
}

function isDeadList(params: unknown): boolean {
  return typeof params === "object" && params !== null && "state" in params && params.state === "dead";
}
