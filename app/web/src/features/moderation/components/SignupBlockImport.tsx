// SPDX-License-Identifier: AGPL-3.0-only
// The import of a list of sign-up blocks (FR-719), a section of the sign-up blocks screen: the kind, the list
// (pasted into a text area, or read from a text file with React Aria's FileTrigger behind a Button), the
// effect, the duration and the comment, then what the import added and the lines it skipped with why. The form
// follows the browser's constraint validation (React Aria's native validation, with `validate` for the count of
// values); blocks take effect at once and are lifted from the list below, so the import does not ask first.
// What the import did stays on the page in a StatusMessage (role="status"); the skipped lines are a plain
// table (the APG table pattern recommends the native element) named by its heading.
import {
  Box,
  Button,
  DescriptionItem,
  DescriptionList,
  DurationSelect,
  FileTrigger,
  Heading,
  Hint,
  InlineError,
  Radio,
  RadioGroup,
  ReasonField,
  Stack,
  StatusMessage,
  Table,
  TableBody,
  TableCell,
  TableColumnHeader,
  TableHead,
  TableRow,
  Text,
  TextArea,
} from "@my/ui";
import { useId, useRef, useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  signupBlockCommentLimit,
  type SignupBlockImportSkipReason,
  signupBlockImportSkipReasons,
  signupBlockImportValueLimit,
  signupBlockKinds,
  signupBlockSeverities,
} from "../api/signupBlocks";
import {
  type SignupBlockImportOutcome,
  useSignupBlockImportForm,
  validateSignupBlockList,
} from "../hooks/useSignupBlockImport";
import { signupBlockKindLabels, signupBlockSeverityLabels } from "../hooks/useSignupBlocks";
import { useFocusAfterChange } from "./parts";
import {
  day,
  signupBlockDurations,
  signupBlockKindDescriptions,
  signupBlockSeverityDescriptions,
} from "./signupBlockFields";

/** The file types the file button offers: plain text and CSV, the forms of the public lists. */
const listFileTypes = ["text/plain", "text/csv", ".txt", ".csv"];

const skipReasonLabels: Readonly<Record<SignupBlockImportSkipReason, ClientMessageKey>> = {
  invalid: "client:admin.signup_blocks.import.reason.invalid",
  duplicate: "client:admin.signup_blocks.import.reason.duplicate",
  already_blocked: "client:admin.signup_blocks.import.reason.already_blocked",
};

/** The skipped lines the table shows at first; the rest follow on request, as a long list is slow to lay out. */
const skippedShownAtFirst = 100;

export function SignupBlockImportForm() {
  const t = useT();
  const form = useSignupBlockImportForm();
  const { draft } = form;
  const fileHelpId = useId();
  return (
    <Stack gap="6">
      <Hint size="inherit">{t("client:admin.signup_blocks.import.description")}</Hint>
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
        <Stack gap="3">
          <TextArea
            label={t("client:admin.signup_blocks.import.list")}
            description={t("client:admin.signup_blocks.import.list.description", { max: signupBlockImportValueLimit })}
            value={draft.text}
            onChange={(text) => {
              form.change({ text });
            }}
            rows={8}
            isRequired
            // The error is the one `validate` returns, also for an empty list (React Aria shows it as the
            // field's validation message).
            validate={(text) => validateSignupBlockList(t, text)}
            autoComplete="off"
            spellCheck="false"
          />
          <Stack align="start" gap="2">
            <FileTrigger
              acceptedFileTypes={listFileTypes}
              onSelect={(files) => {
                const chosen = files?.[0];
                if (chosen !== undefined) form.chooseFile(chosen);
              }}
            >
              <Button size="sm" aria-describedby={fileHelpId}>
                {t("client:admin.signup_blocks.import.file")}
              </Button>
            </FileTrigger>
            <Hint id={fileHelpId}>{t("client:admin.signup_blocks.import.file.description")}</Hint>
            {form.file.status === "failed" && <InlineError alert>{form.file.message}</InlineError>}
            <StatusMessage tone="neutral">{form.file.status === "read" ? form.file.message : undefined}</StatusMessage>
          </Stack>
        </Stack>
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
          description={t("client:admin.signup_blocks.import.comment.description")}
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
            {t("client:admin.signup_blocks.import.failed", { reason: form.error })}
          </InlineError>
        )}
        <Box>
          <Button type="submit" variant="primary" isPending={form.isPending}>
            {t("client:admin.signup_blocks.import.submit")}
          </Button>
        </Box>
        <StatusMessage tone={form.outcome !== undefined && form.outcome.added === 0 ? "neutral" : "success"}>
          {form.outcome === undefined
            ? undefined
            : t("client:admin.signup_blocks.import.done", {
                added: form.outcome.added,
                skipped: form.outcome.skipped.length,
                kind: t(signupBlockKindLabels[form.outcome.kind]),
                severity: t(signupBlockSeverityLabels[form.outcome.severity]),
              })}
        </StatusMessage>
      </Stack>
      {/* A new import starts the table afresh, with its first rows only (R10). */}
      {form.outcome !== undefined && form.outcome.skipped.length > 0 && (
        <SkippedLines key={form.outcome.id} outcome={form.outcome} />
      )}
    </Stack>
  );
}

/** The lines an import skipped, in line order, with the count of each reason. */
function SkippedLines({ outcome }: { outcome: SignupBlockImportOutcome }) {
  const t = useT();
  const headingId = useId();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const [showAll, setShowAll] = useState(false);
  // The button goes away with the rows it shows: focus moves to the heading of the table.
  const focusHeading = useFocusAfterChange(headingRef, showAll);
  const { skipped } = outcome;
  const shown = showAll ? skipped : skipped.slice(0, skippedShownAtFirst);
  return (
    <Stack gap="3">
      <Heading ref={headingRef} id={headingId} level={3} focusable>
        {t("client:admin.signup_blocks.import.skipped.title")}
      </Heading>
      <DescriptionList columns={3}>
        {signupBlockImportSkipReasons.map((reason) => (
          <DescriptionItem key={reason} term={t(skipReasonLabels[reason])}>
            <Text numeric>
              {t("client:admin.signup_blocks.import.skipped.lines", {
                count: skipped.filter((line) => line.reason === reason).length,
              })}
            </Text>
          </DescriptionItem>
        ))}
      </DescriptionList>
      <Table density="compact" aria-labelledby={headingId}>
        <TableHead>
          <TableColumnHeader width="narrow">{t("client:admin.signup_blocks.import.skipped.line")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.signup_blocks.import.skipped.value")}</TableColumnHeader>
          <TableColumnHeader width="wide">{t("client:admin.signup_blocks.import.skipped.reason")}</TableColumnHeader>
        </TableHead>
        <TableBody>
          {shown.map((line) => (
            <TableRow key={line.line}>
              <TableCell numeric>
                {t("client:admin.signup_blocks.import.skipped.line_number", { line: line.line })}
              </TableCell>
              {/* As written in the list, which may be in any script. */}
              <TableCell dir="auto" wrap="anywhere" align="matchParent">
                {line.value}
              </TableCell>
              <TableCell>{t(skipReasonLabels[line.reason])}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {!showAll && skipped.length > skippedShownAtFirst && (
        <Stack align="start" gap="2">
          <Hint>
            {t("client:admin.signup_blocks.import.skipped.shown", {
              shown: skippedShownAtFirst,
              count: skipped.length,
            })}
          </Hint>
          <Button
            size="sm"
            onPress={() => {
              focusHeading();
              setShowAll(true);
            }}
          >
            {t("client:admin.signup_blocks.import.skipped.show_all", { count: skipped.length })}
          </Button>
        </Stack>
      )}
    </Stack>
  );
}
