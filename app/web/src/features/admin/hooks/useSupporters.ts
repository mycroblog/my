// SPDX-License-Identifier: AGPL-3.0-only
// The supporters screen (ADR-0072, manage_supporters): the supporter program with enabling and disabling it, the plans
// with creating, changing and retiring them, the subscriptions with ending one at once, and the state of the payment
// provider with reconciling with it now (ADR-0074 §4, at most once an hour). The server keeps every
// rule (the plan's role, the order of the plans, the provider's minimum amount); a refusal is said in the server's
// words next to the field it is about, and what a change did is announced and kept in a StatusMessage.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseInfiniteQuery, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { type PagedPages, toPagedList } from "../../moderation";
import {
  cancelSupporterSubscriptionMutation,
  createSupporterPlanMutation,
  maxAnnualDiscount,
  maxEmojiLimit,
  maxPlanRank,
  type PaymentStatus,
  paymentStatusQueryOptions,
  refreshPaymentStatus,
  refreshSupporters,
  retireSupporterPlanMutation,
  runPaymentReconcileMutation,
  setSupporterProgramMutation,
  type SupporterPlan,
  type SupporterPlanChange,
  supporterPlanNameLimit,
  type SupporterPlanProblemReason,
  supporterPlanProblemReason,
  supporterPlansQueryOptions,
  type SupporterPlanVisibility,
  type SupporterProgram,
  supporterProgramQueryOptions,
  type SupporterSubscription,
  type SupporterSubscriptionFilter,
  supporterSubscriptionsQueryOptions,
  updateSupporterPlanMutation,
} from "../api/supporters";

type Translate = ReturnType<typeof useT>;

/** A plan for a sentence: its name, written by an administrator in any language, isolated (U+2068 … U+2069). */
export function planName(plan: Pick<SupporterPlan, "name">): string {
  return `⁨${plan.name}⁩`;
}

/** The program and enabling or disabling it. A refusal (409) is said in the server's words. */
export function useSupporterProgram() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const program = useSuspenseQuery(supporterProgramQueryOptions(api)).data;
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation({
    ...setSupporterProgramMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async (saved: SupporterProgram) => {
      await refreshSupporters(api, queryClient);
      const message = t(
        saved.enabled
          ? "client:admin.supporters.program.enabled_done"
          : "client:admin.supporters.program.disabled_done",
      );
      setStatus(message);
      notice.inPlace(message);
    },
  });
  return {
    program,
    /** Enables (`true`) or disables (`false`) the program. */
    set: (enabled: boolean) => {
      if (mutation.isPending) return;
      mutation.mutate(enabled);
    },
    isPending: mutation.isPending,
    /** Why the server refused, in its words, until the next change. */
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
    status,
  };
}

/** The payment provider of the deployment's configuration, or undefined for none (from the program). */
export function useConfiguredPaymentProvider(): string | undefined {
  const api = useApi();
  return useSuspenseQuery(supporterProgramQueryOptions(api)).data.configured_provider;
}

/** The state of the payment provider: read only when the program names a configured provider (a 404 without one). */
export function usePaymentStatus(): PaymentStatus {
  const api = useApi();
  return useSuspenseQuery(paymentStatusQueryOptions(api)).data;
}

/** Reconciling with the payment provider now: at most once an hour (429), which the screen explains. */
export function useRunPaymentReconcile() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [result, setResult] = useState<{ status: string } | { error: string } | undefined>(undefined);
  const mutation = useMutation({
    ...runPaymentReconcileMutation(api),
    onMutate: () => {
      setResult(undefined);
    },
    onSuccess: async () => {
      await refreshPaymentStatus(queryClient);
      const message = t("client:admin.supporters.payment.reconcile.queued");
      setResult({ status: message });
      notice.inPlace(message);
    },
    onError: (failure) => {
      const tooSoon = failure instanceof ApiError && failure.status === 429;
      setResult({
        error: tooSoon
          ? t("client:admin.supporters.payment.reconcile.too_soon")
          : t("client:admin.supporters.payment.reconcile.failed", { reason: errorMessage(failure) }),
      });
    },
  });
  return {
    run: () => {
      if (mutation.isPending) return;
      mutation.mutate();
    },
    isPending: mutation.isPending,
    status: result !== undefined && "status" in result ? result.status : undefined,
    error: result !== undefined && "error" in result ? result.error : undefined,
  };
}

/** The plans: those that are not retired by rank, and the retired ones. */
export function useSupporterPlans(): { active: SupporterPlan[]; retired: SupporterPlan[] } {
  const api = useApi();
  const plans = useSuspenseQuery(supporterPlansQueryOptions(api)).data.items;
  return { active: plans.filter((plan) => !plan.retired), retired: plans.filter((plan) => plan.retired) };
}

/** The fields of a plan's form that a refusal can be about. */
type PlanField =
  | "role"
  | "name"
  | "description"
  | "amount"
  | "currency"
  | "discount"
  | "emojiLimit"
  | "rank"
  | "visibility"
  | "rolePosition"
  | "form";

/** Why a plan cannot be saved, next to its field. */
export type PlanFieldError = { field: PlanField; message: string };

/** A plan as the forms edit it: the numbers as typed. */
export type PlanDraft = {
  role: string;
  name: string;
  description: string;
  amount: string;
  currency: string;
  discount: string;
  emojiLimit: string;
  rank: string;
  visibility: SupporterPlanVisibility;
  rolePosition: string;
};

const emptyDraft: PlanDraft = {
  role: "",
  name: "",
  description: "",
  amount: "",
  currency: "JPY",
  discount: "0",
  emojiLimit: "0",
  rank: "0",
  visibility: "public",
  rolePosition: "",
};

/** The draft of an existing plan. */
export function draftOfPlan(plan: SupporterPlan): PlanDraft {
  return {
    role: plan.role.key,
    name: plan.name,
    description: plan.description,
    amount: plan.monthly_price.amount,
    currency: plan.monthly_price.currency,
    discount: String(plan.annual_discount_percent),
    emojiLimit: String(plan.emoji_limit),
    rank: String(plan.rank),
    visibility: plan.visibility,
    rolePosition: String(plan.role.position),
  };
}

const reasonFields: Readonly<Record<SupporterPlanProblemReason, PlanField>> = {
  name: "name",
  description: "description",
  monthly_price: "amount",
  currency: "currency",
  annual_discount_percent: "discount",
  emoji_limit: "emojiLimit",
  rank: "rank",
  visibility: "visibility",
  role_builtin: "role",
  role_taken: "role",
  role_held: "role",
  role_permissions: "role",
  role_position: "role",
  order_rank: "rank",
  order_position: "form",
  order_permissions: "form",
  order_emoji_limit: "emojiLimit",
};

/** Which field a refusal of the server is about, in its words; `roleField` is where a refusal of the role goes. */
function refusal(error: unknown, roleField: PlanField): PlanFieldError {
  const reason = supporterPlanProblemReason(error);
  const field = reason === undefined ? "form" : reasonFields[reason];
  return { field: field === "role" ? roleField : field, message: errorMessage(error) };
}

/** A whole number from 0 to `max` typed in a draft, or undefined. */
function parseCount(text: string, max: number): number | undefined {
  const trimmed = text.trim();
  if (!/^\d+$/u.test(trimmed)) return undefined;
  const n = Number(trimmed);
  return n <= max ? n : undefined;
}

const amountPattern = /^[0-9]+(\.[0-9]+)?$/u;
const currencyPattern = /^[A-Z]{3}$/u;

/** Checks the fields of a draft that the form can check before sending; undefined when it can be sent. */
function checkDraft(draft: PlanDraft, t: Translate, isNew: boolean): PlanFieldError | undefined {
  if (isNew && draft.role.trim() === "")
    return { field: "role", message: t("client:admin.supporters.plan.role.required") };
  const name = draft.name.trim();
  if (name === "" || name.length > supporterPlanNameLimit) {
    return {
      field: "name",
      message: t("client:admin.supporters.plan.name.invalid", { max: String(supporterPlanNameLimit) }),
    };
  }
  if (!amountPattern.test(draft.amount.trim()))
    return { field: "amount", message: t("client:admin.supporters.plan.amount.invalid") };
  if (isNew && !currencyPattern.test(draft.currency.trim()))
    return { field: "currency", message: t("client:admin.supporters.plan.currency.invalid") };
  if (parseCount(draft.discount, maxAnnualDiscount) === undefined) {
    return {
      field: "discount",
      message: t("client:admin.supporters.plan.number.invalid", { max: String(maxAnnualDiscount) }),
    };
  }
  if (parseCount(draft.emojiLimit, maxEmojiLimit) === undefined) {
    return {
      field: "emojiLimit",
      message: t("client:admin.supporters.plan.number.invalid", { max: String(maxEmojiLimit) }),
    };
  }
  if (parseCount(draft.rank, maxPlanRank) === undefined) {
    return { field: "rank", message: t("client:admin.supporters.plan.number.invalid", { max: String(maxPlanRank) }) };
  }
  if (!isNew && parseCount(draft.rolePosition, 999) === undefined) {
    return { field: "rolePosition", message: t("client:admin.supporters.plan.number.invalid", { max: "999" }) };
  }
  return undefined;
}

/**
 * The form of a new plan. A success empties it, keeping the currency, and gives what it did to `onCreated` (the list's
 * result).
 */
export function useNewPlanForm(onCreated: (message: string) => void) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<PlanDraft>(emptyDraft);
  const [invalid, setInvalid] = useState<PlanFieldError | undefined>(undefined);
  const mutation = useMutation({
    ...createSupporterPlanMutation(api),
    onSuccess: async (plan) => {
      await refreshSupporters(api, queryClient);
      setDraft((current) => ({ ...emptyDraft, currency: current.currency }));
      onCreated(t("client:admin.supporters.plan.created", { name: planName(plan) }));
    },
  });
  return {
    draft,
    change: (change: Partial<PlanDraft>) => {
      setDraft((previous) => ({ ...previous, ...change }));
    },
    /** Creates the plan once its draft is valid; `onAdded` runs once it is created (to close the add panel). */
    submit: (event: SyntheticEvent<HTMLFormElement>, onAdded?: () => void) => {
      event.preventDefault();
      if (mutation.isPending) return;
      const checked = checkDraft(draft, t, true);
      setInvalid(checked);
      if (checked !== undefined) {
        mutation.reset();
        return;
      }
      const description = draft.description.trim();
      mutation.mutate(
        {
          role: draft.role.trim(),
          name: draft.name.trim(),
          ...(description === "" ? {} : { description }),
          monthly_price: { amount: draft.amount.trim(), currency: draft.currency.trim() },
          annual_discount_percent: Number(draft.discount.trim()),
          emoji_limit: Number(draft.emojiLimit.trim()),
          rank: Number(draft.rank.trim()),
          visibility: draft.visibility,
        },
        { onSuccess: () => onAdded?.() },
      );
    },
    isPending: mutation.isPending,
    /** Why the plan was refused, next to its field; the draft stays. */
    error: invalid ?? (mutation.isError ? refusal(mutation.error, "role") : undefined),
  };
}

/** What a draft changes of a plan; only what differs is sent. */
function changeOf(plan: SupporterPlan, draft: PlanDraft): SupporterPlanChange {
  const change: SupporterPlanChange = {};
  const name = draft.name.trim();
  if (name !== plan.name) change.name = name;
  if (draft.description !== plan.description) change.description = draft.description;
  const amount = draft.amount.trim();
  if (amount !== plan.monthly_price.amount) change.monthly_price = { amount, currency: plan.monthly_price.currency };
  const discount = Number(draft.discount.trim());
  if (discount !== plan.annual_discount_percent) change.annual_discount_percent = discount;
  const emojiLimit = Number(draft.emojiLimit.trim());
  if (emojiLimit !== plan.emoji_limit) change.emoji_limit = emojiLimit;
  const rank = Number(draft.rank.trim());
  if (rank !== plan.rank) change.rank = rank;
  if (draft.visibility !== plan.visibility) change.visibility = draft.visibility;
  const position = Number(draft.rolePosition.trim());
  if (position !== plan.role.position) change.role_position = position;
  return change;
}

export type PlanActions = {
  /** Saves a draft of a plan; resolves with why it was refused, or undefined once it is saved. */
  update: (plan: SupporterPlan, draft: PlanDraft) => Promise<PlanFieldError | undefined>;
  /** Retires a plan; resolves with whether it succeeded. */
  retire: (plan: SupporterPlan) => Promise<boolean>;
  /** Why the last retirement failed, in the server's words. */
  error: string | undefined;
  /** What the last change did, for the list's result. */
  status: string | undefined;
  /** Shows and announces what a change made elsewhere (creating in the add panel) did, as the list's result. */
  report: (message: string) => void;
};

/** Changing and retiring the plans of the list. */
export function usePlanActions(): PlanActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const update = useMutation(updateSupporterPlanMutation(api));
  const retire = useMutation(retireSupporterPlanMutation(api));
  const report = (message: string) => {
    setStatus(message);
    notice.inPlace(message);
  };
  return {
    update: async (plan, draft) => {
      setStatus(undefined);
      const checked = checkDraft(draft, t, false);
      if (checked !== undefined) return checked;
      const change = changeOf(plan, draft);
      if (Object.keys(change).length === 0) return undefined;
      try {
        await update.mutateAsync({ id: plan.id, change });
      } catch (error) {
        return refusal(error, "rolePosition");
      }
      await refreshSupporters(api, queryClient);
      report(t("client:admin.supporters.plan.updated", { name: planName({ name: change.name ?? plan.name }) }));
      return undefined;
    },
    retire: async (plan) => {
      setStatus(undefined);
      retire.reset();
      try {
        await retire.mutateAsync(plan.id);
      } catch {
        return false;
      }
      await refreshSupporters(api, queryClient);
      report(t("client:admin.supporters.plan.retired_done", { name: planName(plan) }));
      return true;
    },
    error: retire.isError ? errorMessage(retire.error) : undefined,
    status,
    report: (message) => {
      retire.reset();
      report(message);
    },
  };
}

/** The subscriptions of the filter, newest first. */
export function useSupporterSubscriptions(filter: SupporterSubscriptionFilter): PagedPages<SupporterSubscription> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(supporterSubscriptionsQueryOptions(api, filter)));
}

export type SubscriptionActions = {
  /** Ends a subscription at once; resolves with whether it succeeded. */
  cancel: (subscription: SupporterSubscription) => Promise<boolean>;
  /** Why the last cancellation failed, in the server's words. */
  error: string | undefined;
  /** What the last cancellation did, for the list's StatusMessage. */
  status: string | undefined;
};

/** Ending subscriptions at once, without a refund (ADR-0072 §10). */
export function useSubscriptionActions(): SubscriptionActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const cancel = useMutation(cancelSupporterSubscriptionMutation(api));
  return {
    cancel: async (subscription) => {
      setStatus(undefined);
      cancel.reset();
      try {
        await cancel.mutateAsync(subscription.id);
      } catch {
        return false;
      }
      await refreshSupporters(api, queryClient);
      const message = t("client:admin.supporters.subscription.canceled", {
        plan: planName({ name: subscription.plan_name }),
      });
      setStatus(message);
      notice.inPlace(message);
      return true;
    },
    error: cancel.isError ? errorMessage(cancel.error) : undefined,
    status,
  };
}
