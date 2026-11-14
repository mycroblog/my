// SPDX-License-Identifier: AGPL-3.0-only

// The lines of a post shown on its own page (the `details` of the focused post of a thread), in place of the stamp at
// the end of its author's line: when it was posted, in full, and to whom (its visibility), then how many boosted and
// favourited it, those that are not none.

import { DetailCount, DetailLines, GlyphText, useLocale } from "@my/design-system";
import { stringsIn } from "./locale";
import { visibilities } from "./post";
import type { PostStatus } from "./status";

export function PostDetails({ status }: { status: PostStatus }) {
  const { t, lang } = stringsIn(useLocale().locale);
  const date = new Date(status.created_at);
  const visibility = visibilities[status.visibility];
  const counts = [
    status.boosts_count > 0 && t("post.boosts_count", { count: status.boosts_count }),
    status.favourites_count > 0 && t("post.favourites_count", { count: status.favourites_count }),
  ].filter((c) => c !== false);
  return (
    <DetailLines>
      <p lang={lang}>
        <time dateTime={status.created_at}>
          {new Intl.DateTimeFormat(lang, { dateStyle: "medium", timeStyle: "short" }).format(date)}
        </time>
        <span aria-hidden="true"> · </span>
        <GlyphText icon={visibility.icon}>{t(visibility.label)}</GlyphText>
      </p>
      {counts.length > 0 && (
        <p lang={lang}>
          {counts.map((count, i) => (
            <span key={i}>
              {i > 0 && <span aria-hidden="true"> · </span>}
              <DetailCount>{count}</DetailCount>
            </span>
          ))}
        </p>
      )}
    </DetailLines>
  );
}
