// SPDX-License-Identifier: AGPL-3.0-only
// What moderators did to the viewer's own account, and appealing it: the actions on their account and posts, newest
// first, each with its reason, whether it can be appealed now, and the appeal and its outcome; and the appeal of a post
// that a server hook refused. A write puts what the server answered into the cache. What the server refuses of an
// appeal is told by its kind (`appealRefusalOf`), and a refused post that can be appealed by its decision
// (`hookRefusalOf`).
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQueries, useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type Page, type PagedList, type Pages } from "./paged";
import { useAppData } from "./provider";
import { useShadow } from "./shadow";
import type { Status } from "./statuses";
import { statusQuery } from "./thread";

/** An action of the moderators on the viewer's account or posts. */
export type ModerationAction = components["schemas"]["ModerationAction"];

/** The viewer's appeal of an action, and what the moderators decided. */
export type ModerationAppeal = components["schemas"]["ModerationAppeal"];

/** The viewer's appeal of the refusal of a post by a hook of the server. */
export type HookAppeal = components["schemas"]["OwnHookAppeal"];

/** What the viewer writes to appeal the refusal of a post: the post they tried, and why it should be let through. */
export type HookAppealRequest = components["schemas"]["AppealHookDecisionRequest"];

/** The most characters of what the viewer writes to appeal an action. */
export const appealLimits = { text: 2000 } as const;

/**
 * Why the server refused an appeal: it was made already (`appealed`), the time to appeal has passed (`closed`), what
 * it appeals cannot be appealed (`not_appealable`: a ban whose account is deleted, a decision that is no refusal in
 * force), or what it appeals is not the viewer's or no longer kept (`gone`).
 */
export type AppealRefusal = "appealed" | "closed" | "not_appealable" | "gone";

/** The kind of the server's refusal of an appeal that `error` is; undefined for any other failure. */
export function appealRefusalOf(error: unknown): AppealRefusal | undefined {
  if (!(error instanceof ProblemError)) return undefined;
  if (error.type === "/api/v1/problems/already-appealed") return "appealed";
  if (error.type === "/api/v1/problems/appeal-window-closed") return "closed";
  if (error.type === "/api/v1/problems/not-appealable") return "not_appealable";
  return error.status === 404 ? "gone" : undefined;
}

/** A post the server's automated review refused, as its author may appeal it. */
export type HookRefusal = {
  /** The decision to appeal (`useHookAppeal`, `appealHookDecision`). */
  readonly decision: string;
  /** The last moment it can be appealed, where the server says it. */
  readonly deadline: string | undefined;
};

/**
 * The refusal of a post by the server's automated review that `error` is, when its author may appeal it (the 422
 * `/api/v1/problems/status-rejected` that names its decision); undefined for any other failure, a refused edit among
 * them, which names none.
 */
export function hookRefusalOf(error: unknown): HookRefusal | undefined {
  if (!(error instanceof ProblemError) || error.type !== "/api/v1/problems/status-rejected") return undefined;
  const { hook_decision_id: decision, appeal_deadline: deadline } = error.problem;
  if (typeof decision !== "string" || decision === "") return undefined;
  return { decision, deadline: typeof deadline === "string" && deadline !== "" ? deadline : undefined };
}

/** The actions on the viewer's account, newest first, a page at a time; only those in effect now with `active`. */
export function ownModerationActionsQuery({ api, queries }: AppData, active = false) {
  const init = { params: { query: active ? { active: true } : {} } };
  return pagedQuery<ModerationAction>(
    [...queries.queryOptions("get", "/moderation/actions", init).queryKey, active ? "active" : "all"],
    async (cursor, signal) => {
      const query = cursor === undefined ? init.params.query : { ...init.params.query, cursor };
      const { data } = await api.GET("/moderation/actions", { params: { query }, signal });
      return data;
    },
  );
}

/** The actions on the viewer's account, for a screen. */
export function useOwnModerationActions(active = false): PagedList<ModerationAction> {
  return usePagedList(ownModerationActionsQuery(useAppData(), active));
}

/**
 * The action of `id` on the viewer's account. The server reads no action alone, so it is found in the list of them
 * all, read a page after another until one holds it: `pending` until it is found or the list is read whole, `missing`
 * when the whole list does not hold it, `error` when a page could not be read (`retry` reads it again). `reread`
 * reads the list anew, for an action the server says is no longer as the cache has it.
 */
export function useOwnModerationAction(id: string): {
  readonly action: ModerationAction | undefined;
  readonly status: "pending" | "error" | "missing" | "success";
  readonly retry: () => void;
  readonly reread: () => void;
} {
  const data = useAppData();
  const list = useOwnModerationActions();
  const action = list.items.find((a) => a.id === id);
  const failed = list.status === "error" || list.error !== null;
  const more = action === undefined && list.status === "success" && !failed && list.hasNextPage;
  const { fetchNextPage, isFetchingNextPage } = list;
  useEffect(() => {
    if (more && !isFetchingNextPage) fetchNextPage();
  });
  return {
    action,
    status:
      action !== undefined ? "success" : failed ? "error" : list.status === "pending" || more ? "pending" : "missing",
    retry: list.status === "error" ? list.refetch : list.fetchNextPage,
    reread: () => {
      // Every list of the actions: the ones in effect too, which the settings count from.
      void data.queryClient.invalidateQueries({ queryKey: ownModerationActionsQuery(data).queryKey.slice(0, 2) });
    },
  };
}

/** Whether `error` is the server's answer that a post is not there for the viewer: not found, or gone. */
const postGone = (error: unknown): boolean =>
  error instanceof ProblemError && (error.status === 404 || error.status === 410);

/**
 * The posts an action is about (its `status_ids`) as the viewer may open them now: those the server still shows them,
 * in the action's order; how many it no longer does (`gone`: the server answered 404 or 410, or the viewer deleted
 * the post here: deleted, by the action or since); and how many could not be read for any other reason (`failed`:
 * no answer, an error of the server), which are not known to be gone and are counted apart. `pending` until the
 * server answered for every one, both counts 0 until then.
 */
export function useActionStatuses(ids: readonly string[]): {
  readonly statuses: readonly Status[];
  readonly gone: number;
  readonly failed: number;
  readonly pending: boolean;
} {
  const data = useAppData();
  const show = useShadow();
  const reads = useQueries({ queries: ids.map((id) => ({ ...statusQuery(data, id), retry: false })) });
  const statuses: Status[] = [];
  let gone = 0;
  let failed = 0;
  for (const read of reads) {
    const shown = read.data === undefined ? undefined : show(read.data);
    if (shown !== undefined) statuses.push(shown);
    else if (read.data !== undefined || postGone(read.error)) gone += 1;
    else if (read.isError) failed += 1;
  }
  const pending = reads.some((read) => read.isPending);
  return { statuses, gone: pending ? 0 : gone, failed: pending ? 0 : failed, pending };
}

/**
 * Appeals the action of `id` with `text` (why the moderators should reconsider it), and resolves with the action as
 * the server answered, which carries the appeal, once the cache holds it in the lists. It rejects with the server's
 * problem (an action that cannot be appealed now); when the server says the action can no longer be appealed (the time
 * has passed, or it is not appealable), the cached lists hold it so from then on, whatever they said before.
 */
export async function appealModerationAction(data: AppData, id: string, text: string): Promise<ModerationAction> {
  /** Puts what `change` makes of the action of `id` in every cached list of the actions. */
  const cache = (change: (old: ModerationAction) => ModerationAction) => {
    const base = ownModerationActionsQuery(data).queryKey;
    data.queryClient.setQueriesData<Pages<ModerationAction>>({ queryKey: base.slice(0, 2) }, (now) =>
      now === undefined
        ? now
        : {
            ...now,
            pages: now.pages.map((p): Page<ModerationAction> => ({
              ...p,
              items: p.items.map((a) => (a.id === id ? change(a) : a)),
            })),
          },
    );
  };
  try {
    const { data: action } = await data.api.POST("/moderation/actions/{id}/appeal", {
      params: { path: { id } },
      body: { text },
    });
    if (action === undefined) throw new Error("app-data: the appeal came without an answer");
    cache(() => action);
    return action;
  } catch (error) {
    const refusal = appealRefusalOf(error);
    if (refusal === "closed" || refusal === "not_appealable") cache((old) => ({ ...old, appealable: false }));
    throw error;
  }
}

/** The appeal of the hook decision of `id`: the refusal's `hook_decision_id`. */
export function hookAppealQuery(data: AppData, id: string) {
  return {
    queryKey: ["get", "/hook_decisions/{id}/appeal", { id }] as const,
    queryFn: async ({ signal }: { signal: AbortSignal }): Promise<HookAppeal | null> => {
      try {
        const { data: appeal } = await data.api.GET("/hook_decisions/{id}/appeal", {
          params: { path: { id } },
          signal,
        });
        return appeal ?? null;
      } catch (error) {
        // The viewer has not appealed it.
        if (error instanceof ProblemError && error.status === 404) return null;
        throw error;
      }
    },
  };
}

/**
 * The viewer's appeal of the hook decision of `id`: null when they made none, undefined until the server answered,
 * when it could not be read, and while there is no decision to ask about (`enabled` false).
 */
export function useHookAppeal(id: string, enabled = true): HookAppeal | null | undefined {
  return useQuery({ ...hookAppealQuery(useAppData(), id), enabled }).data;
}

/**
 * Appeals the refusal of the hook decision of `id`, and resolves with the appeal once the cache holds it. It rejects
 * with the server's problem (an appeal already made, a decision that is not the viewer's).
 */
export async function appealHookDecision(data: AppData, id: string, request: HookAppealRequest): Promise<HookAppeal> {
  const { data: appeal } = await data.api.POST("/hook_decisions/{id}/appeal", {
    params: { path: { id } },
    body: request,
  });
  if (appeal === undefined) throw new Error("app-data: the appeal came without an answer");
  data.queryClient.setQueryData(hookAppealQuery(data, id).queryKey, appeal);
  return appeal;
}

/** Appeals an action and a hook's refusal, from a screen (`appealModerationAction`, `appealHookDecision`). */
export function useAppeals(): {
  readonly action: (id: string, text: string) => Promise<ModerationAction>;
  readonly hook: (id: string, request: HookAppealRequest) => Promise<HookAppeal>;
} {
  const data = useAppData();
  return {
    action: (id, text) => appealModerationAction(data, id, text),
    hook: (id, request) => appealHookDecision(data, id, request),
  };
}
