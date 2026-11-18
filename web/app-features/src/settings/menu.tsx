// SPDX-License-Identifier: AGPL-3.0-only

// The menu of the settings (ADR-0110 §5): the entries it lists for the reader (entries.tsx) in their groups, a group with
// none left out, and Sign out…, apart at its end, which leads to the confirmation the app shows at `signOutHref`. At
// /settings it is the screen, each entry with a line of what its screen holds; beside an entry's screen it is compact,
// the entry the reader is at marked. It paints nothing until the server has said which entries it lists, then all at once.

import { SettingsGroup, SettingsItem, SettingsMenu, useLocale } from "@my/design-system";
import { stringsIn } from "../locale";
import { settingsGroups, useSettingsFactsKnown, useShownEntries } from "./entries";

/** The address of the screen of the entry at `path`. */
export const settingsEntryPath = (path: string): string => `/settings/${path}`;

type MenuProps = {
  /** The address that asks whether to sign out, where the app shows the confirmation. */
  signOutHref: string;
};

function Menu({ compact, current, signOutHref }: MenuProps & { compact: boolean; current?: string | undefined }) {
  const { t } = stringsIn(useLocale().locale);
  const entries = useShownEntries().filter((entry) => entry.inMenu);
  const known = useSettingsFactsKnown();
  // Nothing until the entries it lists are known, so that none comes after the others painted.
  if (!known) return null;
  return (
    <SettingsMenu
      label={t("settings.menu")}
      compact={compact}
      end={<SettingsItem icon="signOut" label={t("settings.sign_out")} count={undefined} href={signOutHref} away />}
    >
      {settingsGroups.map((group) => {
        const own = entries.filter((entry) => entry.group === group.id);
        if (own.length === 0) return null;
        return (
          <SettingsGroup key={group.id} heading={t(group.heading)}>
            {own.map((entry) => (
              <SettingsItem
                key={entry.path}
                icon={entry.icon}
                label={t(entry.name)}
                hint={t(entry.hint)}
                count={entry.waiting}
                countLabel={
                  entry.waiting === undefined ? undefined : t("settings.entry.count", { count: entry.waiting })
                }
                href={settingsEntryPath(entry.path)}
                current={entry.path === current}
              />
            ))}
          </SettingsGroup>
        );
      })}
    </SettingsMenu>
  );
}

/** The menu as the screen at /settings: every entry with what its screen holds. */
export function SettingsIndex({ signOutHref }: MenuProps) {
  return <Menu compact={false} signOutHref={signOutHref} />;
}

/**
 * The menu beside a screen of the settings, in the column beside the main one: compact, the entry at `current`
 * marked (none at an address that names no entry).
 */
export function SettingsBeside({ current, signOutHref }: MenuProps & { current: string | undefined }) {
  return <Menu compact current={current} signOutHref={signOutHref} />;
}
