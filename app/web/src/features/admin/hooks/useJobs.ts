// SPDX-License-Identifier: AGPL-3.0-only
// The queue dashboard (ADR-0064): the state of the job queue, its jobs under a state filter, the records of a job's
// runs that did not succeed, and running dead jobs again. A job that is enqueued again leaves the dead jobs on the
// server, so it leaves the cached pages at once.
import { useAnnouncer, useToast } from "@my/ui";
import {
  useInfiniteQuery,
  useMutation,
  useMutationState,
  useQueryClient,
  useSuspenseInfiniteQuery,
  useSuspenseQuery,
} from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  cacheRetriedJob,
  type Job,
  type JobAttempt,
  jobAttemptsQueryOptions,
  type JobFilter,
  type JobQueue,
  jobQueueQueryOptions,
  jobsQueryOptions,
  retryJobMutation,
} from "../api/jobs";
import { type PagedList, toPagedList } from "../../moderation";

/** The state of the queue. */
export function useJobQueue(): JobQueue {
  const api = useApi();
  return useSuspenseQuery(jobQueueQueryOptions(api)).data;
}

/** The jobs under `filter`. */
export function useJobs(filter: JobFilter): PagedList<Job> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(jobsQueryOptions(api, filter)));
}

/** The records of a job's runs that did not succeed, read when the history is opened. */
export type JobAttempts =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | {
      status: "ready";
      items: readonly JobAttempt[];
      hasMore: boolean;
      loadMore: () => void;
      isLoadingMore: boolean;
      /** Why the next page failed to load; undefined when it did not. */
      moreError: string | undefined;
    };

/** The records of the runs of the job `id` that did not succeed, the most recently finished first. */
export function useJobAttempts(id: string): JobAttempts {
  const api = useApi();
  const query = useInfiniteQuery(jobAttemptsQueryOptions(api, id));
  if (query.data !== undefined) {
    return {
      status: "ready",
      items: query.data.pages.flatMap((page) => page.items),
      hasMore: query.hasNextPage,
      loadMore: () => {
        if (!query.isFetching) void query.fetchNextPage();
      },
      isLoadingMore: query.isFetchingNextPage,
      moreError: query.isFetchNextPageError ? errorMessage(query.error) : undefined,
    };
  }
  if (query.error !== null)
    return {
      status: "error",
      reason: errorMessage(query.error),
      retry: () => void query.refetch(),
      isRetrying: query.isFetching,
    };
  return { status: "loading" };
}

export type RetryJob = {
  /** Enqueues the dead job again; the result is announced, and a failure is shown in a toast. */
  retry: (job: Job) => void;
  /** The IDs of the jobs being enqueued again. */
  pendingIds: readonly string[];
};

/**
 * Running dead jobs again. `onRetried(id)` is called when a job is enqueued, just before it leaves the list (to keep
 * focus, lib/useFocusAfterRemoval). Several jobs may be retried at once, so everything a retry does after it succeeds
 * is in the mutation's own callbacks, which run for every call (unlike those of `mutate`; TanStack Query, "Mutations").
 */
export function useRetryJob(onRetried: (id: string) => void): RetryJob {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const toast = useToast();
  const options = retryJobMutation(api);
  const mutation = useMutation({
    ...options,
    onSuccess: (_, id) => {
      onRetried(id);
      const job = cacheRetriedJob(api, queryClient, id);
      announce(t("client:admin.jobs.retried", { type: job?.type ?? id }));
    },
    // The job stays a dead job, and its row and focus stay (web-client/README.md E2).
    onError: (error) => {
      toast.show({ message: t("client:admin.jobs.retry_failed", { reason: errorMessage(error) }), tone: "danger" });
    },
  });
  const pendingIds = useMutationState({
    filters: { mutationKey: options.mutationKey, status: "pending" },
    select: (pending) => pending.state.variables as string,
  });
  return {
    retry: (job) => {
      mutation.mutate(job.id);
    },
    pendingIds,
  };
}
