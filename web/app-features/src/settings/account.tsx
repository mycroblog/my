// SPDX-License-Identifier: AGPL-3.0-only

// Account and data, the entry of the settings at /settings/account (ADR-0110 §5): what becomes of the account and of
// its data, a part each, one under another, each part a section of its own that reads what it shows and says so while
// it does (`AccountStatus`), so that a part is added as one more child of the screen; what cannot be undone stands
// last and apart.
//
// Its first part is the account's status (FR-715, ADR-0058 §9), said once and with one door. While moderators'
// actions are in effect on the account, the status summary heads the screen: how many, what the page it leads to
// says of each, until when the reader can appeal while they still can (the nearest last day, which the menu's count
// waits for), and the button to that page, Account status (account-status.tsx, a screen under the entry). While
// none is, the section Account status holds one row that says so and leads to the same page, where the past actions
// and the appeals stay listed. The menu's count of the entry is how many actions in effect the reader can still
// appeal (`useAppealableActions`).

import { ownModerationActionsQuery, useOwnModerationActions, type AppData } from "@my/app-data";
import {
  Button,
  dateFormat,
  LinkButton,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  StatusSummary,
  useLocale,
  useTimeZone,
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
 * A moment of an action (when it ends, when it was appealed), as the reader's locale and zone write it: its day and
 * its time, or its day alone without `time`, joined so that a line never breaks inside it.
 */
export function useActionDate(time = true): (at: string) => string {
  const { locale } = useLocale();
  const zone = useTimeZone();
  const format = dateFormat(locale, zone, time ? { dateStyle: "medium", timeStyle: "short" } : { dateStyle: "medium" });
  return (at) => format.format(new Date(at)).replaceAll(" ", " ");
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
  const date = useActionDate(false);
  const heading = t("settings.account.status");
  if (active.status !== "success")
    return (
      <SettingsSection heading={heading}>
        {active.status === "pending" ? (
          <SettingsEmpty>{t("settings.account.status.loading")}</SettingsEmpty>
        ) : (
          <SettingsEmpty
            action={
              <Button size="timeline" touch onPress={active.refetch}>
                {t("settings.retry")}
              </Button>
            }
          >
            {t("settings.account.status.failed")}
          </SettingsEmpty>
        )}
      </SettingsSection>
    );
  const count = active.items.length;
  if (count > 0) {
    const open = active.items.filter((action) => action.appealable);
    // The nearest of the last days to appeal, of the actions that say one: what the count of the entry waits for.
    const [nearest] = open
      .flatMap((action) => action.appeal_deadline ?? [])
      .sort((a, b) => Date.parse(a) - Date.parse(b));
    return (
      <StatusSummary
        icon="caution"
        title={t("settings.account.status.in_effect", { count })}
        about={t("settings.account.status.in_effect.about", {
          appealable: open.length === 0 ? "no" : nearest === undefined ? "yes" : "deadline",
          date: nearest === undefined ? "" : date(nearest),
        })}
        action={
          <LinkButton href={accountStatusPath} kind="primary">
            {t("settings.account.status.open")}
          </LinkButton>
        }
      />
    );
  }
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
