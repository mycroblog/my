// SPDX-License-Identifier: AGPL-3.0-only

// The preview of the draft of the instance theme (ADR-0059): while an administrator previews it in this tab, the
// draft's stylesheet lies over every screen, after the theme in effect, so that each screen of the app shows what
// visitors would see once it is published; it goes as the preview stops, and changes as the draft is saved.

import { useThemePreview } from "@my/app-data";
import { adoptStyleSheet } from "@my/platform";
import { useEffect } from "react";

/** Lays the draft's stylesheet over the page while the reader previews it. */
export function useThemePreviewSheet(): void {
  const { stylesheet } = useThemePreview();
  useEffect(() => (stylesheet === undefined ? undefined : adoptStyleSheet(stylesheet)), [stylesheet]);
}
