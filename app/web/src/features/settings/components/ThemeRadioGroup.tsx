// SPDX-License-Identifier: AGPL-3.0-only
import { Radio, RadioGroup } from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { setTheme, type Theme, themes, useTheme } from "../../../lib/theme";

const labels: Record<Theme, ClientMessageKey> = {
  system: "client:settings.theme.system",
  light: "client:settings.theme.light",
  dark: "client:settings.theme.dark",
};

/**
 * The color theme: the OS setting, light or dark (a radio group, WAI-ARIA APG: the arrow keys move and check).
 * A choice applies at once and is kept on this browser (lib/theme), so it needs no saving and no sign-in.
 */
export function ThemeRadioGroup() {
  const t = useT();
  const theme = useTheme();
  return (
    <RadioGroup
      label={t("client:settings.theme.label")}
      description={t("client:settings.theme.description")}
      value={theme}
      onChange={(value) => {
        const chosen = themes.find((candidate) => candidate === value);
        if (chosen !== undefined) setTheme(chosen);
      }}
    >
      {themes.map((option) => (
        <Radio key={option} value={option}>
          {t(labels[option])}
        </Radio>
      ))}
    </RadioGroup>
  );
}
