// SPDX-License-Identifier: AGPL-3.0-only
// The viewer's private note on an account (FR-310): writing it, or removing it with an empty note. The note is part of
// the relationship (server data in the query cache, web-client/README.md S2); a save stores the relationship the
// server returns.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { isolate } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { saveNote, storeNote } from "../api/relationship";

export type AccountNoteState = {
  /** Saves `note` (trimmed); an empty one removes the note. */
  save: (note: string) => void;
  isPending: boolean;
  /** Why the last save failed, localized; undefined when it did not. */
  error: string | undefined;
};

/**
 * Returns the way to save the viewer's note on the account `accountId`, named `name` in the UI. A saved note is
 * announced in a toast, since the dialog it was written in closes (`onSaved`), and a failure stays in the dialog.
 */
export function useAccountNote(accountId: string, name: string, onSaved: () => void): AccountNoteState {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const mutation = useMutation({
    mutationFn: (note: string) => saveNote(api, accountId, note),
    onSuccess: (relationship) => {
      storeNote(queryClient, api, relationship);
      const values = { name: isolate(name) };
      notice.toast({
        message:
          (relationship.note ?? "") === ""
            ? t("client:profile.note.removed", values)
            : t("client:profile.note.saved", values),
        tone: "success",
      });
      onSaved();
    },
  });
  return {
    save: (note) => {
      if (!mutation.isPending) mutation.mutate(note.trim());
    },
    isPending: mutation.isPending,
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
  };
}
