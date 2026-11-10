// SPDX-License-Identifier: AGPL-3.0-only
// The article editor (web-client/screens/article-editor.md, ADR-0160): the title, summary, cover, hashtags, language,
// sensitivity and visibility, and the Markdown body with its buttons (bold, link) and its images (dropped, pasted, or
// picked: each uploads and goes into the body as `![](media:ID)`). The preview is the server's rendering (usePreview).
//
// A new article is a draft on the server while it is written: the autosave (autosaver.ts) saves it as the author
// pauses, and publishing or scheduling saves it once more and publishes or schedules that draft (ADR-0156 §9), so the
// article is published once. A published article is edited in place (PUT /articles/{id}) and is not autosaved.
import { type ArticleCoverImage, useToast } from "@my/ui";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useEffect, useId, useRef, useState, useSyncExternalStore } from "react";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { imageSources } from "../../../lib/images";
import { useLocale, useT } from "../../../lib/i18n";
import {
  acceptedImageTypes,
  cacheDraftChange,
  cachePostedStatus,
  cacheUploadedMedia,
  createDraft,
  isDraftConflict,
  type LanguageOption,
  languageOptions,
  type MediaAttachment,
  mediaQueryOptions,
  prepareImage,
  publishDraft,
  scheduleDraft,
  updateDraft,
  uploadMedia,
  useOfferedVisibilities,
} from "../../compose";
import { cacheEditedStatus, type Status } from "../../status";
import { editArticle } from "../api/articles";
import {
  type ArticleFields,
  type ArticleVisibility,
  draftRequestOf,
  editRequestOf,
  hasContent,
  type PublishBlocker,
  publishBlocker,
} from "./articleFields";
import {
  ArticleAutosaver,
  AutosaveConflictError,
  type AutosaveSnapshot,
  type AutosaveStatus,
  type DraftLink,
} from "./autosaver";
import { insertImages, insertLink, maxTags, type TextEdit, toggleBold } from "./markdown";
import { type Preview, usePreview } from "./usePreview";

/** Where the editor starts: a new article (from nothing, or from its draft on the server), or a published one. */
export type EditorStart =
  { mode: "new"; fields: ArticleFields; link: DraftLink | null } | { mode: "edit"; id: string; fields: ArticleFields };

/** The localized texts of the publish blockers. */
const blockerMessages = {
  title: "client:articles.blocked.title",
  text: "client:articles.blocked.text",
  tags: "client:articles.blocked.tags",
  uploading: "client:articles.blocked.uploading",
} as const satisfies Record<PublishBlocker, string>;

/**
 * The cover as the editor shows it: none, uploading, or an upload with its description and, once the server has
 * processed it, its image (undefined while it is processing).
 */
type CoverView =
  | { status: "none" }
  | { status: "uploading" }
  | { status: "uploaded"; description: string; image: ArticleCoverImage | undefined; failed: boolean };

/** The cover image of an upload that is ready, as @my/ui's ArticleCover takes it. */
function coverImageOf(media: MediaAttachment | undefined, description: string): ArticleCoverImage | undefined {
  if (media?.state !== "ready") return undefined;
  const sources = imageSources(media.variants);
  if (sources === undefined) return undefined;
  return {
    sources,
    width: media.variants?.large.width ?? media.meta.width,
    height: media.variants?.large.height ?? media.meta.height,
    blurhash: media.blurhash,
    ...(description.trim() === "" ? {} : { alt: description.trim() }),
  };
}

export type ArticleEditor = {
  mode: EditorStart["mode"];
  fields: ArticleFields;
  edit: (patch: Partial<ArticleFields>) => void;
  /** The id of the body's text area, which the buttons read the selection of. */
  bodyId: string;
  bold: () => void;
  link: () => void;
  /** Uploads the images of `files` and puts them into the body at the caret. */
  addImages: (files: readonly File[]) => void;
  /** How many images are uploading for the body. */
  uploadingImages: number;
  /** Why the last images did not go into the body (localized), or null. */
  imageError: string | null;
  cover: CoverView;
  chooseCover: (files: readonly File[]) => void;
  removeCover: () => void;
  coverError: string | null;
  preview: Preview;
  visibilities: readonly ArticleVisibility[];
  languages: LanguageOption[];
  maxTags: number;
  /** The autosave of a new article; null while editing a published one. */
  autosave: { status: AutosaveStatus; link: DraftLink | null; saveAsNew: () => void } | null;
  /** The field to mark after a press of Publish or Schedule while the article cannot be published. */
  invalid: PublishBlocker | null;
  publish: () => void;
  isPublishing: boolean;
  /** What the action bar says after the last publish or save: why it could not be, or null. */
  message: string | null;
  /** Why the article cannot be scheduled (localized), checked when Schedule is pressed; null when it can. */
  scheduleCheck: () => string | null;
  /** Saves and schedules the draft for `at`: resolves to null once scheduled, or to the reason it is not. */
  schedule: (at: Date) => Promise<string | null>;
};

/** The autosave's snapshot while a published article is edited, which is not autosaved. */
const notAutosaved: AutosaveSnapshot = { status: { kind: "idle" }, link: null };

/** The selection of the body's text area, or the end of `value` when the area is not on the page. */
function selectionOf(id: string, value: string): { start: number; end: number } {
  const element = document.getElementById(id);
  if (!(element instanceof HTMLTextAreaElement)) return { start: value.length, end: value.length };
  return { start: element.selectionStart, end: element.selectionEnd };
}

/** Puts focus back in the body with `edit`'s selection once React has rendered the new text. */
function restoreSelection(id: string, edit: TextEdit): void {
  requestAnimationFrame(() => {
    const element = document.getElementById(id);
    if (!(element instanceof HTMLTextAreaElement)) return;
    element.focus();
    element.setSelectionRange(edit.selectionStart, edit.selectionEnd);
  });
}

/** The article editor's state and actions, from `start`. */
export function useArticleEditor(start: EditorStart): ArticleEditor {
  const api = useApi();
  const t = useT();
  const intl = useIntl();
  const locale = useLocale();
  const toast = useToast();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const bodyId = useId();
  const [fields, setFields] = useState(start.fields);
  // The latest fields for what finishes after a render (an upload), which the state of that render does not hold.
  const latestRef = useRef(start.fields);
  const [invalid, setInvalid] = useState<PublishBlocker | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [uploadingImages, setUploadingImages] = useState(0);
  const [imageError, setImageError] = useState<string | null>(null);
  const [isCoverUploading, setIsCoverUploading] = useState(false);
  const [coverError, setCoverError] = useState<string | null>(null);
  // Uploads of this session whose processing the preview waits for: an image shows once it is ready.
  const [uploads, setUploads] = useState<readonly string[]>([]);

  const [saver] = useState(() =>
    start.mode === "new"
      ? new ArticleAutosaver(
          {
            create: (body) => createDraft(api, body),
            update: (id, etag, body) => updateDraft(api, id, etag, body),
            isConflict: isDraftConflict,
            onSaved: () => {
              void cacheDraftChange(queryClient);
            },
          },
          start.link,
        )
      : null,
  );
  const saved = useSyncExternalStore(
    (listener) => saver?.subscribe(listener) ?? (() => undefined),
    () => saver?.getSnapshot() ?? notAutosaved,
  );
  // The autosaver is the external system: leaving the editor saves the last change now (R8).
  useEffect(
    () => () => {
      saver?.dispose();
    },
    [saver],
  );

  const visibilities = useOfferedVisibilities().filter((v): v is ArticleVisibility => v !== "direct");
  const languages = languageOptions(locale, [fields.language], t("client:compose.language.und"));
  const coverQuery = useQuery({
    ...mediaQueryOptions(api, fields.cover?.mediaId ?? ""),
    enabled: fields.cover !== null,
  });
  const uploadQueries = useQueries({ queries: uploads.map((id) => mediaQueryOptions(api, id)) });
  const readyUploads = uploads.filter((_, i) => uploadQueries[i]?.data?.state === "ready").join(",");
  const preview = usePreview(fields.text, fields.summary, readyUploads);

  const edit = (patch: Partial<ArticleFields>) => {
    const next = { ...latestRef.current, ...patch };
    latestRef.current = next;
    setFields(next);
    setMessage(null);
    if (invalid !== null && publishBlocker(next, false) !== invalid) setInvalid(null);
    // Nothing is saved until there is something to save; a draft once saved follows every change, even to empty.
    if (saver !== null && (hasContent(next) || saver.getSnapshot().link !== null)) saver.change(draftRequestOf(next));
  };

  const changeText = (change: (value: string, start: number, end: number) => TextEdit) => {
    const value = latestRef.current.text;
    const { start: from, end: to } = selectionOf(bodyId, value);
    const next = change(value, from, to);
    edit({ text: next.value });
    restoreSelection(bodyId, next);
  };

  /** Uploads one image prepared like an attachment's (its metadata drawn away) and caches its first answer. */
  const upload = async (file: File): Promise<MediaAttachment> => {
    const media = await uploadMedia(api, await prepareImage(file));
    cacheUploadedMedia(api, queryClient, media);
    return media;
  };

  const blocker = publishBlocker(fields, uploadingImages > 0 || isCoverUploading);

  /** The localized reason of `reason`. */
  const blockerText = (reason: PublishBlocker): string =>
    reason === "tags" ? t(blockerMessages.tags, { max: maxTags }) : t(blockerMessages[reason]);

  /** Says why the article cannot be published, or returns false when it can. */
  const blocked = (): boolean => {
    if (blocker === null) return false;
    setInvalid(blocker);
    setMessage(blockerText(blocker));
    return true;
  };

  /** The localized reason of a failed save, publish, or schedule of the draft. */
  const failure = (error: unknown, words: "client:articles.publish.failed" | "client:drafts.schedule.failed") =>
    error instanceof AutosaveConflictError || isDraftConflict(error)
      ? t("client:articles.autosave.conflict")
      : t(words, { reason: errorMessage(error) });

  const publish = useMutation({
    mutationFn: async (current: ArticleFields): Promise<Status> => {
      if (start.mode === "edit") return editArticle(api, start.id, editRequestOf(current));
      if (saver === null) throw new Error("a new article has an autosave");
      const { id } = await saver.flush(draftRequestOf(current));
      return publishDraft(api, id);
    },
    onSuccess: async (status) => {
      if (start.mode === "edit") {
        await cacheEditedStatus(api, queryClient, status);
        toast.show({ message: t("client:articles.updated"), tone: "success" });
      } else {
        await Promise.all([cachePostedStatus(api, queryClient, status), cacheDraftChange(queryClient)]);
        toast.show({ message: t("client:articles.published"), tone: "success" });
      }
      await navigate({ to: "/@{$acct}/status/$id", params: { acct: status.account.acct, id: status.id } });
    },
    onError: (error) => {
      setMessage(
        start.mode === "edit"
          ? t("client:articles.update.failed", { reason: errorMessage(error) })
          : failure(error, "client:articles.publish.failed"),
      );
    },
  });

  const cover: CoverView = isCoverUploading
    ? { status: "uploading" }
    : fields.cover === null
      ? { status: "none" }
      : {
          status: "uploaded",
          description: fields.cover.description,
          image: coverImageOf(coverQuery.data, fields.cover.description),
          failed: coverQuery.data?.state === "failed",
        };

  return {
    mode: start.mode,
    fields,
    edit,
    bodyId,
    bold: () => {
      changeText(toggleBold);
    },
    link: () => {
      changeText(insertLink);
    },
    addImages: (files) => {
      const images = files.filter((file) => acceptedImageTypes.includes(file.type));
      if (images.length < files.length) setImageError(t("client:articles.images.unsupported"));
      else setImageError(null);
      if (images.length === 0) return;
      setUploadingImages((n) => n + images.length);
      void Promise.all(images.map(upload))
        .then((media) => {
          const ids = media.map((m) => m.id);
          setUploads((current) => [...current, ...ids]);
          changeText((value, from, to) => insertImages(value, from, to, ids));
        })
        .catch((error: unknown) => {
          setImageError(t("client:articles.images.failed", { reason: errorMessage(error) }));
        })
        .finally(() => {
          setUploadingImages((n) => n - images.length);
        });
    },
    uploadingImages,
    imageError,
    cover,
    chooseCover: (files) => {
      const [file] = files.filter((f) => acceptedImageTypes.includes(f.type));
      if (file === undefined) {
        setCoverError(files.length === 0 ? null : t("client:articles.images.unsupported"));
        return;
      }
      setCoverError(null);
      setIsCoverUploading(true);
      void upload(file)
        .then((media) => {
          edit({ cover: { mediaId: media.id, description: latestRef.current.cover?.description ?? "" } });
        })
        .catch((error: unknown) => {
          setCoverError(t("client:articles.images.failed", { reason: errorMessage(error) }));
        })
        .finally(() => {
          setIsCoverUploading(false);
        });
    },
    removeCover: () => {
      setCoverError(null);
      edit({ cover: null });
    },
    coverError,
    preview,
    visibilities,
    languages,
    maxTags,
    autosave:
      saver === null
        ? null
        : {
            status: saved.status,
            link: saved.link,
            saveAsNew: () => {
              setMessage(null);
              void saver.saveAsNew(draftRequestOf(latestRef.current)).catch(() => {
                // The autosave's status says why.
              });
            },
          },
    invalid,
    publish: () => {
      if (blocked()) return;
      setMessage(null);
      publish.mutate(latestRef.current);
    },
    isPublishing: publish.isPending,
    message,
    scheduleCheck: () => (blocker === null ? null : blockerText(blocker)),
    schedule: async (at) => {
      if (saver === null) return null;
      try {
        const { id } = await saver.flush(draftRequestOf(latestRef.current));
        const scheduled = await scheduleDraft(api, id, at);
        await cacheDraftChange(queryClient);
        const date = intl.formatDate(scheduled.scheduled_at ?? at, { dateStyle: "medium", timeStyle: "short" });
        toast.show({ message: t("client:drafts.schedule.scheduled", { date }), tone: "success" });
        await navigate({ to: "/drafts" });
        return null;
      } catch (error) {
        return failure(error, "client:drafts.schedule.failed");
      }
    },
  };
}
