// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of domains, remote servers and the audit log share (FR-702–FR-706, FR-716–FR-718): their addresses;
// the pages of a list by the server's cursor, the way back along them kept in the address; the status a press says what
// it did in, which takes the focus; the words of a refused write; a labelled choice; the words of an action's kind and
// its state; and a neutral button that leads to another screen.

import { staffRefusalOf, type DomainAction, type DomainActionKind } from "@my/app-data";
import {
  Box,
  CaptionLine,
  FieldError,
  FieldLabel,
  FormAlert,
  Icon,
  LinkButton,
  RecordPages,
  Row,
  Select,
  Stack,
  StateMark,
  type IconName,
  type SelectOption,
} from "@my/design-system";
import { useId, useRef, useState, type ReactNode } from "react";
import { staffHref, type StaffAddress, type StaffSearch } from "../entries";
import { useMoment, useStaffStrings, type T } from "../parts";

/** The address of the list of domain actions, of those on `domain` where given. */
export const domainActionsPath = "/moderation/domain-actions";
export const domainActionsOf = (domain: string): string => staffHref(domainActionsPath, { domain });
/** The address of the domain action of `id`. */
export const domainActionPath = (id: string): string => `${domainActionsPath}/${encodeURIComponent(id)}`;
/** The address of the form of a new action, on `domain` where given. */
export const newDomainActionPath = (domain?: string): string =>
  staffHref(`${domainActionsPath}/new`, domain === undefined ? {} : { domain });
/** The address of the import and the export of Mastodon's CSV. */
export const domainImportPath = `${domainActionsPath}/import`;
/** The address of the allowlist of federation. */
export const domainAllowsPath = `${domainActionsPath}/allowed`;
/** The address of the subscriptions to block lists, of a new one, and of the subscription of `id`. */
export const domainListsPath = `${domainActionsPath}/subscriptions`;
export const newDomainListPath = `${domainListsPath}/new`;
export const domainListPath = (id: string): string => `${domainListsPath}/${encodeURIComponent(id)}`;
/** The address of the remote servers, and of the server of `domain`. */
export const remoteServersPath = "/moderation/instances";
export const remoteServerPath = (domain: string): string => `${remoteServersPath}/${encodeURIComponent(domain)}`;
/** The address of the audit log, at `search` where given. */
export const auditLogPath = "/moderation/audit-log";

/** The cursor of the page an address's query asks for; the first page without one. */
export const cursorOf = (search: StaffSearch, name = "cursor"): string | undefined =>
  search[name] === undefined || search[name] === "" ? undefined : search[name];

/** The cursors of the pages before the one shown, the first page's as empty: the way back by Previous. */
const trailOf = (search: StaffSearch, name: string): string[] =>
  search[name] === undefined ? [] : search[name].split(",").map((c) => decodeURIComponent(c));

/** `search` at the page of `cursor`, after the `trail` of pages before it, under the names `of` the list. */
function atPage(
  search: StaffSearch,
  cursor: string | undefined,
  trail: readonly string[],
  of: { cursor: string; trail: string },
): StaffSearch {
  const rest = Object.fromEntries(Object.entries(search).filter(([k]) => k !== of.cursor && k !== of.trail));
  return {
    ...rest,
    ...(cursor === undefined ? {} : { [of.cursor]: cursor }),
    ...(trail.length === 0 ? {} : { [of.trail]: trail.map((c) => encodeURIComponent(c)).join(",") }),
  };
}

const firstList = { cursor: "cursor", trail: "trail" };

/** `search` with `change` made and at the first page, as a change of the filters starts. */
export function filtered(search: StaffSearch, change: StaffSearch): StaffSearch {
  return atPage({ ...search, ...change }, undefined, [], firstList);
}

/**
 * Previous and Next under a list, along the pages of the address of `address`, Next to the page of `next`; a second
 * list of a screen keeps its pages under names of its own (`of`: its cursor's and its trail's).
 */
export function Pages({
  address,
  next,
  of = firstList,
}: {
  address: StaffAddress;
  next: string | undefined;
  of?: { cursor: string; trail: string };
}) {
  const { search, onSearch } = address;
  const trail = trailOf(search, of.trail);
  const previous = trail.at(-1);
  return (
    <RecordPages
      previous={
        previous === undefined
          ? undefined
          : {
              onPress: () => {
                onSearch(atPage(search, previous === "" ? undefined : previous, trail.slice(0, -1), of));
              },
            }
      }
      next={
        next === undefined
          ? undefined
          : {
              onPress: () => {
                onSearch(atPage(search, next, [...trail, cursorOf(search, of.cursor) ?? ""], of));
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

/**
 * A sentence with each number joined to the word after it (「12 件」, "12 follows"), so that a line never breaks between
 * the two: the catalog writes a plain space there.
 */
export const tied = (text: string): string => text.replace(/(\p{Nd}) (?=\S)/gu, "$1 ");

/** What the reader is told of a refused write of theirs, by what the server's problem means. */
export const refusalWords = (t: T, error: unknown): string =>
  t("staff_domains.refused", { why: staffRefusalOf(error) });

/** A choice from a list under its label, with what is wrong with it under it. */
export function LabelledSelect({
  id: given,
  label,
  options,
  value,
  onChange,
  error,
  isDisabled,
}: {
  /** The id of the choice, which a refusal's focus finds it by; one of its own where left out. */
  id?: string | undefined;
  label: string;
  options: readonly SelectOption[];
  value: string;
  onChange: (value: string) => void;
  error?: string | undefined;
  isDisabled?: boolean | undefined;
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
        isDisabled={isDisabled}
      />
      {error !== undefined && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}

/** The kind of a domain action, in words ("Suspend"). */
export const domainKindWords = (t: T, kind: DomainActionKind): string => t("staff_domains.kind", { kind });

/** The state of a domain action, marked: in effect, ended by itself, or lifted. */
export function DomainActionStateMark({ action }: { action: DomainAction }) {
  const { t } = useStaffStrings();
  return <StateMark state={action.state}>{t("staff_domains.action_state", { state: action.state })}</StateMark>;
}

/** When a domain action ends: the moment, or that it lasts until it is lifted. */
export function useEnd(): (expires: string | undefined) => string {
  const { t } = useStaffStrings();
  const moment = useMoment();
  return (expires) => (expires === undefined ? t("staff_domains.until_lifted") : moment(expires));
}

/** A neutral button that leads to another screen, its glyph before its words. */
export function GoButton({ href, icon, children }: { href: string; icon: IconName; children: string }) {
  return (
    <LinkButton href={href} size="timeline">
      <Row as="span" inline align="center" gap="entry-caption">
        <Icon name={icon} place="caption" />
        {children}
      </Row>
    </LinkButton>
  );
}
