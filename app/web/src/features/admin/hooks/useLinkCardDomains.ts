// SPDX-License-Identifier: AGPL-3.0-only
// The rules of the link card crawler on the admin screens (manage_settings): the list, the form that sets the rule of a
// new domain (which then opens the rule's page), the form that changes the mode and the note of a rule on its page, and
// removing one (which leaves its page). A refusal of the new rule stays next to the domain; the result of a change
// stays next to its save button.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { useLoadFailure } from "../../../lib/loadFailure";
import { shellQueryRetry } from "../../../lib/query";
import { domainInText } from "../../moderation";
import {
  type LinkCardDomainMode,
  type LinkCardDomainRule,
  linkCardDomainsQueryOptions,
  refreshLinkCardDomainRules,
  removeLinkCardDomainRuleMutation,
  setLinkCardDomainRuleMutation,
} from "../api/linkCardDomains";

/** The rules as the screen shows them. */
export type LinkCardDomainRules =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | { status: "ready"; rules: LinkCardDomainRule[] };

/** The rules, by domain. A failure of the first load stays on screen while it loads again. */
export function useLinkCardDomainRules(): LinkCardDomainRules {
  const api = useApi();
  const query = useQuery({ ...linkCardDomainsQueryOptions(api), retry: shellQueryRetry });
  const failure = useLoadFailure(query);
  if (query.data !== undefined) return { status: "ready", rules: query.data.items };
  if (failure.error !== null) {
    return {
      status: "error",
      reason: errorMessage(failure.error),
      retry: () => void query.refetch(),
      isRetrying: failure.isRetrying,
    };
  }
  return { status: "loading" };
}

/** A rule as the form edits it. */
export type LinkCardDomainDraft = { domain: string; mode: LinkCardDomainMode; note: string };

const emptyDraft: LinkCardDomainDraft = { domain: "", mode: "no_images", note: "" };

/** Why the form failed: on the domain when the server refused the rule (422), else under the form. */
export type LinkCardDomainFormError = { field: "domain" | "form"; message: string };

/** The rule of a domain in the list, or undefined when the domain has none. */
export function findRule(rules: readonly LinkCardDomainRule[], domain: string): LinkCardDomainRule | undefined {
  const wanted = domain.toLowerCase();
  return rules.find((rule) => rule.domain.toLowerCase() === wanted);
}

/** The rules whose domain holds `search` (any case), or every rule without one. */
export function searchRules(rules: readonly LinkCardDomainRule[], search: string | undefined): LinkCardDomainRule[] {
  if (search === undefined) return [...rules];
  const wanted = search.toLowerCase();
  return rules.filter((rule) => rule.domain.toLowerCase().includes(wanted));
}

/**
 * The form of a new rule. A success calls `onSaved` with the rule once the list has it, to open its page (the page's
 * heading is the result, web-client/16-look-states-and-wording.md G24).
 */
export function useLinkCardDomainForm(onSaved: (rule: LinkCardDomainRule) => void) {
  const api = useApi();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<LinkCardDomainDraft>(emptyDraft);
  const mutation = useMutation({
    ...setLinkCardDomainRuleMutation(api),
    onSuccess: async (rule) => {
      await refreshLinkCardDomainRules(queryClient);
      onSaved(rule);
    },
  });
  const error = (): LinkCardDomainFormError | undefined => {
    if (!mutation.isError) return undefined;
    const refused = mutation.error instanceof ApiError && mutation.error.status === 422;
    return { field: refused ? "domain" : "form", message: errorMessage(mutation.error) };
  };
  return {
    draft,
    change: (change: Partial<LinkCardDomainDraft>) => {
      setDraft((previous) => ({ ...previous, ...change }));
    },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (mutation.isPending) return;
      const note = draft.note.trim();
      mutation.mutate({ domain: draft.domain.trim(), mode: draft.mode, ...(note === "" ? {} : { note }) });
    },
    isPending: mutation.isPending,
    /** Why saving failed; the draft stays. */
    error: error(),
  };
}

/** The mode and the note of a rule as its page edits them. */
export type LinkCardDomainChange = { mode: LinkCardDomainMode; note: string };

/**
 * The form of a rule on its page: its mode and note, saved together (G24, FormActionBar). The result of the last save
 * stays next to its button until the form changes.
 */
export function useLinkCardDomainEdit(rule: LinkCardDomainRule) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<LinkCardDomainChange>({ mode: rule.mode, note: rule.note });
  const mutation = useMutation({
    ...setLinkCardDomainRuleMutation(api),
    onSuccess: () => refreshLinkCardDomainRules(queryClient),
  });
  return {
    draft,
    change: (change: Partial<LinkCardDomainChange>) => {
      mutation.reset();
      setDraft((previous) => ({ ...previous, ...change }));
    },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (mutation.isPending) return;
      mutation.mutate({ domain: rule.domain, mode: draft.mode, note: draft.note.trim() });
    },
    isPending: mutation.isPending,
    /** The result of the last save, for the FormActionBar. */
    result: mutation.isError
      ? {
          tone: "danger" as const,
          message: t("client:admin.link_card_domains.save_failed", { reason: errorMessage(mutation.error) }),
        }
      : mutation.isSuccess
        ? {
            tone: "success" as const,
            message: t("client:admin.link_card_domains.saved", { domain: domainInText(t, rule.domain) }),
          }
        : undefined,
  };
}

export type LinkCardDomainRemoval = {
  /** Removes the rule; resolves with whether it succeeded. */
  remove: (rule: LinkCardDomainRule) => Promise<boolean>;
  /** Why the last removal failed, localized, until the next one. */
  error: string | undefined;
};

/**
 * Removing a rule from its page. `onRemoved(rule)` is called before the list is fetched again, so that the page can
 * leave for the list and say what happened there (a toast: the page that would show it is gone).
 */
export function useLinkCardDomainRemoval(onRemoved: (rule: LinkCardDomainRule) => void): LinkCardDomainRemoval {
  const api = useApi();
  const queryClient = useQueryClient();
  const mutation = useMutation(removeLinkCardDomainRuleMutation(api));
  return {
    remove: async (rule) => {
      mutation.reset();
      try {
        await mutation.mutateAsync(rule.domain);
      } catch {
        return false;
      }
      onRemoved(rule);
      await refreshLinkCardDomainRules(queryClient);
      return true;
    },
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
  };
}
