// SPDX-License-Identifier: AGPL-3.0-only
// The domain block list in Mastodon's CSV (FR-706), a section of the domain actions screen: "Export CSV" saves
// the actions in effect as the file Mastodon imports; the import form takes a CSV (pasted into a text area, or read
// from a file with React Aria's FileTrigger behind a Button) and previews it. The preview lists the rows in file
// order: each row that takes actions is a checkbox (chosen at first) named by its domain and described by its line
// and actions; the others say why they take none. The chosen rows are imported with an optional private note,
// after a confirmation in an alert dialog when they suspend a domain (ConfirmDialog, NFR-MOD-7). What the export
// and the import did stays on the page in a StatusMessage (role="status"); the import's result replaces the preview,
// and focus moves to the heading of the import.
import {
  Box,
  Button,
  Checkbox,
  ConfirmDialog,
  FileTrigger,
  Heading,
  Hint,
  InlineError,
  ReasonField,
  Stack,
  StatusMessage,
  Text,
  TextArea,
} from "@my/ui";
import { useId, useRef } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  type DomainBlockImportRow,
  type DomainBlockImportStatus,
  domainBlockImportRowLimit,
} from "../api/domainBlocks";
import { domainActionTextLimit } from "../api/staff";
import {
  type DomainBlockPreview,
  useDomainBlockExport,
  useDomainBlockImportForm,
  validateDomainBlockCsv,
} from "../hooks/useDomainBlockCsv";
import { domainActionKindLabels } from "../hooks/useDomainActions";
import { DomainName, useFocusAfterChange } from "./parts";

/** The file types the file button offers: Mastodon's export is a CSV. */
const csvFileTypes = ["text/csv", "text/plain", ".csv"];

const skipReasonLabels: Readonly<Record<Exclude<DomainBlockImportStatus, "importable">, ClientMessageKey>> = {
  already_applied: "client:admin.domain_actions.csv.status.already_applied",
  no_action: "client:admin.domain_actions.csv.status.no_action",
  duplicate: "client:admin.domain_actions.csv.status.duplicate",
  invalid: "client:admin.domain_actions.csv.status.invalid",
};

export function DomainBlockCsv() {
  const t = useT();
  return (
    <Stack gap="6">
      <Hint size="inherit">{t("client:admin.domain_actions.csv.description")}</Hint>
      <DomainBlockExport />
      <DomainBlockImport />
    </Stack>
  );
}

function DomainBlockExport() {
  const t = useT();
  const exporting = useDomainBlockExport();
  return (
    <Stack align="start" gap="2">
      <Button onPress={exporting.exportCsv} isPending={exporting.isPending}>
        {t("client:admin.domain_actions.csv.export")}
      </Button>
      {exporting.error !== undefined && (
        <InlineError alert>
          {t("client:admin.domain_actions.csv.export.failed", { reason: exporting.error })}
        </InlineError>
      )}
      <StatusMessage>{exporting.status}</StatusMessage>
    </Stack>
  );
}

function DomainBlockImport() {
  const t = useT();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const fileHelpId = useId();
  const form = useDomainBlockImportForm();
  // The import button goes away with the preview once the import is done: focus moves to the heading of the import.
  const focusHeading = useFocusAfterChange(headingRef, form.outcome?.id);
  return (
    <Stack gap="4">
      <Heading ref={headingRef} level={3} focusable>
        {t("client:admin.domain_actions.csv.import.title")}
      </Heading>
      <Stack as="form" onSubmit={form.submitPreview} gap="4">
        <TextArea
          label={t("client:admin.domain_actions.csv.csv")}
          description={t("client:admin.domain_actions.csv.csv.description", { max: domainBlockImportRowLimit })}
          value={form.text}
          onChange={form.changeText}
          rows={6}
          isRequired
          // The error is the one `validate` returns, also for an empty CSV (React Aria shows it as the field's
          // validation message).
          validate={(text) => validateDomainBlockCsv(t, text)}
          autoComplete="off"
          spellCheck="false"
        />
        <Stack align="start" gap="2">
          <FileTrigger
            acceptedFileTypes={csvFileTypes}
            onSelect={(files) => {
              const chosen = files?.[0];
              if (chosen !== undefined) form.chooseFile(chosen);
            }}
          >
            <Button size="sm" aria-describedby={fileHelpId}>
              {t("client:admin.domain_actions.csv.file")}
            </Button>
          </FileTrigger>
          <Hint id={fileHelpId}>{t("client:admin.domain_actions.csv.file.description")}</Hint>
          {form.file.status === "failed" && <InlineError alert>{form.file.message}</InlineError>}
          <StatusMessage tone="neutral">{form.file.status === "read" ? form.file.message : undefined}</StatusMessage>
        </Stack>
        {form.previewError !== undefined && (
          <InlineError alert size="inherit">
            {t("client:admin.domain_actions.csv.preview.failed", { reason: form.previewError })}
          </InlineError>
        )}
        <Box>
          <Button type="submit" isPending={form.isPreviewing}>
            {t("client:admin.domain_actions.csv.preview")}
          </Button>
        </Box>
      </Stack>
      <StatusMessage tone={form.outcome !== undefined && form.outcome.issued.length === 0 ? "neutral" : "success"}>
        {form.outcome === undefined
          ? undefined
          : t("client:admin.domain_actions.csv.import.done", {
              issued: form.outcome.issued.length,
              skipped: form.outcome.skipped.length,
            })}
      </StatusMessage>
      {/* A new preview starts its rows afresh (R10). */}
      {form.preview !== undefined && (
        <PreviewRows
          key={form.preview.id}
          preview={form.preview}
          isChosen={form.isChosen}
          choose={form.choose}
          chosenCount={form.chosenCount}
          privateNote={form.privateNote}
          changePrivateNote={form.changePrivateNote}
          submit={() => {
            focusHeading();
            form.submitImport();
          }}
          isPending={form.isImporting}
          error={form.confirmation === null ? form.importError : undefined}
        />
      )}
      <ConfirmDialog
        isOpen={form.confirmation !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) form.confirmation?.cancel();
        }}
        title={t("client:admin.domain_actions.csv.confirm.title", { count: form.confirmation?.count ?? 0 })}
        description={t("client:admin.domain_actions.confirm_suspend.description")}
        confirmLabel={t("client:admin.domain_actions.csv.confirm.confirm")}
        cancelLabel={t("client:admin.domain_actions.confirm_suspend.cancel")}
        onConfirm={() => {
          form.confirmation?.confirm();
        }}
        isPending={form.isImporting}
        {...(form.confirmation !== null && form.importError !== undefined
          ? { errorMessage: t("client:admin.domain_actions.csv.import.failed", { reason: form.importError }) }
          : {})}
      />
    </Stack>
  );
}

type PreviewRowsProps = {
  preview: DomainBlockPreview;
  isChosen: (line: number) => boolean;
  choose: (line: number, chosen: boolean) => void;
  chosenCount: number;
  privateNote: string;
  changePrivateNote: (note: string) => void;
  submit: () => void;
  isPending: boolean;
  error: string | undefined;
};

/** The rows of a previewed CSV, the private note, and the import of the chosen rows. */
function PreviewRows({
  preview,
  isChosen,
  choose,
  chosenCount,
  privateNote,
  changePrivateNote,
  submit,
  isPending,
  error,
}: PreviewRowsProps) {
  const t = useT();
  const headingId = useId();
  const { rows } = preview;
  const importable = rows.filter((row) => row.status === "importable").length;
  return (
    <Stack gap="4">
      <Heading id={headingId} level={4}>
        {t("client:admin.domain_actions.csv.preview.title")}
      </Heading>
      <p>{t("client:admin.domain_actions.csv.preview.summary", { count: rows.length, importable })}</p>
      <Stack as="ul" aria-labelledby={headingId} divided>
        {rows.map((row) => (
          <Box as="li" key={row.line} paddingY="2">
            <PreviewRow row={row} isChosen={isChosen(row.line)} choose={choose} />
          </Box>
        ))}
      </Stack>
      <ReasonField
        label={t("client:admin.domain_actions.private_note")}
        description={t("client:admin.domain_actions.csv.private_note.description")}
        value={privateNote}
        onChange={changePrivateNote}
        characterLimit={domainActionTextLimit}
        formatRemaining={(remaining) =>
          remaining < 0
            ? t("client:admin.domain_actions.characters_over", { count: -remaining })
            : t("client:admin.domain_actions.characters_left", { count: remaining })
        }
        errorMessage={t("client:admin.domain_actions.too_long", { max: domainActionTextLimit })}
      />
      {error !== undefined && (
        <InlineError alert size="inherit">
          {t("client:admin.domain_actions.csv.import.failed", { reason: error })}
        </InlineError>
      )}
      <Box>
        <Button variant="primary" onPress={submit} isPending={isPending}>
          {t("client:admin.domain_actions.csv.submit", { count: chosenCount })}
        </Button>
      </Box>
    </Stack>
  );
}

function PreviewRow({
  row,
  isChosen,
  choose,
}: {
  row: DomainBlockImportRow;
  isChosen: boolean;
  choose: (line: number, chosen: boolean) => void;
}) {
  const t = useT();
  const intl = useIntl();
  if (row.status === "importable" && row.block !== undefined) {
    const kinds = intl.formatList(
      row.issue.map((kind) => t(domainActionKindLabels[kind])),
      { type: "conjunction" },
    );
    return (
      <Checkbox
        isSelected={isChosen}
        onChange={(chosen) => {
          choose(row.line, chosen);
        }}
        description={t("client:admin.domain_actions.csv.row.issue", { line: row.line, kinds })}
      >
        <Text wrap="anywhere">
          <DomainName domain={row.block.domain} />
        </Text>
      </Checkbox>
    );
  }
  const reason = row.status === "importable" ? skipReasonLabels.invalid : skipReasonLabels[row.status];
  return (
    <Stack gap="1">
      {/* As written in the CSV, which may not be a domain. */}
      <Text dir="auto" wrap="anywhere">
        {row.written_domain}
      </Text>
      <Hint as="span">{t("client:admin.domain_actions.csv.row.skipped", { line: row.line, reason: t(reason) })}</Hint>
    </Stack>
  );
}
