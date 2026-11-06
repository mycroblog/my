// SPDX-License-Identifier: AGPL-3.0-only
// The pages of the supporters screen (ADR-0072, manage_supporters): the program with enabling and disabling it and who
// may read the supporter list (one form, one save), the state of the payment provider with reconciling with it now
// (ADR-0074 §4, at most once an hour), the plans with creating, changing and retiring one on its own page, and the
// subscriptions with ending one at once on its page. The server keeps every rule (the plan's role, the order of the
// plans, the provider's minimum amount); a refusal is said in the server's words next to the field it is about, and
// what a change did is announced and kept where it shows.
import { type FormActionResult, useNotice } from "@my/ui";
import {
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseInfiniteQuery,
  useSuspenseQuery,
} from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { type PagedPages, toPagedList } from "../../moderation";
import { cacheSavedSettings, instanceSettingsQueryOptions, updateInstanceSettingsMutation } from "../api/settings";
import {
  endSupporterSubscriptionMutation,
  createSupporterPlanMutation,
  maxAnnualDiscount,
  maxEmojiLimit,
  maxPlanRank,
  type PaymentStatus,
  paymentStatusQueryOptions,
  refreshPaymentStatus,
  refreshSupporters,
  retireSupporterPlanMutation,
  runPaymentReconciliationMutation,
  setSupporterProgramMutation,
  type SupporterPlan,
  type SupporterPlanState,
  type SupporterPlanChange,
  supporterPlanNameLimit,
  type SupporterPlanProblemReason,
  supporterPlanProblemReason,
  supporterPlansQueryOptions,
  type SupporterPlanVisibility,
  supporterProgramQueryOptions,
  type SupporterSubscription,
  type SupporterSubscriptionFilter,
  supporterSubscriptionQueryOptions,
  supporterSubscriptionsQueryOptions,
  updateSupporterPlanMutation,
} from "../api/supporters";

type Translate = ReturnType<typeof useT>;

/** A plan for a sentence: its name, written by an administrator in any language, isolated (U+2068 … U+2069). */
export function planName(plan: Pick<SupporterPlan, "name">): string {
  return `⁨${plan.name}⁩`;
}

/** The values of the supporter list's setting (supporter_list_visibility), in the order of the form. */
export const supporterListVisibilities = ["off", "everyone", "signed_in"] as const;

/**
 * The program's form: enabling or disabling it, and who may read the supporter list for a viewer who may set it
 * (`maySetVisibility`, manage_settings). One save sends what changed: the program (PUT /admin/supporter_program), then
 * the setting (PATCH /admin/settings). A refusal (409) is said in the server's words, and the drafts stay.
 */
export function useSupporterProgramForm(maySetVisibility: boolean) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const program = useSuspenseQuery(supporterProgramQueryOptions(api)).data;
  const { data: settings } = useQuery({
    ...instanceSettingsQueryOptions(api),
    enabled: maySetVisibility,
  });
  const savedVisibility = settings?.items.find((setting) => setting.key === "supporter_list_visibility")?.value;
  const [enabledDraft, setEnabledDraft] = useState<boolean | undefined>(undefined);
  const [visibilityDraft, setVisibilityDraft] = useState<string | undefined>(undefined);
  const [result, setResult] = useState<FormActionResult | undefined>(undefined);
  const [isSaving, setIsSaving] = useState(false);
  const setProgram = useMutation(setSupporterProgramMutation(api));
  const setSettings = useMutation(updateInstanceSettingsMutation(api));
  const enabled = enabledDraft ?? program.enabled;
  const visibility = visibilityDraft ?? savedVisibility;
  return {
    program,
    /** Whether the program is enabled in the form. */
    enabled,
    setEnabled: (next: boolean) => {
      setResult(undefined);
      setEnabledDraft(next);
    },
    /** Who may read the supporter list in the form; undefined until the settings are read (or without manage_settings). */
    visibility,
    setVisibility: (next: string) => {
      setResult(undefined);
      setVisibilityDraft(next);
    },
    submit: async (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (isSaving) return;
      const programChanged = enabled !== program.enabled;
      const visibilityChange =
        maySetVisibility && visibility !== undefined && visibility !== savedVisibility ? visibility : undefined;
      if (!programChanged && visibilityChange === undefined) {
        setResult({ tone: "success", message: t("client:admin.supporters.program.unchanged") });
        return;
      }
      setIsSaving(true);
      setResult(undefined);
      try {
        if (programChanged) {
          await setProgram.mutateAsync(enabled);
          await refreshSupporters(api, queryClient);
          setEnabledDraft(undefined);
        }
        if (visibilityChange !== undefined) {
          await cacheSavedSettings(
            queryClient,
            api,
            await setSettings.mutateAsync({ supporter_list_visibility: visibilityChange }),
          );
          setVisibilityDraft(undefined);
        }
        setIsSaving(false);
        setResult({ tone: "success", message: t("client:admin.supporters.program.saved") });
      } catch (error) {
        setIsSaving(false);
        setResult({
          tone: "danger",
          message: t("client:admin.supporters.program.failed", { reason: errorMessage(error) }),
        });
      }
    },
    isSaving,
    /** The result of the last save, for the form's action bar. */
    result,
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
    ...runPaymentReconciliationMutation(api),
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
          : t("client:admin.supporters.payment.reconcile.failed", {
              reason: errorMessage(failure),
            }),
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

/** The plans by rank, those in effect first: every one, or those in `state`. */
export function useSupporterPlans(state?: SupporterPlanState): SupporterPlan[] {
  const api = useApi();
  const plans = useSuspenseQuery(supporterPlansQueryOptions(api)).data.items;
  const inEffect = plans.filter((plan) => !plan.retired);
  const retired = plans.filter((plan) => plan.retired);
  if (state === "in_effect") return inEffect;
  if (state === "retired") return retired;
  return [...inEffect, ...retired];
}

/** One plan, from the list of every plan (there is no GET of one plan); undefined for an unknown id. */
export function useSupporterPlan(id: string): SupporterPlan | undefined {
  const api = useApi();
  return useSuspenseQuery(supporterPlansQueryOptions(api)).data.items.find((plan) => plan.id === id);
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
function draftOfPlan(plan: SupporterPlan): PlanDraft {
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
      message: t("client:admin.supporters.plan.name.invalid", {
        max: String(supporterPlanNameLimit),
      }),
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
    return {
      field: "rank",
      message: t("client:admin.supporters.plan.number.invalid", { max: String(maxPlanRank) }),
    };
  }
  if (!isNew && parseCount(draft.rolePosition, 999) === undefined) {
    return {
      field: "rolePosition",
      message: t("client:admin.supporters.plan.number.invalid", { max: "999" }),
    };
  }
  return undefined;
}

/**
 * The form of a new plan, on its own page. A success opens the new plan's page and tells what it did in a toast, since
 * the form it was sent from is gone.
 */
export function useNewPlanForm() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const notice = useNotice();
  const [draft, setDraft] = useState<PlanDraft>(emptyDraft);
  const [invalid, setInvalid] = useState<PlanFieldError | undefined>(undefined);
  const mutation = useMutation({
    ...createSupporterPlanMutation(api),
    onSuccess: async (plan) => {
      await refreshSupporters(api, queryClient);
      notice.toast({
        message: t("client:admin.supporters.plan.created", { name: planName(plan) }),
        tone: "success",
      });
      await navigate({ to: "/admin/supporters/plans/$id", params: { id: plan.id } });
    },
  });
  return {
    draft,
    change: (change: Partial<PlanDraft>) => {
      setDraft((previous) => ({ ...previous, ...change }));
    },
    /** Creates the plan once its draft is valid. */
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (mutation.isPending) return;
      const checked = checkDraft(draft, t, true);
      setInvalid(checked);
      if (checked !== undefined) {
        mutation.reset();
        return;
      }
      const description = draft.description.trim();
      mutation.mutate({
        role: draft.role.trim(),
        name: draft.name.trim(),
        ...(description === "" ? {} : { description }),
        monthly_price: { amount: draft.amount.trim(), currency: draft.currency.trim() },
        annual_discount_percent: Number(draft.discount.trim()),
        emoji_limit: Number(draft.emojiLimit.trim()),
        rank: Number(draft.rank.trim()),
        visibility: draft.visibility,
      });
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

/**
 * The form that changes a plan, on the plan's page: only what differs is sent, a refusal stays on its field, and
 * what the save did is the result of the form's action bar.
 */
export function usePlanEditForm(plan: SupporterPlan) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<PlanDraft>(() => draftOfPlan(plan));
  const [invalid, setInvalid] = useState<PlanFieldError | undefined>(undefined);
  const [result, setResult] = useState<FormActionResult | undefined>(undefined);
  const mutation = useMutation({
    ...updateSupporterPlanMutation(api),
    onSuccess: async (saved) => {
      await refreshSupporters(api, queryClient);
      setDraft(draftOfPlan(saved));
      setResult({
        tone: "success",
        message: t("client:admin.supporters.plan.updated", { name: planName(saved) }),
      });
    },
  });
  return {
    draft,
    change: (change: Partial<PlanDraft>) => {
      setResult(undefined);
      setDraft((previous) => ({ ...previous, ...change }));
    },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (mutation.isPending) return;
      setResult(undefined);
      const checked = checkDraft(draft, t, false);
      setInvalid(checked);
      if (checked !== undefined) {
        mutation.reset();
        return;
      }
      const change = changeOf(plan, draft);
      if (Object.keys(change).length === 0) {
        mutation.reset();
        setResult({ tone: "success", message: t("client:admin.supporters.plan.unchanged") });
        return;
      }
      mutation.mutate({ id: plan.id, change });
    },
    isPending: mutation.isPending,
    /** Why the change was refused, next to its field; the draft stays. */
    error: invalid ?? (mutation.isError ? refusal(mutation.error, "rolePosition") : undefined),
    result,
  };
}

/** Retiring a plan, on its page; what it did stays in the page's StatusMessage. */
export function usePlanRetire(plan: SupporterPlan) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const retire = useMutation(retireSupporterPlanMutation(api));
  return {
    /** Retires the plan; resolves with whether it succeeded. */
    retire: async (): Promise<boolean> => {
      setStatus(undefined);
      retire.reset();
      try {
        await retire.mutateAsync(plan.id);
      } catch {
        return false;
      }
      await refreshSupporters(api, queryClient);
      const message = t("client:admin.supporters.plan.retired_done", { name: planName(plan) });
      setStatus(message);
      notice.inPlace(message);
      return true;
    },
    /** Why the retirement failed, in the server's words. */
    error: retire.isError ? errorMessage(retire.error) : undefined,
    status,
  };
}

/** The subscriptions of the filter, newest first. */
export function useSupporterSubscriptions(filter: SupporterSubscriptionFilter): PagedPages<SupporterSubscription> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(supporterSubscriptionsQueryOptions(api, filter)));
}

/** One subscription, for its page. */
export function useSupporterSubscription(id: string): SupporterSubscription {
  const api = useApi();
  return useSuspenseQuery(supporterSubscriptionQueryOptions(api, id)).data;
}

export type SubscriptionActions = {
  /** Ends a subscription at once; resolves with whether it succeeded. */
  cancel: (subscription: SupporterSubscription) => Promise<boolean>;
  /** Why the last cancellation failed, in the server's words. */
  error: string | undefined;
  /** What the last cancellation did, for the page's StatusMessage. */
  status: string | undefined;
};

/** Ending subscriptions at once, without a refund unless still pending (ADR-0072 §10). */
export function useSubscriptionActions(): SubscriptionActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const cancel = useMutation(endSupporterSubscriptionMutation(api));
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
