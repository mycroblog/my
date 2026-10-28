// SPDX-License-Identifier: AGPL-3.0-only
// The user's page of the supporter plans (ADR-0072 §6, §8, §13, §14): the plans, whether the user may subscribe,
// the final confirmation and the way to the provider's hosted checkout, the return from it, the user's subscription
// with changing the plan, cancelling, resuming and withdrawing a scheduled change, and the provider's portal. The Tauri
// app shows none of it (§14).
import { useAnnouncer } from "@my/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useApi, useLeaveApp } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { type Translate, useT } from "../../../lib/i18n";
import { isTauriApp } from "../../../lib/platform";
import { shellQueryReader, shellQueryRetry } from "../../../lib/query";
import {
  cacheSubscription,
  instanceQueryOptions,
  openPortalMutation,
  ownSupporterSubscriptionQueryOptions,
  type SubscriptionCommand,
  startCheckoutMutation,
  subscriptionCommandMutation,
  type SupporterIneligibleReason,
  type SupporterInterval,
  type SupporterPlan,
  type SupporterPlanChoice,
  supporterPlansQueryOptions,
  supporterProblems,
  type SupporterSubscription,
} from "../api/supporters";

/** Where the provider sent the browser back from: the checkout, completed or left, or the portal. */
export type SupporterReturn = "checkout-success" | "checkout-canceled" | "portal" | undefined;

/** How often, and how many times, the page reads a pending subscription again after the checkout returned. */
const pendingPollInterval = 3000;
const pendingPollLimit = 20;

export type SupporterPage =
  /** The Tauri app, which offers no way to subscribe (ADR-0072 §14). */
  | { status: "app" }
  /** The program is disabled: the API answers 404. */
  | { status: "disabled" }
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | {
      status: "ready";
      plans: readonly SupporterPlan[];
      /** The subscription that goes on; undefined without one. */
      subscription: SupporterSubscription | undefined;
      /** Why the user may not subscribe; undefined when they may. */
      ineligible: SupporterIneligibleReason | undefined;
      /** The server's page of its commerce disclosures; undefined without one. */
      commerceDisclosureUrl: string | undefined;
      /** Whether the page waits for the provider to confirm the payment that the checkout returned from. */
      confirming: boolean;
    };

/** The user's page of the supporter plans, as far as the queries that the route's loader asked for have answered. */
export function useSupporterPage(returned: SupporterReturn): SupporterPage {
  const api = useApi();
  const inApp = isTauriApp();
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const plans = useQuery({
    ...supporterPlansQueryOptions(api),
    retry: shellQueryRetry,
    retryOnMount: false,
    enabled: !inApp,
  });
  const own = useQuery({
    ...ownSupporterSubscriptionQueryOptions(api),
    retry: shellQueryRetry,
    retryOnMount: false,
    enabled: !inApp,
    // After the checkout, the provider's confirmation reaches the server in a few seconds: read it again meanwhile.
    refetchInterval: (query) =>
      returned === "checkout-success" &&
      query.state.data?.subscription?.state === "pending" &&
      query.state.dataUpdateCount < pendingPollLimit
        ? pendingPollInterval
        : false,
  });
  if (inApp) return { status: "app" };
  const disabled = [plans.error, own.error].some((error) => error instanceof ApiError && error.status === 404);
  if (disabled) return { status: "disabled" };
  const failed = plans.error ?? own.error;
  if (failed !== null) {
    return {
      status: "error",
      reason: errorMessage(failed),
      retry: () => {
        void plans.refetch();
        void own.refetch();
      },
      isRetrying: plans.isFetching || own.isFetching,
    };
  }
  if (plans.data === undefined || own.data === undefined) return { status: "loading" };
  return {
    status: "ready",
    plans: plans.data.items,
    subscription: own.data.subscription,
    ineligible: own.data.ineligible_reason,
    commerceDisclosureUrl: instance.data?.commerce_disclosure_url,
    confirming: returned === "checkout-success" && own.data.subscription?.state === "pending",
  };
}

/** Whether the settings show the way to the page of the supporter plans: the program is enabled, outside the Tauri app. */
export function useSupporterEntry(): boolean {
  const api = useApi();
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  return !isTauriApp() && instance.data?.supporters_enabled === true;
}

/** Why a change of the subscription failed, localized: what the screen knows of the problem's type, or the server's reason. */
function subscriptionFailure(t: Translate, error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.type) {
      case supporterProblems.ineligible:
        return t("client:supporter.error.ineligible");
      case supporterProblems.subscribed:
        return t("client:supporter.error.subscribed");
      case supporterProblems.planUnavailable:
        return t("client:supporter.error.plan_unavailable");
      case supporterProblems.subscriptionState:
        return t("client:supporter.error.state");
      case supporterProblems.samePlan:
        return t("client:supporter.error.same_plan");
      case supporterProblems.subscriptionEnded:
        return t("client:supporter.error.ended");
    }
  }
  return errorMessage(error);
}

/** Going to the provider's hosted checkout of a plan: the page leaves the app for it in the same tab. */
export function useCheckout() {
  const api = useApi();
  const t = useT();
  const leaveApp = useLeaveApp();
  const mutation = useMutation({
    ...startCheckoutMutation(api),
    onSuccess: (checkout) => {
      leaveApp(new URL(checkout.redirect_url));
    },
  });
  return {
    start: (choice: SupporterPlanChoice) => {
      mutation.mutate(choice);
    },
    // The browser is on its way to the checkout once it succeeded, so the button keeps waiting.
    isPending: mutation.isPending || mutation.isSuccess,
    error: mutation.isError ? subscriptionFailure(t, mutation.error) : undefined,
    reset: mutation.reset,
  };
}

/** Opening the provider's portal of the user's payment methods and invoices: the page leaves the app for it. */
export function usePortal() {
  const api = useApi();
  const t = useT();
  const leaveApp = useLeaveApp();
  const mutation = useMutation({
    ...openPortalMutation(api),
    onSuccess: (portal) => {
      leaveApp(new URL(portal.url));
    },
  });
  return {
    open: () => {
      mutation.mutate();
    },
    isPending: mutation.isPending || mutation.isSuccess,
    error: mutation.isError ? subscriptionFailure(t, mutation.error) : undefined,
  };
}

/** The messages that a change of the subscription announces and leaves in the page's StatusMessage. */
const commandDone = {
  change: "client:supporter.subscription.changed",
  cancel: "client:supporter.subscription.canceled",
  resume: "client:supporter.subscription.resumed",
  cancel_scheduled_change: "client:supporter.subscription.unscheduled",
} as const;

/** The changes of the user's subscription; each one answers the subscription after it, which replaces the cached one. */
export function useSubscriptionCommands() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation({
    ...subscriptionCommandMutation(api),
    onSuccess: async (subscription, command) => {
      await cacheSubscription(api, queryClient, subscription);
      const message = t(commandDone[command.kind]);
      setStatus(message);
      announce(message);
    },
  });
  return {
    status,
    /** Sends a command and reports whether it succeeded, so that its dialog closes only then. */
    run: async (command: SubscriptionCommand): Promise<boolean> => {
      setStatus(undefined);
      try {
        await mutation.mutateAsync(command);
        return true;
      } catch {
        return false;
      }
    },
    pending: mutation.isPending ? mutation.variables.kind : undefined,
    error: mutation.isError ? subscriptionFailure(t, mutation.error) : undefined,
    failed: mutation.isError ? mutation.variables.kind : undefined,
  };
}

/** When a change from one plan and interval to another applies (ADR-0072 §8): now with proration, or at the period's end. */
export type ChangeTiming = "now" | "period_end";

/**
 * When a change applies: to a higher plan at once, to a lower one at the end of the period; within one plan, to the
 * yearly price at once and to the monthly price at the end of the period. Undefined when the current plan's rank is
 * not known (a plan that no longer takes subscriptions), or for no change.
 */
export function changeTiming(
  from: { rank: number | undefined; interval: SupporterInterval },
  to: { rank: number; interval: SupporterInterval },
): ChangeTiming | undefined {
  if (from.rank === undefined) return undefined;
  if (to.rank !== from.rank) return to.rank > from.rank ? "now" : "period_end";
  if (to.interval === from.interval) return undefined;
  return to.interval === "year" ? "now" : "period_end";
}
