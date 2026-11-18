// SPDX-License-Identifier: AGPL-3.0-only

// Import, the screen under the settings' entry Account and data at /settings/account/import (FR-008): where the
// reader brings a list in from a CSV file, in the format Mastodon exports. A form sent as a whole by its submit bar,
// Import, over the imports the server lists.
//
// - What the file lists (follows, followed hashtags, blocked and muted accounts, muted servers, bookmarks), the file
//   itself, and how it is imported: merged into what the reader has, or replacing it.
// - What the import would do, as the server reads the file, asked again whenever one of the three changes: how many
//   it adds, how many the reader has already, and the lines it cannot read, the first few written out with why, each
//   line of the file in quotation marks as the file writes it; then, last and apart under its own name, what a
//   replacement removes. Nothing changes until the form is sent.
// - The imports of the last days, which the server carries out in the background: what each lists and how, when it
//   started, and where it stands in the text's color (how far it is, or what came of it, and after a failure that
//   importing the file again with Merge tries the rest: the server carries out every entry again and finds the ones
//   it has done), read again while one runs. The API counts the failures and does not say which lines they were.
//
// What is wrong with the file (none chosen, too large, nothing in it to import, what the server refuses of it) stands
// under the file's field, which takes the focus, the submit bar saying only that nothing was imported. What a
// replacement removes cannot be put back, so the submit bar says it beside Import while it removes something, and
// Import then asks before it acts, saying how many it removes and that it cannot be undone. Once the
// server took the import, the form is empty again and the imports' status, a polite live region there from the
// start, says that it started and takes the focus; a server that already carries one out says so in the submit bar,
// and the form stays as it was.

import {
  csvKinds,
  dataImportsQuery,
  importLimits,
  importRunning,
  importTooLarge,
  refusalDetail,
  refusedField,
  useDataImports,
  useImportPreview,
  type AppData,
  type DataImport,
  type DataImports,
  type ImportKind,
  type ImportMode,
  type ImportPreviewState,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  ConfirmDialog,
  FileField,
  focusRefusedField,
  FormAlert,
  ModalPresence,
  Select,
  SettingRow,
  SettingsEmpty,
  SettingsFacts,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  Stack,
  SubmitBar,
  useLocale,
} from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { stringsIn } from "../locale";
import { tied, useActionDate } from "./account";

type T = ReturnType<typeof stringsIn>["t"];

/** What the route's loader waits for: the reader's imports, from the cache when it has them. */
export async function loadAccountImport(data: AppData): Promise<void> {
  await data.queryClient.query({ ...dataImportsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The title of the screen. */
export function useAccountImportTitle(): string {
  return stringsIn(useLocale().locale).t("settings.account.import");
}

/** How many of the lines a file cannot be read at are written out. */
const skippedShown = 5;

/**
 * What holds the place of a line of the file in a sentence until the sentence is formatted: the file's text goes in
 * after, untouched by what joins the sentence's own numbers to their words. A character of the private use area, which
 * no catalog writes.
 */
const echoed = "\uE000";

/** The file the reader chose: its name and its text. */
type Picked = { readonly name: string; readonly text: string };

/** Where the form's last send is; one that was not sent, with the reason the submit bar gives, if it has one. */
type Sent = { readonly kind: "idle" | "sending" } | { readonly kind: "refused"; readonly reason: string | undefined };

/**
 * Where an import stands, for its line: in progress, finished with nothing to do, with everything done, with some
 * entries not carried out, or with none carried out.
 */
function importStateOf(i: DataImport): "running" | "nothing" | "done" | "partly" | "failed" {
  if (i.state === "in_progress") return "running";
  if (i.total === 0) return "nothing";
  if (i.failed === 0) return "done";
  return i.failed >= i.total ? "failed" : "partly";
}

/** The imports the server lists, each with where it stands; and what came of the reader's last send. */
function Recent({ list, status, t }: { list: DataImports; status: ReactNode; t: T }) {
  const date = useActionDate();
  const { imports, status: read, retry } = list;
  const heading = t("settings.account.import.recent");
  return (
    <SettingsSection heading={heading} about={t("settings.account.import.recent.about")}>
      <Stack>
        {status}
        {read !== "success" || imports === undefined ? (
          read === "error" ? (
            <SettingsEmpty
              action={
                <Button size="timeline" touch onPress={retry}>
                  {t("settings.retry")}
                </Button>
              }
            >
              {t("settings.account.import.recent.failed")}
            </SettingsEmpty>
          ) : (
            <SettingsEmpty>{t("settings.account.import.recent.loading")}</SettingsEmpty>
          )
        ) : imports.length === 0 ? (
          <SettingsEmpty>{t("settings.account.import.recent.none")}</SettingsEmpty>
        ) : (
          <SettingsList label={heading}>
            {imports.map((i) => (
              <SettingsListRow
                key={i.id}
                title={t("settings.account.csv.kind", { kind: i.kind })}
                facts={[
                  [
                    t("settings.account.import.mode.choice", { mode: i.mode }),
                    t("settings.account.import.row.started", { date: date(i.created_at) }),
                  ],
                ]}
                standing={t("settings.account.import.row.state", {
                  state: importStateOf(i),
                  done: i.processed - i.failed,
                  processed: i.processed,
                  failed: i.failed,
                  total: i.total,
                })}
              />
            ))}
          </SettingsList>
        )}
      </Stack>
    </SettingsSection>
  );
}

/** What the import would do, as the server read the file: a fact each, and the lines it cannot read. */
function Preview({
  previewed,
  kind,
  mode,
  t,
}: {
  previewed: ImportPreviewState;
  kind: ImportKind;
  mode: ImportMode;
  t: T;
}) {
  const heading = t("settings.account.import.preview");
  const { preview } = previewed;
  const more = (preview?.skipped.length ?? 0) - skippedShown;
  // What the import leaves the reader with or leaves out of the file: nothing of these is lost.
  const items =
    preview === undefined
      ? []
      : [
          { text: t("settings.account.import.preview.add", { kind, count: preview.entries - preview.present }) },
          ...(preview.present > 0
            ? [{ text: t("settings.account.import.preview.present", { kind, count: preview.present }) }]
            : []),
          ...(preview.skipped.length > 0
            ? [
                {
                  text: t("settings.account.import.preview.skipped", { count: preview.skipped.length }),
                  about: [
                    // The file's own line stands in the sentence as the file writes it, in the catalog's quotes.
                    ...preview.skipped.slice(0, skippedShown).map((s) =>
                      t("settings.account.import.preview.skipped.line", {
                        line: s.line,
                        reason: s.reason,
                        value: echoed,
                      }).replace(echoed, () => s.value),
                    ),
                    ...(more > 0 ? [t("settings.account.import.preview.skipped.more", { count: more })] : []),
                  ],
                },
              ]
            : []),
        ];
  return (
    <SettingsSection heading={heading} about={t("settings.account.import.preview.about")}>
      {/* Its place is a polite live region from the start: what the server answers is read when it comes. */}
      <Box role="status">
        {preview === undefined ? (
          <Box as="p" tone="muted" breaks="phrase">
            {t("settings.account.import.preview.waiting", { state: previewed.status })}
          </Box>
        ) : (
          <Stack gap="form-page-form-gap">
            <SettingsFacts label={heading} items={items} />
            {/* What a replacement removes stands last and apart, under its own name: it is what cannot be put back. */}
            {mode === "overwrite" && (
              <SettingsFacts
                label={t("settings.account.import.preview.removes")}
                shown
                items={[{ text: t("settings.account.import.preview.remove", { kind, count: preview.removals }) }]}
              />
            )}
          </Stack>
        )}
      </Box>
    </SettingsSection>
  );
}

/** The screen: the form, what it would do, and the imports; `onBack` goes back to the entry. */
export function AccountImportScreen({ onBack }: { onBack: () => void }) {
  const strings = stringsIn(useLocale().locale);
  // A number stays with the word after it (「7 件」), in every sentence of the screen.
  const t: T = (key, values) => tied(strings.t(key, values));
  const id = useId();
  const imports = useDataImports();
  const [kind, setKind] = useState<ImportKind>("following");
  const [mode, setMode] = useState<ImportMode>("merge");
  const [file, setFile] = useState<Picked | undefined>(undefined);
  // The name of the file picked last while the browser reads it, and how many were picked: a file read after another
  // was picked is dropped.
  const [reading, setReading] = useState<string | undefined>(undefined);
  const picks = useRef(0);
  // What the form itself finds wrong with the file, or the server when the form is sent.
  const [found, setFound] = useState<string | undefined>(undefined);
  const [sent, setSent] = useState<Sent>({ kind: "idle" });
  const [asking, setAsking] = useState(false);
  const [said, setSaid] = useState<{ readonly text: string; readonly n: number } | undefined>(undefined);
  const count = useRef(0);
  // What takes the focus once the screen is drawn again: the imports' status, or the file's field that was refused.
  const focusNext = useRef<"status" | "file" | undefined>(undefined);
  const fileId = `${id}-file`;
  const statusId = `${id}-status`;
  const kindId = `${id}-kind`;
  const modeId = `${id}-mode`;
  useEffect(() => {
    if (focusNext.current === "file") focusRefusedField(fileId);
    else if (focusNext.current === "status") document.getElementById(statusId)?.focus();
    focusNext.current = undefined;
  });

  /** What the field says of a file too large to import, `name`. */
  const tooLarge = (name: string) => t("settings.account.import.file.too_large", { name, max: importLimits.text });
  /**
   * What the server says of the file `name` it refuses, for under the file's field: its words about the file, if it
   * has some; the screen's own sentence of a file too large where the request was larger than the server takes.
   */
  const fileRefusal = (error: unknown, name: string): string | undefined => {
    if (importTooLarge(error)) return tooLarge(name);
    const named = refusedField(error);
    if (named !== undefined) return named.member === "text" ? named.detail : undefined;
    return refusalDetail(error);
  };
  // What importing the file would do, asked whenever the file, what it lists or how it is imported changes.
  const previewed = useImportPreview(file === undefined ? undefined : { kind, mode, text: file.text });
  const unread =
    previewed.status === "error" && file !== undefined
      ? (fileRefusal(previewed.error, file.name) ?? t("settings.account.import.file.unread"))
      : undefined;
  const fileError = found ?? unread;

  const busy = sent.kind === "sending";
  /** A change of the form: what the last send said was about the form as it was. */
  const changed = () => {
    setFound(undefined);
    setSent((was) => (was.kind === "refused" ? { kind: "idle" } : was));
  };
  const refuseFile = (text: string) => {
    setFound(text);
    setSent({ kind: "refused", reason: undefined });
    focusNext.current = "file";
  };

  const pick = (picked: File) => {
    changed();
    // Only the file picked last is kept: a file read meanwhile holds no place in the form, and while it is read the
    // form holds no file, its name standing in the field and Import waiting.
    picks.current += 1;
    const n = picks.current;
    setFile(undefined);
    setReading(picked.name);
    picked.text().then(
      (text) => {
        if (n !== picks.current) return;
        setReading(undefined);
        // The server counts characters; only a text that may pass its limit is counted. A file too large is no file
        // to import: the field holds none, and says which one it was. The API names no limit of the request's size,
        // past which the server answers 413, said as the same sentence (fileRefusal).
        if (text.length > importLimits.text && Array.from(text).length > importLimits.text) {
          setFound(tooLarge(picked.name));
        } else setFile({ name: picked.name, text });
      },
      () => {
        if (n !== picks.current) return;
        setReading(undefined);
        setFound(t("settings.account.import.file.unread"));
      },
    );
  };

  const start = () => {
    if (file === undefined) return;
    setAsking(false);
    setSent({ kind: "sending" });
    imports.start({ kind, mode, text: file.text }).then(
      () => {
        // The form is empty again, and the imports' status says what came of sending.
        setFile(undefined);
        setFound(undefined);
        setMode("merge");
        setSent({ kind: "idle" });
        count.current += 1;
        setSaid({ text: t("settings.account.import.started"), n: count.current });
        focusNext.current = "status";
      },
      (error: unknown) => {
        if (importRunning(error)) {
          imports.retry();
          setSent({ kind: "refused", reason: t("settings.account.import.running") });
          return;
        }
        const words = fileRefusal(error, file.name);
        if ((importTooLarge(error) || refusedField(error) !== undefined) && words !== undefined) refuseFile(words);
        else setSent({ kind: "refused", reason: words ?? t("settings.account.import.failed") });
      },
    );
  };
  const send = () => {
    if (busy || reading !== undefined) return;
    if (file === undefined) {
      refuseFile(found ?? t("settings.account.import.file.missing"));
      return;
    }
    const { preview } = previewed;
    if (preview === undefined) {
      // The server refused the file, and its field says why: the focus goes back to it.
      if (unread !== undefined) refuseFile(unread);
      return;
    }
    if (preview.entries === 0 && !(mode === "overwrite" && preview.removals > 0)) {
      refuseFile(t("settings.account.import.file.empty"));
      return;
    }
    // A replacement that removes something asks first: what it removes cannot be put back.
    if (mode === "overwrite" && preview.removals > 0) setAsking(true);
    else start();
  };

  // What the submit bar says beside Import: what a replacement removes, which cannot be put back; nothing for a
  // merge, nor for a replacement that removes nothing.
  const removesId = `${id}-removes`;
  const removals = mode === "overwrite" && file !== undefined ? (previewed.preview?.removals ?? 0) : 0;
  const removes = removals > 0 ? t("settings.account.import.submit.removes", { kind, count: removals }) : undefined;
  const status = (
    <Box as="p" id={statusId} role="status" tabIndex={-1} breaks="phrase">
      {said !== undefined && <span key={said.n}>{said.text}</span>}
    </Box>
  );
  return (
    <>
      <SettingsScreen
        sub
        title={t("settings.account.import")}
        back={<BackButton onPress={onBack}>{t("settings.entry.account")}</BackButton>}
        foot={
          <SubmitBar
            result={
              sent.kind === "refused" ? (
                <FormAlert>{sent.reason ?? t("settings.account.import.refused")}</FormAlert>
              ) : undefined
            }
            about={removes === undefined ? undefined : { id: removesId, text: removes }}
          >
            <Button
              kind="primary"
              busy={busy}
              isDisabled={reading !== undefined || previewed.status === "pending"}
              onPress={send}
              {...(removes === undefined ? {} : { "aria-describedby": removesId })}
            >
              {t("settings.account.import.submit")}
            </Button>
          </SubmitBar>
        }
      >
        <SettingsSection heading={t("settings.account.import.file.section")} about={t("settings.account.import.about")}>
          <SettingRow id={kindId} label={t("settings.account.import.kind")} form>
            <Select
              name="kind"
              options={csvKinds.map((value) => ({ value, label: t("settings.account.csv.kind", { kind: value }) }))}
              value={kind}
              onChange={(value) => {
                setKind(csvKinds.find((k) => k === value) ?? "following");
                changed();
              }}
              isDisabled={busy}
            />
          </SettingRow>
          <FileField
            id={fileId}
            label={t("settings.account.import.file")}
            error={fileError}
            choose={t("settings.account.import.file.choose", {
              chosen: file === undefined && reading === undefined ? "no" : "yes",
            })}
            file={reading ?? file?.name}
            none={t("settings.account.import.file.none")}
            accept=".csv,text/csv,text/plain"
            onFile={pick}
            isDisabled={busy}
            row
          />
          <SettingRow
            id={modeId}
            label={t("settings.account.import.mode")}
            about={t("settings.account.import.mode.about", { mode })}
            form
          >
            <Select
              name="mode"
              options={(["merge", "overwrite"] as const).map((value) => ({
                value,
                label: t("settings.account.import.mode.choice", { mode: value }),
              }))}
              value={mode}
              onChange={(value) => {
                setMode(value === "overwrite" ? "overwrite" : "merge");
                changed();
              }}
              isDisabled={busy}
            />
          </SettingRow>
        </SettingsSection>
        <Preview previewed={previewed} kind={kind} mode={mode} t={t} />
        <Recent list={imports} status={status} t={t} />
      </SettingsScreen>
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            key="replace"
            title={t("settings.account.import.replace.title", { kind })}
            cancel={t("settings.account.import.replace.cancel")}
            confirm={t("settings.account.import.replace.confirm")}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={start}
          >
            {t("settings.account.import.replace.text", { kind, count: previewed.preview?.removals ?? 0 })}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </>
  );
}
