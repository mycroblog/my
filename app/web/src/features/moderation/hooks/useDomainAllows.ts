// SPDX-License-Identifier: AGPL-3.0-only
// The allowlist of federation (ADR-0159 §1, manage_domains): the allowed domains, adding one with a comment, and
// removing one. The allowlist bears on federation only while the instance setting federation_scope is `allowlist`,
// which this screen does not read (it is manage_settings'). What the last change did is announced and kept for the
// list's StatusMessage; a failure to add stays under the form and a failure to remove in its dialog.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  addDomainAllowMutation,
  type DomainAllow,
  domainAllowsQueryOptions,
  refreshDomainAllows,
  removeDomainAllowMutation,
} from "../api/domainLists";
import { domainInText } from "./useDomainActions";

/** The allowed domains, by domain. */
export function useDomainAllowList(): DomainAllow[] {
  const api = useApi();
  return useSuspenseQuery(domainAllowsQueryOptions(api)).data.items;
}

/** The form of a new allow. */
type AllowDraft = { domain: string; comment: string };

const emptyDraft: AllowDraft = { domain: "", comment: "" };

/**
 * Changing the allowlist: the form that adds a domain, and removing one. `onRemoved(domain)` is called once the
 * allow is removed, just before the list is fetched again without it (for focus, lib/useFocusAfterRemoval).
 */
export function useDomainAllows(onRemoved: (domain: string) => void) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [draft, setDraft] = useState<AllowDraft>(emptyDraft);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const report = (message: string) => {
    setStatus(message);
    notice.inPlace(message);
  };
  const add = useMutation({
    ...addDomainAllowMutation(api),
    onSuccess: async ({ allow, existed }) => {
      // The allow is added once the list shows it (the button stays pending until then).
      await refreshDomainAllows(queryClient);
      setDraft(emptyDraft);
      const domain = domainInText(t, allow.domain);
      report(
        existed
          ? t("client:admin.domain_allows.exists", { domain })
          : t("client:admin.domain_allows.added", { domain }),
      );
    },
  });
  const remove = useMutation(removeDomainAllowMutation(api));
  return {
    draft,
    change: (change: Partial<AllowDraft>) => {
      setDraft((previous) => ({ ...previous, ...change }));
    },
    /** Adds the domain once the browser has checked the fields; an empty comment is left out. */
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (add.isPending) return;
      add.mutate({
        domain: draft.domain.trim(),
        ...(draft.comment.trim() === "" ? {} : { comment: draft.comment }),
      });
    },
    isAdding: add.isPending,
    /** Why adding failed, localized; the form keeps what was typed. */
    addError: add.isError ? errorMessage(add.error) : undefined,
    /** Removes the allow of `domain`; resolves with whether it succeeded. */
    remove: async (domain: string): Promise<boolean> => {
      add.reset();
      try {
        await remove.mutateAsync(domain);
      } catch {
        return false;
      }
      // Where the row was is taken before the list changes.
      onRemoved(domain);
      await refreshDomainAllows(queryClient);
      report(t("client:admin.domain_allows.removed", { domain: domainInText(t, domain) }));
      return true;
    },
    /** The domain whose allow is being removed. */
    removingDomain: remove.isPending ? remove.variables : undefined,
    /** Why the last removal failed, localized, until the next one. */
    removeError: remove.isError ? errorMessage(remove.error) : undefined,
    /** What the last change did, for the list's StatusMessage. */
    status,
  };
}

export type DomainAllowsForm = ReturnType<typeof useDomainAllows>;
