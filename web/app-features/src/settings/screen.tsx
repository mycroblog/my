// SPDX-License-Identifier: AGPL-3.0-only

// The screen at an entry's address, /settings/<path> (ADR-0110 §5): the screen the entry declares (entries.tsx),
// headed by the way back to the menu where the menu does not stand beside it; an address that names no entry the
// reader is shown says so, with the link to the menu.

import { BackButton, FeedNotice, TextLink, useLocale } from "@my/design-system";
import { stringsIn } from "../locale";
import { useShownEntries, type SettingsEntryProps } from "./entries";

/** The name of the entry at `path`, the title of its screen; undefined for an address that names no entry. */
export function useSettingsTitle(path: string): string | undefined {
  const { t } = stringsIn(useLocale().locale);
  const entry = useShownEntries().find((e) => e.path === path);
  return entry === undefined ? undefined : t(entry.name);
}

export type SettingsEntryScreenProps = Pick<SettingsEntryProps, "languages"> & {
  /** The last part of the address. */
  entry: string;
  /** Goes back to the menu. */
  onBack: () => void;
};

export function SettingsEntryScreen({ entry, onBack, languages }: SettingsEntryScreenProps) {
  const { t } = stringsIn(useLocale().locale);
  const found = useShownEntries().find((e) => e.path === entry);
  if (found === undefined)
    return (
      <FeedNotice alert action={<TextLink href="/settings">{t("settings.back")}</TextLink>}>
        {t("settings.not_found")}
      </FeedNotice>
    );
  return <found.Screen back={<BackButton onPress={onBack}>{t("settings.back")}</BackButton>} languages={languages} />;
}
