// SPDX-License-Identifier: AGPL-3.0-only
// The custom emoji of the admin screen (ADR-0051, manage_custom_emojis): the list under a filter, one emoji, registering
// a local emoji with an image file, changing its settings, replacing its image, deleting it, enabling and disabling any
// emoji, and copying a remote one. A change that keeps its page says what it did in a StatusMessage next to its control
// and announces it; registering and copying open the new emoji's page, and deleting goes back to the list with a toast.
// A failure stays next to its cause.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseInfiniteQuery, useSuspenseQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type Translate, useT } from "../../../lib/i18n";
import { type PagedPages, toPagedList } from "../../moderation";
import { MediaProcessingError } from "../../settings";
import {
  type AdminCustomEmoji,
  adminCustomEmojiQueryOptions,
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
  setCustomEmojiEnabledMutation,
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

/** One emoji, which its route has loaded. */
export function useAdminCustomEmoji(id: string): AdminCustomEmoji {
  const api = useApi();
  return useSuspenseQuery(adminCustomEmojiQueryOptions(api, id)).data;
}

/** The form of a new local emoji, filled in by the administrator. */
type Draft = {
  file: File | null;
  shortcode: string;
  category: string;
  aliases: string;
  visibleInPicker: boolean;
};

const emptyDraft: Draft = {
  file: null,
  shortcode: "",
  category: "",
  aliases: "",
  visibleInPicker: true,
};

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
};

/**
 * The form of a new local emoji. A success opens the new emoji's page, whose heading shows what it did
 * (web-client/16-look-states-and-wording.md G24), once the lists and the audit log are marked stale.
 */
export function useCustomEmojiForm(): CustomEmojiForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [missingImage, setMissingImage] = useState(false);
  const mutation = useMutation({
    ...createCustomEmojiMutation(api),
    onSuccess: async (emoji) => {
      await cacheCreatedCustomEmoji(queryClient, emoji);
      await navigate({ to: "/admin/custom-emojis/$id", params: { id: emoji.id } });
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
      if (mutation.isPending) return;
      if (draft.file === null) {
        setMissingImage(true);
        return;
      }
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
  };
}

/** The result of a change that keeps its page: what it did, or why it failed, next to its control. */
type CustomEmojiChangeResult = {
  /** What the last change did, for the StatusMessage next to the control. */
  status: string | undefined;
  /** Why the last change failed, localized, until the next change. */
  error: string | undefined;
};

export type CustomEmojiEnabling = CustomEmojiChangeResult & {
  /** Enables or disables an emoji (a remote one has nothing else to change). */
  setEnabled: (emoji: AdminCustomEmoji, enabled: boolean) => void;
  /** The emoji being enabled or disabled. */
  pendingId: string | undefined;
};

/** Enabling and disabling emoji, from a row of the list or from an emoji's page; the emoji shows the answer at once. */
export function useCustomEmojiEnabling(): CustomEmojiEnabling {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const toggle = useMutation({
    ...setCustomEmojiEnabledMutation(api),
    // Awaited by the mutation: a failure here leaves it in its error state, which the control shows.
    onSuccess: async (saved, { enabled }) => {
      await cacheChangedCustomEmoji(queryClient, saved);
      const message = t(
        enabled ? "client:admin.custom_emojis.enabled_status" : "client:admin.custom_emojis.disabled_status",
        { emoji: emojiInText(saved) },
      );
      setStatus(message);
      notice.inPlace(message);
    },
  });
  return {
    setEnabled: (emoji, enabled) => {
      setStatus(undefined);
      toggle.mutate({ id: emoji.id, enabled });
    },
    pendingId: toggle.isPending ? toggle.variables.id : undefined,
    status,
    error: toggle.isError ? failure(t, toggle.error) : undefined,
  };
}

export type CustomEmojiSettingsForm = {
  category: string;
  aliases: string;
  visibleInPicker: boolean;
  change: (change: Partial<Pick<CustomEmojiSettingsForm, "category" | "aliases" | "visibleInPicker">>) => void;
  /** Saves the settings, once the browser has checked the fields. */
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** What saving did, or why it failed, for the form's action bar. */
  result: CustomEmojiChangeResult;
};

/** The settings of a local emoji on its page: its category, its other names and whether the picker shows it. */
export function useCustomEmojiSettingsForm(emoji: AdminCustomEmoji): CustomEmojiSettingsForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [draft, setDraft] = useState(() => ({
    category: emoji.category ?? "",
    aliases: emoji.aliases.join(" "),
    visibleInPicker: emoji.visible_in_picker,
  }));
  const [status, setStatus] = useState<string | undefined>(undefined);
  const update = useMutation({
    ...updateCustomEmojiMutation(api),
    // Awaited by the mutation: a failure here leaves it in its error state, which the control shows.
    onSuccess: async (saved) => {
      await cacheChangedCustomEmoji(queryClient, saved);
      const message = t("client:admin.custom_emojis.updated", {
        emoji: emojiInText(saved),
      });
      setStatus(message);
      notice.inPlace(message);
    },
  });
  const change: CustomEmojiChange = {
    category: draft.category.trim(),
    aliases: parseAliases(draft.aliases),
    visible_in_picker: draft.visibleInPicker,
  };
  return {
    ...draft,
    change: (next) => {
      setDraft((previous) => ({ ...previous, ...next }));
      setStatus(undefined);
    },
    submit: (event) => {
      event.preventDefault();
      if (update.isPending) return;
      setStatus(undefined);
      update.mutate({ id: emoji.id, change });
    },
    isPending: update.isPending,
    result: { status, error: update.isError ? failure(t, update.error) : undefined },
  };
}

export type CustomEmojiImageReplacement = CustomEmojiChangeResult & {
  /** Uploads a new image of a local emoji and makes it its image. */
  replace: (file: File) => void;
  isPending: boolean;
};

/** Replacing the image of a local emoji on its page. */
export function useCustomEmojiImageReplacement(emoji: AdminCustomEmoji): CustomEmojiImageReplacement {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const replace = useMutation({
    ...replaceCustomEmojiImageMutation(api),
    // Awaited by the mutation: a failure here leaves it in its error state, which the control shows.
    onSuccess: async (saved) => {
      await cacheChangedCustomEmoji(queryClient, saved);
      const message = t("client:admin.custom_emojis.image_replaced", {
        emoji: emojiInText(saved),
      });
      setStatus(message);
      notice.inPlace(message);
    },
  });
  return {
    replace: (file) => {
      setStatus(undefined);
      replace.mutate({ id: emoji.id, file });
    },
    isPending: replace.isPending,
    status,
    error: replace.isError ? failure(t, replace.error) : undefined,
  };
}

/**
 * Deleting a local emoji from its page; resolves with whether it succeeded. A success goes back to the list, which
 * the emoji has left, and says what it did in a toast: the page it was on is gone (G24).
 */
export function useDeleteCustomEmoji(emoji: AdminCustomEmoji): {
  remove: () => Promise<boolean>;
  error: string | undefined;
} {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const navigate = useNavigate();
  const remove = useMutation({
    ...deleteCustomEmojiMutation(api),
    // Awaited by the mutation: a failure here leaves it in its error state, which the dialog shows.
    onSuccess: async () => {
      await navigate({ to: "/admin/custom-emojis" });
      await cacheDeletedCustomEmoji(queryClient, emoji.id);
      notice.toast({
        tone: "success",
        message: t("client:admin.custom_emojis.deleted", { emoji: emojiInText(emoji) }),
      });
    },
  });
  return {
    remove: async () => {
      try {
        await remove.mutateAsync(emoji.id);
      } catch {
        return false;
      }
      return true;
    },
    error: remove.isError ? failure(t, remove.error) : undefined,
  };
}

/** Copying a remote emoji into a local one from its page; a success opens the copy's page, whose heading shows it. */
export function useCopyCustomEmoji(emoji: AdminCustomEmoji): {
  copy: (event: SyntheticEvent<HTMLFormElement>, shortcode: string) => void;
  isPending: boolean;
  error: string | undefined;
} {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const copy = useMutation({
    ...copyCustomEmojiMutation(api),
    // Awaited by the mutation: a failure here leaves it in its error state, which the control shows.
    onSuccess: async (created) => {
      await cacheCreatedCustomEmoji(queryClient, created);
      await navigate({ to: "/admin/custom-emojis/$id", params: { id: created.id } });
    },
  });
  return {
    copy: (event, shortcode) => {
      event.preventDefault();
      if (copy.isPending) return;
      // The remote one's shortcode is the server's default; another one is sent.
      const named = shortcode === emoji.shortcode ? undefined : shortcode;
      copy.mutate({ id: emoji.id, shortcode: named });
    },
    isPending: copy.isPending,
    error: copy.isError ? failure(t, copy.error) : undefined,
  };
}
