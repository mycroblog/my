// SPDX-License-Identifier: AGPL-3.0-only
// What moderators did to the viewer's own account, and appealing it: the actions on their account and posts, newest
// first, each with its reason, whether it can be appealed now, and the appeal and its outcome; and the appeal of a post
// that a server hook refused. A write puts what the server answered into the cache.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type Page, type PagedList, type Pages } from "./paged";
import { useAppData } from "./provider";

/** An action of the moderators on the viewer's account or posts. */
export type ModerationAction = components["schemas"]["ModerationAction"];

/** The viewer's appeal of an action, and what the moderators decided. */
export type ModerationAppeal = components["schemas"]["ModerationAppeal"];

/** The viewer's appeal of the refusal of a post by a hook of the server. */
export type HookAppeal = components["schemas"]["OwnHookAppeal"];

/** What the viewer writes to appeal the refusal of a post: the post they tried, and why it should be let through. */
export type HookAppealRequest = components["schemas"]["AppealHookDecisionRequest"];

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
 * Appeals the action of `id` with `text` (why the moderators should reconsider it), and resolves with the action as
 * the server answered, which carries the appeal, once the cache holds it in the lists. It rejects with the server's
 * problem (an action that cannot be appealed now).
 */
export async function appealModerationAction(data: AppData, id: string, text: string): Promise<ModerationAction> {
  const { data: action } = await data.api.POST("/moderation/actions/{id}/appeal", {
    params: { path: { id } },
    body: { text },
  });
  if (action === undefined) throw new Error("app-data: the appeal came without an answer");
  const base = ownModerationActionsQuery(data).queryKey;
  data.queryClient.setQueriesData<Pages<ModerationAction>>({ queryKey: base.slice(0, 2) }, (now) =>
    now === undefined
      ? now
      : {
          ...now,
          pages: now.pages.map((p): Page<ModerationAction> => ({
            ...p,
            items: p.items.map((a) => (a.id === action.id ? action : a)),
          })),
        },
  );
  return action;
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

/** The viewer's appeal of the hook decision of `id`: null when they made none, undefined until the server answered. */
export function useHookAppeal(id: string): HookAppeal | null | undefined {
  return useQuery(hookAppealQuery(useAppData(), id)).data;
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
