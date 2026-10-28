// SPDX-License-Identifier: AGPL-3.0-only
// The server settings screen: the instance's display name, which names the instance to people in place of its
// domain (the navigation, the tab title, the sign-in pages, emails, link previews). The administrator writes it once,
// in any language, and every language shows it as it is; with no name the domain shows. Handles and links keep the
// domain.
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { instanceQueryOptions, instanceTextsQueryOptions } from "../api/documents";
import { cacheSavedTexts, instanceNameLimit, setInstanceTextsMutation } from "../api/settings";
import { characterCount } from "./languages";

export function useServerSettings() {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const { data } = useSuspenseQuery(instanceTextsQueryOptions(api));
  const { data: instance } = useSuspenseQuery(instanceQueryOptions(api));
  const saved = data.items.find((text) => text.key === "name")?.body ?? "";
  const [draft, setDraft] = useState<string | undefined>(undefined);
  const [status, setStatus] = useState<string | undefined>(undefined);

  const value = draft ?? saved;
  const error = (() => {
    if (characterCount(value) > instanceNameLimit)
      return t("client:admin.settings.name.too_long", { max: String(instanceNameLimit) });
    if (value !== "" && value.trim() === "") return t("client:admin.settings.name.blank");
    return undefined;
  })();

  const save = useMutation({
    ...setInstanceTextsMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async () => {
      // The navigation and the tab title show the new name once GET /instance is fetched again (the button
      // stays pending until then); the audit log has the change when it is opened next (cacheSavedTexts).
      await cacheSavedTexts(queryClient);
      setDraft(undefined);
      setStatus(t("client:admin.settings.name.saved"));
    },
  });

  return {
    /** The domain that names the instance without a name, and that handles and links use. */
    domain: instance.domain,
    /** The name: what the field holds, how to change it, and why it cannot be saved (localized; undefined when it can). */
    field: { value, onChange: setDraft, error },
    /** Whether the field holds a name that is not saved. */
    hasChanges: value !== saved,
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (error !== undefined || save.isPending) return;
      if (value === saved) {
        setStatus(t("client:admin.settings.name.unchanged"));
        return;
      }
      save.mutate([{ key: "name", body: value }]);
    },
    isSaving: save.isPending,
    /** Why the last save failed, localized; the server names the invalid property in it. */
    saveError: save.isError ? errorMessage(save.error) : undefined,
    /** What the last save did, for the region that reports it. */
    status,
  };
}
