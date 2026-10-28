// SPDX-License-Identifier: AGPL-3.0-only
// The sign-up blocks screen below its heading (FR-719): the form of a new block, the import of a list of them
// (SignupBlockImport), then the blocks, in effect or all, newest first. The form follows the browser's constraint validation (React Aria's native validation); a
// block takes effect at once and is lifted as easily, so neither asks first. What adding or lifting did stays on
// the page in a StatusMessage (role="status") next to what caused it. The list is in the admin mode's
// table form: a toolbar with the filter (kept in the URL by the route, web-client/README.md S3), one page of the table
// at a time, previous / next.
import {
  Box,
  Button,
  DescriptionItem,
  DescriptionList,
  DurationSelect,
  EmptyState,
  Heading,
  InlineError,
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
  type SignupBlock,
  signupBlockCommentLimit,
  type SignupBlockFilter,
  signupBlockFilters,
  type SignupBlockKind,
  signupBlockKinds,
  signupBlockSeverities,
  type SignupBlockState,
  signupBlockValueLimit,
} from "../api/signupBlocks";
import {
  type LiftSignupBlock,
  signupBlockKindLabels,
  signupBlockSeverityLabels,
  signupBlockValueInText,
  useLiftSignupBlock,
  useSignupBlockForm,
  useSignupBlocks,
} from "../hooks/useSignupBlocks";
import { DateTime, DetailSection, DomainName, RecordTable, RequirePermission } from "./parts";
import { SignupBlockImportForm } from "./SignupBlockImport";
import {
  day,
  signupBlockDurations,
  signupBlockKindDescriptions,
  signupBlockSeverityDescriptions,
} from "./signupBlockFields";

/** The label, help and error of the value field for each kind. */
const valueFields: Readonly<
  Record<SignupBlockKind, { label: ClientMessageKey; description: ClientMessageKey; required: ClientMessageKey }>
> = {
  email_domain: {
    label: "client:admin.signup_blocks.value.email_domain",
    description: "client:admin.signup_blocks.value.email_domain.description",
    required: "client:admin.signup_blocks.value.email_domain.required",
  },
  email_hash: {
    label: "client:admin.signup_blocks.value.email_hash",
    description: "client:admin.signup_blocks.value.email_hash.description",
    required: "client:admin.signup_blocks.value.email_hash.required",
  },
  ip_range: {
    label: "client:admin.signup_blocks.value.ip_range",
    description: "client:admin.signup_blocks.value.ip_range.description",
    required: "client:admin.signup_blocks.value.ip_range.required",
  },
};

const stateLabels: Readonly<Record<SignupBlockState, ClientMessageKey>> = {
  active: "client:admin.signup_blocks.state.active",
  expired: "client:admin.signup_blocks.state.expired",
  lifted: "client:admin.signup_blocks.state.lifted",
};

const filterLabels: Readonly<Record<SignupBlockFilter, ClientMessageKey>> = {
  active: "client:admin.signup_blocks.filter.active",
  all: "client:admin.signup_blocks.filter.all",
};

function isFilter(key: unknown): key is SignupBlockFilter {
  return signupBlockFilters.some((filter) => filter === key);
}

export type SignupBlocksProps = {
  /** The selected filter, from the URL. */
  filter: SignupBlockFilter;
  /** Selects another filter: the route puts it into the URL. */
  onFilterChange: (filter: SignupBlockFilter) => void;
};

export function SignupBlocks(props: SignupBlocksProps) {
  const t = useT();
  return (
    <RequirePermission permission="manage_accounts">
      <Stack gap="8">
        <DetailSection title={t("client:admin.signup_blocks.new.title")}>
          <NewSignupBlockForm />
        </DetailSection>
        <DetailSection title={t("client:admin.signup_blocks.import.title")}>
          <SignupBlockImportForm />
        </DetailSection>
        <SignupBlockTabs {...props} />
      </Stack>
    </RequirePermission>
  );
}

function NewSignupBlockForm() {
  const t = useT();
  const form = useSignupBlockForm();
  const { draft } = form;
  const valueField = valueFields[draft.kind];
  return (
    <Stack as="form" onSubmit={form.submit} gap="6">
      <RadioGroup
        label={t("client:admin.signup_blocks.kind")}
        value={draft.kind}
        onChange={(kind) => {
          const chosen = signupBlockKinds.find((item) => item === kind);
          if (chosen !== undefined) form.change({ kind: chosen });
        }}
        isRequired
      >
        {signupBlockKinds.map((kind) => (
          <Radio key={kind} value={kind} description={t(signupBlockKindDescriptions[kind])}>
            {t(signupBlockKindLabels[kind])}
          </Radio>
        ))}
      </RadioGroup>
      <TextField
        label={t(valueField.label)}
        description={t(valueField.description)}
        value={draft.value}
        onChange={(value) => {
          form.change({ value });
        }}
        isRequired
        maxLength={signupBlockValueLimit}
        autoComplete="off"
        spellCheck="false"
        errorMessage={t(valueField.required)}
      />
      <RadioGroup
        label={t("client:admin.signup_blocks.severity")}
        value={draft.severity}
        onChange={(severity) => {
          const chosen = signupBlockSeverities.find((item) => item === severity);
          if (chosen !== undefined) form.change({ severity: chosen });
        }}
        isRequired
        errorMessage={t("client:admin.signup_blocks.severity.required")}
      >
        {signupBlockSeverities.map((severity) => (
          <Radio key={severity} value={severity} description={t(signupBlockSeverityDescriptions[severity])}>
            {t(signupBlockSeverityLabels[severity])}
          </Radio>
        ))}
      </RadioGroup>
      <DurationSelect
        label={t("client:admin.signup_blocks.duration")}
        description={t("client:admin.signup_blocks.duration.description")}
        durations={signupBlockDurations}
        formatDuration={(seconds) => t("client:admin.signup_blocks.duration.days", { count: seconds / day })}
        indefiniteLabel={t("client:admin.signup_blocks.duration.indefinite")}
        value={draft.expiresIn}
        onChange={(expiresIn) => {
          form.change({ expiresIn });
        }}
      />
      <ReasonField
        label={t("client:admin.signup_blocks.comment")}
        description={t("client:admin.signup_blocks.comment.description")}
        value={draft.comment}
        onChange={(comment) => {
          form.change({ comment });
        }}
        characterLimit={signupBlockCommentLimit}
        formatRemaining={(remaining) =>
          remaining < 0
            ? t("client:admin.signup_blocks.characters_over", { count: -remaining })
            : t("client:admin.signup_blocks.characters_left", { count: remaining })
        }
        errorMessage={t("client:admin.signup_blocks.too_long", { max: signupBlockCommentLimit })}
      />
      {form.error !== undefined && (
        <InlineError alert size="inherit">
          {t("client:admin.signup_blocks.add_failed", { reason: form.error })}
        </InlineError>
      )}
      <Box>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.signup_blocks.submit")}
        </Button>
      </Box>
      <StatusMessage>{form.status}</StatusMessage>
    </Stack>
  );
}

function SignupBlockTabs({ filter, onFilterChange }: SignupBlocksProps) {
  const t = useT();
  const [status, setStatus] = useState<string | undefined>(undefined);
  return (
    <Stack gap="3">
      <Heading level={2}>{t("client:admin.signup_blocks.list.title")}</Heading>
      <ListToolbar label={t("client:admin.list.filters")}>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.signup_blocks.filter.label")}
          value={filter}
          onChange={(key) => {
            if (isFilter(key)) onFilterChange(key);
          }}
        >
          {signupBlockFilters.map((item) => (
            <SelectItem key={item} id={item}>
              {t(filterLabels[item])}
            </SelectItem>
          ))}
        </Select>
      </ListToolbar>
      <StatusMessage>{status}</StatusMessage>
      {/* Another filter is another list: it starts at its first page. */}
      <SignupBlockList
        key={filter}
        filter={filter}
        onLifted={(block) => {
          setStatus(
            t("client:admin.signup_blocks.lifted", {
              kind: t(signupBlockKindLabels[block.kind]),
              value: signupBlockValueInText(t, block),
            }),
          );
        }}
      />
    </Stack>
  );
}

function SignupBlockList({ filter, onLifted }: { filter: SignupBlockFilter; onLifted: (block: SignupBlock) => void }) {
  const t = useT();
  const listRef = useRef<HTMLDivElement>(null);
  const list = useSignupBlocks(filter);
  const removed = useFocusAfterRemoval(
    listRef,
    list.items.map((block) => block.id),
  );
  const keepFocus = useFocusHeadingAfterLift(listRef);
  const lift = useLiftSignupBlock((block) => {
    // The lift button goes away: with the row from the list of blocks in effect, or from the lifted row.
    if (filter === "active") removed(block.id);
    else keepFocus(block.id);
    onLifted(block);
  });
  const empty =
    filter === "active" ? (
      <EmptyState
        headingLevel={3}
        heading={t("client:admin.signup_blocks.empty.active")}
        description={t("client:admin.signup_blocks.empty.active.description")}
      />
    ) : (
      <EmptyState headingLevel={3} heading={t("client:admin.signup_blocks.empty.all")} />
    );
  return (
    <Box ref={listRef}>
      <RecordTable
        list={list}
        caption={t(filterLabels[filter])}
        getId={(block) => block.id}
        empty={empty}
        head={
          <>
            <TableColumnHeader>{t("client:admin.signup_blocks.column.value")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.signup_blocks.kind")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.signup_blocks.severity")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.signup_blocks.state")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.signup_blocks.created_at")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.signup_blocks.expires_at")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.signup_blocks.column.details")}</TableColumnHeader>
            <TableColumnHeader>
              <VisuallyHidden>{t("client:admin.list.column.actions")}</VisuallyHidden>
            </TableColumnHeader>
          </>
        }
      >
        {(block) => <SignupBlockRow block={block} lift={lift} />}
      </RecordTable>
    </Box>
  );
}

/** What a block matches: a domain in both of its forms, else the value as the server keeps it. */
function SignupBlockValue({ block }: { block: SignupBlock }) {
  if (block.kind === "email_domain") return <DomainName domain={block.value} />;
  return <span dir="ltr">{block.value}</span>;
}

function SignupBlockRow({ block, lift }: { block: SignupBlock; lift: LiftSignupBlock }) {
  const t = useT();
  const intl = useIntl();
  const kind = t(signupBlockKindLabels[block.kind]);
  return (
    <TableRow data-row-id={block.id}>
      {/* Focusable from script: it takes focus when the lift button goes away (useFocusHeadingAfterLift). */}
      <TableRowHeader tabIndex={-1} wrap="anywhere">
        <SignupBlockValue block={block} />
      </TableRowHeader>
      <TableCell>{kind}</TableCell>
      <TableCell>{t(signupBlockSeverityLabels[block.severity])}</TableCell>
      <TableCell>
        <Text {...(block.state === "active" ? { weight: "semibold" as const } : {})}>
          {t(stateLabels[block.state])}
        </Text>
      </TableCell>
      <TableCell>
        <DateTime value={block.created_at} />
      </TableCell>
      <TableCell>
        {block.expires_at === undefined ? (
          t("client:admin.signup_blocks.duration.indefinite")
        ) : (
          <DateTime value={block.expires_at} />
        )}
      </TableCell>
      <TableCell wrap="anywhere">
        <DescriptionList>
          {block.lifted_at !== undefined && (
            <DescriptionItem term={t("client:admin.signup_blocks.lifted_at")}>
              <DateTime value={block.lifted_at} />
            </DescriptionItem>
          )}
          <DescriptionItem term={t("client:admin.signup_blocks.source")}>
            {block.moderation_action_id === undefined
              ? t("client:admin.signup_blocks.source.manual")
              : t("client:admin.signup_blocks.source.ban")}
          </DescriptionItem>
          {block.comment !== undefined && (
            <DescriptionItem term={t("client:admin.signup_blocks.comment")}>
              {/* Written by a moderator, in any language. */}
              <Text dir="auto" whitespace="preWrap">
                {block.comment}
              </Text>
            </DescriptionItem>
          )}
        </DescriptionList>
      </TableCell>
      <TableCell>
        {block.state === "active" && (
          <Button
            size="sm"
            isPending={lift.pendingIds.includes(block.id)}
            aria-label={t("client:admin.signup_blocks.lift_action", {
              kind,
              value: signupBlockValueInText(t, block),
              date: intl.formatDate(block.created_at, { dateStyle: "medium", timeStyle: "short" }),
            })}
            onPress={() => {
              lift.lift(block);
            }}
          >
            {t("client:admin.signup_blocks.lift")}
          </Button>
        )}
      </TableCell>
    </TableRow>
  );
}
