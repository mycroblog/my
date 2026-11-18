// SPDX-License-Identifier: AGPL-3.0-only
// The viewer's policy that deletes their old posts by itself, and the server's maximum over it: reading it, saving it,
// and an estimate of what a policy that is not yet saved would delete. The server keeps the policy; a save puts what
// it answered, with the maximum's effect, into the cache.
import type { components } from "@mycroblog/mykit";
import { skipToken, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** What the viewer chooses: whether old posts are deleted, after how long, and which are kept. */
export type AutoDeletePolicy = components["schemas"]["AutoDeletePolicy"];

/** The policy as saved, and the maximum the server sets over it with what it makes of the policy. */
export type AutoDeleteSettings = components["schemas"]["AutoDeleteSettings"];

/** What the server's maximum is, and what it makes of the viewer's policy. */
export type AutoDeleteServerLimit = components["schemas"]["AutoDeleteServerLimit"];

/** An estimate of what a policy would delete now. */
export type AutoDeletePreview = components["schemas"]["AutoDeletePreview"];

/** What the server takes of a policy: the days a post is kept, and the boosts or favourites that keep one. */
export const autoDeleteLimits = { minDays: 1, maxDays: 3650, maxCount: 1_000_000 } as const;

/** The viewer's policy, with the server's maximum when it sets one. */
export function autoDeleteQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/preferences/auto_delete");
}

/** The viewer's policy, undefined until the server answered. */
export function useAutoDelete(): AutoDeleteSettings | undefined {
  return useQuery(autoDeleteQuery(useAppData())).data;
}

/** The viewer's policy, undefined until the server answered, and `retry`, which reads it again after a failure. */
export function useAutoDeleteRead(): {
  settings: AutoDeleteSettings | undefined;
  status: "pending" | "error" | "success";
  retry: () => void;
} {
  const query = useQuery(autoDeleteQuery(useAppData()));
  return {
    settings: query.data,
    status: query.status,
    retry: () => {
      void query.refetch();
    },
  };
}

/**
 * Saves `policy`, and resolves with the policy as saved once the cache holds it. A save is no preview: the posts it
 * deletes go on the server's schedule. It rejects with the server's problem (a policy beyond the maximum).
 */
export async function saveAutoDelete(data: AppData, policy: AutoDeletePolicy): Promise<AutoDeleteSettings> {
  const { data: saved } = await data.api.PUT("/preferences/auto_delete", { body: policy });
  if (saved === undefined) throw new Error("app-data: the policy was saved without an answer");
  data.queryClient.setQueryData(autoDeleteQuery(data).queryKey, saved);
  return saved;
}

/**
 * How many posts `policy` would delete now, and up to 3 of them, oldest first; nothing changes on the server. It
 * rejects with the server's problem.
 */
export async function previewAutoDelete(
  data: AppData,
  policy: AutoDeletePolicy,
  signal?: AbortSignal,
): Promise<AutoDeletePreview> {
  const { data: preview } = await data.api.POST("/preferences/auto_delete/preview", {
    body: policy,
    ...(signal === undefined ? {} : { signal }),
  });
  if (preview === undefined) throw new Error("app-data: the estimate came without an answer");
  return preview;
}

/** How long an estimate is taken as it came: posts grow old meanwhile, a few at most. */
const estimateStaleMs = 60_000;

/** An estimate of a policy as a screen follows it. */
export type AutoDeleteEstimate = {
  /** The estimate of the policy given, undefined until the server answered for it. */
  readonly preview: AutoDeletePreview | undefined;
  /** Where the estimate is: `idle` with no policy to ask about. */
  readonly status: "idle" | "pending" | "error" | "success";
  /** Asks again after a failure. */
  readonly retry: () => void;
};

/**
 * What `policy`, which is not saved, would delete now, asked again whenever it changes; nothing is asked without
 * one. Nothing changes on the server.
 */
export function useAutoDeleteEstimate(policy: AutoDeletePolicy | undefined): AutoDeleteEstimate {
  const data = useAppData();
  const query = useQuery({
    queryKey: ["post", "/preferences/auto_delete/preview", policy] as const,
    queryFn: policy === undefined ? skipToken : ({ signal }) => previewAutoDelete(data, policy, signal),
    staleTime: estimateStaleMs,
    retry: false,
  });
  return {
    preview: policy === undefined ? undefined : query.data,
    status: policy === undefined ? "idle" : query.status,
    retry: () => {
      void query.refetch();
    },
  };
}

/** Saves the policy and estimates one, from a screen (`saveAutoDelete`, `previewAutoDelete`). */
export function useAutoDeleteWrites(): {
  readonly save: (policy: AutoDeletePolicy) => Promise<AutoDeleteSettings>;
  readonly preview: (policy: AutoDeletePolicy, signal?: AbortSignal) => Promise<AutoDeletePreview>;
} {
  const data = useAppData();
  return {
    save: (policy) => saveAutoDelete(data, policy),
    preview: (policy, signal) => previewAutoDelete(data, policy, signal),
  };
}
