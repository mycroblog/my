// SPDX-License-Identifier: AGPL-3.0-only

// An account as a holder of `manage_accounts` moderates it (FR-703, FR-713, FR-714, ADR-0058): three pages under its
// handle, Review (/moderation/accounts/{id}), Actions (…/actions, and each action under it) and Management (…/manage,
// for a local account, or a remote one the reader may refetch the posts of), between which its tabs lead. Review holds
// what an action is decided on and nothing else (NFR-MOD-4): the account's state and what its actions take away, its
// registration and who invited it, the form of a new action, its newest actions and its newest reports.

import { moderatedAccountPath, reportPath, sentenceHandleOf } from "@my/app-components";
import {
  usePermissions,
  useModeratedAccount,
  type AccountAction,
  type ModeratedAccountDetail,
  type QueuedReport,
} from "@my/app-data";
import {
  BackButton,
  FactList,
  FeedNotice,
  RecordTable,
  RelativeTime,
  ScreenStack,
  ScreenTitle,
  Stack,
  StateMark,
  TabRow,
  TextLink,
  type Fact,
  type RecordColumn,
  type RecordRow,
} from "@my/design-system";
import type { ReactNode } from "react";
import { staffEntryPath } from "../entries";
import { ReadNotice, useMoment, useStaffStrings, type T } from "../parts";
import { categoryWords } from "../reports/queue";
import { ActionForm } from "./action-form";
import { AccountActionsPage } from "./actions";
import { ManagePage } from "./manage";
import { AccountStateMark, actionCategoryWords, ActionStateMark, bannedBy, kindWords, useEnd, Section } from "./parts";

/** The pages of an account's moderation. */
export type AccountPage = "review" | "actions" | "manage";

/** The address of the page of the account of `id`. */
export const accountPagePath = (id: string, page: AccountPage): string =>
  page === "review" ? moderatedAccountPath(id) : `${moderatedAccountPath(id)}/${page}`;

/** The address of the action of `actionId` on the account of `id`. */
export const accountActionPath = (id: string, actionId: string): string =>
  `${moderatedAccountPath(id)}/actions/${encodeURIComponent(actionId)}`;

export type AccountScreenProps = {
  id: string;
  page: AccountPage;
  /** The report the reader came from (`?report=` of the address), whose posts an action on posts names. */
  report?: string | undefined;
  /** The query of the page's address (the actions' filter and page). */
  search: Readonly<Record<string, string | undefined>>;
  onSearch: (search: Readonly<Record<string, string | undefined>>) => void;
  /** Shows another page of the account. */
  onPage: (page: AccountPage) => void;
  /** Goes back to the report the reader came from, else to the search of accounts. */
  onBack: () => void;
};

/** The title of an account's screens, its handle; undefined until it is read. */
export function useModeratedAccountTitle(id: string): string | undefined {
  const { value } = useModeratedAccount(id);
  return value === undefined ? undefined : sentenceHandleOf(value.account);
}

/** Whether the reader has a page of Management on `account`: its local, or the reader may refetch its posts. */
const managed = (account: ModeratedAccountDetail, permissions: readonly string[]) =>
  account.local || permissions.includes("manage_domains");

/** The account of `id` on its page, or why it shows none. */
export function AccountScreen(props: AccountScreenProps): ReactNode {
  const { t } = useStaffStrings();
  const read = useModeratedAccount(props.id);
  const permissions = usePermissions();
  if (read.missing)
    return (
      <FeedNotice
        alert
        lead
        action={<TextLink href={staffEntryPath("moderation", "accounts")}>{t("staff_accounts.account.all")}</TextLink>}
      >
        {t("staff_accounts.account.missing")}
      </FeedNotice>
    );
  if (read.forbidden) return <FeedNotice lead>{t("staff_accounts.forbidden")}</FeedNotice>;
  if (read.value === undefined)
    return (
      <ReadNotice
        status={read.status === "error" ? "error" : "pending"}
        loading={t("staff_accounts.account.loading")}
        failed={t("staff_accounts.account.failed")}
        retry={read.refetch}
      />
    );
  const account = read.value;
  const pages: { id: AccountPage; name: string }[] = [
    { id: "review", name: t("staff_accounts.page.review") },
    { id: "actions", name: t("staff_accounts.page.actions") },
    ...(managed(account, permissions) ? [{ id: "manage" as const, name: t("staff_accounts.page.manage") }] : []),
  ];
  const page = pages.some((p) => p.id === props.page) ? props.page : "review";
  return (
    <ScreenStack>
      <ScreenTitle
        back={
          <BackButton onPress={props.onBack}>
            {t(props.report === undefined ? "staff_accounts.account.back" : "staff_accounts.account.back_report")}
          </BackButton>
        }
      >
        {sentenceHandleOf(account.account)}
      </ScreenTitle>
      <TabRow label={t("staff_accounts.page.tabs")} tabs={pages} current={page} onChange={props.onPage}>
        {page === "review" ? (
          <Review account={account} report={props.report} />
        ) : page === "actions" ? (
          <AccountActionsPage account={account} search={props.search} onSearch={props.onSearch} />
        ) : (
          <ManagePage account={account} />
        )}
      </TabRow>
    </ScreenStack>
  );
}

/** Who created the invite an account signed up with, in words. */
function invitedBy(t: T, invite: NonNullable<NonNullable<ModeratedAccountDetail["registration"]>["invite"]>): string {
  if (invite.created_by !== undefined)
    return t("staff_accounts.invited_by", { handle: sentenceHandleOf(invite.created_by) });
  return t(invite.created_by_deleted ? "staff_accounts.invited_by.gone" : "staff_accounts.invited_by.cli");
}

/** What is known of the account: its state, what is taken away, where it is from, its registration and its sign-in. */
function useAccountFacts(account: ModeratedAccountDetail): Fact[] {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const r = account.registration;
  return [
    {
      term: t("staff_accounts.fact.state"),
      value: <AccountStateMark state={account.state} banned={bannedBy(account.actions)} />,
    },
    {
      term: t("staff_accounts.fact.restrictions"),
      value:
        account.restrictions.length === 0 ? (
          t("staff_accounts.restrictions.none")
        ) : (
          // One a line, each whole: a list joined in a sentence broke inside its items on a phone.
          <Stack as="ul" gap="entry-line">
            {account.restrictions.map((x) => (
              <li key={x}>{t("staff_accounts.restriction", { restriction: x })}</li>
            ))}
          </Stack>
        ),
    },
    {
      term: t("staff_accounts.fact.origin"),
      value: account.local
        ? t("staff_accounts.origin.local")
        : account.account.acct.slice(account.account.acct.indexOf("@") + 1),
    },
    { term: t("staff_accounts.fact.created"), value: moment(account.account.created_at) },
    ...(r === undefined
      ? []
      : [
          { term: t("staff_accounts.fact.signed_up"), value: moment(r.submitted_at) },
          ...(account.registration_state === undefined
            ? []
            : [
                {
                  term: t("staff_accounts.fact.registration"),
                  value: t("staff_accounts.registration_state", { state: account.registration_state }),
                },
              ]),
          ...(r.reason === undefined ? [] : [{ term: t("staff_accounts.fact.reason"), value: <bdi>{r.reason}</bdi> }]),
          ...(r.invite === undefined ? [] : [{ term: t("staff_accounts.fact.invite"), value: invitedBy(t, r.invite) }]),
          ...(r.decision_note === undefined
            ? []
            : [{ term: t("staff_accounts.fact.decision_note"), value: <bdi>{r.decision_note}</bdi> }]),
        ]),
    ...(account.email_verified === undefined
      ? []
      : [
          {
            term: t("staff_accounts.fact.email_verified"),
            value: t(account.email_verified ? "staff_accounts.email.verified" : "staff_accounts.email.unverified"),
          },
        ]),
    ...(account.local && account.roles.length > 0
      ? [{ term: t("staff_accounts.fact.roles"), value: account.roles.join(", ") }]
      : []),
  ];
}

/** Review: the account's facts, the form of a new action, its newest actions and its newest reports. */
function Review({ account, report }: { account: ModeratedAccountDetail; report: string | undefined }) {
  const { t } = useStaffStrings();
  const facts = useAccountFacts(account);
  return (
    <ScreenStack>
      <FactList label={t("staff_accounts.account.facts")} items={facts} />
      <Section title={t("staff_accounts.act.title")}>
        <ActionForm account={account} report={report} />
      </Section>
      <Section title={t("staff_accounts.account.actions")}>
        <ActionTable account={account.id} actions={account.actions} />
      </Section>
      {account.reports !== undefined && (
        <Section title={t("staff_accounts.account.reports")}>
          <ReportTable reports={account.reports} />
        </Section>
      )}
    </ScreenStack>
  );
}

type ActionColumn = "category" | "issued" | "ends" | "state";

/** A table of actions on the account of `account`, each opening its own page. */
export function ActionTable({
  account,
  actions,
  busy,
  empty,
}: {
  account: string;
  actions: readonly AccountAction[];
  busy?: boolean | undefined;
  empty?: ReactNode;
}) {
  const { t } = useStaffStrings();
  const end = useEnd();
  const columns: RecordColumn<ActionColumn>[] = [
    { key: "category", label: t("staff_accounts.actions.column.category"), bare: true },
    { key: "issued", label: t("staff_accounts.actions.column.issued"), kind: "fit", bare: true },
    { key: "ends", label: t("staff_accounts.actions.column.ends"), kind: "fit" },
    { key: "state", label: t("staff_accounts.actions.column.state"), kind: "fit", beside: true },
  ];
  const rows = actions.map((a): RecordRow<ActionColumn> => ({
    id: a.id,
    head: kindWords(t, a.kind),
    caption: a.public_reason,
    href: accountActionPath(account, a.id),
    cells: {
      category: actionCategoryWords(t, a.reason_category),
      issued: <RelativeTime date={a.issued_at} />,
      ends: a.kind === "ban" ? t("staff_accounts.ends.never") : end(a.expires_at),
      state: <ActionStateMark action={a} />,
    },
  }));
  return (
    <RecordTable
      label={t("staff_accounts.actions.table")}
      head={{ label: t("staff_accounts.actions.column.kind") }}
      columns={columns}
      rows={rows}
      busy={busy}
      empty={empty ?? <FeedNotice start>{t("staff_accounts.actions.none")}</FeedNotice>}
      even
    />
  );
}

type ReportColumn = "filed" | "state";

/** The account's newest reports, each its category, opening its screen; who filed it is the report's to say (§9). */
function ReportTable({ reports }: { reports: readonly QueuedReport[] }) {
  const { t } = useStaffStrings();
  const columns: RecordColumn<ReportColumn>[] = [
    { key: "filed", label: t("staff_accounts.reports.column.filed"), kind: "fit", bare: true },
    { key: "state", label: t("staff_accounts.reports.column.state"), kind: "fit", beside: true },
  ];
  const rows = reports.map((r): RecordRow<ReportColumn> => ({
    id: r.id,
    head: categoryWords(t, r.category),
    href: reportPath(r.id),
    cells: {
      filed: <RelativeTime date={r.filed_at} />,
      state: (
        <StateMark state={r.state === "open" ? "open" : "resolved"}>
          {t("staff_accounts.report_state", { state: r.state })}
        </StateMark>
      ),
    },
  }));
  return (
    <RecordTable
      label={t("staff_accounts.reports.table")}
      head={{ label: t("staff_accounts.reports.column.category") }}
      columns={columns}
      rows={rows}
      empty={<FeedNotice start>{t("staff_accounts.reports.none")}</FeedNotice>}
    />
  );
}
