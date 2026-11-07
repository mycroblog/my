// SPDX-License-Identifier: AGPL-3.0-only
import type { FormActionResult } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { type Translate, useT } from "../../../lib/i18n";
import {
  type AutoDeletePolicy,
  type AutoDeletePolicyProperty,
  autoDeletePolicyProperties,
  autoDeletePolicyQueryOptions,
  type AutoDeleteServerLimit,
  cacheSavedAutoDeletePolicy,
  policyOf,
  updateAutoDeletePolicyMutation,
} from "../api/autoDelete";
import type { Visibility } from "../api/me";

/** The ages after which posts may be deleted, in days: a week to two years. */
const ageChoices: readonly number[] = [7, 14, 30, 60, 90, 180, 365, 730];

/** The thresholds of boosts and favourites that keep a post; 0 keeps none for them. */
const thresholdChoices: readonly number[] = [0, 1, 2, 5, 10, 20, 50, 100];

/** The visibilities in the order of ADR-0017, widest first, which the API keeps its list in too. */
const visibilityOrder: readonly Visibility[] = ["public", "local", "unlisted", "private", "direct"];

/** The boolean exceptions of a policy. */
export type AutoDeleteFlag = "keep_pinned" | "keep_bookmarked" | "keep_favourited" | "keep_media" | "keep_polls";

/** Which allowance of the server maximum lets each boolean exception keep posts. */
const flagAllowances = {
  keep_pinned: "allow_pinned",
  keep_bookmarked: "allow_bookmarked",
  keep_favourited: "allow_favourited",
  keep_media: "allow_media",
  keep_polls: "allow_polls",
} as const satisfies Record<AutoDeleteFlag, keyof AutoDeleteServerLimit>;

/** Whether the server maximum lets `flag` keep posts; true without a maximum. */
export function flagAllowed(limit: AutoDeleteServerLimit | undefined, flag: AutoDeleteFlag): boolean {
  return limit === undefined || limit[flagAllowances[flag]];
}

/**
 * The properties of `policy` that the server maximum does not allow (ADR-0151 §3), each with why: the automatic
 * deletion turned off, an age over the maximum, or an exception the operator does not allow. Empty without a maximum.
 */
function limitErrors(
  policy: AutoDeletePolicy,
  limit: AutoDeleteServerLimit | undefined,
  t: Translate,
): Partial<Record<AutoDeletePolicyProperty, string>> {
  if (limit === undefined) return {};
  const errors: Partial<Record<AutoDeletePolicyProperty, string>> = {};
  const exception = t("client:settings.auto_delete.limit.error.exception");
  if (!policy.enabled) errors.enabled = t("client:settings.auto_delete.limit.error.enabled");
  if (policy.min_age_days > limit.max_age_days)
    errors.min_age_days = t("client:settings.auto_delete.limit.error.age", { days: limit.max_age_days });
  for (const flag of Object.keys(flagAllowances) as AutoDeleteFlag[]) {
    if (policy[flag] && !flagAllowed(limit, flag)) errors[flag] = exception;
  }
  if (policy.keep_visibilities.length > 0 && !limit.allow_visibilities) errors.keep_visibilities = exception;
  if (policy.keep_min_boosts > 0 && !limit.allow_thresholds) errors.keep_min_boosts = exception;
  if (policy.keep_min_favourites > 0 && !limit.allow_thresholds) errors.keep_min_favourites = exception;
  return errors;
}

/**
 * The property that a 422 of the save names in its detail ("min_age_days is not valid."), the longest one that stands
 * there as a word; undefined for another error.
 */
function refusedProperty(error: unknown): AutoDeletePolicyProperty | undefined {
  if (!(error instanceof ApiError) || error.status !== 422 || error.detail === undefined) return undefined;
  const detail = error.detail;
  return autoDeletePolicyProperties
    .filter((property) => new RegExp(`(?<![a-z_])${property}(?![a-z_])`, "u").test(detail))
    .toSorted((a, b) => b.length - a.length)[0];
}

/** `choices` with `saved` among them when it is not one of them (a value set through the API), in order. */
function withSaved(choices: readonly number[], saved: number): number[] {
  return [...new Set([...choices, saved])].toSorted((a, b) => a - b);
}

function samePolicy(a: AutoDeletePolicy, b: AutoDeletePolicy): boolean {
  return (
    a.enabled === b.enabled &&
    a.min_age_days === b.min_age_days &&
    a.keep_pinned === b.keep_pinned &&
    a.keep_bookmarked === b.keep_bookmarked &&
    a.keep_favourited === b.keep_favourited &&
    a.keep_media === b.keep_media &&
    a.keep_polls === b.keep_polls &&
    a.keep_min_boosts === b.keep_min_boosts &&
    a.keep_min_favourites === b.keep_min_favourites &&
    a.keep_visibilities.length === b.keep_visibilities.length &&
    a.keep_visibilities.every((v) => b.keep_visibilities.includes(v))
  );
}

/**
 * The form of the automatic deletion of old posts (FR-118): the policy being edited, and saving it as a whole. The
 * form starts from the saved policy and keeps the edits until they are saved; leaving it with unsaved edits asks first.
 * While the server maximum applies to the user (ADR-0151 §3), the form offers only what the maximum allows: it keeps
 * the deletion on, offers ages up to the maximum, and locks the exceptions the operator does not allow. A saved value
 * beyond the maximum stays, marked on its field, and the form is not sent until it is changed.
 */
export function useAutoDeleteForm() {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const { data: settings } = useSuspenseQuery(autoDeletePolicyQueryOptions(api));
  const saved = policyOf(settings);
  const limit = settings.server_limit;
  const [draft, setDraft] = useState<AutoDeletePolicy | undefined>(undefined);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const policy = draft ?? saved;

  const save = useMutation({
    ...updateAutoDeletePolicyMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: (answered) => {
      cacheSavedAutoDeletePolicy(api, queryClient, answered);
      setDraft(undefined);
      setStatus(
        answered.enabled
          ? t("client:settings.auto_delete.saved_on", { days: answered.min_age_days })
          : t("client:settings.auto_delete.saved_off"),
      );
    },
  });

  const change = (next: Partial<AutoDeletePolicy>) => {
    setDraft({ ...policy, ...next });
  };

  const hasChanges = !samePolicy(policy, saved);
  const overLimit = limitErrors(policy, limit, t);
  const refused = save.isError ? refusedProperty(save.error) : undefined;
  /** Why a field's value cannot be saved: beyond the maximum, or refused by the last save; undefined when it can. */
  const fieldError = (property: AutoDeletePolicyProperty): string | undefined => {
    const over = overLimit[property];
    if (over !== undefined) return over;
    if (property === refused) return errorMessage(save.error);
    return undefined;
  };
  const maxAge = limit?.max_age_days;
  let result: FormActionResult | undefined;
  if (save.isError)
    result = {
      tone: "danger",
      message: t("client:settings.auto_delete.save_failed", { reason: errorMessage(save.error) }),
    };
  else if (status !== undefined && !hasChanges) result = { tone: "success", message: status };

  return {
    policy,
    /** The server maximum as it applies to the user, with the effective policy; undefined when none applies. */
    limit,
    /** Why a field's value cannot be saved; undefined when it can. */
    fieldError,
    /**
     * Whether the maximum locks a field where it is: the deletion that is on stays on, and an exception the operator
     * does not allow cannot be turned on. A value beyond the maximum is not locked, so that it can be changed.
     */
    locked: {
      enabled: limit !== undefined && policy.enabled,
      flag: (flag: AutoDeleteFlag) => !flagAllowed(limit, flag) && !policy[flag],
      visibility: (visibility: Visibility) =>
        limit !== undefined && !limit.allow_visibilities && !policy.keep_visibilities.includes(visibility),
      minBoosts: limit !== undefined && !limit.allow_thresholds && policy.keep_min_boosts === 0,
      minFavourites: limit !== undefined && !limit.allow_thresholds && policy.keep_min_favourites === 0,
    },
    /** Whether the form holds edits that are not saved: leaving the page asks first. */
    hasChanges,
    setEnabled: (enabled: boolean) => {
      change({ enabled });
    },
    setMinAgeDays: (days: number) => {
      change({ min_age_days: days });
    },
    setFlag: (flag: AutoDeleteFlag, selected: boolean) => {
      change({ [flag]: selected });
    },
    setKeepVisibility: (visibility: Visibility, selected: boolean) => {
      const kept = new Set(policy.keep_visibilities);
      if (selected) kept.add(visibility);
      else kept.delete(visibility);
      change({ keep_visibilities: visibilityOrder.filter((v) => kept.has(v)) });
    },
    setMinBoosts: (count: number) => {
      change({ keep_min_boosts: count });
    },
    setMinFavourites: (count: number) => {
      change({ keep_min_favourites: count });
    },
    /** The ages to choose from, with the saved one when it is not among them. */
    ageChoices: withSaved(
      maxAge === undefined ? ageChoices : [...ageChoices.filter((days) => days < maxAge), maxAge],
      policy.min_age_days,
    ),
    boostChoices: withSaved(thresholdChoices, policy.keep_min_boosts),
    favouriteChoices: withSaved(thresholdChoices, policy.keep_min_favourites),
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (save.isPending) return;
      // The browser keeps the form from being sent while a field beyond the maximum is marked (React Aria's native
      // validation moves the focus to it); this keeps a policy the server would refuse from being sent at all.
      if (Object.keys(overLimit).length > 0) return;
      save.mutate(policy);
    },
    isSaving: save.isPending,
    /**
     * The result of the last save beside the save button (web-client/16-look-states-and-wording.md G24): the policy it
     * saved, until the form changes again; or why it failed, when nothing was changed and the form keeps the edits.
     */
    result,
  };
}
