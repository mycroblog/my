// SPDX-License-Identifier: AGPL-3.0-only
import type { AudioPlayerLabels } from "@my/ui";
import { useT } from "../../../lib/i18n";

/** The texts of the player of a post's audio attachment (@my/ui's MediaGrid). */
export function useAudioPlayerLabels(): AudioPlayerLabels {
  const t = useT();
  return {
    play: t("client:timeline.media.audio.play"),
    pause: t("client:timeline.media.audio.pause"),
    seek: t("client:timeline.media.audio.seek"),
    loading: t("client:media.loading"),
    failed: t("client:media.failed"),
    retry: t("client:media.retry"),
  };
}
