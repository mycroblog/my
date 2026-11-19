// SPDX-License-Identifier: AGPL-3.0-only

// The actions on an account (FR-703, FR-713): every one, a page at a time, or those in effect alone, each opening its
// page at /moderation/accounts/{id}/actions/{actionId}; and that page, with all an action holds (its reason, the
// moderators' note, who took it, when it ends or was lifted and why, the report it follows from) and, while it is in
// effect, Lift, with the reason for lifting. A ban cannot be lifted: its page says that only an appeal can overturn
// it, with the way to its appeal where there is one.

import { reportPath, sentenceHandleOf } from "@my/app-components";
import {
  actionTextLimit,
  refusedMember,
  useAccountAction,
  useAccountActions,
  useAccountWrites,
  useModeratedAccount,
  useModeratedAppeals,
  useStaffAccounts,
  type AccountAction,
  type ModeratedAccountDetail,
} from "@my/app-data";
import {
  BackButton,
  Button,
  FactList,
  FeedNotice,
  FilterBar,
  FilterToggle,
  FormAlert,
  Row,
  ScreenStack,
  ScreenTitle,
  Stack,
  TextField,
  TextLink,
  type Fact,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import type { StaffSearch } from "../entries";
import { ReadNotice, useMoment, useStaffStrings } from "../parts";
import { appealHref } from "./appeals";
import { accountPagePath, ActionTable } from "./account";
import {
  actionCategoryWords,
  ActionStateMark,
  cursorOf,
  filtered,
  kindWords,
  Pages,
  refusalWords,
  useEnd,
  useSaid,
  Section,
  whoIn,
} from "./parts";

/** The actions page of an account: every action, or those in effect, a page at a time. */
export function AccountActionsPage({
  account,
  search,
  onSearch,
}: {
  account: ModeratedAccountDetail;
  search: StaffSearch;
  onSearch: (search: StaffSearch) => void;
}): ReactNode {
  const { t } = useStaffStrings();
  const active = search["active"] === "true";
  const page = useAccountActions(account.id, active, cursorOf(search));
  const filters = (
    <FilterBar
      label={t("staff_accounts.actions.filters")}
      applied={active}
      onClear={() => {
        onSearch({});
      }}
    >
      <FilterToggle
        label={t("staff_accounts.actions.active")}
        on={active}
        onChange={(on) => {
          onSearch(filtered(search, { active: on ? "true" : undefined }));
        }}
      />
    </FilterBar>
  );
  if (page.status !== "success")
    return (
      <ScreenStack>
        {filters}
        <ReadNotice
          status={page.status}
          loading={t("staff_accounts.actions.loading")}
          failed={t("staff_accounts.actions.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  return (
    <ScreenStack busy={page.stale}>
      {filters}
      <ActionTable
        account={account.id}
        actions={page.items}
        busy={page.stale}
        empty={
          <FeedNotice>{t(active ? "staff_accounts.actions.none_active" : "staff_accounts.actions.none")}</FeedNotice>
        }
      />
      <Pages address={{ search, onSearch }} next={page.next} />
    </ScreenStack>
  );
}

export type AccountActionScreenProps = {
  /** The account the action is on, as the address names it. */
  account: string;
  id: string;
  /** Goes back to the account's actions. */
  onBack: () => void;
};

/** The title of an action's page: its kind and the account it is on; undefined until both are read. */
export function useAccountActionTitle(account: string, id: string): string | undefined {
  const { t } = useStaffStrings();
  const action = useAccountAction(id).value;
  const target = useModeratedAccount(account).value;
  return action === undefined || target === undefined
    ? undefined
    : t("staff_accounts.action.title", { kind: kindWords(t, action.kind), handle: sentenceHandleOf(target.account) });
}

/** The action of `id` on its page, or why it shows none. */
export function AccountActionScreen({ account, id, onBack }: AccountActionScreenProps): ReactNode {
  const { t } = useStaffStrings();
  const read = useAccountAction(id);
  const title = useAccountActionTitle(account, id);
  // Said apart from Lift, which goes as the action is lifted.
  const { say, status } = useSaid();
  if (read.missing || (read.value !== undefined && read.value.account_id !== account))
    return (
      <FeedNotice
        alert
        lead
        action={<TextLink href={accountPagePath(account, "actions")}>{t("staff_accounts.action.all")}</TextLink>}
      >
        {t("staff_accounts.action.missing")}
      </FeedNotice>
    );
  if (read.forbidden) return <FeedNotice lead>{t("staff_accounts.forbidden")}</FeedNotice>;
  if (read.value === undefined)
    return (
      <ReadNotice
        status={read.status === "error" ? "error" : "pending"}
        loading={t("staff_accounts.action.loading")}
        failed={t("staff_accounts.action.failed")}
        retry={read.refetch}
      />
    );
  const action = read.value;
  return (
    <ScreenStack>
      <ScreenTitle back={<BackButton onPress={onBack}>{t("staff_accounts.action.back")}</BackButton>}>
        {title ?? kindWords(t, action.kind)}
      </ScreenTitle>
      <ActionFacts action={action} />
      {status}
      {action.kind === "ban" ? (
        <BanNotice action={action} />
      ) : (
        action.state === "active" && <Lift action={action} say={say} />
      )}
    </ScreenStack>
  );
}

/**
 * In a ban's place of Lift: that it cannot be lifted, in the screen's own text, and the way to its appeal where one
 * was made (among the newest appeals).
 */
function BanNotice({ action }: { action: AccountAction }) {
  const { t } = useStaffStrings();
  const appeal = useModeratedAppeals(undefined, undefined).items.find((a) => a.action.id === action.id);
  return (
    <FeedNotice
      lead
      action={
        appeal === undefined ? undefined : (
          <TextLink href={appealHref(appeal.id, {})}>{t("staff_accounts.action.appeal_open")}</TextLink>
        )
      }
    >
      {t("staff_accounts.action.ban_lifted_by_appeal")}
    </FeedNotice>
  );
}

/** All an action holds, as facts. */
function ActionFacts({ action }: { action: AccountAction }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const end = useEnd();
  const people = useStaffAccounts([action.issued_by_account_id]);
  const items: Fact[] = [
    { term: t("staff_accounts.action.fact.state"), value: <ActionStateMark action={action} /> },
    { term: t("staff_accounts.action.fact.category"), value: actionCategoryWords(t, action.reason_category) },
    {
      term: t("staff_accounts.action.fact.public_reason"),
      value: action.public_reason === undefined ? t("staff_accounts.none") : <bdi>{action.public_reason}</bdi>,
    },
    ...(action.private_note === undefined
      ? []
      : [{ term: t("staff_accounts.action.fact.private_note"), value: <bdi>{action.private_note}</bdi> }]),
    { term: t("staff_accounts.action.fact.issued"), value: moment(action.issued_at) },
    {
      term: t("staff_accounts.action.fact.issuer"),
      value:
        action.issued_by_account_id === undefined
          ? t("staff_accounts.action.issuer.cli")
          : whoIn(t, people, action.issued_by_account_id),
    },
    {
      term: t("staff_accounts.action.fact.notified"),
      value: t(action.notify_target ? "staff_accounts.action.notified.yes" : "staff_accounts.action.notified.no"),
    },
    {
      term: t("staff_accounts.action.fact.ends"),
      value: action.kind === "ban" ? t("staff_accounts.ends.never") : end(action.expires_at),
    },
    ...(action.lifted_at === undefined
      ? []
      : [{ term: t("staff_accounts.action.fact.lifted"), value: moment(action.lifted_at) }]),
    ...(action.lift_reason === undefined
      ? []
      : [{ term: t("staff_accounts.action.fact.lift_reason"), value: <bdi>{action.lift_reason}</bdi> }]),
    ...(action.report_id === undefined
      ? []
      : [
          {
            term: t("staff_accounts.action.fact.report"),
            value: <TextLink href={reportPath(action.report_id)}>{t("staff_accounts.action.report_open")}</TextLink>,
          },
        ]),
    ...(action.status_ids === undefined
      ? []
      : [
          {
            term: t("staff_accounts.action.fact.posts"),
            value: t("staff_accounts.action.posts", { count: action.status_ids.length }),
          },
        ]),
  ];
  return <FactList label={t("staff_accounts.action.facts")} items={items} />;
}

/** Lift, with the reason for lifting, which the moderators read. */
function Lift({ action, say }: { action: AccountAction; say: (words: string) => void }) {
  const { t } = useStaffStrings();
  const { lift } = useAccountWrites();
  const id = useId();
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | undefined>(undefined);
  const [alert, setAlert] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const send = () => {
    if (busy) return;
    setBusy(true);
    setError(undefined);
    setAlert(undefined);
    lift(action.id, reason.trim()).then(
      () => {
        setBusy(false);
        say(t("staff_accounts.action.lifted", { kind: kindWords(t, action.kind) }));
      },
      (e: unknown) => {
        setBusy(false);
        const member = refusedMember(e);
        if (member?.member === "reason") {
          setError(member.said);
          document.getElementById(id)?.focus();
        } else setAlert(refusalWords(t, e));
      },
    );
  };
  return (
    <Section title={t("staff_accounts.action.lift")}>
      <Stack
        gap="screen-field-gap"
        as={
          <form
            onSubmit={(e) => {
              e.preventDefault();
              send();
            }}
          />
        }
      >
        <TextField
          id={id}
          name="reason"
          label={t("staff_accounts.action.lift.reason")}
          multiline
          rows={2}
          maxLength={actionTextLimit}
          error={error}
          onChange={setReason}
        />
        {alert !== undefined && <FormAlert regular>{alert}</FormAlert>}
        <Row>
          <Button type="submit" busy={busy}>
            {t("staff_accounts.action.lift.submit")}
          </Button>
        </Row>
      </Stack>
    </Section>
  );
}
