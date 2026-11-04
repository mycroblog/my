// SPDX-License-Identifier: AGPL-3.0-only
import { Switch } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { setAutoplay, useAutoplay } from "../../../lib/autoplay";

/**
 * Whether animated images play by themselves (ADR-0005 §6; the APG switch pattern). A change applies at once and is
 * kept on this browser (lib/autoplay), so it needs no saving and no sign-in.
 */
export function AutoplaySwitch() {
  const t = useT();
  const autoplay = useAutoplay();
  return (
    <Switch isSelected={autoplay} onChange={setAutoplay} description={t("client:settings.autoplay.description")}>
      {t("client:settings.autoplay.label")}
    </Switch>
  );
}
