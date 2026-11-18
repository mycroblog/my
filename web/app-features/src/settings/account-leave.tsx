// SPDX-License-Identifier: AGPL-3.0-only

// The last parts of the settings' entry Account and data (account.tsx), by which the reader leaves this account
// (FR-008, FR-009, ADR-0089 §3, §11): moving to another account, a section whose one row is the door to Move account
// (account-move.tsx) and says where the account moved once it has; and, last and apart, the section of what ends the
// account, the doors to Deactivate account and Delete account (account-end.tsx). Each is a screen of its own under
// the entry: what it does is said there, before the reader acts. A door is there whether or not its read answered.
// Also the addresses of the two screens that end the account; the move's is app-components', which the closure's
// banner leads to.

import { accountMovePath, handleOf } from "@my/app-components";
import { useViewer } from "@my/app-data";
import { SettingsList, SettingsListRow, SettingsSection, useLocale } from "@my/design-system";
import { stringsIn } from "../locale";

/** The address of Deactivate account, the screen under the entry that deactivates the reader's account. */
export const accountDeactivatePath = "/settings/account/deactivate";

/** The address of Delete account, the screen under the entry that deletes the reader's account. */
export const accountDeletePath = "/settings/account/delete";

/** Moving to another account, a part of the entry: the door to Move account, with where the account moved. */
export function AccountDeparture() {
  const { t } = stringsIn(useLocale().locale);
  const moved = useViewer()?.moved;
  const heading = t("settings.account.departure");
  return (
    <SettingsSection heading={heading}>
      <SettingsList label={heading}>
        <SettingsListRow
          title={t("settings.account.move")}
          caption={moved === undefined ? t("settings.account.move.row.about") : undefined}
          standing={
            moved === undefined ? undefined : t("settings.account.move.row.standing", { handle: handleOf(moved) })
          }
          href={accountMovePath}
        />
      </SettingsList>
    </SettingsSection>
  );
}

/** What ends the account, the entry's last part: the doors to Deactivate account and Delete account. */
export function AccountEnd() {
  const { t } = stringsIn(useLocale().locale);
  const heading = t("settings.account.end");
  return (
    <SettingsSection heading={heading}>
      <SettingsList label={heading}>
        <SettingsListRow
          title={t("settings.account.deactivate")}
          caption={t("settings.account.deactivate.row.about")}
          href={accountDeactivatePath}
        />
        <SettingsListRow
          title={t("settings.account.delete")}
          caption={t("settings.account.delete.row.about")}
          href={accountDeletePath}
        />
      </SettingsList>
    </SettingsSection>
  );
}
