// SPDX-License-Identifier: AGPL-3.0-only
// The server's descriptions (FR-705): the short one, a paragraph that the about page and other servers' directories
// show, and the full one. The administrator writes each once, in any language, and every language shows it as it
// is. Saving sends only the texts that changed, each an audited change.
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { instanceTextsQueryOptions } from "../api/documents";
import {
  cacheSavedTexts,
  descriptionLimit,
  type InstanceTextChange,
  setInstanceTextsMutation,
  shortDescriptionLimit,
} from "../api/settings";
import { characterCount } from "./languages";

type DescriptionKey = Extract<InstanceTextChange["key"], "short_description" | "description">;

/** The most characters of each description. */
export const descriptionLimits: Readonly<Record<DescriptionKey, number>> = {
  short_description: shortDescriptionLimit,
  description: descriptionLimit,
};

/** One description: what the field holds and how to change it. */
export type DescriptionField = {
  key: DescriptionKey;
  value: string;
  onChange: (value: string) => void;
};

const descriptionKeys = ["short_description", "description"] as const;

export function useServerDescriptions() {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const { data } = useSuspenseQuery(instanceTextsQueryOptions(api));
  const savedOf = (key: DescriptionKey) => data.items.find((text) => text.key === key)?.body ?? "";
  const [drafts, setDrafts] = useState<Readonly<Partial<Record<DescriptionKey, string>>>>({});
  const [status, setStatus] = useState<string | undefined>(undefined);

  const valueOf = (key: DescriptionKey) => drafts[key] ?? savedOf(key);
  const changes = descriptionKeys
    .filter((key) => valueOf(key) !== savedOf(key))
    .map((key) => ({ key, body: valueOf(key) }));
  const tooLong = changes.some((change) => characterCount(change.body) > descriptionLimits[change.key]);

  const save = useMutation({
    ...setInstanceTextsMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async () => {
      // The about page shows the new descriptions once GET /instance is fetched again (the button stays pending
      // until then); the audit log has the change when it is opened next (cacheSavedTexts).
      await cacheSavedTexts(queryClient);
      setDrafts({});
      setStatus(t("client:admin.settings.descriptions.saved"));
    },
    // One description may have been saved before the other failed: the form compares with what is saved now, and
    // keeps the other draft to send again (web-client/README.md E2).
    onError: () => cacheSavedTexts(queryClient),
  });

  const field = (key: DescriptionKey): DescriptionField => ({
    key,
    value: valueOf(key),
    onChange: (next) => {
      setDrafts({ ...drafts, [key]: next });
    },
  });

  return {
    short: field("short_description"),
    full: field("description"),
    /** Whether a field holds a description that is not saved. */
    hasChanges: changes.length > 0,
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      // A description over its limit shows why under its field (its count); the server would refuse it.
      if (tooLong || save.isPending) return;
      if (changes.length === 0) {
        setStatus(t("client:admin.settings.descriptions.unchanged"));
        return;
      }
      save.mutate(changes);
    },
    isSaving: save.isPending,
    /** Why the last save failed, localized; the server names the invalid property in it. */
    saveError: save.isError ? errorMessage(save.error) : undefined,
    /** What the last save did, for the region that reports it. */
    status,
  };
}
