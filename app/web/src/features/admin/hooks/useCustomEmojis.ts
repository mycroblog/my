// SPDX-License-Identifier: AGPL-3.0-only
// The custom emoji of the admin screen (ADR-0051, manage_custom_emojis): the list under a filter, registering a local
// emoji with an image file, changing its settings, replacing its image, deleting it, and enabling, disabling or
// copying a remote one. What each change did is announced and kept in a StatusMessage; a failure stays next to its cause.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type Translate, useT } from "../../../lib/i18n";
import { type PagedPages, toPagedList } from "../../moderation";
import { MediaProcessingError } from "../../settings";
import {
  type AdminCustomEmoji,
  adminCustomEmojisQueryOptions,
  cacheChangedCustomEmoji,
  cacheCreatedCustomEmoji,
  cacheDeletedCustomEmoji,
  copyCustomEmojiMutation,
  createCustomEmojiMutation,
  type CustomEmojiChange,
  type CustomEmojiFilter,
  deleteCustomEmojiMutation,
  replaceCustomEmojiImageMutation,
  updateCustomEmojiMutation,
} from "../api/customEmojis";

/**
 * An emoji for a sentence: its `:shortcode:`, with `@domain` for a remote one, isolated left to right from the text
 * around it (U+2066 LEFT-TO-RIGHT ISOLATE … U+2069 POP DIRECTIONAL ISOLATE).
 */
export function emojiInText(emoji: Pick<AdminCustomEmoji, "shortcode" | "domain">): string {
  return `⁦:${emoji.shortcode}:${emoji.domain === undefined ? "" : `@${emoji.domain}`}⁩`;
}

/** The other names typed in one field, separated by spaces or commas. */
export function parseAliases(text: string): string[] {
  return [...new Set(text.split(/[\s,、，]+/u).filter((alias) => alias !== ""))];
}

/** Why a change failed, localized: the server's reason, or that it could not process the image. */
function failure(t: Translate, error: unknown): string {
  return error instanceof MediaProcessingError ? t("client:admin.custom_emojis.image_failed") : errorMessage(error);
}

/** The custom emoji under `filter`, newest first. */
export function useAdminCustomEmojis(filter: CustomEmojiFilter): PagedPages<AdminCustomEmoji> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(adminCustomEmojisQueryOptions(api, filter)));
}

/** The form of a new local emoji, filled in by the administrator. */
type Draft = { file: File | null; shortcode: string; category: string; aliases: string; visibleInPicker: boolean };

const emptyDraft: Draft = { file: null, shortcode: "", category: "", aliases: "", visibleInPicker: true };

export type CustomEmojiForm = {
  draft: Draft;
  change: (change: Partial<Draft>) => void;
  /** Uploads the image and registers the emoji, once the browser has checked the fields and an image is chosen. */
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** That no image is chosen, after a submit without one. */
  missingImage: boolean;
  /** Why registering failed, localized; the draft stays. */
  error: string | undefined;
  /** What the last registration did, for the form's StatusMessage. */
  status: string | undefined;
};

/** The form of a new local emoji. A success empties it and says so once the lists and the audit log are fetched again. */
export function useCustomEmojiForm(): CustomEmojiForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [missingImage, setMissingImage] = useState(false);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation({
    ...createCustomEmojiMutation(api),
    onSuccess: async (emoji) => {
      await cacheCreatedCustomEmoji(queryClient);
      setDraft(emptyDraft);
      setStatus(t("client:admin.custom_emojis.created", { emoji: emojiInText(emoji) }));
    },
  });
  return {
    draft,
    change: (change) => {
      setDraft((previous) => ({ ...previous, ...change }));
      if (change.file !== undefined) setMissingImage(false);
    },
    submit: (event) => {
      event.preventDefault();
      if (draft.file === null) {
        setMissingImage(true);
        return;
      }
      setStatus(undefined);
      const category = draft.category.trim();
      mutation.mutate({
        file: draft.file,
        shortcode: draft.shortcode.trim(),
        aliases: parseAliases(draft.aliases),
        visible_in_picker: draft.visibleInPicker,
        ...(category === "" ? {} : { category }),
      });
    },
    isPending: mutation.isPending,
    missingImage,
    error: mutation.isError ? failure(t, mutation.error) : undefined,
    status,
  };
}

export type CustomEmojiActions = {
  /** Changes an emoji's settings; resolves with whether it succeeded, the failure in `error`. */
  update: (emoji: AdminCustomEmoji, change: CustomEmojiChange) => Promise<boolean>;
  /** Enables or disables an emoji (a remote one has nothing else to change). */
  setEnabled: (emoji: AdminCustomEmoji, enabled: boolean) => void;
  /** Uploads a new image of a local emoji and makes it its image. */
  replaceImage: (emoji: AdminCustomEmoji, file: File) => void;
  /** Deletes a local emoji; resolves with whether it succeeded. */
  remove: (emoji: AdminCustomEmoji) => Promise<boolean>;
  /** Copies a remote emoji into a local one named `shortcode`; resolves with whether it succeeded. */
  copy: (emoji: AdminCustomEmoji, shortcode: string) => Promise<boolean>;
  /** The emoji being changed. */
  pendingId: string | undefined;
  /** Why the last change failed, localized, until the next change. */
  error: string | undefined;
  /** What the last change did, for the list's StatusMessage. */
  status: string | undefined;
};

/**
 * Changing the emoji of the list. `onDeleted(id)` is called once a deleted emoji is about to leave the list (to keep
 * focus, lib/useFocusAfterRemoval); a changed emoji keeps its row, which shows the answer.
 */
export function useCustomEmojiActions(onDeleted: (id: string) => void): CustomEmojiActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const report = (message: string) => {
    setStatus(message);
    notice.inPlace(message);
  };
  const update = useMutation(updateCustomEmojiMutation(api));
  const replace = useMutation(replaceCustomEmojiImageMutation(api));
  const remove = useMutation(deleteCustomEmojiMutation(api));
  const copy = useMutation(copyCustomEmojiMutation(api));
  const pending = [update, replace, copy].find((mutation) => mutation.isPending);
  const pendingId = remove.isPending ? remove.variables : pending?.variables.id;
  const failed = [update, replace, remove, copy].find((mutation) => mutation.isError);
  const changed = async (emoji: AdminCustomEmoji, message: string) => {
    await cacheChangedCustomEmoji(queryClient, emoji);
    report(message);
  };
  return {
    update: async (emoji, change) => {
      replace.reset();
      remove.reset();
      copy.reset();
      try {
        const saved = await update.mutateAsync({ id: emoji.id, change });
        await changed(saved, t("client:admin.custom_emojis.updated", { emoji: emojiInText(saved) }));
        return true;
      } catch {
        return false;
      }
    },
    setEnabled: (emoji, enabled) => {
      replace.reset();
      remove.reset();
      copy.reset();
      update.mutate(
        { id: emoji.id, change: { enabled } },
        {
          onSuccess: (saved) => {
            void changed(
              saved,
              t(enabled ? "client:admin.custom_emojis.enabled_status" : "client:admin.custom_emojis.disabled_status", {
                emoji: emojiInText(saved),
              }),
            );
          },
        },
      );
    },
    replaceImage: (emoji, file) => {
      update.reset();
      remove.reset();
      copy.reset();
      replace.mutate(
        { id: emoji.id, file },
        {
          onSuccess: (saved) => {
            void changed(saved, t("client:admin.custom_emojis.image_replaced", { emoji: emojiInText(saved) }));
          },
        },
      );
    },
    remove: async (emoji) => {
      update.reset();
      replace.reset();
      copy.reset();
      try {
        await remove.mutateAsync(emoji.id);
      } catch {
        return false;
      }
      onDeleted(emoji.id);
      await cacheDeletedCustomEmoji(queryClient, emoji.id);
      report(t("client:admin.custom_emojis.deleted", { emoji: emojiInText(emoji) }));
      return true;
    },
    copy: async (emoji, shortcode) => {
      update.reset();
      replace.reset();
      remove.reset();
      // The remote one's shortcode is the server's default; another one is sent.
      const named = shortcode === emoji.shortcode ? undefined : shortcode;
      let created: AdminCustomEmoji;
      try {
        created = await copy.mutateAsync({ id: emoji.id, shortcode: named });
      } catch {
        return false;
      }
      await cacheCreatedCustomEmoji(queryClient);
      report(t("client:admin.custom_emojis.copied", { emoji: emojiInText(emoji), copy: emojiInText(created) }));
      return true;
    },
    pendingId,
    error: failed === undefined ? undefined : failure(t, failed.error),
    status,
  };
}
