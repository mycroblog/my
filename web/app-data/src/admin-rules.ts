// SPDX-License-Identifier: AGPL-3.0-only
// The instance's rules as a holder of `manage_rules` keeps them (FR-705, ADR-0058): the rules in effect by position and
// the retired ones after them; writing a new rule, revising one's text and hint, moving one among the rules in effect,
// and retiring one, at once and for good (reports that cite it keep citing it). Every write reads the list again,
// whose positions it changes, and the instance, which lists the rules in effect.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { instanceQuery } from "./instance";
import { useAppData } from "./provider";
import { readOf, type ScreenRead } from "./reads";

/** A rule of the instance, in effect or retired. */
export type AdminRule = components["schemas"]["AdminRule"];
/** The text and the hint of a rule. */
export type AdminRuleInput = components["schemas"]["InstanceRuleInput"];

/** The most characters of a rule's text and of its hint, and the most rules in effect. */
export const ruleLimits = { text: 300, hint: 1000, inEffect: 100 } as const;

/** Every rule, the retired ones after the rules in effect. */
export function adminRulesQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/rules", { params: { query: { include_retired: true } } });
}

/** Reads the rules, for a route's loader. */
export async function loadAdminRules(data: AppData): Promise<void> {
  await data.queryClient.query({ ...adminRulesQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The rules in effect by position, the retired ones newest first, and where their read is. */
export function useAdminRules(): ScreenRead & {
  readonly inEffect: readonly AdminRule[] | undefined;
  readonly retired: readonly AdminRule[] | undefined;
} {
  const query = useQuery(adminRulesQuery(useAppData()));
  const items = query.data?.items;
  return {
    ...readOf([query]),
    inEffect: items?.filter((r) => r.retired_at === undefined),
    retired: items
      ?.filter((r) => r.retired_at !== undefined)
      .toSorted((a, b) => (b.retired_at ?? "").localeCompare(a.retired_at ?? "")),
  };
}

/** The rule of `id`, found in the list; undefined until it is read, or for none. */
export function useAdminRule(id: string): ScreenRead & { readonly rule: AdminRule | undefined; missing: boolean } {
  const query = useQuery(adminRulesQuery(useAppData()));
  const rule = query.data?.items.find((r) => r.id === id);
  return { ...readOf([query]), rule, missing: query.data !== undefined && rule === undefined };
}

/** The list and the instance are read again: a write changes the positions and the rules in effect. */
async function rulesChanged(data: AppData): Promise<void> {
  void data.queryClient.invalidateQueries({ queryKey: instanceQuery(data).queryKey });
  await data.queryClient.invalidateQueries({ queryKey: adminRulesQuery(data).queryKey });
}

/** The input the API takes: an empty hint is none. */
const inputOf = ({ text, hint }: AdminRuleInput): AdminRuleInput => ({
  text,
  ...(hint === undefined || hint === "" ? {} : { hint }),
});

/** The writes of the rules; each rejects with the server's problem (a 422 names `text`, `hint` or `rules`). */
export type AdminRuleWrites = {
  /** Adds a rule after the rules in effect, and resolves with it. */
  readonly create: (input: AdminRuleInput) => Promise<AdminRule>;
  /** Replaces the text and the hint of the rule of `id`. */
  readonly revise: (id: string, input: AdminRuleInput) => Promise<AdminRule>;
  /** Moves the rule of `id` to `position` among the rules in effect, from 1. */
  readonly move: (id: string, position: number) => Promise<AdminRule>;
  /** Retires the rule of `id`, for good. */
  readonly retire: (id: string) => Promise<AdminRule>;
};

/** What a rule's write answered, once the list was read again. */
async function written(data: AppData, rule: AdminRule | undefined): Promise<AdminRule> {
  if (rule === undefined) throw new Error("app-data: a write of a rule answered without the rule");
  await rulesChanged(data);
  return rule;
}

/** The writes of the rules, for a screen. */
export function useAdminRuleWrites(): AdminRuleWrites {
  const data = useAppData();
  const path = (id: string) => ({ params: { path: { id } } });
  return {
    create: async (input) => written(data, (await data.api.POST("/admin/rules", { body: inputOf(input) })).data),
    revise: async (id, input) =>
      written(data, (await data.api.PUT("/admin/rules/{id}", { ...path(id), body: inputOf(input) })).data),
    move: async (id, position) =>
      written(data, (await data.api.POST("/admin/rules/{id}/move", { ...path(id), body: { position } })).data),
    retire: async (id) => written(data, (await data.api.POST("/admin/rules/{id}/retire", path(id))).data),
  };
}
