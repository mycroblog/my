// SPDX-License-Identifier: AGPL-3.0-only
// The server's rules (FR-705, manage_rules): the rules in effect in order; the text and the hint of a new or
// changed rule, written once in any language and shown as they are in every language; moving a rule up or down
// among the others; and retiring one, which reports keep citing. Every change goes to the audit log.
import { useMutation, useMutationState, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
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
export function useRuleDraft(rule?: Rule) {
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
    /** Empties the fields, for the next rule. */
    reset: () => {
      setText("");
      setHint("");
      setSubmitted(false);
    },
  };
}

/** The form of a new rule: it is added after the others, and the form empties once the list shows it. */
export function useNewRuleForm() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const draft = useRuleDraft();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const create = useMutation({
    ...createRuleMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async () => {
      await refreshRules(queryClient);
      draft.reset();
      setStatus(t("client:admin.rules.added"));
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
    /** Why adding failed, localized; the draft stays. */
    error: create.isError ? errorMessage(create.error) : undefined,
    status,
  };
}

/** What the list of rules does to a rule. */
export type RuleActions = {
  /** Replaces the rule's text and hint; resolves whether that worked (the dialog stays open with the error if not). */
  update: (rule: Rule, input: RuleInput) => Promise<boolean>;
  /** Moves the rule to a place among the rules in effect, from 1. */
  move: (rule: Rule, position: number) => void;
  /** Retires the rule; resolves whether that worked. */
  retire: (rule: Rule) => Promise<boolean>;
  /** The IDs of the rules being moved. */
  movingIds: readonly string[];
  /** Why the last change failed, localized. */
  error: string | undefined;
};

/**
 * The changes of the rules in the list. Each says what it did through `onDone(rule, status)` once the list shows
 * it (the rules are fetched again first), so that the screen can move focus and keep the status next to the list.
 */
export function useRuleActions(
  onDone: (rule: Rule, status: string, change: "updated" | "moved" | "retired") => void,
): RuleActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | undefined>(undefined);
  const nameOf = (rule: Rule) => rule.text;
  const update = useMutation({ ...updateRuleMutation(api) });
  const retire = useMutation({ ...retireRuleMutation(api) });
  const moveOptions = moveRuleMutation(api);
  const move = useMutation({
    ...moveOptions,
    onMutate: () => {
      setError(undefined);
    },
    onSuccess: async (rule, { position }) => {
      await refreshRules(queryClient);
      onDone(rule, t("client:admin.rules.moved", { rule: nameOf(rule), position: String(position) }), "moved");
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
    update: async (rule, input) => {
      setError(undefined);
      try {
        const saved = await update.mutateAsync({ id: rule.id, input });
        await refreshRules(queryClient);
        onDone(saved, t("client:admin.rules.updated", { rule: nameOf(saved) }), "updated");
        return true;
      } catch (failure) {
        setError(errorMessage(failure));
        return false;
      }
    },
    move: (rule, position) => {
      move.mutate({ id: rule.id, position });
    },
    retire: async (rule) => {
      setError(undefined);
      try {
        const retired = await retire.mutateAsync(rule.id);
        // The rule leaves the list once it is fetched again: focus moves before that (useFocusAfterRemoval).
        onDone(retired, t("client:admin.rules.retired", { rule: nameOf(retired) }), "retired");
        await refreshRules(queryClient);
        return true;
      } catch (failure) {
        setError(errorMessage(failure));
        return false;
      }
    },
    movingIds,
    error,
  };
}
