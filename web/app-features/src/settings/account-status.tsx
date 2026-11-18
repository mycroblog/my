// SPDX-License-Identifier: AGPL-3.0-only

// Account status, the screen under the settings' entry Account and data at /settings/account/status (FR-715,
// ADR-0058 §9): where the reader comes to see where their account stands with the moderators. Two lists, each a
// section: the actions in effect now, and the past ones (ended, or lifted), newest first, a page at a time. A row
// says what is known of an action, the whole of it a link to the action's own screen (account-action.tsx), where its
// posts are and where it is appealed: what it does, in the server's words for its kind; the moderators' explanation
// to the reader, where they gave one; when it ends, ended or was lifted; and where its appeal stands (it can be
// appealed until a day, it no longer can, the appeal waits, or what the moderators decided). Neither who reported the
// account nor the moderators' own notes are shown: the server does not send them. An empty list says what would stand
// there.

import { ownModerationActionsQuery, useOwnModerationActions, type AppData, type ModerationAction } from "@my/app-data";
import {
  BackButton,
  Button,
  dateFormat,
  Row,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  TextButton,
  useLocale,
  useTimeZone,
} from "@my/design-system";
import { stringsIn } from "../locale";
import { moderationActionPath } from "./account";
import { ReadingNotice } from "./controls";

type T = ReturnType<typeof stringsIn>["t"];

/** What the route's loader waits for: the actions in effect and the newest of them all, from the cache when it has them. */
export async function loadAccountStatus(data: AppData): Promise<void> {
  await Promise.all([
    data.queryClient.infiniteQuery({ ...ownModerationActionsQuery(data, true), staleTime: Infinity }),
    data.queryClient.infiniteQuery({ ...ownModerationActionsQuery(data), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/** The title of the screen. */
export function useAccountStatusTitle(): string {
  return stringsIn(useLocale().locale).t("settings.account.status");
}

/** A moment of an action (when it ends, when it was appealed), as the reader's locale and zone write it. */
export function useActionDate(): (at: string) => string {
  const { locale } = useLocale();
  const zone = useTimeZone();
  const format = dateFormat(locale, zone, { dateStyle: "medium", timeStyle: "short" });
  return (at) => format.format(new Date(at));
}

/** When `action` ends, ended or was lifted, as a line. */
export function endsLine(action: ModerationAction, date: (at: string) => string, t: T): string {
  if (action.state === "lifted" && action.lifted_at !== undefined)
    return t("settings.account.action.ends", { state: "lifted", date: date(action.lifted_at) });
  if (action.expires_at === undefined) return t("settings.account.action.ends", { state: "open", date: "" });
  return t("settings.account.action.ends", {
    state: action.state === "active" ? "until" : "expired",
    date: date(action.expires_at),
  });
}

/** Whether the last moment to appeal `action`, where the server says one, has passed. */
export const appealPassed = (action: ModerationAction): boolean =>
  action.appeal_deadline !== undefined && Date.parse(action.appeal_deadline) <= Date.now();

/** Where the appeal of `action` stands, as a line: what the moderators decided, that it waits, or whether it can be made. */
function appealLine(action: ModerationAction, date: (at: string) => string, t: T): string {
  const deadline = action.appeal_deadline === undefined ? "" : date(action.appeal_deadline);
  if (action.appeal !== undefined)
    return t("settings.account.action.appeal_state", { state: action.appeal.state, date: "" });
  if (action.appealable)
    return t("settings.account.action.appeal_state", {
      state: deadline === "" ? "open_always" : "open",
      date: deadline,
    });
  return t("settings.account.action.appeal_state", {
    state: appealPassed(action) ? "passed" : "closed",
    date: deadline,
  });
}

/** The rows of `actions`, each the door to its action's screen. */
function ActionList({ label, actions, t }: { label: string; actions: readonly ModerationAction[]; t: T }) {
  const date = useActionDate();
  return (
    <SettingsList label={label}>
      {actions.map((action) => (
        <SettingsListRow
          key={action.id}
          title={action.kind_label}
          caption={action.public_reason}
          facts={[endsLine(action, date, t), appealLine(action, date, t)]}
          href={moderationActionPath(action.id)}
        />
      ))}
    </SettingsList>
  );
}

/** The screen: the actions in effect and the past ones, once they are read; `onBack` goes back to the entry. */
export function AccountStatusScreen({ onBack }: { onBack: () => void }) {
  const { t, lang } = stringsIn(useLocale().locale);
  const active = useOwnModerationActions(true);
  const all = useOwnModerationActions();
  const title = t("settings.account.status");
  const back = <BackButton onPress={onBack}>{t("settings.entry.account")}</BackButton>;
  const pending = active.status === "pending" || all.status === "pending";
  if (pending || active.status === "error" || all.status === "error")
    return (
      <SettingsScreen
        sub
        title={title}
        back={back}
        notice={
          <ReadingNotice
            failed={!pending}
            onRetry={() => {
              active.refetch();
              all.refetch();
            }}
          />
        }
      />
    );
  const count = (n: number) => (n === 0 ? undefined : new Intl.NumberFormat(lang).format(n));
  const past = all.items.filter((action) => action.state !== "active");
  const activeHeading = t("settings.account.actions.active");
  const pastHeading = t("settings.account.actions.past");
  const failedMore = all.error !== null && all.hasNextPage;
  return (
    <SettingsScreen sub title={title} back={back}>
      <SettingsSection heading={activeHeading} count={active.hasNextPage ? undefined : count(active.items.length)}>
        {active.items.length === 0 ? (
          <SettingsEmpty>{t("settings.account.actions.active.empty")}</SettingsEmpty>
        ) : (
          <ActionList label={activeHeading} actions={active.items} t={t} />
        )}
      </SettingsSection>
      <SettingsSection heading={pastHeading} count={all.hasNextPage ? undefined : count(past.length)}>
        {past.length > 0 ? (
          <ActionList label={pastHeading} actions={past} t={t} />
        ) : (
          !all.hasNextPage && <SettingsEmpty>{t("settings.account.actions.past.empty")}</SettingsEmpty>
        )}
        {failedMore && !all.isFetchingNextPage ? (
          <SettingsEmpty action={<TextButton onPress={all.fetchNextPage}>{t("settings.retry")}</TextButton>}>
            {t("settings.account.actions.failed_more")}
          </SettingsEmpty>
        ) : (
          all.hasNextPage && (
            <Row justify="start">
              <Button size="timeline" busy={all.isFetchingNextPage} onPress={all.fetchNextPage}>
                {t("settings.account.actions.more")}
              </Button>
            </Row>
          )
        )}
      </SettingsSection>
    </SettingsScreen>
  );
}
