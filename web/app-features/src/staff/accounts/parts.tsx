// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of accounts, appeals, sign-ups and automated reviews share (FR-703–FR-721): the pages of a list by
// the server's cursor, the way back along them kept in the address; the status a press says what it did in, which takes
// the focus; the words of a refused write; a labelled choice; and the words of an action's kind, its state and its end.

import { staffRefusalOf, type AccountAction, type AccountActionKind, type ModeratedAccountState } from "@my/app-data";
import { copyText } from "@my/platform";
import {
  Box,
  Button,
  CaptionLine,
  CopyBlock,
  FieldError,
  FieldLabel,
  FormAlert,
  RecordPages,
  Select,
  Stack,
  StateMark,
  type RecordState,
  type SelectOption,
} from "@my/design-system";
import { useId, useRef, useState, type ReactNode } from "react";
import type { StaffAddress, StaffSearch } from "../entries";
import { useMoment, useStaffStrings, type T } from "../parts";

/** The cursor of the page an address's query asks for; the first page without one. */
export const cursorOf = (search: StaffSearch): string | undefined =>
  search["cursor"] === undefined || search["cursor"] === "" ? undefined : search["cursor"];

/** The cursors of the pages before the one shown, the first page's as empty: the way back by Previous. */
const trailOf = (search: StaffSearch): string[] =>
  search["trail"] === undefined ? [] : search["trail"].split(",").map((c) => decodeURIComponent(c));

/** `search` at the page of `cursor`, after the `trail` of pages before it. */
function atPage(search: StaffSearch, cursor: string | undefined, trail: readonly string[]): StaffSearch {
  const { cursor: _cursor, trail: _trail, ...rest } = search;
  return {
    ...rest,
    ...(cursor === undefined ? {} : { cursor }),
    ...(trail.length === 0 ? {} : { trail: trail.map((c) => encodeURIComponent(c)).join(",") }),
  };
}

/** `search` with `change` made and at the first page, as a change of the filters starts. */
export function filtered(search: StaffSearch, change: StaffSearch): StaffSearch {
  return atPage({ ...search, ...change }, undefined, []);
}

/** Previous and Next under a list, along the pages of the address of `address`, Next to the page of `next`. */
export function Pages({ address, next }: { address: StaffAddress; next: string | undefined }) {
  const { search, onSearch } = address;
  const trail = trailOf(search);
  const previous = trail.at(-1);
  return (
    <RecordPages
      previous={
        previous === undefined
          ? undefined
          : {
              onPress: () => {
                onSearch(atPage(search, previous === "" ? undefined : previous, trail.slice(0, -1)));
              },
            }
      }
      next={
        next === undefined
          ? undefined
          : {
              onPress: () => {
                onSearch(atPage(search, next, [...trail, cursorOf(search) ?? ""]));
              },
            }
      }
    />
  );
}

/** What a press said: its words, whether they say it failed, and a count that makes each saying a new element. */
type Said = { readonly n: number; readonly words: string; readonly alert: boolean };

/**
 * A status of a screen that says what a press did, or that it failed: `say` puts the words in it and moves the focus
 * there, since the press may have taken its own button away; `status` is the region, there from the start.
 */
export function useSaid(): { say: (words: string, alert?: boolean) => void; status: ReactNode } {
  const [said, setSaid] = useState<Said | undefined>(undefined);
  const ref = useRef<HTMLDivElement>(null);
  const say = (words: string, alert = false) => {
    setSaid((s) => ({ n: (s?.n ?? 0) + 1, words, alert }));
    requestAnimationFrame(() => ref.current?.focus());
  };
  const status = (
    <Box as={<div ref={ref} role="status" tabIndex={-1} />}>
      {said !== undefined &&
        (said.alert ? (
          <FormAlert key={said.n} regular>
            {said.words}
          </FormAlert>
        ) : (
          <CaptionLine key={said.n} icon="accepted">
            {said.words}
          </CaptionLine>
        ))}
    </Box>
  );
  return { say, status };
}

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

/** The state of an account, in words, marked as a record's state is. */
export function AccountStateMark({ state }: { state: ModeratedAccountState }) {
  const { t } = useStaffStrings();
  // In force while it may use the server (limited too), waiting while its registration waits, else shut.
  const mark: RecordState =
    state === "active" || state === "limited" ? "active" : state === "pending" ? "pending" : "disabled";
  return <StateMark state={mark}>{t("staff_accounts.account_state", { state })}</StateMark>;
}

/** The state of an action, marked: in effect, ended by itself, or lifted. */
export function ActionStateMark({ action }: { action: AccountAction }) {
  const { t } = useStaffStrings();
  return <StateMark state={action.state}>{t("staff_accounts.action_state", { state: action.state })}</StateMark>;
}

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
