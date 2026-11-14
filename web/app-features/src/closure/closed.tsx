// SPDX-License-Identifier: AGPL-3.0-only

// The screen of a closed server (ADR-0088 §2), in place of every other for a signed-in reader: that it closed, until
// when the reader can still take their data, the operator's words, and the ways to take it: the archive of everything,
// requested here, built in the background and downloaded here once ready (the latest export's state says which, and
// the request waits while one is being built), and the CSV of whom they follow, which another server imports. Signing
// out ends it.

import { useDataExports, type Closure, type DataExport } from "@my/app-data";
import {
  Button,
  FormActions,
  FormHead,
  FormLink,
  FormLinks,
  FormPage,
  FormText,
  useLocale,
  type FormPageServer,
} from "@my/design-system";
import { useEffect, useState, type ReactNode } from "react";
import { stringsIn } from "../locale";

/** Where the server's sign-out page is, which ends the session. */
const signOutPath = "/auth/sign_out";

/** What last failed of what the reader pressed: the request of the archive, its download, or the CSV's. */
type Failed = "request" | "download" | "csv" | null;

/** Saves `blob` as a file named `name`, as a download of the page. */
function save(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  link.click();
  URL.revokeObjectURL(url);
}

export type ClosedScreenProps = {
  server: FormPageServer;
  /** The built-in wordmark, for light and dark. */
  wordmark: { light: string; dark: string };
  closure: Closure;
  /** The page's footer (FormPageFooter). */
  footer?: ReactNode;
};

export function ClosedScreen({ server, wordmark, closure, footer }: ClosedScreenProps) {
  const { t, lang } = stringsIn(useLocale().locale);
  const { exports, request, download, csv } = useDataExports();
  const [sending, setSending] = useState(false);
  const [failed, setFailed] = useState<Failed>(null);
  const date = (iso: string) => new Intl.DateTimeFormat(lang, { dateStyle: "long" }).format(new Date(iso));
  const title = t("closure.closed_title", { name: server.name });
  useEffect(() => {
    document.title = title;
  }, [title]);
  // The latest export says where the archive is: being built, ready, or failed.
  const latest: DataExport | undefined = exports?.[0];
  const building = latest?.state === "pending";
  const ready = latest?.state === "ready" ? latest : undefined;
  const requestArchive = () => {
    setSending(true);
    setFailed(null);
    request().then(
      () => {
        setSending(false);
      },
      () => {
        setSending(false);
        setFailed("request");
      },
    );
  };
  const saveFile = (what: "download" | "csv", get: () => Promise<{ blob: Blob; name: string }>) => () => {
    setFailed(null);
    get().then(
      ({ blob, name }) => {
        save(blob, name);
      },
      () => {
        setFailed(what);
      },
    );
  };
  return (
    <FormPage server={server} wordmark={wordmark} footer={footer}>
      <FormHead title={title}>
        <FormText>{t("closure.closed_export", { date: date(closure.export_until) })}</FormText>
      </FormHead>
      {closure.message !== undefined && <FormText>{closure.message}</FormText>}
      {building && <FormText role="status">{t("closure.export_requested")}</FormText>}
      {ready !== undefined && (
        <FormText role="status">
          {t("closure.export_ready", { date: date(ready.expires_at ?? closure.export_until) })}
        </FormText>
      )}
      {latest?.state === "failed" && <FormText role="status">{t("closure.export_build_failed")}</FormText>}
      <FormActions>
        {ready !== undefined && (
          <Button kind="primary" onPress={saveFile("download", () => download(ready.id))}>
            {t("closure.download_export")}
          </Button>
        )}
        <Button
          kind={ready === undefined ? "primary" : "secondary"}
          onPress={requestArchive}
          busy={sending}
          isDisabled={building || exports === undefined}
        >
          {t("closure.request_export")}
        </Button>
        <Button onPress={saveFile("csv", () => csv("following"))}>{t("closure.download_follows")}</Button>
      </FormActions>
      {failed === "request" && <FormText role="status">{t("closure.export_failed")}</FormText>}
      {failed === "download" && <FormText role="status">{t("closure.export_download_failed")}</FormText>}
      {failed === "csv" && <FormText role="status">{t("closure.download_failed")}</FormText>}
      <FormLinks>
        <FormLink href={signOutPath}>{t("closure.sign_out")}</FormLink>
      </FormLinks>
    </FormPage>
  );
}
