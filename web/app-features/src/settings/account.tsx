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
//
// Then the reader's data and posts, a section of doors, each a row that leads to its screen under the entry and says
// in one line what is behind it, or, in the text's color, where it stands while it has something to say: Export
// (account-export.tsx: an archive being prepared, or ready until a day), Import (account-import.tsx: an import in
// progress) and Delete old posts automatically (account-auto-delete.tsx: off, the age posts are deleted past, or the
// day the server's maximum starts to apply). And moving to this account: Aliases (account-aliases.tsx), with how
// many there are. A door is there whether or not its read answered; its screen says what could not be read.

import { autoDeletePath, dataExportPath } from "@my/app-components";
import {
  ownModerationActionsQuery,
  useAccountAliasList,
  useAutoDeleteRead,
  useDataExports,
  useDataImports,
  useOwnModerationActions,
  type AppData,
  type DataExport,
} from "@my/app-data";
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
import { AccountDeparture, AccountEnd } from "./account-leave";
import type { SettingsEntryProps } from "./entries";

/** The address of the entry's screen. */
const accountPath = "/settings/account";

/** The address of Account status, the screen under the entry that lists the moderators' actions on the account. */
export const accountStatusPath = `${accountPath}/status`;

/** The address of the screen of the moderators' action of `id`, under Account status. */
export const moderationActionPath = (id: string): string => `${accountStatusPath}/${id}`;

/** The address of Import, the screen under the entry that imports a CSV file. */
const accountImportPath = `${accountPath}/import`;

/** The address of Aliases, the screen under the entry that lists the accounts this one is also known as. */
const accountAliasesPath = `${accountPath}/aliases`;

/** Where an archive stands: being prepared, ready to download, past the day the server keeps it until, or failed. */
type ArchiveState = "preparing" | "ready" | "expired" | "failed";

/** Where the archive of `e` stands now. */
export function archiveStateOf(e: DataExport): ArchiveState {
  if (e.state === "pending") return "preparing";
  if (e.state === "failed") return "failed";
  return e.expires_at !== undefined && Date.parse(e.expires_at) <= Date.now() ? "expired" : "ready";
}

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
 * A sentence with each number joined to the word after it (a counter of ja, 「365 日」; "365 days"), so that a line never
 * breaks between the two: the catalog writes a plain space there, and the screen that shows a count joins them.
 */
export function tied(text: string): string {
  return text.replace(/(\p{Nd}) (?=\S)/gu, "$1 ");
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

/** The door to Export: what it holds, or the archive that is being prepared or ready to download. */
function ExportDoor() {
  const { t } = stringsIn(useLocale().locale);
  const date = useActionDate(false);
  const { exports } = useDataExports();
  const [latest] = exports ?? [];
  const ready = exports?.find((e) => archiveStateOf(e) === "ready");
  const standing =
    latest?.state === "pending"
      ? t("settings.account.export.row.standing", { state: "preparing", date: "" })
      : ready !== undefined
        ? t("settings.account.export.row.standing", {
            state: ready.expires_at === undefined ? "ready" : "ready_until",
            date: ready.expires_at === undefined ? "" : date(ready.expires_at),
          })
        : undefined;
  return (
    <SettingsListRow
      title={t("settings.account.export")}
      caption={standing === undefined ? t("settings.account.export.row.about") : undefined}
      standing={standing}
      href={dataExportPath}
    />
  );
}

/** The door to Import: what it holds, or how far the import in progress is. */
function ImportDoor() {
  const { t } = stringsIn(useLocale().locale);
  const { imports } = useDataImports();
  const running = imports?.find((i) => i.state === "in_progress");
  return (
    <SettingsListRow
      title={t("settings.account.import")}
      caption={running === undefined ? t("settings.account.import.row.about") : undefined}
      standing={
        running === undefined
          ? undefined
          : t("settings.account.import.row.standing", { processed: running.processed, total: running.total })
      }
      href={accountImportPath}
    />
  );
}

/**
 * The door to Delete old posts automatically: whether it is on and the age posts are deleted past, or, for a reader
 * the server's maximum binds, that the maximum applies and from which day.
 */
function AutoDeleteDoor() {
  const { t } = stringsIn(useLocale().locale);
  const date = useActionDate(false);
  const { settings } = useAutoDeleteRead();
  const limit = settings?.server_limit;
  const standing =
    settings === undefined
      ? undefined
      : limit?.bound === true
        ? t("settings.account.auto_delete.bound", {
            state:
              limit.enforce_from === undefined ? "soon" : Date.parse(limit.enforce_from) <= Date.now() ? "now" : "from",
            date: limit.enforce_from === undefined ? "" : date(limit.enforce_from),
          })
        : t("settings.account.auto_delete.row.standing", {
            enabled: (limit?.effective ?? settings).enabled ? "on" : "off",
            days: (limit?.effective ?? settings).min_age_days,
          });
  return (
    <SettingsListRow
      title={t("settings.account.auto_delete")}
      caption={standing === undefined ? t("settings.account.auto_delete.row.about") : undefined}
      standing={standing}
      href={autoDeletePath}
    />
  );
}

/** The reader's data and posts, the entry's second part: the doors to Export, Import and the automatic deletion. */
function AccountData() {
  const { t } = stringsIn(useLocale().locale);
  const heading = t("settings.account.data");
  return (
    <SettingsSection heading={heading}>
      <SettingsList label={heading}>
        <ExportDoor />
        <ImportDoor />
        <AutoDeleteDoor />
      </SettingsList>
    </SettingsSection>
  );
}

/** Moving to this account, the entry's third part: the door to Aliases, with how many there are. */
function AccountArrival() {
  const { t } = stringsIn(useLocale().locale);
  const { aliases, status } = useAccountAliasList();
  const heading = t("settings.account.arrival");
  const some = status === "success" && aliases.length > 0;
  return (
    <SettingsSection heading={heading}>
      <SettingsList label={heading}>
        <SettingsListRow
          title={t("settings.account.aliases")}
          caption={some ? undefined : t("settings.account.aliases.row.about")}
          standing={some ? t("settings.account.aliases.row.standing", { count: aliases.length }) : undefined}
          href={accountAliasesPath}
        />
      </SettingsList>
    </SettingsSection>
  );
}

/**
 * The entry's screen: its parts, the account's status first, then the reader's data and posts, then moving to this
 * account; a part that cannot be undone stands after them, last and apart.
 */
export function AccountSettings({ back }: SettingsEntryProps) {
  const { t } = stringsIn(useLocale().locale);
  return (
    <SettingsScreen icon="accountData" title={t("settings.entry.account")} back={back}>
      <AccountStatus />
      <AccountData />
      <AccountArrival />
      <AccountDeparture />
      <AccountEnd />
    </SettingsScreen>
  );
}
