// SPDX-License-Identifier: AGPL-3.0-only
// The publication settings next to the screens whose data they publish (audit A8 of #1638): how much of the running
// costs is published (FR-903) and shown to the holders of view_finance (ADR-0070 §5) and how much of the income is
// published (FR-917), on the costs screen; who may read the supporter list (ADR-0071 §4), on the supporters screen.
// They stay instance settings (manage_settings): the form reads and saves them with the server settings' API, sends
// the ones that changed together, and each change goes to the audit log (FR-704).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  cacheSavedSettings,
  type InstanceSettingKey,
  instanceSettingsQueryOptions,
  updateInstanceSettingsMutation,
} from "../api/settings";

/** The publication settings of the costs screen, in the order of its form. */
export const financePublicationKeys = [
  "finance_visibility",
  "finance_viewer_item_names",
  "finance_viewer_item_memos",
  "finance_viewer_income",
  "income_visibility",
] as const satisfies readonly InstanceSettingKey[];

/** The publication setting of the supporters screen. */
export const supporterPublicationKeys = ["supporter_list_visibility"] as const satisfies readonly InstanceSettingKey[];

export type PublicationKey = (typeof financePublicationKeys)[number] | (typeof supporterPublicationKeys)[number];

/** A form of the publication settings `keys`: loading until the settings are read. */
export function usePublicationSettings(keys: readonly PublicationKey[]) {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const { data } = useQuery(instanceSettingsQueryOptions(api));
  const saved = new Map((data?.items ?? []).map((setting) => [setting.key, setting.value]));
  const [drafts, setDrafts] = useState<Readonly<Partial<Record<PublicationKey, string>>>>({});
  const [status, setStatus] = useState<string | undefined>(undefined);

  const savedValue = (key: PublicationKey) => saved.get(key) ?? "";
  const valueOf = (key: PublicationKey) => drafts[key] ?? savedValue(key);
  const changes = Object.fromEntries(
    keys.filter((key) => valueOf(key) !== savedValue(key)).map((key) => [key, valueOf(key)]),
  );

  const save = useMutation({
    ...updateInstanceSettingsMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async (settings) => {
      // GET /instance and the totals say what is published; they are fetched again before the form says it is done.
      await cacheSavedSettings(queryClient, api, settings);
      setDrafts({});
      setStatus(t("client:admin.publication.saved"));
    },
  });

  return {
    /** Whether the settings are read: the form shows once they are. */
    ready: data !== undefined,
    /** The value of a setting in the form: the draft, else what is saved. */
    value: valueOf,
    /** Changes a setting in the form; nothing is sent until the form is saved. */
    change: (key: PublicationKey, value: string) => {
      setStatus(undefined);
      setDrafts({ ...drafts, [key]: value });
    },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (save.isPending) return;
      if (Object.keys(changes).length === 0) {
        setStatus(t("client:admin.publication.unchanged"));
        return;
      }
      save.mutate(changes);
    },
    isSaving: save.isPending,
    /** Why the last save failed, localized; nothing was changed and the form keeps the drafts. */
    saveError: save.isError ? errorMessage(save.error) : undefined,
    /** What the last save did, for the form's action bar. */
    status,
  };
}
