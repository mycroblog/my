// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of accounts, appeals, sign-ups and automated reviews share (FR-703–FR-721): the pages of a list by
// the server's cursor, the way back along them kept in the address; the status a press says what it did in, which takes
// the focus; the words of a refused write; a labelled choice; and the words of an action's kind, its state and its end.

import { sentenceHandleOf } from "@my/app-components";
import {
  staffRefusalOf,
  type Account,
  type AccountAction,
  type AccountActionKind,
  type ModeratedAccountState,
} from "@my/app-data";
import { copyText } from "@my/platform";
import {
  Button,
  CopyBlock,
  FieldError,
  FieldLabel,
  Select,
  Stack,
  StateMark,
  type RecordState,
  type SelectOption,
} from "@my/design-system";
import { useId, useState } from "react";
import { useMoment, useStaffStrings, type T } from "../parts";

export { cursorOf, filtered, Pages, useSaid } from "../parts";

/** What the reader is told of a refused write of theirs, by what the server's problem means. */
export function refusalWords(t: T, error: unknown): string {
  return t("staff_accounts.refused", { why: staffRefusalOf(error) });
}

/** A choice from a list under its label, with what is wrong with it under it. */
export function LabelledSelect({
  id: given,
  label,
  options,
  value,
  onChange,
  error,
}: {
  /** The id of the choice, which a refusal's focus finds it by; one of its own where left out. */
  id?: string | undefined;
  label: string;
  options: readonly SelectOption[];
  value: string;
  onChange: (value: string) => void;
  error?: string | undefined;
}) {
  const own = useId();
  const id = given ?? own;
  return (
    <Stack gap="entry-line">
      <FieldLabel htmlFor={id} id={`${id}-label`}>
        {label}
      </FieldLabel>
      <Select
        id={id}
        labelledBy={`${id}-label`}
        describedBy={error === undefined ? undefined : `${id}-error`}
        options={options}
        value={value}
        onChange={onChange}
      />
      {error !== undefined && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}

/** The kind of an action, in words ("Limit"). */
export const kindWords = (t: T, kind: AccountActionKind): string => t("staff_accounts.kind", { kind });

/** How each state of an account is marked: the graver what it shuts out, the stronger. */
const accountMarks: Readonly<Record<ModeratedAccountState, RecordState>> = {
  active: "active",
  limited: "limited",
  pending: "pending",
  disabled: "suspended",
  suspended: "suspended",
  rejected: "disabled",
  deactivated: "disabled",
  deleted: "disabled",
};

/**
 * The state of an account, in words, marked as a record's state is; `banned`, a suspension that a ban in effect made,
 * named and marked as the ban, which nothing lifts.
 */
export function AccountStateMark({ state, banned = false }: { state: ModeratedAccountState; banned?: boolean }) {
  const { t } = useStaffStrings();
  if (banned && state === "suspended")
    return <StateMark state="permanent">{t("staff_accounts.account_state", { state: "banned" })}</StateMark>;
  return <StateMark state={accountMarks[state]}>{t("staff_accounts.account_state", { state })}</StateMark>;
}

/** Whether a ban is in effect among `actions`. */
export const bannedBy = (actions: readonly AccountAction[]): boolean =>
  actions.some((a) => a.kind === "ban" && a.state === "active");

/** The state of an action, marked: in effect (a ban in effect, permanent), ended by itself, or lifted. */
export function ActionStateMark({ action }: { action: AccountAction }) {
  const { t } = useStaffStrings();
  const state = action.kind === "ban" && action.state === "active" ? "permanent" : action.state;
  return <StateMark state={state}>{t("staff_accounts.action_state", { state })}</StateMark>;
}

/** The kind of reason of an action, in the words the account it is on reads. */
export const actionCategoryWords = (t: T, category: AccountAction["reason_category"]): string =>
  t("staff_accounts.action_category", { category });

/** When an action or a block ends: the moment, or that it lasts until it is lifted. */
export function useEnd(): (expires: string | undefined) => string {
  const { t } = useStaffStrings();
  const moment = useMoment();
  return (expires) => (expires === undefined ? t("staff_accounts.until_lifted") : moment(expires));
}

/** The lengths an action or a block may last for, as a choice: until lifted, or a number of days. */
const lengths = [0, 1, 7, 30, 90, 365] as const;

/** The choice of how long something lasts, its value the days ("0" for until lifted). */
export function lengthOptions(t: T): SelectOption[] {
  return lengths.map((days) => ({
    value: String(days),
    label: days === 0 ? t("staff_accounts.until_lifted") : t("staff_accounts.length.days", { count: days }),
  }));
}

/** The seconds of a choice of `lengthOptions`; undefined for until lifted. */
export const secondsOf = (days: string): number | undefined => (days === "0" ? undefined : Number(days) * 86_400);

/**
 * A value the reader copies and passes on (a password reset link, an invite's link), named `label`, with the button
 * that copies it, named `copy`, the screen's primary action, and what came of copying.
 */
export function CopyValue({ label, value, copy }: { label: string; value: string; copy: string }) {
  const { t } = useStaffStrings();
  const [copied, setCopied] = useState<boolean>();
  return (
    <CopyBlock
      label={label}
      values={[value]}
      action={
        <Button
          kind="primary"
          size="timeline"
          onPress={() => {
            void copyText(value).then(setCopied);
          }}
        >
          {copy}
        </Button>
      }
      result={copied === undefined ? undefined : t(copied ? "staff_accounts.copied" : "staff_accounts.copy_failed")}
    />
  );
}

/**
 * The handle a staff screen names the account of `id` by, from what `useStaffAccounts` read: while it is read, the
 * words for that, never that it is gone; the words for a gone account once the server said so.
 */
export function whoIn(t: T, accounts: ReadonlyMap<string, Account | null>, id: string | undefined): string {
  if (id === undefined) return t("staff.account_gone");
  const account = accounts.get(id);
  if (account === undefined) return t("staff_accounts.account.loading");
  return account === null ? t("staff.account_gone") : sentenceHandleOf(account);
}
