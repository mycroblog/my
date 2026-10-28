// SPDX-License-Identifier: AGPL-3.0-only
// The import and export page below its heading (FR-008): the CSV files of the follows, blocked and muted accounts, muted
// domains, and bookmarks to download; a form that imports such a file after checking what it would do; and the latest imports with
// their progress. Dates are in the user's locale and time zone, numbers in the user's locale.
import {
  Box,
  Button,
  Cluster,
  DescriptionItem,
  DescriptionList,
  EmptyState,
  FileTrigger,
  Heading,
  InlineError,
  Radio,
  RadioGroup,
  Section,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { useId } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  csvFileNames,
  type DataImport,
  type DataImportKind,
  type DataImportMode,
  type DataImportPreview,
  dataImportKinds,
  type DataImportSkipped,
} from "../api/importExport";
import {
  type CsvExports,
  type ImportForm,
  useCsvExports,
  useDataImports,
  useImportForm,
} from "../hooks/useImportExport";

const kindLabels: Readonly<Record<DataImportKind, ClientMessageKey>> = {
  following: "client:settings.import_export.kind.following",
  blocks: "client:settings.import_export.kind.blocks",
  mutes: "client:settings.import_export.kind.mutes",
  domain_mutes: "client:settings.import_export.kind.domain_mutes",
  bookmarks: "client:settings.import_export.kind.bookmarks",
};

const modeLabels: Readonly<Record<DataImportMode, ClientMessageKey>> = {
  merge: "client:settings.import_export.import.mode.merge",
  overwrite: "client:settings.import_export.import.mode.overwrite",
};

const skipReasons: Readonly<Record<DataImportSkipped["reason"], ClientMessageKey>> = {
  invalid: "client:settings.import_export.import.preview.skip.invalid",
  duplicate: "client:settings.import_export.import.preview.skip.duplicate",
};

const stateLabels: Readonly<Record<DataImport["state"], ClientMessageKey>> = {
  in_progress: "client:settings.import_export.history.state.in_progress",
  finished: "client:settings.import_export.history.state.finished",
};

/** The format of every date and time on the page. */
const dateTimeFormat = { dateStyle: "medium", timeStyle: "short" } as const;

/** The skipped lines a preview lists; the rest are only counted by the server's answer. */
const maxSkippedShown = 20;

/** The file types the file picker offers. */
const csvFileTypes = [".csv", "text/csv"];

export function ImportExport() {
  const t = useT();
  const exports = useCsvExports();
  const form = useImportForm();
  const imports = useDataImports();
  return (
    <Stack gap="8">
      <Section title={t("client:settings.import_export.export.title")}>
        <ExportsView exports={exports} />
      </Section>
      <Section title={t("client:settings.import_export.import.title")}>
        <ImportView form={form} />
      </Section>
      <Section title={t("client:settings.import_export.history.title")}>
        <HistoryView imports={imports} />
      </Section>
    </Stack>
  );
}

/** The CSV files to download alone, for the screen of a closed server, where importing is closed. */
export function CsvExportsSection() {
  const t = useT();
  const exports = useCsvExports();
  return (
    <Section title={t("client:settings.import_export.export.title")}>
      <ExportsView exports={exports} />
    </Section>
  );
}

function ExportsView({ exports }: { exports: CsvExports }) {
  const t = useT();
  return (
    <Stack gap="4">
      <p>{t("client:settings.import_export.export.description")}</p>
      {exports.error !== undefined && (
        <InlineError alert size="inherit">
          {exports.error}
        </InlineError>
      )}
      <Stack as="ul" divided>
        {dataImportKinds.map((kind) => (
          <Cluster as="li" key={kind} align="center" justify="between" gap="2" paddingY="3">
            <Stack as="span" shrink>
              <span>{t(kindLabels[kind])}</span>
              <Text size="sm" tone="muted" wrap="anywhere">
                {csvFileNames[kind]}
              </Text>
            </Stack>
            <Button
              size="sm"
              aria-label={t("client:settings.import_export.export.download_of", { kind: t(kindLabels[kind]) })}
              isPending={exports.downloadingKind === kind}
              onPress={() => {
                exports.download(kind);
              }}
            >
              {t("client:settings.import_export.export.download")}
            </Button>
          </Cluster>
        ))}
      </Stack>
    </Stack>
  );
}

function ImportView({ form }: { form: ImportForm }) {
  const t = useT();
  const fileHeadingId = useId();
  const fileStateId = useId();
  return (
    <Stack gap="4">
      <p>{t("client:settings.import_export.import.description")}</p>
      <RadioGroup
        label={t("client:settings.import_export.import.kind")}
        value={form.kind}
        onChange={(value) => {
          if (isKind(value)) form.changeKind(value);
        }}
      >
        {dataImportKinds.map((kind) => (
          <Radio key={kind} value={kind}>
            {t(kindLabels[kind])}
          </Radio>
        ))}
      </RadioGroup>
      <RadioGroup
        label={t("client:settings.import_export.import.mode")}
        value={form.mode}
        onChange={(value) => {
          if (value === "merge" || value === "overwrite") form.changeMode(value);
        }}
      >
        <Radio value="merge" description={t("client:settings.import_export.import.mode.merge.description")}>
          {t(modeLabels.merge)}
        </Radio>
        <Radio value="overwrite" description={t("client:settings.import_export.import.mode.overwrite.description")}>
          {t(modeLabels.overwrite)}
        </Radio>
      </RadioGroup>
      <Stack gap="2">
        <Heading level={3} id={fileHeadingId} size="label">
          {t("client:settings.import_export.import.file")}
        </Heading>
        <Cluster align="center" gap="2">
          <FileTrigger
            acceptedFileTypes={csvFileTypes}
            onSelect={(files) => {
              const file = files?.[0];
              if (file !== undefined) form.chooseFile(file);
            }}
          >
            <Button size="sm" aria-describedby={`${fileHeadingId} ${fileStateId}`}>
              {t("client:settings.import_export.import.choose")}
            </Button>
          </FileTrigger>
          <Text id={fileStateId} shrink size="sm" tone="muted" wrap="anywhere">
            {form.fileName === undefined
              ? t("client:settings.import_export.import.no_file")
              : t("client:settings.import_export.import.chosen", {
                  name: form.fileName,
                })}
          </Text>
        </Cluster>
        {form.fileError !== undefined && <InlineError alert>{form.fileError}</InlineError>}
      </Stack>
      <div>
        <Button onPress={form.preview} isDisabled={!form.canPreview} isPending={form.isPreviewing}>
          {t("client:settings.import_export.import.preview")}
        </Button>
      </div>
      {form.result !== undefined && <PreviewView result={form.result} form={form} />}
      {form.error !== undefined && (
        <InlineError alert size="inherit">
          {form.error}
        </InlineError>
      )}
      <StatusMessage tone="success">{form.status}</StatusMessage>
    </Stack>
  );
}

function isKind(value: string): value is DataImportKind {
  return (dataImportKinds as readonly string[]).includes(value);
}

function PreviewView({ result, form }: { result: DataImportPreview; form: ImportForm }) {
  const t = useT();
  const intl = useIntl();
  const headingId = useId();
  const skippedId = useId();
  return (
    <Stack as="section" aria-labelledby={headingId} gap="3" border="md" paddingX="4" paddingY="3">
      <Heading level={3} id={headingId}>
        {t("client:settings.import_export.import.preview.title")}
      </Heading>
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:settings.import_export.import.preview.entries")}>
          <Text numeric>{intl.formatNumber(result.entries)}</Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:settings.import_export.import.preview.present")}>
          <Text numeric>{intl.formatNumber(result.present)}</Text>
        </DescriptionItem>
        {form.mode === "overwrite" && (
          <DescriptionItem term={t("client:settings.import_export.import.preview.removals")}>
            <Text numeric>{intl.formatNumber(result.removals)}</Text>
          </DescriptionItem>
        )}
      </DescriptionList>
      {result.skipped.length > 0 && (
        <Stack gap="1">
          <Heading level={4} id={skippedId} size="label">
            {t("client:settings.import_export.import.preview.skipped")}
          </Heading>
          <Stack as="ul" aria-labelledby={skippedId} gap="1" textSize="sm">
            {result.skipped.slice(0, maxSkippedShown).map((line) => (
              <Stack as="li" key={line.line}>
                <Text dir="auto" wrap="anywhere">
                  {t("client:settings.import_export.import.preview.skipped_line", {
                    line: line.line,
                    value: line.value,
                  })}
                </Text>
                <Text tone="muted">{t(skipReasons[line.reason])}</Text>
              </Stack>
            ))}
          </Stack>
        </Stack>
      )}
      <div>
        <Button
          variant={form.mode === "overwrite" ? "danger" : "primary"}
          onPress={form.start}
          isPending={form.isStarting}
        >
          {t("client:settings.import_export.import.start")}
        </Button>
      </div>
    </Stack>
  );
}

function HistoryView({ imports }: { imports: DataImport[] }) {
  const t = useT();
  if (imports.length === 0) {
    return <EmptyState headingLevel={3} heading={t("client:settings.import_export.history.empty")} />;
  }
  return (
    <Stack as="ul" divided>
      {imports.map((item) => (
        <Box as="li" key={item.id} container paddingY="3">
          <ImportRow item={item} />
        </Box>
      ))}
    </Stack>
  );
}

function ImportRow({ item }: { item: DataImport }) {
  const t = useT();
  const intl = useIntl();
  const headingId = `import-${item.id}`;
  return (
    <Stack as="article" aria-labelledby={headingId} gap="3">
      <Heading level={3} id={headingId}>
        {t("client:settings.import_export.history.item", {
          kind: t(kindLabels[item.kind]),
          date: intl.formatDate(item.created_at, dateTimeFormat),
        })}
      </Heading>
      <DescriptionList columns={4}>
        <DescriptionItem term={t("client:settings.import_export.history.state")}>
          {t(stateLabels[item.state])}
        </DescriptionItem>
        <DescriptionItem term={t("client:settings.import_export.history.mode")}>
          {t(modeLabels[item.mode])}
        </DescriptionItem>
        <DescriptionItem term={t("client:settings.import_export.history.progress")}>
          <Text numeric>
            {t("client:settings.import_export.history.progress_value", {
              processed: item.processed,
              total: item.total,
            })}
          </Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:settings.import_export.history.failed")}>
          <Text numeric>{intl.formatNumber(item.failed)}</Text>
        </DescriptionItem>
      </DescriptionList>
    </Stack>
  );
}
