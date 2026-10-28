// SPDX-License-Identifier: AGPL-3.0-only
// The domains the user mutes (FR-304): the list, muting a domain with a form, and unmuting one from its row. The server
// keeps a domain in IDNA A-labels; the screen shows it with the U-labels it stands for (lib/idn).
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { domainToUnicode } from "../../../lib/idn";
import {
  type DomainMute,
  domainMutesQueryOptions,
  muteDomainMutation,
  refreshDomainMutes,
  removeDomainMute,
  unmuteDomainMutation,
} from "../api/domainMutes";
import type { AccountList } from "./useAccountLists";

/** A domain for a sentence: its U-labels, isolated left to right from the text around it. */
function domainInText(domain: string): string {
  // U+2066 LEFT-TO-RIGHT ISOLATE … U+2069 POP DIRECTIONAL ISOLATE.
  return `\u2066${domainToUnicode(domain)}\u2069`;
}

/** The domain mutes, in the order of the domains, and loading more of them. */
export function useDomainMuteList(): AccountList<DomainMute> {
  const api = useApi();
  const query = useSuspenseInfiniteQuery(domainMutesQueryOptions(api));
  return {
    items: query.data.pages.flatMap((page) => page.domain_mutes),
    hasMore: query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    loadMore: () => {
      // A refetch in progress would overwrite the page fetched now (TanStack Query, "Infinite Queries").
      if (!query.isFetching) void query.fetchNextPage();
    },
  };
}

/** Unmuting a domain from its row. */
export type UnmuteDomain = {
  /** Unmutes `domain`; `onDone` is called once it is off the list. */
  run: (domain: string, onDone: () => void) => void;
  /** The domain being unmuted. */
  pendingDomain: string | undefined;
};

/**
 * Unmuting a domain: the row leaves the list once the posts and notifications on screen are fetched again (the row stays
 * pending until then), then the caller moves focus to the next row and the result is announced. A failure keeps the row and its focus,
 * and says why (web-client/README.md E2).
 */
export function useUnmuteDomain(): UnmuteDomain {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const unmute = useMutation({
    ...unmuteDomainMutation(api),
    onSuccess: async () => {
      await refreshDomainMutes(queryClient, false);
    },
  });
  return {
    pendingDomain: unmute.isPending ? unmute.variables : undefined,
    run: (domain, onDone) => {
      unmute.mutate(domain, {
        onSuccess: () => {
          removeDomainMute(queryClient, api, domain);
          onDone();
          notice.inPlace(t("client:settings.domain_mutes.unmuted", { domain: domainInText(domain) }));
        },
        onError: (error) => {
          notice.inPlace(errorMessage(error), "assertive");
        },
      });
    },
  };
}

/** What the form that mutes a domain needs. */
export type MuteDomainForm = {
  domain: string;
  setDomain: (domain: string) => void;
  /** Mutes the domain once the browser has checked the field. */
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** Why muting failed, localized, for the field; the domain typed stays. */
  error: string | undefined;
  /** What the last mute did, for the form's StatusMessage; undefined before the first one. */
  status: string | undefined;
};

/**
 * The form that mutes a domain. A success empties the field and says which domain is muted, as the server normalized
 * it, once the list, the posts, and the notifications on screen are fetched again.
 */
export function useMuteDomainForm(): MuteDomainForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [domain, setDomain] = useState("");
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mute = useMutation({
    ...muteDomainMutation(api),
    onSuccess: async (muted) => {
      await refreshDomainMutes(queryClient, true);
      setDomain("");
      setStatus(t("client:settings.domain_mutes.muted", { domain: domainInText(muted.domain) }));
    },
  });
  return {
    domain,
    setDomain: (value) => {
      setDomain(value);
      if (mute.isError) mute.reset();
    },
    submit: (event) => {
      event.preventDefault();
      const value = domain.trim();
      if (mute.isPending || value === "") return;
      mute.reset();
      mute.mutate(value);
    },
    isPending: mute.isPending,
    error: mute.isError ? errorMessage(mute.error) : undefined,
    status,
  };
}
