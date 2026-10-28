// SPDX-License-Identifier: AGPL-3.0-only
import { useMutation, useQueries, useQueryClient } from "@tanstack/react-query";
import { useAnnouncer } from "@my/ui";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { cacheUploadedMedia, type MediaAttachment, mediaQueryOptions, uploadMedia } from "../api/compose";
import type { AttachmentDraft, DraftStore } from "./draftStore";
import { maxAttachments, mediaKind } from "./policy";
import { type PreparedMedia, prepareMedia } from "./prepareMedia";

/** Where an attachment is on its way to a post. */
type AttachmentStatus =
  | { status: "uploading" }
  | { status: "processing" }
  | { status: "ready" }
  /** `message` is localized. */
  | { status: "failed"; message: string };

/** An attachment as the form shows it. */
export type AttachmentView = AttachmentDraft & {
  /** Its number in the post, from 1. */
  position: number;
  /** The preview to show: the local image or the poster of a video, else the server's thumbnail once it has one. */
  src: string | null;
} & AttachmentStatus;

/** The id of an attachment's description field, which the missing-description dialog focuses. */
export function descriptionFieldId(key: string): string {
  return `compose-media-description-${key}`;
}

export type Attachments = {
  items: AttachmentView[];
  /** Whether more files can be added. */
  canAdd: boolean;
  /** Prepares and uploads the picked images, videos, and audio files, up to the limit. */
  add: (files: readonly File[]) => void;
  remove: (key: string) => void;
  describe: (key: string, description: string) => void;
  /** Whether an attachment is still uploading or processing. */
  isBusy: boolean;
  /** Whether an attachment failed; it must be removed before posting. */
  hasFailed: boolean;
};

function newKey(): string {
  return crypto.randomUUID();
}

/** The status of an attachment from its draft and its media query. */
function attachmentStatus(
  attachment: AttachmentDraft,
  media: MediaAttachment | undefined,
  queryError: unknown,
  failedProcessing: string,
): AttachmentStatus {
  if (attachment.error !== null) return { status: "failed", message: attachment.error };
  if (attachment.mediaId === null) return { status: "uploading" };
  if (media === undefined)
    return queryError === null ? { status: "processing" } : { status: "failed", message: errorMessage(queryError) };
  if (media.state === "failed") return { status: "failed", message: failedProcessing };
  return { status: media.state === "ready" ? "ready" : "processing" };
}

/**
 * The attachments of the draft: picking prepares each file in the browser (an image is drawn again without its
 * metadata; a video gets a poster; an audio file is sent as it is and the server removes its tags, prepareMedia), uploads it, and polls it until the server has processed it (the media query). The draft keeps the uploaded file's ID and the
 * description; the state of the file is the server's, read from the query.
 */
export function useAttachments(store: DraftStore, attachments: readonly AttachmentDraft[]): Attachments {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const announce = useAnnouncer();
  const upload = useMutation({
    mutationFn: ({ file, poster }: PreparedMedia) => uploadMedia(api, file, poster),
    onSuccess: (media) => {
      cacheUploadedMedia(api, queryClient, media);
    },
  });
  const uploadedIds = attachments.flatMap((a) => (a.mediaId === null ? [] : [a.mediaId]));
  const media = useQueries({ queries: uploadedIds.map((id) => mediaQueryOptions(api, id)) });
  const byId = new Map(uploadedIds.map((id, i) => [id, media[i]]));

  const items: AttachmentView[] = attachments.map((attachment, index) => {
    const result = attachment.mediaId === null ? undefined : byId.get(attachment.mediaId);
    const status = attachmentStatus(
      attachment,
      result?.data,
      result?.error ?? null,
      t("client:compose.media.failed_processing"),
    );
    return {
      ...attachment,
      ...status,
      position: index + 1,
      src: attachment.previewUrl ?? result?.data?.variants?.small.url ?? null,
    };
  });

  const start = (file: File) => {
    const key = newKey();
    store.edit({
      attachments: [
        ...store.getSnapshot().draft.attachments,
        { key, mediaId: null, description: "", previewUrl: null, error: null },
      ],
    });
    void (async () => {
      try {
        const prepared = await prepareMedia(file);
        if (prepared.preview !== null) store.editAttachment(key, { previewUrl: URL.createObjectURL(prepared.preview) });
        const uploaded = await upload.mutateAsync(prepared);
        store.editAttachment(key, { mediaId: uploaded.id });
      } catch (error) {
        store.editAttachment(key, { error: errorMessage(error) });
      }
    })();
  };

  return {
    items,
    canAdd: attachments.length < maxAttachments,
    add(files) {
      const accepted = files.filter((file) => {
        if (mediaKind(file.type) !== null) return true;
        announce(t("client:compose.media.unsupported", { name: file.name }), "assertive");
        return false;
      });
      const room = maxAttachments - store.getSnapshot().draft.attachments.length;
      if (accepted.length > room) announce(t("client:compose.media.too_many", { max: maxAttachments }), "assertive");
      for (const file of accepted.slice(0, Math.max(0, room))) start(file);
    },
    remove(key) {
      const attachment = store.getSnapshot().draft.attachments.find((a) => a.key === key);
      if (attachment?.previewUrl != null) URL.revokeObjectURL(attachment.previewUrl);
      store.edit({ attachments: store.getSnapshot().draft.attachments.filter((a) => a.key !== key) });
    },
    describe(key, description) {
      store.editAttachment(key, { description });
    },
    isBusy: items.some((item) => item.status === "uploading" || item.status === "processing"),
    hasFailed: items.some((item) => item.status === "failed"),
  };
}
