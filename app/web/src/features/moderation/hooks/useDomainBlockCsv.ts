// SPDX-License-Identifier: AGPL-3.0-only
// The domain block list in Mastodon's CSV (FR-706): saving the export as a file, and importing a CSV in two
// steps. The preview says what each row would do; the moderator clears the rows not wanted and imports the
// rest, after a confirmation when they suspend a domain (NFR-MOD-7). The result stays on the page until the next
// preview.
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type Translate, useT } from "../../../lib/i18n";
import {
  cacheImportedDomainBlocks,
  type DomainBlockImportResult,
  type DomainBlockImportRow,
  domainBlockFileName,
  domainBlockImportByteLimit,
  fetchDomainBlockExport,
  importDomainBlocksMutation,
  previewDomainBlockImportMutation,
} from "../api/domainBlocks";

/** How long the address of a saved file stays valid: long enough for the browser to start the download. */
const downloadUrlLifetime = 60_000;

/** Saves `blob` as a file named `name` through a temporary link, as the browser's download does. */
function saveFile(blob: Blob, name: string): void {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  link.click();
  setTimeout(() => {
    URL.revokeObjectURL(url);
  }, downloadUrlLifetime);
}

/** What the export button needs. */
export type DomainBlockExport = {
  /** Saves the CSV of the actions in effect as a file. */
  exportCsv: () => void;
  isPending: boolean;
  /** What the last export did, for its StatusMessage. */
  status: string | undefined;
  /** Why the last export failed, localized. */
  error: string | undefined;
};

export function useDomainBlockExport(): DomainBlockExport {
  const api = useApi();
  const t = useT();
  const mutation = useMutation({
    mutationKey: ["get", "/admin/domain_actions/export"] as const,
    mutationFn: async () => {
      saveFile(await fetchDomainBlockExport(api), domainBlockFileName);
    },
  });
  return {
    exportCsv: () => {
      if (!mutation.isPending) mutation.mutate();
    },
    isPending: mutation.isPending,
    status: mutation.isSuccess
      ? t("client:admin.domain_actions.csv.export.done", { name: domainBlockFileName })
      : undefined,
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
  };
}

/** The kilobytes of the byte limit, for the messages. */
const kilobytes = domainBlockImportByteLimit / 1000;

/** Why a CSV cannot be sent, localized, or null when it can: it is empty or larger than the server reads. */
export function validateDomainBlockCsv(t: Translate, text: string): string | null {
  if (text.trim() === "") return t("client:admin.domain_actions.csv.csv.required");
  if (new TextEncoder().encode(text).length > domainBlockImportByteLimit)
    return t("client:admin.domain_actions.csv.csv.too_large", { max: kilobytes });
  return null;
}

/** What reading the chosen file did, for next to its button: nothing yet, read, or failed. */
type FileState = { status: "none" } | { status: "read"; message: string } | { status: "failed"; message: string };

/** The preview of the last CSV: its rows, and an ID that changes with every preview (a `key`). */
export type DomainBlockPreview = { rows: readonly DomainBlockImportRow[]; id: number };

/** The outcome of the last import, and an ID that changes with every import. */
type DomainBlockImportOutcome = DomainBlockImportResult & { id: number };

/** What the import form needs. */
export type DomainBlockImportForm = {
  text: string;
  changeText: (text: string) => void;
  /** Reads a CSV file into the text area, in place of what it held. */
  chooseFile: (file: File) => void;
  file: FileState;
  /** Previews the CSV once the browser has checked it. */
  submitPreview: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPreviewing: boolean;
  /** Why the preview failed, localized, for under the form. */
  previewError: string | undefined;
  /** The preview of the CSV; undefined before it and after an import. */
  preview: DomainBlockPreview | undefined;
  /** Whether a row that takes actions is chosen: every one is, until cleared. */
  isChosen: (line: number) => boolean;
  choose: (line: number, chosen: boolean) => void;
  /** The number of chosen rows. */
  chosenCount: number;
  privateNote: string;
  changePrivateNote: (note: string) => void;
  /** Imports the chosen rows; when they suspend a domain, asks first. */
  submitImport: () => void;
  isImporting: boolean;
  /** Why the import cannot be sent or failed, localized: in the confirmation while it is open, else under the rows. */
  importError: string | undefined;
  /** The confirmation of an import that suspends domains: how many, or null while it is closed. */
  confirmation: { count: number; confirm: () => void; cancel: () => void } | null;
  /** What the last import did; undefined before the first one. */
  outcome: DomainBlockImportOutcome | undefined;
};

/**
 * The import of a domain block CSV. A success says what was imported once what the import changed is fetched
 * again, and the preview gives way to that result; the CSV and the note are emptied.
 */
export function useDomainBlockImportForm(): DomainBlockImportForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [text, setText] = useState("");
  const [file, setFile] = useState<FileState>({ status: "none" });
  const [cleared, setCleared] = useState<ReadonlySet<number>>(() => new Set());
  const [privateNote, setPrivateNote] = useState("");
  const [confirming, setConfirming] = useState(false);
  const [noneChosen, setNoneChosen] = useState(false);
  const preview = useMutation({
    ...previewDomainBlockImportMutation(api),
    onSuccess: () => {
      // Every row that takes actions starts chosen.
      setCleared(new Set());
    },
  });
  const importing = useMutation({
    ...importDomainBlocksMutation(api),
    onSuccess: async (result) => {
      await cacheImportedDomainBlocks(queryClient, result);
      setConfirming(false);
      setText("");
      setPrivateNote("");
      setFile({ status: "none" });
      preview.reset();
    },
  });
  const rows = preview.data?.rows;
  const chosenRows = (rows ?? []).filter(
    (row) => row.status === "importable" && row.block !== undefined && !cleared.has(row.line),
  );
  const run = () => {
    importing.mutate({
      blocks: chosenRows.flatMap((row) => (row.block === undefined ? [] : [row.block])),
      ...(privateNote === "" ? {} : { private_note: privateNote }),
    });
  };
  const suspended = chosenRows.filter((row) => row.issue.includes("suspend")).length;
  const importError = noneChosen
    ? t("client:admin.domain_actions.csv.none_selected")
    : importing.isError
      ? errorMessage(importing.error)
      : undefined;
  return {
    text,
    changeText: (next) => {
      setText(next);
    },
    chooseFile: (chosen) => {
      if (chosen.size > domainBlockImportByteLimit) {
        setFile({
          status: "failed",
          message: t("client:admin.domain_actions.csv.file.too_large", { name: chosen.name, max: kilobytes }),
        });
        return;
      }
      chosen.text().then(
        (read) => {
          setText(read);
          setFile({ status: "read", message: t("client:admin.domain_actions.csv.file.read", { name: chosen.name }) });
        },
        () => {
          setFile({
            status: "failed",
            message: t("client:admin.domain_actions.csv.file.failed", { name: chosen.name }),
          });
        },
      );
    },
    file,
    submitPreview: (event) => {
      event.preventDefault();
      if (preview.isPending) return;
      importing.reset();
      setNoneChosen(false);
      preview.mutate(text);
    },
    isPreviewing: preview.isPending,
    previewError: preview.isError ? errorMessage(preview.error) : undefined,
    preview: rows === undefined ? undefined : { rows, id: preview.submittedAt },
    isChosen: (line) => !cleared.has(line),
    choose: (line, chosen) => {
      const next = new Set(cleared);
      if (chosen) next.delete(line);
      else next.add(line);
      setCleared(next);
      setNoneChosen(false);
    },
    chosenCount: chosenRows.length,
    privateNote,
    changePrivateNote: (note) => {
      setPrivateNote(note);
    },
    submitImport: () => {
      if (importing.isPending) return;
      importing.reset();
      if (chosenRows.length === 0) {
        setNoneChosen(true);
        return;
      }
      if (suspended > 0) setConfirming(true);
      else run();
    },
    isImporting: importing.isPending,
    importError,
    confirmation: confirming
      ? {
          count: suspended,
          confirm: run,
          cancel: () => {
            setConfirming(false);
            importing.reset();
          },
        }
      : null,
    outcome: importing.isSuccess ? { ...importing.data, id: importing.submittedAt } : undefined,
  };
}
