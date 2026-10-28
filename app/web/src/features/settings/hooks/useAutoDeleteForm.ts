// SPDX-License-Identifier: AGPL-3.0-only
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  type AutoDeletePolicy,
  autoDeletePolicyQueryOptions,
  cacheSavedAutoDeletePolicy,
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
 */
export function useAutoDeleteForm() {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const { data: saved } = useSuspenseQuery(autoDeletePolicyQueryOptions(api));
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

  return {
    policy,
    /** Whether the form holds edits that are not saved: leaving the page asks first. */
    hasChanges: !samePolicy(policy, saved),
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
    ageChoices: withSaved(ageChoices, policy.min_age_days),
    boostChoices: withSaved(thresholdChoices, policy.keep_min_boosts),
    favouriteChoices: withSaved(thresholdChoices, policy.keep_min_favourites),
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (save.isPending) return;
      save.mutate(policy);
    },
    isSaving: save.isPending,
    /** Why the last save failed, localized; nothing was changed and the form keeps the edits. */
    saveError: save.isError ? errorMessage(save.error) : undefined,
    /** What the last save did, for the region that reports it. */
    status,
  };
}
