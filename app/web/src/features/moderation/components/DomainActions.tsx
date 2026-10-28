// SPDX-License-Identifier: AGPL-3.0-only
// The domain actions screen below its heading (FR-702): the form of a new action, then the actions, in effect
// or all, newest first. The form follows the browser's constraint validation (React Aria's native validation);
// a suspension asks first in an alert dialog (ConfirmDialog, NFR-MOD-7), the other kinds take effect at once.
// What an action or a lift did stays on the page in a StatusMessage (role="status") next to what caused it.
// The list is in the admin mode's table form: a toolbar with the filter (kept in the URL by the route, web-client/README.md
// S3), one page of the table at a time, previous / next. Last, the list in Mastodon's CSV (DomainBlockCsv).
import {
  Box,
  Button,
  Checkbox,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DurationSelect,
  EmptyState,
  Heading,
  InlineError,
  Link,
  ListToolbar,
  Radio,
  RadioGroup,
  ReasonField,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
  TableCell,
  TableColumnHeader,
  TableRow,
  TableRowHeader,
  Text,
  TextField,
  VisuallyHidden,
} from "@my/ui";
import { useRef, useState } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFocusAfterRemoval, useFocusHeadingAfterLift } from "../../../lib/useFocusAfterRemoval";
import {
  type DomainAction,
  type DomainActionFilter,
  domainActionFilters,
  type DomainActionKind,
  domainActionKinds,
  type DomainActionState,
  domainActionTextLimit,
} from "../api/staff";
import {
  domainActionKindLabels,
  domainInText,
  type LiftDomainAction,
  useDomainActionForm,
  useDomainActions,
  useLiftDomainAction,
} from "../hooks/useDomainActions";
import { DomainBlockCsv } from "./DomainBlockCsv";
import { DateTime, Section, DomainName, RecordTable, RequirePermission } from "./parts";

const kindDescriptions: Readonly<Record<DomainActionKind, ClientMessageKey>> = {
  limit: "client:admin.domain_actions.kind.limit.description",
  suspend: "client:admin.domain_actions.kind.suspend.description",
  reject_media: "client:admin.domain_actions.kind.reject_media.description",
  reject_reports: "client:admin.domain_actions.kind.reject_reports.description",
  force_sensitive: "client:admin.domain_actions.kind.force_sensitive.description",
};

const stateLabels: Readonly<Record<DomainActionState, ClientMessageKey>> = {
  active: "client:admin.domain_actions.state.active",
  expired: "client:admin.domain_actions.state.expired",
  lifted: "client:admin.domain_actions.state.lifted",
};

const filterLabels: Readonly<Record<DomainActionFilter, ClientMessageKey>> = {
  active: "client:admin.domain_actions.filter.active",
  all: "client:admin.domain_actions.filter.all",
};

/** The durations an action can last, in seconds, from the shortest. */
const day = 86_400;
const durations = [day, 7 * day, 30 * day, 90 * day, 180 * day, 365 * day] as const;

function isFilter(key: unknown): key is DomainActionFilter {
  return domainActionFilters.some((filter) => filter === key);
}

export type DomainActionsProps = {
  /** The selected filter, from the URL. */
  filter: DomainActionFilter;
  /** The domain whose actions to list and to fill in the form, from the URL; undefined lists every domain. */
  domain: string | undefined;
  /** Selects another filter: the route puts it into the URL. */
  onFilterChange: (filter: DomainActionFilter) => void;
  /** The address of the list of every domain under the same filter. */
  everyDomainHref: string;
};

export function DomainActions(props: DomainActionsProps) {
  const t = useT();
  return (
    <RequirePermission permission="manage_domains">
      <Stack gap="8">
        <Section surface="card" title={t("client:admin.domain_actions.new.title")}>
          {/* Another domain from the URL starts another form (web-client/README.md R10). */}
          <NewDomainActionForm key={props.domain ?? ""} initialDomain={props.domain ?? ""} />
        </Section>
        <DomainActionTabs {...props} />
        <Section surface="card" title={t("client:admin.domain_actions.csv.title")}>
          <DomainBlockCsv />
        </Section>
      </Stack>
    </RequirePermission>
  );
}

function NewDomainActionForm({ initialDomain }: { initialDomain: string }) {
  const t = useT();
  const form = useDomainActionForm(initialDomain);
  const { draft } = form;
  const formatRemaining = (remaining: number) =>
    remaining < 0
      ? t("client:admin.domain_actions.characters_over", { count: -remaining })
      : t("client:admin.domain_actions.characters_left", { count: remaining });
  const tooLong = t("client:admin.domain_actions.too_long", { max: domainActionTextLimit });
  return (
    <>
      <Stack as="form" onSubmit={form.submit} gap="6">
        <TextField
          label={t("client:admin.domain_actions.domain")}
          description={t("client:admin.domain_actions.domain.description")}
          value={draft.domain}
          onChange={(domain) => {
            form.change({ domain });
          }}
          isRequired
          maxLength={253}
          autoComplete="off"
          errorMessage={t("client:admin.domain_actions.domain.required")}
        />
        <RadioGroup
          label={t("client:admin.domain_actions.kind")}
          value={draft.kind}
          onChange={(kind) => {
            const chosen = domainActionKinds.find((item) => item === kind);
            if (chosen !== undefined) form.change({ kind: chosen });
          }}
          isRequired
          errorMessage={t("client:admin.domain_actions.kind.required")}
        >
          {domainActionKinds.map((kind) => (
            <Radio key={kind} value={kind} description={t(kindDescriptions[kind])}>
              {t(domainActionKindLabels[kind])}
            </Radio>
          ))}
        </RadioGroup>
        <DurationSelect
          label={t("client:admin.domain_actions.duration")}
          description={t("client:admin.domain_actions.duration.description")}
          durations={durations}
          formatDuration={(seconds) => t("client:admin.domain_actions.duration.days", { count: seconds / day })}
          indefiniteLabel={t("client:admin.domain_actions.duration.indefinite")}
          value={draft.expiresIn}
          onChange={(expiresIn) => {
            form.change({ expiresIn });
          }}
        />
        <ReasonField
          label={t("client:admin.domain_actions.public_comment")}
          description={t("client:admin.domain_actions.public_comment.description")}
          value={draft.publicComment}
          onChange={(publicComment) => {
            form.change({ publicComment });
          }}
          characterLimit={domainActionTextLimit}
          formatRemaining={formatRemaining}
          errorMessage={tooLong}
        />
        <ReasonField
          label={t("client:admin.domain_actions.private_note")}
          description={t("client:admin.domain_actions.private_note.description")}
          value={draft.privateNote}
          onChange={(privateNote) => {
            form.change({ privateNote });
          }}
          characterLimit={domainActionTextLimit}
          formatRemaining={formatRemaining}
          errorMessage={tooLong}
        />
        <Checkbox
          isSelected={draft.obfuscate}
          onChange={(obfuscate) => {
            form.change({ obfuscate });
          }}
          description={t("client:admin.domain_actions.obfuscate.description")}
        >
          {t("client:admin.domain_actions.obfuscate")}
        </Checkbox>
        {form.confirmation === null && form.error !== undefined && (
          <InlineError alert size="inherit">
            {t("client:admin.domain_actions.issue_failed", { reason: form.error })}
          </InlineError>
        )}
        <Box>
          <Button type="submit" variant="primary" isPending={form.isPending && form.confirmation === null}>
            {t("client:admin.domain_actions.submit")}
          </Button>
        </Box>
        <StatusMessage>{form.status}</StatusMessage>
      </Stack>
      {/* Outside the form: React sends the dialog's own submit event up its component tree, through a portal too. */}
      <ConfirmDialog
        isOpen={form.confirmation !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) form.confirmation?.cancel();
        }}
        title={t("client:admin.domain_actions.confirm_suspend.title", {
          domain: domainInText(t, form.confirmation?.domain ?? draft.domain),
        })}
        description={t("client:admin.domain_actions.confirm_suspend.description")}
        confirmLabel={t("client:admin.domain_actions.confirm_suspend.confirm")}
        cancelLabel={t("client:admin.domain_actions.confirm_suspend.cancel")}
        onConfirm={() => {
          form.confirmation?.confirm();
        }}
        isPending={form.isPending}
        {...(form.confirmation !== null && form.error !== undefined
          ? { errorMessage: t("client:admin.domain_actions.issue_failed", { reason: form.error }) }
          : {})}
      />
    </>
  );
}

function DomainActionTabs({ filter, domain, onFilterChange, everyDomainHref }: DomainActionsProps) {
  const t = useT();
  const [status, setStatus] = useState<string | undefined>(undefined);
  return (
    <Stack gap="3">
      <Heading level={2}>{t("client:admin.domain_actions.list.title")}</Heading>
      {domain !== undefined && (
        <Cluster as="p" align="baseline" gap="2" rowGap="0">
          <Text wrap="anywhere">
            {t("client:admin.domain_actions.list.of_domain", { domain: domainInText(t, domain) })}
          </Text>
          <Link href={everyDomainHref} variant="standalone">
            {t("client:admin.domain_actions.list.every_domain")}
          </Link>
        </Cluster>
      )}
      <ListToolbar label={t("client:admin.list.filters")}>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.domain_actions.filter.label")}
          value={filter}
          onChange={(key) => {
            if (isFilter(key)) onFilterChange(key);
          }}
        >
          {domainActionFilters.map((item) => (
            <SelectItem key={item} id={item}>
              {t(filterLabels[item])}
            </SelectItem>
          ))}
        </Select>
      </ListToolbar>
      <StatusMessage>{status}</StatusMessage>
      {/* Another filter is another list: it starts at its first page. */}
      <DomainActionList
        key={filter}
        filter={filter}
        domain={domain}
        onLifted={(action) => {
          setStatus(
            t("client:admin.domain_actions.lifted", {
              kind: t(domainActionKindLabels[action.kind]),
              domain: domainInText(t, action.domain),
            }),
          );
        }}
      />
    </Stack>
  );
}

type DomainActionListProps = {
  filter: DomainActionFilter;
  domain: string | undefined;
  onLifted: (action: DomainAction) => void;
};

function DomainActionList({ filter, domain, onLifted }: DomainActionListProps) {
  const t = useT();
  const listRef = useRef<HTMLDivElement>(null);
  const list = useDomainActions({ filter, domain });
  const removed = useFocusAfterRemoval(
    listRef,
    list.items.map((action) => action.id),
  );
  const keepFocus = useFocusHeadingAfterLift(listRef);
  const lift = useLiftDomainAction((action) => {
    // The lift button goes away: with the row from a list of actions in effect, or from the lifted row.
    if (filter === "active") removed(action.id);
    else keepFocus(action.id);
    onLifted(action);
  });
  const empty =
    filter === "active" ? (
      <EmptyState
        headingLevel={3}
        heading={t("client:admin.domain_actions.empty.active")}
        description={t("client:admin.domain_actions.empty.active.description")}
      />
    ) : (
      <EmptyState headingLevel={3} heading={t("client:admin.domain_actions.empty.all")} />
    );
  return (
    <Box ref={listRef}>
      <RecordTable
        list={list}
        caption={t(filterLabels[filter])}
        getId={(action) => action.id}
        empty={empty}
        head={
          <>
            <TableColumnHeader>{t("client:admin.domain_actions.column.domain")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.domain_actions.kind")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.domain_actions.state")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.domain_actions.issued_at")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.domain_actions.expires_at")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.domain_actions.column.details")}</TableColumnHeader>
            <TableColumnHeader>
              <VisuallyHidden>{t("client:admin.list.column.actions")}</VisuallyHidden>
            </TableColumnHeader>
          </>
        }
      >
        {(action) => <DomainActionRow action={action} lift={lift} />}
      </RecordTable>
    </Box>
  );
}

function DomainActionRow({ action, lift }: { action: DomainAction; lift: LiftDomainAction }) {
  const t = useT();
  const intl = useIntl();
  const kind = t(domainActionKindLabels[action.kind]);
  return (
    <TableRow data-row-id={action.id}>
      {/* Focusable from script: it takes focus when the lift button goes away (useFocusHeadingAfterLift). */}
      <TableRowHeader tabIndex={-1} wrap="anywhere">
        <DomainName domain={action.domain} />
      </TableRowHeader>
      <TableCell>{kind}</TableCell>
      <TableCell>
        <Text {...(action.state === "active" ? { weight: "semibold" as const } : {})}>
          {t(stateLabels[action.state])}
        </Text>
      </TableCell>
      <TableCell>
        <DateTime value={action.issued_at} />
      </TableCell>
      <TableCell>
        {action.expires_at === undefined ? (
          t("client:admin.domain_actions.duration.indefinite")
        ) : (
          <DateTime value={action.expires_at} />
        )}
      </TableCell>
      <TableCell wrap="anywhere">
        <DescriptionList>
          {action.lifted_at !== undefined && (
            <DescriptionItem term={t("client:admin.domain_actions.lifted_at")}>
              <DateTime value={action.lifted_at} />
            </DescriptionItem>
          )}
          <DescriptionItem term={t("client:admin.domain_actions.obfuscate.term")}>
            {action.obfuscate
              ? t("client:admin.domain_actions.obfuscate.yes")
              : t("client:admin.domain_actions.obfuscate.no")}
          </DescriptionItem>
          {action.public_comment !== undefined && (
            <DescriptionItem term={t("client:admin.domain_actions.public_comment")}>
              {/* Written by a moderator, in any language. */}
              <Text dir="auto" whitespace="preWrap">
                {action.public_comment}
              </Text>
            </DescriptionItem>
          )}
          {action.private_note !== undefined && (
            <DescriptionItem term={t("client:admin.domain_actions.private_note")}>
              <Text dir="auto" whitespace="preWrap">
                {action.private_note}
              </Text>
            </DescriptionItem>
          )}
        </DescriptionList>
      </TableCell>
      <TableCell>
        {action.state === "active" && (
          <Button
            size="sm"
            isPending={lift.pendingIds.includes(action.id)}
            aria-label={t("client:admin.domain_actions.lift_action", {
              kind,
              domain: domainInText(t, action.domain),
              date: intl.formatDate(action.issued_at, { dateStyle: "medium", timeStyle: "short" }),
            })}
            onPress={() => {
              lift.lift(action);
            }}
          >
            {t("client:admin.domain_actions.lift")}
          </Button>
        )}
      </TableCell>
    </TableRow>
  );
}
