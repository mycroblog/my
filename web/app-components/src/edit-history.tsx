// SPDX-License-Identifier: AGPL-3.0-only

// A post's edit history (FR-109), opened from its edited mark: a panel over the page (a dialog from `tablet`, a screen
// of its own on a phone) listing every version newest first, each headed by Current, Edit or Original and when it was
// made, in full in the reader's locale, then its content warning, its text and its images, what it changed from the
// version before it marked inline: its words, word by word in the post's language, and its images, by their id.

import {
  CaptionLine,
  DiffMedia,
  DiffText,
  PanelDialog,
  Version,
  VersionList,
  useLocale,
  type DiffImage,
  type DiffMarks,
} from "@my/design-system";
import { diffWords, plainText } from "./diff";
import { stringsIn } from "./locale";
import { EmojiText, type ContentEmoji } from "./post-content";
import type { PostMedia } from "./status";

/** One version of a post (the API's StatusEdit). */
export type PostEdit = {
  content: string;
  spoiler_text: string;
  /** When it was made; the first is when the post was posted. */
  created_at: string;
  media_attachments: readonly PostMedia[];
  emojis: readonly ContentEmoji[];
};

/** A version's images as previews, each kept, added since `before`, or (after them) removed since. */
function imagesOf(edit: PostEdit, before: PostEdit | undefined): DiffImage[] {
  const tile = (m: PostMedia, kind: DiffImage["kind"]) => {
    const src = m.preview_url ?? m.url;
    return src === undefined ? [] : [{ id: m.id, src, alt: m.description ?? "", kind }];
  };
  if (before === undefined) return edit.media_attachments.flatMap((m) => tile(m, "same"));
  const had = new Set(before.media_attachments.map((m) => m.id));
  const has = new Set(edit.media_attachments.map((m) => m.id));
  return [
    ...edit.media_attachments.flatMap((m) => tile(m, had.has(m.id) ? "same" : "added")),
    ...before.media_attachments.filter((m) => !has.has(m.id)).flatMap((m) => tile(m, "removed")),
  ];
}

export type EditHistoryProps = {
  /** The versions, oldest first, as the API gives them. */
  edits: readonly PostEdit[];
  /** The language of the post. */
  postLang?: string | undefined;
  onClose: () => void;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
};

export function EditHistory({ edits, postLang, onClose, layer = true }: EditHistoryProps) {
  const { t, lang } = stringsIn(useLocale().locale);
  const when = new Intl.DateTimeFormat(lang, { dateStyle: "medium", timeStyle: "short" });
  const newest = [...edits].reverse();
  const marks: DiffMarks = {
    lang,
    added: { start: t("post.history.added_start"), end: t("post.history.added_end") },
    removed: { start: t("post.history.removed_start"), end: t("post.history.removed_end") },
    addedImage: t("post.history.added_image"),
    removedImage: t("post.history.removed_image"),
  };
  return (
    <PanelDialog title={t("post.history.title")} closeLabel={t("post.history.close")} onClose={onClose} layer={layer}>
      <VersionList label={t("post.history.title")}>
        {newest.map((edit, i) => {
          const before = newest[i + 1];
          const label =
            i === 0
              ? t("post.history.current")
              : before === undefined
                ? t("post.history.original")
                : t("post.history.earlier");
          const text = plainText(edit.content);
          return (
            <Version
              key={edit.created_at}
              label={label}
              time={<time dateTime={edit.created_at}>{when.format(new Date(edit.created_at))}</time>}
              lang={postLang}
            >
              {edit.spoiler_text !== "" && (
                <CaptionLine icon="hidden">
                  <EmojiText text={edit.spoiler_text} emojis={edit.emojis} />
                </CaptionLine>
              )}
              <DiffText
                parts={
                  before === undefined
                    ? [{ text, kind: "same" }]
                    : diffWords(plainText(before.content), text, postLang ?? lang)
                }
                marks={marks}
                render={(s) => <EmojiText text={s} emojis={edit.emojis} />}
              />
              <DiffMedia items={imagesOf(edit, before)} marks={marks} />
            </Version>
          );
        })}
      </VersionList>
    </PanelDialog>
  );
}
