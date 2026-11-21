// SPDX-License-Identifier: AGPL-3.0-only

// The line of a post shown on its own page (the `details` of the focused post of a thread), in place of the stamp at
// the end of its author's line: when it was posted, in full in the reader's zone, and to whom (its visibility, or the
// board of an event it is posted on), and that it was edited, which opens its edit history. Its counts are its actions'
// own, and who reacted stands after them.

import { dateFormat, DetailLines, GlyphText, TextButton, useLocale, useTimeZone } from "@my/design-system";
import { stringsIn } from "./locale";
import { audienceOf } from "./post";
import type { PostStatus } from "./status";

export function PostDetails({ status, onHistory }: { status: PostStatus; onHistory?: (() => void) | undefined }) {
  const { t, lang } = stringsIn(useLocale().locale);
  const date = new Date(status.created_at);
  const timeZone = useTimeZone();
  const visibility = audienceOf(status);
  return (
    <DetailLines>
      <p lang={lang}>
        <time dateTime={status.created_at}>
          {dateFormat(lang, timeZone, { dateStyle: "medium", timeStyle: "short" }).format(date)}
        </time>
        <span aria-hidden="true"> · </span>
        <GlyphText icon={visibility.icon}>{t(visibility.label)}</GlyphText>
        {status.edited_at !== undefined && (
          <>
            <span aria-hidden="true"> · </span>
            {onHistory === undefined ? (
              t("post.edited")
            ) : (
              <TextButton onPress={onHistory} aria-label={t("post.edited_mark")}>
                {t("post.edited")}
              </TextButton>
            )}
          </>
        )}
      </p>
    </DetailLines>
  );
}
