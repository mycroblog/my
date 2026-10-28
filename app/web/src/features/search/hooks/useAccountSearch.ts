// SPDX-License-Identifier: AGPL-3.0-only
// The account search form's logic (web-client/README.md A4): the typed text, sending it, and why the last search
// found nothing. The hook returns no JSX.
import { useAnnouncer } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type SubmitEvent, useState } from "react";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { isFederationRestricted } from "../../../lib/federation";
import { useFederation } from "../../../lib/federation";
import { type Translate, useT } from "../../../lib/i18n";
import { accountSearchMutationKey, lookupAccount, storeFoundAccount } from "../api/lookup";

export type AccountSearchOptions = {
  /** The text the field starts with: the search in the page's address (web-client/README.md S3). */
  initialQuery: string;
  /** Called with the found account's `acct` and the text that found it, to open its profile. */
  onFound: (acct: string, query: string) => void;
};

export type AccountSearchState = {
  value: string;
  onChange: (value: string) => void;
  /** Sends the search: the form's submit handler. */
  submit: (event: SubmitEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** Why the search of the text in the field found nothing (localized), or null. */
  error: string | null;
  /** Whether the viewer is signed out, so that only the accounts this server knows can be found until they sign in. */
  signedOut: boolean;
  /**
   * Whether only the accounts this server knows can be found because the instance does not take in other servers'
   * posts (its federation mode is `off` or `outbound`, ADR-0076), signed in or not.
   */
  knownOnly: boolean;
};

/** Why the search finds only the accounts this server knows, if it does. */
type Scope = "all" | "signed-out" | "known-only";

/** The localized reason a search failed. 404 and 429 come from the client's catalog (web-client/README.md E2). */
function failureOf(t: Translate, error: unknown, scope: Scope): string {
  if (error instanceof ApiError && error.status === 404) {
    switch (scope) {
      case "signed-out":
        return t("client:search.not_found_signed_out");
      case "known-only":
        return t("client:search.not_found_known_only");
      case "all":
        return t("client:search.not_found");
    }
  }
  if (error instanceof ApiError && error.status === 429) return t("client:search.rate_limited");
  // The mode narrowed after the page loaded it; GET /instance is fetched again (lib/federation.ts).
  if (isFederationRestricted(error)) return t("client:search.not_found_known_only");
  return t("client:search.failed", { reason: errorMessage(error) });
}

/** Returns the search form's state; a found account goes to `onFound`, a failure stays with the text that caused it. */
export function useAccountSearch({ initialQuery, onFound }: AccountSearchOptions): AccountSearchState {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const session = useSessionStatus();
  const [value, setValue] = useState(initialQuery);
  // Resolving needs a token; while the session is still resuming, the request waits for it (lib/api/client.ts).
  const signedOut = session === "signed-out";
  // Resolving reaches the account's server, which the server refuses while it takes nothing in (ADR-0076).
  const { followsRemote } = useFederation();
  const scope: Scope = !followsRemote ? "known-only" : signedOut ? "signed-out" : "all";
  const mutation = useMutation({
    mutationKey: accountSearchMutationKey,
    mutationFn: (variables: { query: string; scope: Scope }) =>
      lookupAccount(api, variables.query, variables.scope === "all"),
    onSuccess: (account, { query }) => {
      storeFoundAccount(queryClient, api, account);
      onFound(account.acct, query);
    },
    onError: (error, variables) => {
      // The message is linked to the field, which keeps the focus; a screen reader hears it at once.
      announce(failureOf(t, error, variables.scope), "assertive");
    },
  });

  const query = value.trim();
  return {
    value,
    onChange: setValue,
    submit: (event) => {
      event.preventDefault();
      if (query === "" || mutation.isPending) return;
      mutation.mutate({ query, scope });
    },
    isPending: mutation.isPending,
    // The failure belongs to the text that caused it: editing the text clears it.
    error:
      mutation.isError && mutation.variables.query === query
        ? failureOf(t, mutation.error, mutation.variables.scope)
        : null,
    signedOut: scope === "signed-out",
    knownOnly: scope === "known-only",
  };
}
