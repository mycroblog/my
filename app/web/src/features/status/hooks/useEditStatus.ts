// SPDX-License-Identifier: AGPL-3.0-only
// Editing the viewer's own post (FR-109): the dialog reads what the author typed (GET /statuses/{id}/source)
// and the instance's character limit, and the form changes the text, the content warning, the sensitivity
// of the media, their descriptions and the language. The visibility and the reply target never change
// (api/openapi.yaml, editStatus), and the media stay as they are. Saving waits for the server; the answer
// replaces every cached copy of the post (cacheEditedStatus), and a failure stays in the dialog with the
// form as typed (web-client/README.md E2).
import { useToast } from "@my/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useLocale, useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import { type AttachmentView, countCharacters, type LanguageOption, languageOptions, rememberTags } from "../../compose";
import {
  cacheEditedStatus,
  editStatus,
  type EditStatusRequest,
  instanceQueryOptions,
  type Status,
  type StatusSource,
  statusSourceQueryOptions,
} from "../api/status";

/** What the edit dialog waits for before it shows the form. */
export type EditSource =
  | { status: "loading" }
  /** `message` is localized. */
  | { status: "error"; message: string; retry: () => void }
  | { status: "ready"; source: StatusSource; maxCharacters: number; isDescriptionRequired: boolean };

/** Reads what the form of post `id` starts from: the author's text, and the instance's character limit. */
export function useEditSource(id: string): EditSource {
  const api = useApi();
  const source = useQuery(statusSourceQueryOptions(api, id));
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  if (source.isError || instance.isError) {
    return {
      status: "error",
      message: errorMessage(source.error ?? instance.error),
      retry: () => {
        if (source.isError) void source.refetch();
        if (instance.isError) void instance.refetch();
      },
    };
  }
  if (source.data === undefined || instance.data === undefined) return { status: "loading" };
  const { max_post_characters: maxCharacters, require_media_description: isDescriptionRequired } = instance.data.limits;
  return { status: "ready", source: source.data, maxCharacters, isDescriptionRequired };
}

/** The fields of the form. */
type EditDraft = {
  text: string;
  hasWarning: boolean;
  spoilerText: string;
  sensitive: boolean;
  language: string;
  /** The description of each attachment, by its ID. */
  descriptions: Readonly<Record<string, string>>;
  /** Whether the edit takes the post's quote away, for good (ADR-0069 §3). */
  removeQuote: boolean;
};

export type EditStatusForm = EditDraft & {
  edit: (change: Partial<Omit<EditDraft, "descriptions">>) => void;
  /** The attachments with their descriptions, as the compose form's image list shows them. */
  attachments: AttachmentView[];
  /** Whether the post quotes another, which the edit can take away. */
  hasQuote: boolean;
  describe: (id: string, description: string) => void;
  /** The characters the text may have: the instance's limit less the content warning. */
  textLimit: number;
  languages: LanguageOption[];
  canSave: boolean;
  isSaving: boolean;
  /** Why the last save failed (localized), or null. */
  error: string | null;
  /** Saves the edit; `onSaved` runs once the post is edited and the cache holds it. */
  save: (onSaved: () => void) => void;
};

/** The draft an edit starts from: the source the author typed, and the post's media and sensitivity. */
function initialDraft(post: Status, source: StatusSource): EditDraft {
  return {
    text: source.text,
    hasWarning: source.spoiler_text !== "",
    spoilerText: source.spoiler_text,
    sensitive: post.sensitive,
    language: source.language,
    descriptions: Object.fromEntries(post.media_attachments.map((media) => [media.id, media.description ?? ""])),
    removeQuote: false,
  };
}

/** The request a draft becomes: every attachment stays, in order, with its description (an omitted list would drop them). */
function requestBody(post: Status, draft: EditDraft): EditStatusRequest {
  const media = post.media_attachments.map((attachment) => {
    const description = (draft.descriptions[attachment.id] ?? "").trim();
    return { id: attachment.id, ...(description === "" ? {} : { description }) };
  });
  return {
    text: draft.text,
    spoiler_text: draft.hasWarning ? draft.spoilerText.trim() : "",
    language: draft.language,
    ...(media.length > 0 ? { media, sensitive: draft.sensitive } : {}),
    ...(draft.removeQuote ? { remove_quote: true } : {}),
  };
}

/**
 * The form that edits the viewer's post `post`, starting from `source` (web-client/README.md A4). The dialog mounts
 * it once the source has arrived, so the draft starts from it and is the dialog's own: closing the dialog
 * drops it, and the compose form's draft is never touched.
 */
export function useEditStatusForm(post: Status, source: StatusSource, maxCharacters: number): EditStatusForm {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const locale = useLocale();
  const toast = useToast();
  const [draft, setDraft] = useState(() => initialDraft(post, source));
  const save = useMutation({
    mutationFn: (body: EditStatusRequest) => editStatus(api, post.id, body),
    onSuccess: async (edited) => {
      await cacheEditedStatus(api, queryClient, edited);
      toast.show({ message: t("client:status.edit.done"), tone: "success" });
    },
  });

  const textLimit = maxCharacters - (draft.hasWarning ? countCharacters(draft.spoilerText) : 0);
  const attachments: AttachmentView[] = post.media_attachments.map((media, index) => ({
    key: media.id,
    mediaId: media.id,
    description: draft.descriptions[media.id] ?? "",
    previewUrl: null,
    error: null,
    position: index + 1,
    src: media.preview_url ?? null,
    status: "ready",
  }));
  const canSave = draft.text.trim() !== "" && countCharacters(draft.text) <= textLimit && !save.isPending;

  return {
    hasQuote: post.quote !== undefined,
    ...draft,
    edit: (change) => {
      setDraft((current) => ({ ...current, ...change }));
    },
    attachments,
    describe: (id, description) => {
      setDraft((current) => ({ ...current, descriptions: { ...current.descriptions, [id]: description } }));
    },
    textLimit,
    languages: languageOptions(locale, [source.language, draft.language], t("client:compose.language.und")),
    canSave,
    isSaving: save.isPending,
    error: save.error === null ? null : t("client:status.edit.failed", { reason: errorMessage(save.error) }),
    save: (onSaved) => {
      if (!canSave) return;
      save.mutate(requestBody(post, draft), { onSuccess: onSaved });
      rememberTags(draft.text);
    },
  };
}
