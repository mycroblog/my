// SPDX-License-Identifier: AGPL-3.0-only
import type { MediaViewerLabels } from "@my/ui";
import { useT } from "../../../lib/i18n";

/** The texts of the image viewer that opens a post's picture at its original size (@my/ui's MediaGrid). */
export function useMediaViewerLabels(): MediaViewerLabels {
  const t = useT();
  return {
    title: (position, count) => t("client:timeline.media.viewer.title", { position, count }),
    close: t("client:timeline.media.viewer.close"),
    position: (position, count) => t("client:timeline.media.viewer.position", { position, count }),
    altText: t("client:timeline.media.viewer.alt_text"),
    previous: t("client:timeline.media.viewer.previous"),
    next: t("client:timeline.media.viewer.next"),
    loading: t("client:media.loading"),
    failed: t("client:media.failed"),
    retry: t("client:media.retry"),
  };
}
