// SPDX-License-Identifier: AGPL-3.0-only
// Confirming an operation by signing in again at a linked login provider (FR-010), in place of the password or a
// passkey, for a user who has neither. The page leaves the app for the provider, which sends the browser back to it
// with the confirmation in the fragment; the page reads it once and takes the fragment out of the address, so that a
// reload does not show it again. The operation then sends the confirmation once (web-client/09-app-shell.md §9).
import { useMutation, useSuspenseQuery } from "@tanstack/react-query";
import { useLocation, useNavigate } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useApi, useLeaveApp } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  beginExternalConfirmationMutation,
  type ConfirmedOperation,
  type ExternalFailure,
  type ExternalOutcome,
  type LinkedIdentity,
  linkedIdentitiesQueryOptions,
  parseExternalOutcome,
} from "../api/linkedIdentities";
import { passkeysQueryOptions } from "../api/passkeys";

/**
 * The outcome of a link or a confirmation that the page's address had when the page opened. The fragment leaves the
 * address (the URL is the outside system this synchronizes with), and the outcome stays for the page's life.
 */
export function useExternalOutcome(): ExternalOutcome | undefined {
  const hash = useLocation({ select: (location) => location.hash });
  const navigate = useNavigate();
  const [outcome] = useState(() => parseExternalOutcome(hash));
  const inAddress = parseExternalOutcome(hash) !== undefined;
  useEffect(() => {
    if (!inAddress) return undefined;
    void navigate({ to: ".", hash: "", replace: true, resetScroll: false });
    return undefined;
  }, [inAddress, navigate]);
  return outcome;
}

/** The message of a failed confirmation with a provider. */
const confirmationFailureMessages: Record<Exclude<ExternalFailure, "linked_elsewhere" | "already_linked">, ClientMessageKey> = {
  mismatch: "client:settings.external_confirmation.failed.mismatch",
  expired: "client:settings.external_confirmation.failed.expired",
  rate_limited: "client:settings.external_confirmation.failed.rate_limited",
  unavailable: "client:settings.external_confirmation.failed.unavailable",
  denied: "client:settings.external_confirmation.failed.denied",
  failed: "client:settings.external_confirmation.failed.failed",
};

/** What confirming an operation with a linked provider needs. */
export type ExternalConfirmation = {
  /**
   * Whether the operation is confirmed this way: the user has a linked account at an available provider and no
   * passkey (and, for the operations the password confirms, no password). The other ways come first (ADR-0106 §3.7).
   */
  isPreferred: boolean;
  /** The linked accounts whose provider confirms. */
  providers: readonly LinkedIdentity[];
  /** The confirmation that came back for the operation and is not used yet, with its provider's name. */
  confirmed: { token: string; providerName: string } | undefined;
  /** Leaves the app to sign in again at the provider of a linked account, which sends the browser back here. */
  start: (providerId: string) => void;
  /** The provider being started. */
  startingId: string | undefined;
  /** Why the last confirmation failed, localized, for an alert; undefined when none did. */
  error: string | undefined;
  /** Marks the confirmation used: an operation took it, which the server accepts once. */
  markUsed: () => void;
};

/**
 * Confirming `operation` with a linked provider. `hasPassword` tells whether the user's password confirms it instead
 * (false for the removal of the password, which only a passkey or a provider confirms). The route loads the linked
 * accounts and the passkeys.
 */
export function useExternalConfirmation(operation: ConfirmedOperation, hasPassword: boolean): ExternalConfirmation {
  const api = useApi();
  const t = useT();
  const leaveApp = useLeaveApp();
  const pathname = useLocation({ select: (location) => location.pathname });
  const outcome = useExternalOutcome();
  const links = useSuspenseQuery(linkedIdentitiesQueryOptions(api)).data.items;
  const hasPasskey = useSuspenseQuery(passkeysQueryOptions(api)).data.items.length > 0;
  const [used, setUsed] = useState(false);
  const begin = useMutation({
    ...beginExternalConfirmationMutation(api),
    onSuccess: (url) => {
      leaveApp(url);
    },
  });
  const providers = links.filter((link) => link.available);
  const mine = outcome !== undefined && outcome.kind !== "linked" && outcome.operation === operation ? outcome : undefined;
  let confirmed: ExternalConfirmation["confirmed"];
  if (mine?.kind === "confirmed" && !used) {
    const link = links.find((l) => l.provider_id === mine.providerId);
    confirmed = { token: mine.token, providerName: link?.provider_name ?? "" };
  }
  let error: string | undefined;
  if (begin.isError) error = t("client:settings.external_confirmation.start_failed", { reason: errorMessage(begin.error) });
  else if (mine?.kind === "failed" && mine.failure !== "linked_elsewhere" && mine.failure !== "already_linked") {
    error = t(confirmationFailureMessages[mine.failure]);
  }
  return {
    isPreferred: providers.length > 0 && !hasPasskey && !hasPassword,
    providers,
    confirmed,
    start: (providerId) => {
      if (begin.isPending) return;
      begin.mutate({ providerId, operation, returnTo: pathname });
    },
    startingId: begin.isPending || begin.isSuccess ? begin.variables.providerId : undefined,
    error,
    markUsed: () => {
      setUsed(true);
    },
  };
}
