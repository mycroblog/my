// SPDX-License-Identifier: AGPL-3.0-only
// The rules of the link card crawler on the admin screens (manage_settings): the list, the form that sets the rule of a
// domain, changing the mode of a rule in its row, and removing one. What a change did is announced and kept in a
// StatusMessage; a refusal of the form stays next to the domain, and a failure of a row next to its control.
import { useNotice } from "@my/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { useLoadFailure } from "../../../lib/loadFailure";
import { shellQueryRetry } from "../../../lib/query";
import { domainInText } from "../../moderation";
import { saveStateOf } from "../../settings";
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

/** The form of a rule. A success empties it and says which domain the rule is on. */
export function useLinkCardDomainForm() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<LinkCardDomainDraft>(emptyDraft);
  const [saved, setSaved] = useState<LinkCardDomainRule | undefined>(undefined);
  const mutation = useMutation({
    ...setLinkCardDomainRuleMutation(api),
    onMutate: () => {
      setSaved(undefined);
    },
    onSuccess: async (rule) => {
      await refreshLinkCardDomainRules(queryClient);
      setDraft(emptyDraft);
      setSaved(rule);
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
    /** What the last submit did, for the form's StatusMessage. */
    status:
      saved === undefined
        ? undefined
        : t("client:admin.link_card_domains.saved", { domain: domainInText(t, saved.domain) }),
  };
}

/** Changing the mode of a rule in its row, saved as soon as it is chosen (G24). */
export function useLinkCardDomainMode(rule: LinkCardDomainRule) {
  const api = useApi();
  const queryClient = useQueryClient();
  const mutation = useMutation({
    ...setLinkCardDomainRuleMutation(api),
    onSuccess: () => refreshLinkCardDomainRules(queryClient),
  });
  return {
    /** The mode to show: the one on its way, else the rule's. */
    mode: mutation.isPending ? mutation.variables.mode : rule.mode,
    change: (mode: LinkCardDomainMode) => {
      if (mode === rule.mode || mutation.isPending) return;
      mutation.mutate({ domain: rule.domain, mode, note: rule.note });
    },
    saveState: saveStateOf(mutation),
    /** Why the last change failed, until the next one. */
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
  };
}

export type LinkCardDomainRemoval = {
  /** Removes the rule; resolves with whether it succeeded. */
  remove: (rule: LinkCardDomainRule) => Promise<boolean>;
  /** Why the last removal failed, localized, until the next one. */
  error: string | undefined;
  /** What the last removal did, for the list's StatusMessage. */
  status: string | undefined;
};

/** Removing rules. `onRemoved(domain)` is called once a rule is about to leave the list (lib/useFocusAfterRemoval). */
export function useLinkCardDomainRemoval(onRemoved: (domain: string) => void): LinkCardDomainRemoval {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation(removeLinkCardDomainRuleMutation(api));
  return {
    remove: async (rule) => {
      mutation.reset();
      try {
        await mutation.mutateAsync(rule.domain);
      } catch {
        return false;
      }
      onRemoved(rule.domain);
      await refreshLinkCardDomainRules(queryClient);
      const message = t("client:admin.link_card_domains.removed", { domain: domainInText(t, rule.domain) });
      setStatus(message);
      notice.inPlace(message);
      return true;
    },
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
    status,
  };
}
