// SPDX-License-Identifier: AGPL-3.0-only
// Saving the form's draft on the server and scheduling it (ADR-0156, web-client/screens/compose.md §4). A save sends
// the whole draft: a new one (POST /drafts) or, for a draft opened from the drafts page, a replacement of the version
// it started from (PUT /drafts/{id} with If-Match). Once saved the form empties, unless the person went on typing
// meanwhile: then the form keeps what they typed, as the continuation of the saved draft. A save that another device's
// save overtook (412) overwrites nothing; the form keeps the draft without its link, so the next save makes a new one.
// Scheduling saves first, then schedules (PUT /drafts/{id}/schedule).
import { useToast } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  cacheDraftChange,
  createDraft,
  type DraftRequest,
  type DraftVersion,
  isDraftConflict,
  scheduleDraft,
  updateDraft,
} from "../api/drafts";
import type { Draft, DraftStore } from "./draftStore";
import type { Attachments } from "./useAttachments";

/** What keeps a draft from being saved, in the order the form checks it. */
type SaveBlocker = "empty" | "uploading" | "upload_failed";

/** The reason of `blocker` that holds first for the draft, or null when it can be saved. */
function saveBlockerOf(draft: Draft, attachments: Attachments): SaveBlocker | null {
  const hasContent =
    draft.text.trim() !== "" ||
    (draft.hasWarning && draft.spoilerText.trim() !== "") ||
    draft.attachments.length > 0 ||
    draft.poll !== null;
  if (!hasContent) return "empty";
  if (attachments.hasFailed) return "upload_failed";
  // Media still being processed can be saved; only an upload without its media ID cannot.
  if (attachments.items.some((item) => item.status === "uploading")) return "uploading";
  return null;
}

/** What a press of "Save draft" left to say: a reason that keeps it from saving, or the failure of the save. */
type SaveOutcome = { kind: "blocked" } | { kind: "failed"; message: string; draft: Draft } | null;

export type ServerDraftForm = {
  /** Saves the draft on the server, or says why it cannot (`message`). */
  save: () => void;
  isSaving: boolean;
  /**
   * What the send bar says after the last press of "Save draft": why the draft cannot be saved, while that holds, or
   * why the save failed, until the draft changes. Null otherwise.
   */
  message: string | null;
  /** Forgets the outcome of the last save, when the person presses the submit button. */
  clearMessage: () => void;
  /**
   * Saves and schedules the draft for `at`; resolves to null once it is scheduled (the form has emptied), or to the
   * localized reason it is not.
   */
  schedule: (at: Date) => Promise<string | null>;
};

/** The localized texts of the save blockers. */
const blockerMessages = {
  empty: "client:drafts.save.blocked.empty",
  uploading: "client:drafts.save.blocked.uploading",
  upload_failed: "client:drafts.save.blocked.upload_failed",
} as const satisfies Record<SaveBlocker, string>;

type SaveVariables = { draft: Draft; body: DraftRequest };

/**
 * Saving and scheduling the draft of `store`. `body` makes the request of the draft as the form shows it (the
 * visibility and the language it follows).
 */
export function useServerDraft(
  store: DraftStore,
  draft: Draft,
  attachments: Attachments,
  body: () => DraftRequest,
): ServerDraftForm {
  const api = useApi();
  const t = useT();
  const intl = useIntl();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [outcome, setOutcome] = useState<SaveOutcome>(null);
  const blocker = saveBlockerOf(draft, attachments);

  const write = ({ draft: saving, body: request }: SaveVariables): Promise<DraftVersion> =>
    saving.saved === null ? createDraft(api, request) : updateDraft(api, saving.saved.id, saving.saved.etag, request);

  /** After a save: the form empties when it still holds what was saved, and continues the saved draft otherwise. */
  const settle = (saving: Draft, version: DraftVersion, empty: boolean) => {
    if (empty && store.getSnapshot().draft === saving) {
      store.replace(null);
      return;
    }
    store.edit({
      saved: { id: version.draft.id, etag: version.etag, scheduledAt: version.draft.scheduled_at ?? null },
    });
  };

  /** A save that another device overtook: the form keeps the draft, unlinked, so that the next save makes a new one. */
  const unlink = () => {
    store.edit({ saved: null });
  };

  const saveMutation = useMutation({
    mutationFn: write,
    onSuccess: async (version, { draft: saving }) => {
      // The lists of the drafts page show the save before it is told (S1).
      await cacheDraftChange(queryClient);
      settle(saving, version, true);
      toast.show({ message: t("client:drafts.saved"), tone: "success" });
    },
    onError: (error) => {
      const message = isDraftConflict(error)
        ? t("client:drafts.save.conflict")
        : t("client:drafts.save.failed", { reason: errorMessage(error) });
      if (isDraftConflict(error)) unlink();
      // The outcome holds for the draft as it is after the failure (an unlinked one is a new snapshot).
      setOutcome({ kind: "failed", message, draft: store.getSnapshot().draft });
    },
  });

  const scheduleMutation = useMutation({
    mutationFn: async ({ draft: saving, body: request, at }: SaveVariables & { at: Date }) => {
      const version = await write({ draft: saving, body: request });
      try {
        const scheduled = await scheduleDraft(api, version.draft.id, at);
        return { version, scheduled };
      } catch (error) {
        // The draft is saved: the form continues it, so that the next try replaces it instead of making another.
        settle(saving, version, false);
        throw error;
      }
    },
  });

  const message =
    outcome?.kind === "blocked"
      ? blocker === null
        ? null
        : t(blockerMessages[blocker])
      : outcome?.kind === "failed" && outcome.draft === draft
        ? outcome.message
        : null;

  return {
    save: () => {
      if (blocker !== null) {
        setOutcome({ kind: "blocked" });
        return;
      }
      setOutcome(null);
      saveMutation.mutate({ draft, body: body() });
    },
    isSaving: saveMutation.isPending,
    message,
    clearMessage: () => {
      setOutcome(null);
    },
    schedule: async (at) => {
      try {
        const { version, scheduled } = await scheduleMutation.mutateAsync({ draft, body: body(), at });
        await cacheDraftChange(queryClient);
        settle(draft, { draft: scheduled, etag: version.etag }, true);
        const date = intl.formatDate(scheduled.scheduled_at ?? at, { dateStyle: "medium", timeStyle: "short" });
        toast.show({ message: t("client:drafts.schedule.scheduled", { date }), tone: "success" });
        return null;
      } catch (error) {
        if (isDraftConflict(error)) {
          unlink();
          return t("client:drafts.save.conflict");
        }
        return t("client:drafts.schedule.failed", { reason: errorMessage(error) });
      }
    },
  };
}
