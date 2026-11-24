// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of domains, remote servers and the audit log share (FR-702–FR-706, FR-716–FR-718): their addresses;
// the status a press says what it did in, which takes the focus; the words of a refused write; a labelled choice; the
// words of an action's kind and its state; and a neutral button that leads to another screen.

import { staffRefusalOf, type DomainAction, type DomainActionKind } from "@my/app-data";
import {
  FieldError,
  FieldLabel,
  Icon,
  LinkButton,
  ListTitle,
  Row,
  Select,
  Stack,
  StateMark,
  type IconName,
  type SelectOption,
} from "@my/design-system";
import { useId, type ReactNode } from "react";
import { staffHref } from "../entries";
import { useMoment, useStaffStrings, type T } from "../parts";

export { cursorOf, filtered, useAddressPages } from "../parts";

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

/**
 * The state of a domain action, marked: one in effect by what it does (a suspend shut, a limit or a rejection held
 * back), else ended by itself, or lifted.
 */
export function DomainActionStateMark({ action }: { action: DomainAction }) {
  const { t } = useStaffStrings();
  const state = action.state !== "active" ? action.state : action.kind === "suspend" ? "suspended" : "limited";
  return <StateMark state={state}>{t("staff_domains.action_state", { state: action.state })}</StateMark>;
}

/** When a domain action ends: the moment, or that it lasts until it is lifted. */
export function useEnd(): (expires: string | undefined) => string {
  const { t } = useStaffStrings();
  const moment = useMoment();
  return (expires) => (expires === undefined ? t("staff_domains.until_lifted") : moment(expires));
}

/**
 * A section of a screen that holds fields or lists (the import, the drafts, the deliveries), under a title of the
 * list's size: never smaller than the labels of the fields under it.
 */
export function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Stack gap="entry-room">
      <ListTitle>{title}</ListTitle>
      {children}
    </Stack>
  );
}

/**
 * The places a screen leads to about what it shows (its server, its other actions, the audit log), apart from what the
 * reader does to it and quieter: quiet buttons that lead on (`Way`), a finger's target each, in a row that wraps.
 */
export function Ways({ children }: { children: ReactNode }) {
  // The first one's words, not its target, line up with the screen's edge, where the text above starts.
  return (
    <Row wrap gap="screen-tab-gap" bleedStart="control-timeline-inline">
      {children}
    </Row>
  );
}

/** A place a screen leads to, among its `Ways`. */
export function Way({ href, children }: { href: string; children: string }) {
  return (
    <LinkButton href={href} kind="quiet" size="timeline">
      {children}
    </LinkButton>
  );
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
