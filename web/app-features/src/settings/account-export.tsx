// SPDX-License-Identifier: AGPL-3.0-only

// Export, the screen under the settings' entry Account and data at /settings/account/export (FR-008): where the
// reader takes their data away. Everything on it acts at once, and says what came of it where it was pressed.
//
// - The archive of the account, which the server prepares in the background: the latest request, and under it the
//   newest archive that can still be downloaded where that is an earlier one. An archive is named by when it was
//   requested and says where it stands in one line, in the text's color: being prepared (the list is read again
//   meanwhile, and another can be requested once it is done), ready until the day the server removes it, with its
//   size and Download, expired, or failed. Request an archive stands at the section's head, or in the list's place
//   while there is none, and not while one is being prepared. What came of a request is said once, in the section's
//   status, a polite live region there from the start, which takes the focus: the button may be gone once it is
//   pressed.
// - The lists as CSV files, in the format Mastodon uses, a row each with Download: follows, followed hashtags (a file
//   of this software alone, which the section says), blocked and muted accounts, muted servers and bookmarks.
//
// The screen has one primary action: Download of the archive that is ready, while there is one, Request an archive
// being neutral beside it; else Request an archive. What came of a download (it started, or failed) is said in the
// row whose Download was pressed, the row's own status, a polite live region there from the start, the focus staying
// on the button. The address the closure's emails carry leads here with nothing more known of why the reader came:
// the screen reads as it does from the entry.
//
// The server says how long to wait when it refuses a request for being one too many: its words are shown.

import {
  csvKinds,
  dataExportsQuery,
  exportBuilding,
  refusalDetail,
  useDataExports,
  type AppData,
  type CsvKind,
  type DataExport,
  type SavedFile,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  Stack,
  useLocale,
  type SettingsRowSaid,
} from "@my/design-system";
import { useEffect, useId, useRef, useState } from "react";
import { stringsIn } from "../locale";
import { saveFile } from "../save-file";
import { archiveStateOf, useActionDate } from "./account";

type T = ReturnType<typeof stringsIn>["t"];

/** What the route's loader waits for: the reader's exports, from the cache when it has them. */
export async function loadAccountExport(data: AppData): Promise<void> {
  await data.queryClient.query({ ...dataExportsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The title of the screen. */
export function useAccountExportTitle(): string {
  return stringsIn(useLocale().locale).t("settings.account.export");
}

/**
 * The exports a reader comes for, of `exports`, newest first: the latest request, and the newest archive that can
 * still be downloaded where that is an earlier one.
 */
function shownExports(exports: readonly DataExport[]): readonly DataExport[] {
  const [latest] = exports;
  if (latest === undefined) return [];
  const ready = exports.find((e) => archiveStateOf(e) === "ready");
  return ready === undefined || ready === latest ? [latest] : [latest, ready];
}

/** The size of a file of `bytes`, as the reader's language writes it, in the unit that keeps it short. */
function sizeOf(bytes: number, lang: string): string {
  const [unit, size] =
    bytes >= 1_000_000_000
      ? (["gigabyte", bytes / 1_000_000_000] as const)
      : bytes >= 1_000_000
        ? (["megabyte", bytes / 1_000_000] as const)
        : (["kilobyte", Math.max(bytes / 1000, 1)] as const);
  return new Intl.NumberFormat(lang, { style: "unit", unit, maximumFractionDigits: size < 10 ? 1 : 0 }).format(size);
}

/** What a status says, the section's or a row's, each a new element of it. */
type Said = SettingsRowSaid;

/** The status of the archive's section: a polite live region there from the start, and a place the focus can be. */
function Status({ id, said }: { id: string; said: Said | undefined }) {
  return (
    <Box as="p" id={id} role="status" tabIndex={-1} breaks="phrase">
      {said !== undefined && <span key={said.n}>{said.text}</span>}
    </Box>
  );
}

/** The archive of the account: the requests the reader comes for, and requesting one. */
function Archive({ t }: { t: T }) {
  const { lang } = stringsIn(useLocale().locale);
  const date = useActionDate();
  const day = useActionDate(false);
  const id = useId();
  const { exports, status, retry, request, download } = useDataExports();
  const [requesting, setRequesting] = useState(false);
  const [downloading, setDownloading] = useState<string | undefined>(undefined);
  const [said, setSaid] = useState<Said | undefined>(undefined);
  // What came of each archive's download, in its row.
  const [saved, setSaved] = useState<ReadonlyMap<string, Said>>(new Map());
  const count = useRef(0);
  // The status takes the focus once it is drawn with what a request came to: its button may be gone.
  const focusStatus = useRef(false);
  const statusId = `${id}-status`;
  useEffect(() => {
    if (!focusStatus.current) return;
    focusStatus.current = false;
    document.getElementById(statusId)?.focus();
  });
  const say = (text: string) => {
    count.current += 1;
    setSaid({ text, n: count.current });
    focusStatus.current = true;
  };
  const sayIn = (row: string, text: string) => {
    count.current += 1;
    const next = { text, n: count.current };
    setSaved((now) => new Map(now).set(row, next));
  };
  const heading = t("settings.account.export.archive");
  const about = t("settings.account.export.archive.about");
  if (status !== "success" || exports === undefined)
    return (
      <SettingsSection heading={heading} about={about}>
        {status === "error" ? (
          <SettingsEmpty
            action={
              <Button size="timeline" touch onPress={retry}>
                {t("settings.retry")}
              </Button>
            }
          >
            {t("settings.account.export.archive.failed_read")}
          </SettingsEmpty>
        ) : (
          <SettingsEmpty>{t("settings.account.export.archive.loading")}</SettingsEmpty>
        )}
      </SettingsSection>
    );

  const requestArchive = () => {
    if (requesting) return;
    setRequesting(true);
    request()
      .then(
        () => {
          say(t("settings.account.export.archive.requested"));
        },
        (error: unknown) => {
          if (exportBuilding(error)) {
            // Requested from elsewhere meanwhile: the list says so once it is read again.
            retry();
            say(t("settings.account.export.archive.building"));
          } else say(refusalDetail(error) ?? t("settings.account.export.archive.request_failed"));
        },
      )
      .finally(() => {
        setRequesting(false);
      });
  };
  const downloadArchive = (e: DataExport) => {
    if (downloading !== undefined) return;
    setDownloading(e.id);
    download(e.id)
      .then(
        ({ blob, name }) => {
          saveFile(blob, name);
          sayIn(e.id, t("settings.account.export.saved", { name }));
        },
        (error: unknown) => {
          sayIn(e.id, refusalDetail(error) ?? t("settings.account.export.archive.download_failed"));
        },
      )
      .finally(() => {
        setDownloading(undefined);
      });
  };

  const shown = shownExports(exports);
  const building = exports[0]?.state === "pending";
  // The screen's one primary action: Download while an archive is ready, else requesting one.
  const ready = shown.some((e) => archiveStateOf(e) === "ready");
  const ask = (
    <Button kind={ready ? "secondary" : "primary"} size="timeline" touch busy={requesting} onPress={requestArchive}>
      {t("settings.account.export.archive.request")}
    </Button>
  );
  return (
    <SettingsSection heading={heading} about={about} action={shown.length > 0 && !building ? ask : undefined}>
      {/* The status keeps its place from the start, with no room of its own until it says something. */}
      <Stack gap={said === undefined ? undefined : "screen-row-gap"}>
        <Status id={statusId} said={said} />
        {shown.length === 0 ? (
          <SettingsEmpty action={ask}>{t("settings.account.export.archive.none")}</SettingsEmpty>
        ) : (
          <SettingsList label={heading}>
            {shown.map((e) => {
              const state = archiveStateOf(e);
              return (
                <SettingsListRow
                  key={e.id}
                  title={t("settings.account.export.archive.row", { date: date(e.requested_at) })}
                  facts={state === "ready" && e.file_size !== undefined ? [sizeOf(e.file_size, lang)] : []}
                  standing={t("settings.account.export.archive.state", {
                    state,
                    date: e.expires_at === undefined ? "" : day(e.expires_at),
                  })}
                  said={state === "ready" ? (saved.get(e.id) ?? null) : undefined}
                  end={
                    state === "ready" && (
                      <Button
                        kind="primary"
                        size="timeline"
                        touch
                        busy={downloading === e.id}
                        onPress={() => {
                          downloadArchive(e);
                        }}
                      >
                        {t("settings.account.export.download")}
                      </Button>
                    )
                  }
                />
              );
            })}
          </SettingsList>
        )}
      </Stack>
    </SettingsSection>
  );
}

/** The lists as CSV files, a row each with its download. */
function CsvFiles({ t }: { t: T }) {
  const { csv } = useDataExports();
  const [busy, setBusy] = useState<CsvKind | undefined>(undefined);
  // What came of each file's download, in its row.
  const [said, setSaid] = useState<ReadonlyMap<CsvKind, Said>>(new Map());
  const count = useRef(0);
  const say = (kind: CsvKind, text: string) => {
    count.current += 1;
    const next = { text, n: count.current };
    setSaid((now) => new Map(now).set(kind, next));
  };
  const save = (kind: CsvKind) => {
    if (busy !== undefined) return;
    setBusy(kind);
    csv(kind)
      .then(
        ({ blob, name }: SavedFile) => {
          saveFile(blob, name);
          say(kind, t("settings.account.export.saved", { name }));
        },
        (error: unknown) => {
          say(kind, refusalDetail(error) ?? t("settings.account.export.csv.failed"));
        },
      )
      .finally(() => {
        setBusy(undefined);
      });
  };
  const heading = t("settings.account.export.csv");
  return (
    <SettingsSection heading={heading} about={t("settings.account.export.csv.about")}>
      <SettingsList label={heading}>
        {csvKinds.map((kind) => (
          <SettingsListRow
            key={kind}
            title={t("settings.account.csv.kind", { kind })}
            said={said.get(kind) ?? null}
            end={
              <Button
                size="timeline"
                touch
                busy={busy === kind}
                onPress={() => {
                  save(kind);
                }}
              >
                {t("settings.account.export.download")}
              </Button>
            }
          />
        ))}
      </SettingsList>
    </SettingsSection>
  );
}

/** The screen: the archive, then the CSV files; `onBack` goes back to the entry. */
export function AccountExportScreen({ onBack }: { onBack: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  return (
    <SettingsScreen
      sub
      title={t("settings.account.export")}
      back={<BackButton onPress={onBack}>{t("settings.entry.account")}</BackButton>}
    >
      <Archive t={t} />
      <CsvFiles t={t} />
    </SettingsScreen>
  );
}
