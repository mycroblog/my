// SPDX-License-Identifier: AGPL-3.0-only
// The server's rules (FR-705, manage_rules): the rules in effect in order and the retired ones; the text and the hint
// of a new or changed rule, written once in any language and shown as they are in every language; moving a rule up
// or down among the others (on the list); and changing or retiring one (on its page), which reports keep citing.
// Every change goes to the audit log.
import { type FormActionResult, useNotice } from "@my/ui";
import { useMutation, useMutationState, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  createRuleMutation,
  moveRuleMutation,
  refreshRules,
  retireRuleMutation,
  type Rule,
  type RuleInput,
  ruleHintLimit,
  ruleQueryOptions,
  ruleTextLimit,
  rulesQueryOptions,
  updateRuleMutation,
} from "../api/rules";
import { characterCount } from "./languages";

/** The rules in effect, in order. */
export function useRuleList(): Rule[] {
  const api = useApi();
  return useSuspenseQuery(rulesQueryOptions(api)).data.items;
}

/** The retired rules; the route's loader has loaded them. */
export function useRetiredRules(): Rule[] {
  const api = useApi();
  return useSuspenseQuery(rulesQueryOptions(api, { includeRetired: true })).data.items.filter(
    (rule) => rule.retired_at !== undefined,
  );
}

/** One rule, and its place among the rules in effect (from 1; undefined once it is retired). */
export function useRule(id: string): { rule: Rule; place: number | undefined } {
  const api = useApi();
  const rule = useSuspenseQuery(ruleQueryOptions(api, id)).data;
  const index = useRuleList().findIndex((other) => other.id === id);
  return { rule, place: rule.retired_at === undefined && index >= 0 ? index + 1 : undefined };
}

/** The address of a rule's page. */
export function rulePath(id: string): string {
  return `/admin/rules/${encodeURIComponent(id)}`;
}

/** The fields of a rule: its text and its hint, as typed. */
export type RuleFields = {
  text: string;
  hint: string;
  changeText: (text: string) => void;
  changeHint: (hint: string) => void;
  /** Why the text cannot be saved, localized; undefined when it can. */
  textError: string | undefined;
  /** Why the hint cannot be saved, localized; undefined when it can. */
  hintError: string | undefined;
};

/** The text and the hint of a rule, from `rule` when it is given. The text is needed; the hint is optional. */
function useRuleDraft(rule?: Rule) {
  const t = useT();
  const [text, setText] = useState(rule?.text ?? "");
  const [hint, setHint] = useState(rule?.hint ?? "");
  // An empty text says why only once the form is sent, not while the field is still empty for the first time.
  const [submitted, setSubmitted] = useState(false);

  const textError = (() => {
    if (characterCount(text) > ruleTextLimit) return t("client:admin.rules.too_long", { max: String(ruleTextLimit) });
    if (text.trim() === "" && (submitted || text !== "")) return t("client:admin.rules.no_text");
    return undefined;
  })();
  const hintError =
    characterCount(hint) > ruleHintLimit ? t("client:admin.rules.too_long", { max: String(ruleHintLimit) }) : undefined;

  const fields: RuleFields = {
    text,
    hint,
    changeText: (next) => {
      setText(next);
    },
    changeHint: (next) => {
      setHint(next);
    },
    textError,
    hintError,
  };

  return {
    fields,
    /** What to send, or null when a field is invalid (it shows why). */
    input: (): RuleInput | null => {
      setSubmitted(true);
      if (text.trim() === "" || textError !== undefined || hintError !== undefined) return null;
      return { text: text.trim(), ...(hint.trim() === "" ? {} : { hint: hint.trim() }) };
    },
  };
}

/**
 * The form of a new rule: it is added after the others, then the rule's page opens (web-client §16 G24, the result
 * is the page it moves to) and a toast says it was added.
 */
export function useNewRuleForm() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const notice = useNotice();
  const draft = useRuleDraft();
  const create = useMutation({
    ...createRuleMutation(api),
    onSuccess: async (rule) => {
      await refreshRules(queryClient);
      await navigate({ to: "/admin/rules/$id", params: { id: rule.id } });
      notice.toast({
        tone: "success",
        message: t("client:admin.rules.added", { rule: rule.text }),
      });
    },
  });
  return {
    draft,
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (create.isPending) return;
      const input = draft.input();
      if (input !== null) create.mutate(input);
    },
    isPending: create.isPending,
    /** Why adding failed, localized, for the form's FormActionBar; the draft stays. */
    result: create.isError
      ? ({
          tone: "danger",
          message: t("client:admin.rules.add_failed", { reason: errorMessage(create.error) }),
        } satisfies FormActionResult)
      : undefined,
  };
}

/** Moving the rules of the list up and down. */
export type RuleMoves = {
  /** Moves the rule to a place among the rules in effect, from 1. */
  move: (rule: Rule, position: number) => void;
  /** The IDs of the rules being moved. */
  movingIds: readonly string[];
  /** Why the last move failed, localized. */
  error: string | undefined;
};

/**
 * Moving a rule of the list. `onMoved(rule, status)` is called once the list shows the new order (the rules are fetched
 * again first), so that the list can move focus to the rule and keep the status next to it.
 */
export function useRuleMoves(onMoved: (rule: Rule, status: string) => void): RuleMoves {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | undefined>(undefined);
  const moveOptions = moveRuleMutation(api);
  const move = useMutation({
    ...moveOptions,
    onMutate: () => {
      setError(undefined);
    },
    onSuccess: async (rule, { position }) => {
      await refreshRules(queryClient);
      onMoved(rule, t("client:admin.rules.moved", { rule: rule.text, position: String(position) }));
    },
    onError: (failure) => {
      setError(errorMessage(failure));
    },
  });
  const movingIds = useMutationState({
    filters: { mutationKey: moveOptions.mutationKey, status: "pending" },
    select: (pending) => (pending.state.variables as { id: string }).id,
  });
  return {
    move: (rule, position) => {
      move.mutate({ id: rule.id, position });
    },
    movingIds,
    error,
  };
}

/** The form of a rule's page: its text and hint, saved with one button, the result beside it (FormActionBar). */
export function useRuleForm(rule: Rule) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const draft = useRuleDraft(rule);
  const [result, setResult] = useState<FormActionResult | undefined>(undefined);
  const update = useMutation({
    ...updateRuleMutation(api),
    onMutate: () => {
      setResult(undefined);
    },
    onSuccess: async (saved) => {
      await refreshRules(queryClient);
      setResult({
        tone: "success",
        message: t("client:admin.rules.updated", { rule: saved.text }),
      });
    },
    onError: (failure) => {
      setResult({
        tone: "danger",
        message: t("client:admin.rules.change_failed", { reason: errorMessage(failure) }),
      });
    },
  });
  return {
    draft,
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (update.isPending) return;
      const input = draft.input();
      if (input !== null) update.mutate({ id: rule.id, input });
    },
    isPending: update.isPending,
    result,
  };
}

/**
 * Retiring a rule from its page. Resolves with why it failed (the alert dialog stays open with it), or undefined once
 * the page shows the rule as retired; `onRetired(status)` is called then, to say so and move focus.
 */
export function useRetireRule(rule: Rule, onRetired: (status: string) => void) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const retire = useMutation(retireRuleMutation(api));
  return {
    retire: async (): Promise<string | undefined> => {
      try {
        const retired = await retire.mutateAsync(rule.id);
        await refreshRules(queryClient);
        onRetired(t("client:admin.rules.retired", { rule: retired.text }));
        return undefined;
      } catch (failure) {
        return t("client:admin.rules.change_failed", { reason: errorMessage(failure) });
      }
    },
    isPending: retire.isPending,
  };
}
