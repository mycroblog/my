// SPDX-License-Identifier: AGPL-3.0-only
// The automated reviews as a holder of `manage_reports` reads them (ADR-0147, ADR-0058): the decision hooks with their
// mode and the counts and waits of their decisions of the last 7 days; their decisions, newest first, filtered by hook,
// mode and outcome, each cancelled where it can be; and the authors' appeals of a post `local.status` refused, upheld
// or overturned. Every write reads again the decisions and the appeals.
import type { components } from "@mycroblog/mykit";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { oneOf, pageOf, staffChanged, type StaffOne, type StaffPage } from "./staff-reads";

/** What a decision hook answered about one subject. */
export type HookDecision = components["schemas"]["AdminHookDecision"];
/** A decision hook. */
export type HookName = components["schemas"]["HookName"];
/** The mode a decision was made in. */
export type HookDecisionMode = components["schemas"]["HookDecisionMode"];
/** What a decision records: a verdict, or a kind of failure. */
export type HookOutcome = components["schemas"]["HookOutcome"];
/** The coded reason of a verdict. */
export type HookReason = components["schemas"]["HookReason"];
/** The hooks with their settings and their decisions of the period. */
export type HookSummary = components["schemas"]["AdminHookSummary"];
/** An author's appeal of a post `local.status` refused, with the text they tried to post. */
export type ModeratedHookAppeal = components["schemas"]["AdminHookAppeal"];
/** Where an appeal of a refused post stands. */
export type HookAppealState = components["schemas"]["HookAppealState"];

/** The hooks, in the server's order. */
export const hookNames = [
  "inbound.status",
  "inbound.follow",
  "signup.check",
  "media.check",
  "local.status",
  "inbound.actor",
  "local.profile",
] as const satisfies readonly HookName[];
/** The modes a decision is made in. */
export const hookDecisionModes = ["observe", "enforce"] as const satisfies readonly HookDecisionMode[];
/** The outcomes of a decision, the verdicts first. */
export const hookOutcomes = [
  "allow",
  "drop",
  "flag",
  "mark_sensitive",
  "require_approval",
  "reject",
  "timeout",
  "error",
  "invalid",
  "breaker_open",
] as const satisfies readonly HookOutcome[];
/** The states of an appeal of a refused post, pending first. */
export const hookAppealStates = ["pending", "upheld", "overturned"] as const satisfies readonly HookAppealState[];

/** What the list of decisions shows: one hook, one mode and one outcome, each every one where left out. */
export type HookDecisionQuery = {
  readonly hook?: HookName | undefined;
  readonly mode?: HookDecisionMode | undefined;
  readonly outcome?: HookOutcome | undefined;
  readonly cursor?: string | undefined;
};

/** A page of the decisions, newest first; without an outcome, `allow` is left out. */
function hookDecisionsQuery({ queries }: AppData, q: HookDecisionQuery) {
  return queries.queryOptions("get", "/admin/hook_decisions", {
    params: {
      query: {
        ...(q.hook === undefined ? {} : { hook: q.hook }),
        ...(q.mode === undefined ? {} : { mode: q.mode }),
        ...(q.outcome === undefined ? {} : { outcome: q.outcome }),
        ...(q.cursor === undefined ? {} : { cursor: q.cursor }),
      },
    },
  });
}

/** The page of the decisions `q` asks for; the page shown before stays while the next is on its way. */
export function useHookDecisions(q: HookDecisionQuery): StaffPage<HookDecision> {
  return pageOf(useQuery({ ...hookDecisionsQuery(useAppData(), q), placeholderData: keepPreviousData }));
}

/** The hooks and their decisions of the last 7 days. */
function hookSummaryQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/hook_decisions/summary");
}

/** The hooks and their decisions of the last 7 days. */
export function useHookSummary(): StaffOne<HookSummary> {
  return oneOf(useQuery(hookSummaryQuery(useAppData())));
}

/** Reads a page of the decisions and the summary, for a route's loader. */
export async function loadHookDecisions(data: AppData, q: HookDecisionQuery): Promise<void> {
  await Promise.all([
    data.queryClient.query(hookDecisionsQuery(data, q)).catch(() => undefined),
    data.queryClient.query(hookSummaryQuery(data)).catch(() => undefined),
  ]);
}

/** One decision. */
function hookDecisionQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/hook_decisions/{id}", { params: { path: { id } } });
}

/** The decision of `id`. */
export function useHookDecision(id: string): StaffOne<HookDecision> {
  return oneOf(useQuery(hookDecisionQuery(useAppData(), id)));
}

/** Reads the decision of `id`, for a route's loader. */
export async function loadHookDecision(data: AppData, id: string): Promise<void> {
  await data.queryClient.query(hookDecisionQuery(data, id)).catch(() => undefined);
}

/** A page of the appeals of refused posts, newest first, of one state (every state where left out). */
function moderatedHookAppealsQuery(
  { queries }: AppData,
  state: HookAppealState | undefined,
  cursor: string | undefined,
) {
  return queries.queryOptions("get", "/admin/hook_appeals", {
    params: { query: { ...(state === undefined ? {} : { state }), ...(cursor === undefined ? {} : { cursor }) } },
  });
}

/** The page after `cursor` of the appeals of refused posts in `state`. */
export function useModeratedHookAppeals(
  state: HookAppealState | undefined,
  cursor: string | undefined,
): StaffPage<ModeratedHookAppeal> {
  return pageOf(
    useQuery({ ...moderatedHookAppealsQuery(useAppData(), state, cursor), placeholderData: keepPreviousData }),
  );
}

/** Reads a page of the appeals of refused posts, for a route's loader. */
export async function loadModeratedHookAppeals(
  data: AppData,
  state: HookAppealState | undefined,
  cursor: string | undefined,
): Promise<void> {
  await data.queryClient.query(moderatedHookAppealsQuery(data, state, cursor)).catch(() => undefined);
}

/** One appeal of a refused post. */
function moderatedHookAppealQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/hook_appeals/{id}", { params: { path: { id } } });
}

/** The appeal of a refused post of `id`. */
export function useModeratedHookAppeal(id: string): StaffOne<ModeratedHookAppeal> {
  return oneOf(useQuery(moderatedHookAppealQuery(useAppData(), id)));
}

/** Reads the appeal of a refused post of `id`, for a route's loader. */
export async function loadModeratedHookAppeal(data: AppData, id: string): Promise<void> {
  await data.queryClient.query(moderatedHookAppealQuery(data, id)).catch(() => undefined);
}

/** What a moderator does to the automated reviews' decisions; each rejects with the server's problem. */
export type HookWrites = {
  /** Cancels the decision of `id`, and resolves with it as cancelled. */
  readonly override: (id: string) => Promise<HookDecision>;
  /** Decides the appeal of `id`: `uphold` keeps the refusal, `overturn` cancels it; resolves with the appeal. */
  readonly decide: (id: string, decision: "uphold" | "overturn") => Promise<ModeratedHookAppeal>;
};

/** What a moderator does to the automated reviews' decisions, for a screen. */
export function useHookWrites(): HookWrites {
  const data = useAppData();
  const changed = () => {
    staffChanged(data, ["/admin/hook_decisions", "/admin/hook_appeals"]);
  };
  return {
    override: async (id) => {
      const { data: answer } = await data.api.POST("/admin/hook_decisions/{id}/override", {
        params: { path: { id } },
      });
      if (answer === undefined) throw new Error("app-data: a cancellation answered without the decision");
      await data.queryClient.cancelQueries({ queryKey: hookDecisionQuery(data, id).queryKey });
      data.queryClient.setQueryData(hookDecisionQuery(data, id).queryKey, answer);
      changed();
      return answer;
    },
    decide: async (id, decision) => {
      const init = { params: { path: { id } } };
      const { data: answer } =
        decision === "uphold"
          ? await data.api.POST("/admin/hook_appeals/{id}/uphold", init)
          : await data.api.POST("/admin/hook_appeals/{id}/overturn", init);
      if (answer === undefined) throw new Error("app-data: a decision answered without the appeal");
      await data.queryClient.cancelQueries({ queryKey: moderatedHookAppealQuery(data, id).queryKey });
      data.queryClient.setQueryData(moderatedHookAppealQuery(data, id).queryKey, answer);
      changed();
      return answer;
    },
  };
}
