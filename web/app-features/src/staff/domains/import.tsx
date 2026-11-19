// SPDX-License-Identifier: AGPL-3.0-only

// Mastodon's domain block CSV, at /moderation/domain-actions/import (FR-706): saving the actions in effect as that CSV,
// which Mastodon imports; and importing one that Mastodon exported, in two steps. A file chosen is read and previewed
// before anything is applied: the rows that would issue actions, each with the kinds it would issue, all kept at first,
// which the reader keeps or leaves; and the rows that would import nothing, each with why. Import issues the rows kept,
// with a private note for every action; a suspend among them asks first, since it cuts follows for good. What was
// issued and what was skipped then stands in the form's place.

import {
  domainCommentLimit,
  useDomainActionWrites,
  useDomainImportPreview,
  type DomainImportResult,
  type DomainImportRow,
} from "@my/app-data";
import {
  BackButton,
  Button,
  ConfirmDialog,
  FeedNotice,
  FileField,
  FormAlert,
  LinkButton,
  ModalPresence,
  RecordTable,
  Row,
  ScreenStack,
  ScreenTitle,
  Stack,
  TextField,
  type RecordRow,
} from "@my/design-system";
import { useId, useRef, useState, type ReactNode, type Ref } from "react";
import { saveFile } from "../../save-file";
import { Titled, useStaffStrings, type T } from "../parts";
import { domainActionsPath, domainKindWords, refusalWords, tied, useSaid } from "./parts";

/** The kinds a row would issue, joined as the reader's language joins a list. */
function kindsOf(t: T, locale: string, row: DomainImportRow): string {
  return new Intl.ListFormat(locale, { type: "conjunction" }).format(row.issue.map((k) => domainKindWords(t, k)));
}

/** What a row would do, in words: the kinds it would issue, or why it would issue nothing. */
const statusWords = (t: T, locale: string, row: DomainImportRow): string =>
  row.status === "importable" ? kindsOf(t, locale, row) : t("staff_domains.import.status", { status: row.status });

/** Saving and importing Mastodon's CSV. */
export function DomainImportScreen({ onBack }: { onBack: () => void }): ReactNode {
  const { t } = useStaffStrings();
  const [result, setResult] = useState<DomainImportResult | undefined>(undefined);
  const titleRef = useRef<HTMLHeadingElement>(null);
  const back = <BackButton onPress={onBack}>{t("staff_domains.actions.back")}</BackButton>;
  if (result !== undefined) return <Imported result={result} back={back} titleRef={titleRef} />;
  return (
    <ScreenStack>
      <ScreenTitle back={back}>{t("staff_domains.actions.csv")}</ScreenTitle>
      <Export />
      <Import
        onImported={(answer) => {
          setResult(answer);
          requestAnimationFrame(() => titleRef.current?.focus());
        }}
      />
    </ScreenStack>
  );
}

/** Saving the actions in effect as Mastodon's CSV, and what came of it. */
function Export() {
  const { t } = useStaffStrings();
  const { exportCsv } = useDomainActionWrites();
  const { say, status } = useSaid();
  const [busy, setBusy] = useState(false);
  return (
    <Titled title={t("staff_domains.export")}>
      <Stack gap="screen-field-gap">
        <Row>
          <Button
            size="timeline"
            busy={busy}
            onPress={() => {
              if (busy) return;
              setBusy(true);
              exportCsv().then(
                ({ blob, name }) => {
                  setBusy(false);
                  saveFile(blob, name);
                  say(t("staff_domains.export.saved", { name }));
                },
                (e: unknown) => {
                  setBusy(false);
                  say(refusalWords(t, e), true);
                },
              );
            }}
          >
            {t("staff_domains.export.save")}
          </Button>
        </Row>
        {status}
      </Stack>
    </Titled>
  );
}

/** A file chosen and read. */
type Picked = { readonly name: string; readonly text: string };

/** Choosing a file, its preview, the rows kept, the private note, and Import. */
function Import({ onImported }: { onImported: (result: DomainImportResult) => void }) {
  const { t, locale } = useStaffStrings();
  const { importBlocks } = useDomainActionWrites();
  const id = useId();
  const fileId = `${id}-file`;
  const noteId = `${id}-note`;
  const [file, setFile] = useState<Picked | undefined>(undefined);
  const [reading, setReading] = useState<string | undefined>(undefined);
  const picks = useRef(0);
  const [found, setFound] = useState<string | undefined>(undefined);
  const [note, setNote] = useState("");
  // The rows the reader left out, by line: every row that would issue actions is kept at first.
  const [left, setLeft] = useState<ReadonlySet<string>>(new Set());
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [alert, setAlert] = useState<string | undefined>(undefined);
  const previewed = useDomainImportPreview(file?.text);
  const refused =
    previewed.status === "error"
      ? (previewed.error as { status?: unknown } | null)?.status === 413
        ? t("staff_domains.import.file.too_large")
        : t("staff_domains.import.file.unread")
      : undefined;
  const pick = (picked: File) => {
    // Only the file picked last is kept; while it is read, the form holds none.
    picks.current += 1;
    const n = picks.current;
    setFile(undefined);
    setFound(undefined);
    setAlert(undefined);
    setLeft(new Set());
    setReading(picked.name);
    picked.text().then(
      (text) => {
        if (n !== picks.current) return;
        setReading(undefined);
        setFile({ name: picked.name, text });
      },
      () => {
        if (n !== picks.current) return;
        setReading(undefined);
        setFound(t("staff_domains.import.file.unread"));
      },
    );
  };
  const rows = previewed.preview?.rows ?? [];
  const importable = rows.filter((r) => r.status === "importable" && r.block !== undefined);
  const others = rows.filter((r) => r.status !== "importable" || r.block === undefined);
  const kept = importable.filter((r) => !left.has(String(r.line)));
  const suspends = kept.filter((r) => r.issue.includes("suspend")).length;
  const send = () => {
    setBusy(true);
    setAlert(undefined);
    importBlocks(
      kept.flatMap((r) => (r.block === undefined ? [] : [r.block])),
      note,
    ).then(
      (answer) => {
        setBusy(false);
        setAsking(false);
        onImported(answer);
      },
      (e: unknown) => {
        setBusy(false);
        setAsking(false);
        setAlert(refusalWords(t, e));
      },
    );
  };
  const start = () => {
    if (busy || kept.length === 0) return;
    if (suspends > 0) setAsking(true);
    else send();
  };
  const keptRows = importable.map((r): RecordRow<"line" | "issues"> => ({
    id: String(r.line),
    head: r.block?.domain ?? r.written_domain,
    caption: r.block?.public_comment,
    cells: { line: t("staff_domains.import.line", { line: r.line }), issues: kindsOf(t, locale, r) },
  }));
  const otherRows = others.map((r): RecordRow<"line" | "status"> => ({
    id: String(r.line),
    head: r.written_domain === "" ? t("staff_domains.import.no_domain") : r.written_domain,
    cells: { line: t("staff_domains.import.line", { line: r.line }), status: statusWords(t, locale, r) },
  }));
  return (
    <Titled title={t("staff_domains.import")}>
      <Stack gap="screen-field-gap">
        <FileField
          id={fileId}
          label={t("staff_domains.import.file")}
          hint={t("staff_domains.import.file.hint")}
          error={found ?? refused}
          choose={t("staff_domains.import.file.choose", {
            chosen: file === undefined && reading === undefined ? "no" : "yes",
          })}
          file={reading ?? file?.name}
          none={t("staff_domains.import.file.none")}
          accept=".csv,text/csv,text/plain"
          onFile={pick}
          isDisabled={busy}
        />
        {file !== undefined && previewed.status === "pending" && (
          <FeedNotice start>{t("staff_domains.import.previewing")}</FeedNotice>
        )}
        {previewed.preview !== undefined && (
          <>
            <Titled title={tied(t("staff_domains.import.importable", { count: importable.length }))}>
              <RecordTable
                label={t("staff_domains.import.importable.table")}
                head={{ label: t("staff_domains.import.column.domain") }}
                columns={[
                  { key: "line", label: t("staff_domains.import.column.line"), kind: "fit", bare: true },
                  { key: "issues", label: t("staff_domains.import.column.issues") },
                ]}
                rows={keptRows}
                selection={{
                  selected: new Set(keptRows.map((r) => r.id).filter((line) => !left.has(line))),
                  onChange: (selected) => {
                    setLeft(new Set(keptRows.map((r) => r.id).filter((line) => !selected.has(line))));
                  },
                  actions: [],
                }}
                empty={<FeedNotice start>{t("staff_domains.import.importable.none")}</FeedNotice>}
              />
            </Titled>
            {others.length > 0 && (
              <Titled title={tied(t("staff_domains.import.others", { count: others.length }))}>
                <RecordTable
                  label={t("staff_domains.import.others.table")}
                  head={{ label: t("staff_domains.import.column.domain") }}
                  columns={[
                    { key: "line", label: t("staff_domains.import.column.line"), kind: "fit", bare: true },
                    { key: "status", label: t("staff_domains.import.column.status") },
                  ]}
                  rows={otherRows}
                />
              </Titled>
            )}
            {importable.length > 0 && (
              <TextField
                id={noteId}
                name="private_note"
                label={t("staff_domains.import.note")}
                hint={t("staff_domains.form.private_note.hint")}
                multiline
                rows={2}
                maxLength={domainCommentLimit}
                onChange={setNote}
              />
            )}
          </>
        )}
        {alert !== undefined && <FormAlert regular>{alert}</FormAlert>}
        {importable.length > 0 && (
          <Row>
            <Button kind="primary" busy={busy && !asking} isDisabled={kept.length === 0} onPress={start}>
              {tied(t("staff_domains.import.submit", { count: kept.length }))}
            </Button>
          </Row>
        )}
      </Stack>
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            title={tied(t("staff_domains.import.confirm.title", { count: kept.length }))}
            cancel={t("staff_domains.cancel")}
            confirm={t("staff_domains.import.confirm")}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={send}
          >
            {tied(t("staff_domains.import.confirm.about", { count: suspends }))}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </Titled>
  );
}

/** What an import issued and skipped, in the form's place, its title taking the focus. */
function Imported({
  result,
  back,
  titleRef,
}: {
  result: DomainImportResult;
  back: ReactNode;
  titleRef: Ref<HTMLHeadingElement>;
}) {
  const { t } = useStaffStrings();
  const domains = new Set(result.issued.map((a) => a.domain)).size;
  return (
    <ScreenStack>
      <ScreenTitle back={back} ref={titleRef}>
        {tied(t("staff_domains.imported", { count: result.issued.length, domains }))}
      </ScreenTitle>
      <Titled title={tied(t("staff_domains.imported.skipped", { count: result.skipped.length }))}>
        <RecordTable
          label={t("staff_domains.imported.skipped.table")}
          head={{ label: t("staff_domains.import.column.domain") }}
          columns={[{ key: "reason", label: t("staff_domains.import.column.status") }]}
          rows={result.skipped.map((s, i) => ({
            id: `${String(i)}-${s.domain}`,
            head: s.domain,
            cells: { reason: t("staff_domains.import.status", { status: s.reason }) },
          }))}
          empty={<FeedNotice start>{t("staff_domains.imported.skipped.none")}</FeedNotice>}
        />
      </Titled>
      <Row>
        <LinkButton href={domainActionsPath}>{t("staff_domains.imported.to_list")}</LinkButton>
      </Row>
    </ScreenStack>
  );
}
