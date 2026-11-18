// SPDX-License-Identifier: AGPL-3.0-only

// Account and data, the entry of the settings at /settings/account (ADR-0110 §5): what becomes of the account and of
// its data, a part each, one under another, each part a section of its own that reads what it shows and says so while
// it does (`AccountStatus`), so that a part is added as one more child of the screen; what cannot be undone stands
// last and apart.
//
// Its first part is the account's status (FR-715, ADR-0058 §9), said once and with one door. While moderators'
// actions are in effect on the account, the status summary heads the screen: how many, what the page it leads to
// says of each, and the button to that page, Account status (account-status.tsx, a screen under the entry). While
// none is, the section Account status holds one row that says so and leads to the same page, where the past actions
// and the appeals stay listed. The menu's count of the entry is how many actions in effect the reader can still
// appeal (`useAppealableActions`).

import { ownModerationActionsQuery, useOwnModerationActions, type AppData } from "@my/app-data";
import {
  LinkButton,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  StatusSummary,
  TextButton,
  useLocale,
} from "@my/design-system";
import { stringsIn } from "../locale";
import type { SettingsEntryProps } from "./entries";

/** The address of the entry's screen. */
const accountPath = "/settings/account";

/** The address of Account status, the screen under the entry that lists the moderators' actions on the account. */
export const accountStatusPath = `${accountPath}/status`;

/** The address of the screen of the moderators' action of `id`, under Account status. */
export const moderationActionPath = (id: string): string => `${accountStatusPath}/${id}`;

/** What the route's loader waits for: the actions in effect on the account, from the cache when it has them. */
export async function loadAccountSettings(data: AppData): Promise<void> {
  await data.queryClient
    .infiniteQuery({ ...ownModerationActionsQuery(data, true), staleTime: Infinity })
    .catch(() => undefined);
}

/**
 * How many moderators' actions in effect on the reader's account they can still appeal (not appealed yet, the time to
 * appeal not passed), of those read: what waits for the reader on the entry's screen, the menu's count of it.
 */
export function useAppealableActions(): number {
  return useOwnModerationActions(true).items.filter((action) => action.appealable).length;
}

/**
 * The account's status, the entry's first part: the summary with its door while an action is in effect, else the
 * section whose one row says that none is and leads to the same page; and, until the actions are read, that they
 * load or could not be read.
 */
function AccountStatus() {
  const { t } = stringsIn(useLocale().locale);
  const active = useOwnModerationActions(true);
  const heading = t("settings.account.status");
  if (active.status !== "success")
    return (
      <SettingsSection heading={heading}>
        {active.status === "pending" ? (
          <SettingsEmpty>{t("settings.account.status.loading")}</SettingsEmpty>
        ) : (
          <SettingsEmpty action={<TextButton onPress={active.refetch}>{t("settings.retry")}</TextButton>}>
            {t("settings.account.status.failed")}
          </SettingsEmpty>
        )}
      </SettingsSection>
    );
  const count = active.items.length;
  if (count > 0)
    return (
      <StatusSummary
        icon="caution"
        title={t("settings.account.status.in_effect", { count })}
        about={t("settings.account.status.in_effect.about", {
          appealable: active.items.some((action) => action.appealable) ? "yes" : "no",
        })}
        action={
          <LinkButton href={accountStatusPath} kind="primary">
            {t("settings.account.status.open")}
          </LinkButton>
        }
      />
    );
  return (
    <SettingsSection heading={heading}>
      <SettingsList label={heading}>
        <SettingsListRow
          icon="moderation"
          title={t("settings.account.status.row")}
          caption={t("settings.account.status.row.none")}
          href={accountStatusPath}
        />
      </SettingsList>
    </SettingsSection>
  );
}

/** The entry's screen: its parts, the account's status first. */
export function AccountSettings({ back }: SettingsEntryProps) {
  const { t } = stringsIn(useLocale().locale);
  return (
    <SettingsScreen icon="accountData" title={t("settings.entry.account")} back={back}>
      <AccountStatus />
    </SettingsScreen>
  );
}
