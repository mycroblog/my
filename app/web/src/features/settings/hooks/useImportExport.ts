// SPDX-License-Identifier: AGPL-3.0-only
// The follows, blocks, mutes, and bookmarks as Mastodon's CSV files (FR-008): downloading them, importing a file after a
// preview of what it would do, and the imports in progress, read again every few seconds until they finish.
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  cacheStartedImport,
  csvFileNames,
  type DataImport,
  type DataImportKind,
  type DataImportMode,
  type DataImportPreview,
  dataImportsQueryOptions,
  fetchCsvExport,
  maxImportTextLength,
  previewDataImportMutation,
  startDataImportMutation,
} from "../api/importExport";
import { saveFile } from "./saveFile";

/** How often an import in progress is checked, in ms. */
const progressPollInterval = 5000;

/** What the export section needs. */
export type CsvExports = {
  /** Saves the CSV file of a kind. */
  download: (kind: DataImportKind) => void;
  /** The kind whose file is being downloaded, if any. */
  downloadingKind: DataImportKind | undefined;
  /** Why the last download failed, localized; undefined when it did not. */
  error: string | undefined;
};

export function useCsvExports(): CsvExports {
  const api = useApi();
  const download = useMutation({
    mutationKey: ["get", "/csv_exports/{kind}"] as const,
    mutationFn: async (kind: DataImportKind) => {
      saveFile(await fetchCsvExport(api, kind), csvFileNames[kind]);
    },
  });
  return {
    download: (kind) => {
      download.mutate(kind);
    },
    downloadingKind: download.isPending ? download.variables : undefined,
    error: download.isError ? errorMessage(download.error) : undefined,
  };
}

/** The chosen file: none, read, or one that cannot be imported. */
type ChosenFile =
  | { status: "none" }
  | { status: "read"; name: string; text: string }
  | { status: "invalid"; name: string; error: string };

/** What the import form needs. */
export type ImportForm = {
  kind: DataImportKind;
  changeKind: (kind: DataImportKind) => void;
  mode: DataImportMode;
  changeMode: (mode: DataImportMode) => void;
  /** The name of the chosen file, if any. */
  fileName: string | undefined;
  /** Why the chosen file cannot be imported, localized. */
  fileError: string | undefined;
  chooseFile: (file: File) => void;
  /** Asks the server what importing the file would do; only with a file. */
  preview: () => void;
  canPreview: boolean;
  isPreviewing: boolean;
  /** What importing the file with this kind and mode would do, once previewed. */
  result: DataImportPreview | undefined;
  /** Starts the import; only after a preview. */
  start: () => void;
  isStarting: boolean;
  /** Why the preview or the start failed, localized. */
  error: string | undefined;
  /** What the last start did, for the section's StatusMessage. */
  status: string | undefined;
};

export function useImportForm(): ImportForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [kind, setKind] = useState<DataImportKind>("following");
  const [mode, setMode] = useState<DataImportMode>("merge");
  const [file, setFile] = useState<ChosenFile>({ status: "none" });
  const [status, setStatus] = useState<string | undefined>(undefined);
  const preview = useMutation(previewDataImportMutation(api));
  const start = useMutation({
    ...startDataImportMutation(api),
    onSuccess: (started) => {
      cacheStartedImport(api, queryClient, started);
      setFile({ status: "none" });
      preview.reset();
      setStatus(t("client:settings.import_export.import.started", { total: started.total }));
    },
  });
  // A preview answers for the file, kind, and mode it was asked for; changing any of them asks again.
  const changed = () => {
    preview.reset();
    start.reset();
    setStatus(undefined);
  };
  const text = file.status === "read" ? file.text : undefined;
  const failed = preview.isError ? preview.error : start.isError ? start.error : null;
  return {
    kind,
    changeKind: (next) => {
      setKind(next);
      changed();
    },
    mode,
    changeMode: (next) => {
      setMode(next);
      changed();
    },
    fileName: file.status === "none" ? undefined : file.name,
    fileError: file.status === "invalid" ? file.error : undefined,
    chooseFile: (chosen) => {
      changed();
      chosen.text().then(
        (content) => {
          setFile(
            content.length > maxImportTextLength
              ? {
                  status: "invalid",
                  name: chosen.name,
                  error: t("client:settings.import_export.import.file_too_large"),
                }
              : { status: "read", name: chosen.name, text: content },
          );
        },
        () => {
          setFile({
            status: "invalid",
            name: chosen.name,
            error: t("client:settings.import_export.import.file_unreadable"),
          });
        },
      );
    },
    preview: () => {
      if (text !== undefined && !preview.isPending) preview.mutate({ kind, mode, text });
    },
    canPreview: text !== undefined,
    isPreviewing: preview.isPending,
    result: preview.data,
    start: () => {
      if (text !== undefined && preview.isSuccess && !start.isPending) start.mutate({ kind, mode, text });
    },
    isStarting: start.isPending,
    error: failed === null ? undefined : errorMessage(failed),
    status,
  };
}

/** The user's latest imports, newest first, read again while one is in progress. */
export function useDataImports(): DataImport[] {
  const api = useApi();
  const queryClient = useQueryClient();
  const query = useSuspenseQuery({
    ...dataImportsQueryOptions(api, queryClient),
    refetchInterval: (q) =>
      q.state.data?.items.some((item) => item.state === "in_progress") === true ? progressPollInterval : false,
  });
  return query.data.items;
}
