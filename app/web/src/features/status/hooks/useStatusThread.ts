// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type Status, statusContextQueryOptions, statusQueryOptions } from "../api/status";

/**
 * A post of a thread, with whether it replies to the post shown above it and whether the post shown below
 * replies to it: the cards draw a line between the two.
 */
export type ThreadPost = { status: Status; lineAbove: boolean; lineBelow: boolean };

/** The thread around the post, which loads after the post and may fail on its own (web-client/README.md E1). */
export type ThreadState =
  | { status: "loading" }
  | { status: "error"; message: string; retry: () => void; isRetrying: boolean }
  | {
      status: "loaded";
      /** The posts the post replies to, oldest first. */
      ancestors: readonly ThreadPost[];
      /** Whether the post replies to the last of the ancestors, which is shown right above it. */
      repliesToAncestor: boolean;
      /** The replies, depth first. */
      descendants: readonly ThreadPost[];
    };

export type StatusThread = {
  /** The post the page is about. */
  status: Status;
  thread: ThreadState;
};

/** The posts in the order shown, each joined to its neighbours where one replies to the other. */
function joined(statuses: readonly Status[]): ThreadPost[] {
  return statuses.map((status, index) => ({
    status,
    lineAbove: index > 0 && status.in_reply_to_id === statuses[index - 1]?.id,
    lineBelow: statuses[index + 1]?.in_reply_to_id === status.id,
  }));
}

/**
 * Reads a post that the route loader has fetched, and the thread around it. The post's failure goes to the
 * route's error boundary; the thread's stays here, so the post stays on screen. Both are read from the
 * query cache on every render, so an action on any post of the thread shows at once (S2).
 */
export function useStatusThread(id: string): StatusThread {
  const api = useApi();
  const { data: status } = useSuspenseQuery(statusQueryOptions(api, id));
  const context = useQuery({
    ...statusContextQueryOptions(api, id),
    // Shown at once with a button to try again, instead of after seconds of silent retries.
    retry: false,
  });
  if (context.isError) {
    return {
      status,
      thread: {
        status: "error",
        message: errorMessage(context.error),
        retry: () => {
          void context.refetch();
        },
        isRetrying: context.isFetching,
      },
    };
  }
  if (context.data === undefined) return { status, thread: { status: "loading" } };
  // The post itself continues the ancestors' line: it is shown right below them.
  const upToPost = joined([...context.data.ancestors, status]);
  return {
    status,
    thread: {
      status: "loaded",
      ancestors: upToPost.slice(0, -1),
      repliesToAncestor: upToPost.at(-1)?.lineAbove ?? false,
      descendants: joined(context.data.descendants),
    },
  };
}
