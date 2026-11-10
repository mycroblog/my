// SPDX-License-Identifier: AGPL-3.0-only
// The user's page of the tips (ADR-0162 §6, §7, §13, §17, §19): the amounts the operators chose, whether the user may
// tip, the final confirmation and the way to the provider's hosted checkout, and the return from it, which reads the
// tip again until its payment is confirmed and then shows only the thanks. The entry to the page on the information
// page and the funding meter's panel says how a visitor who is not signed in becomes able to tip. The Tauri app shows
// none of it (ADR-0162 §3).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useIntl } from "react-intl";
import { useApi, useLeaveApp, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage, rateLimitedUntil } from "../../../lib/errors";
import { useLoadFailure } from "../../../lib/loadFailure";
import { useT } from "../../../lib/i18n";
import { isTauriApp } from "../../../lib/platform";
import { shellQueryReader, shellQueryRetry } from "../../../lib/query";
import {
  instanceQueryOptions,
  ownSupporterSubscriptionQueryOptions,
  type SupporterIneligibleReason,
} from "../api/supporters";
import {
  type Money,
  ownTipQueryOptions,
  startTipMutation,
  type SupporterTip,
  tipAmountsQueryOptions,
  tipProblems,
} from "../api/tips";

/** Where the provider's checkout sent the browser back: the tip it was of, and whether it was completed or left. */
export type TipReturn = { tipId: string; checkout: "success" | "canceled" } | undefined;

/** How often, and how many times, the page reads an open tip again after the checkout returned. */
const openPollInterval = 3000;
const openPollLimit = 20;

/** What the page says of the return from the checkout above the form. */
export type TipReturnNote =
  /** The checkout was left, or it ended unpaid: nothing was charged. */
  | "canceled"
  /** The checkout was completed, and the provider has not confirmed the payment after the page read it many times. */
  | "unconfirmed"
  | undefined;

export type TipPage =
  /** The Tauri app, which offers no way to tip (ADR-0162 §3). */
  | { status: "app" }
  /** The server takes no tips: the instance says so, or the API answers 404. */
  | { status: "disabled" }
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  /** The checkout was completed, and the page waits for the provider to confirm the payment. */
  | { status: "confirming" }
  /** The tip that the checkout returned from is paid: the page shows only the thanks. */
  | { status: "thanks"; tip: SupporterTip }
  | {
      status: "ready";
      amounts: readonly Money[];
      /** Why the user may not tip; undefined when they may. */
      ineligible: SupporterIneligibleReason | undefined;
      /** The server's page of its commerce disclosures; undefined without one. */
      commerceDisclosureUrl: string | undefined;
      note: TipReturnNote;
    };

/** The user's page of the tips, as far as its queries have answered. */
export function useTipPage(returned: TipReturn): TipPage {
  const api = useApi();
  const inApp = isTauriApp();
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const on = instance.data?.tips_enabled === true && !inApp;
  const amounts = useQuery({
    ...tipAmountsQueryOptions(api),
    retry: shellQueryRetry,
    retryOnMount: false,
    enabled: on,
  });
  // The tips take the conditions of a subscription but the subscription itself (ADR-0162 §7), which this answer says.
  const own = useQuery({
    ...ownSupporterSubscriptionQueryOptions(api),
    retry: shellQueryRetry,
    retryOnMount: false,
    enabled: on,
  });
  const queryClient = useQueryClient();
  const tipQuery = ownTipQueryOptions(api, returned?.tipId ?? "");
  const tip = useQuery({
    ...tipQuery,
    retry: shellQueryRetry,
    retryOnMount: false,
    enabled: on && returned !== undefined,
    // Reading an open tip makes the server read its checkout again: the confirmation comes in a few seconds.
    refetchInterval: (query) =>
      returned?.checkout === "success" &&
      query.state.data?.state === "open" &&
      query.state.dataUpdateCount < openPollLimit
        ? openPollInterval
        : false,
  });
  const amountsFailure = useLoadFailure(amounts);
  const ownFailure = useLoadFailure(own);
  if (inApp) return { status: "app" };
  if (instance.data === undefined) return { status: "loading" };
  const notFound = (error: unknown) => error instanceof ApiError && error.status === 404;
  if (!on || notFound(amountsFailure.error) || notFound(ownFailure.error)) return { status: "disabled" };
  const failed = amountsFailure.error ?? ownFailure.error;
  if (failed !== null) {
    return {
      status: "error",
      reason: errorMessage(failed),
      retry: () => {
        void amounts.refetch();
        void own.refetch();
      },
      isRetrying: amounts.isFetching || own.isFetching,
    };
  }
  // A tip that cannot be read (another user's, an unknown one) is a return the page ignores.
  const returnedTip = returned !== undefined && tip.isError ? undefined : tip.data;
  if (returned !== undefined && tip.isPending) return { status: "loading" };
  if (returnedTip?.state === "paid") return { status: "thanks", tip: returnedTip };
  // The query keeps how many answers it had, which ends the reads again (refetchInterval above).
  const answers = queryClient.getQueryState(tipQuery.queryKey)?.dataUpdateCount ?? 0;
  const pollsLeft = answers < openPollLimit;
  if (returned?.checkout === "success" && returnedTip?.state === "open" && pollsLeft) return { status: "confirming" };
  if (amounts.data === undefined || own.data === undefined) return { status: "loading" };
  let note: TipReturnNote;
  if (returnedTip !== undefined) {
    if (returnedTip.state === "expired" || returned?.checkout === "canceled") note = "canceled";
    else note = "unconfirmed";
  }
  return {
    status: "ready",
    amounts: amounts.data.amounts,
    ineligible: own.data.ineligible_reason,
    commerceDisclosureUrl: instance.data.commerce_disclosure_url,
    note,
  };
}

/** Why a start of a tip failed, localized: what the screen knows of the problem's type, or the server's reason. */
function useTipFailure(): (error: unknown) => string {
  const t = useT();
  const intl = useIntl();
  return (error) => {
    if (error instanceof ApiError) {
      if (error.status === 429) {
        const until = rateLimitedUntil(error);
        return until === undefined
          ? t("client:tip.error.rate_limited")
          : t("client:tip.error.rate_limited_until", { time: intl.formatTime(until, { timeStyle: "short" }) });
      }
      if (error.status === 404) return t("client:tip.error.disabled");
      switch (error.type) {
        case tipProblems.ineligible:
          return t("client:tip.error.ineligible");
        case tipProblems.amount:
          return t("client:tip.error.amount");
        case tipProblems.pending:
          return t("client:tip.error.pending");
        case tipProblems.closing:
          return t("client:tip.error.closing");
      }
    }
    return errorMessage(error);
  };
}

/** Starting a tip: the page leaves the app for the provider's checkout, or for the thanks of a tip paid already. */
export function useStartTip() {
  const api = useApi();
  const leaveApp = useLeaveApp();
  const failure = useTipFailure();
  const mutation = useMutation({
    ...startTipMutation(api),
    onSuccess: (checkout) => {
      leaveApp(new URL(checkout.redirect_url));
    },
  });
  return {
    start: (amount: Money) => {
      mutation.mutate(amount);
    },
    // The browser is on its way to the checkout once it succeeded, so the button keeps waiting.
    isPending: mutation.isPending || mutation.isSuccess,
    error: mutation.isError ? failure(mutation.error) : undefined,
    reset: mutation.reset,
  };
}

/** The amount chosen on the page: the first one until the user picks another. */
export function useTipAmountChoice(amounts: readonly Money[]) {
  const [chosen, setChosen] = useState<string | undefined>(undefined);
  const key = (money: Money) => `${money.amount} ${money.currency}`;
  const amount = amounts.find((money) => key(money) === chosen) ?? amounts[0];
  return {
    amount,
    value: amount === undefined ? undefined : key(amount),
    choices: amounts.map((money) => ({ value: key(money), money })),
    choose: setChosen,
  };
}

/** What the entry to the tips shows: nothing, the way to sign in, or the link to the page of the tips. */
export type TipEntry = "hidden" | "signed-out" | "signed-in";

/**
 * The entry to the tips on the information page and the funding meter's panel: none in the Tauri app or while the
 * server takes no tips; for a visitor who is not signed in, the way to sign in, since only the users of the server
 * may tip (ADR-0162 §7); the page of the tips otherwise, which says what else a user needs.
 */
export function useTipEntry(): TipEntry {
  const api = useApi();
  const status = useSessionStatus();
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  // Until the session is known, nothing shows, so that a signed-in user never sees the way to sign in flash by.
  if (isTauriApp() || instance.data?.tips_enabled !== true || status === "unknown") return "hidden";
  return status;
}
