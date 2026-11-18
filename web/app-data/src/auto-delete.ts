// SPDX-License-Identifier: AGPL-3.0-only
// The viewer's policy that deletes their old posts by itself, and the server's maximum over it: reading it, saving it,
// and an estimate of what a policy that is not yet saved would delete. The server keeps the policy; a save puts what
// it answered, with the maximum's effect, into the cache.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
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

/** The viewer's policy, with the server's maximum when it sets one. */
export function autoDeleteQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/preferences/auto_delete");
}

/** The viewer's policy, undefined until the server answered. */
export function useAutoDelete(): AutoDeleteSettings | undefined {
  return useQuery(autoDeleteQuery(useAppData())).data;
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
