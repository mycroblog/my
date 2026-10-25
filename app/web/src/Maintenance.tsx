// SPDX-License-Identifier: AGPL-3.0-only
// The maintenance of the server in place of the whole app (ADR-0084 §3, web-client/09-app-shell.md §9 "メンテナンス"):
// while the server answers with its maintenance problem (lib/maintenance.ts), the shell shows what it says (the
// administrators' notice, when the maintenance is expected to end, the status page) instead of the router, tries the
// server again after Retry-After, and loads the page again once the server answers. "Reload" asks at once.
import { Button, ExternalLink, Heading, StandalonePage, Text, useAnnouncer } from "@my/ui";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { useApi } from "./lib/auth/SessionProvider";
import { useConfig } from "./lib/config";
import { errorMessage, MaintenanceError } from "./lib/errors";
import { useT } from "./lib/i18n";
import { checkMaintenance, type Maintenance, useMaintenance } from "./lib/maintenance";

/** How long to wait before asking again when the server named no time (no Retry-After, or one already past). */
const fallbackRetryMs = 30_000;

export type MaintenanceGateProps = {
  maintenance: Maintenance;
  /** The app, shown while the server is not under maintenance. */
  children: ReactNode;
};

/** Shows the app, or the maintenance in its place while the server is under maintenance. */
export function MaintenanceGate({ maintenance, children }: MaintenanceGateProps) {
  const current = useMaintenance(maintenance);
  return current === null ? children : <MaintenanceScreen error={current} />;
}

function MaintenanceScreen({ error }: { error: MaintenanceError }) {
  const t = useT();
  const intl = useIntl();
  const api = useApi();
  const announce = useAnnouncer();
  const { instanceName } = useConfig();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const [checking, setChecking] = useState(false);
  /** How many times the screen has asked the server; each answer without a new time sets the next try again. */
  const [tries, setTries] = useState(0);
  const title = t("client:maintenance.title");

  // The screen replaces the page that had focus: focus moves to its heading, so that it is read out (Y5).
  useEffect(() => {
    headingRef.current?.focus({ preventScroll: true });
  }, []);

  useEffect(() => {
    document.title = t("client:shell.document_title", { page: title, instance: instanceName });
  }, [t, title, instanceName]);

  // An Effect, because the reason to ask again is the time the server named, not something the user did. A new
  // maintenance problem brings a new time; an answer without one waits the fallback.
  const { retryAt } = error;
  useEffect(() => {
    const wait = retryAt !== undefined && retryAt > Date.now() ? retryAt - Date.now() : fallbackRetryMs;
    const timer = setTimeout(() => {
      void checkMaintenance(api).then(() => {
        setTries((count) => count + 1);
      });
    }, wait);
    return () => {
      clearTimeout(timer);
    };
  }, [api, retryAt, tries]);

  const reload = async () => {
    setChecking(true);
    const failure = await checkMaintenance(api);
    setChecking(false);
    setTries((count) => count + 1);
    // Without a failure the server has answered, and the page is loading again.
    if (failure !== null) announce(failure instanceof MaintenanceError ? t("client:maintenance.still") : errorMessage(failure));
  };

  return (
    <StandalonePage>
      <Heading level={1} focusable ref={headingRef}>
        {title}
      </Heading>
      <Text as="p">{t("client:maintenance.body")}</Text>
      {error.notice !== undefined && (
        // The administrators' notice is one text in whatever language they wrote it in (Y10).
        <Text as="p" dir="auto" lang="" align="matchParent" whitespace="preLine" wrap="anywhere">
          {error.notice}
        </Text>
      )}
      {error.until !== undefined && (
        <Text as="p">{t("client:maintenance.until", { time: intl.formatDate(error.until, { dateStyle: "full", timeStyle: "short" }) })}</Text>
      )}
      {error.statusPageUrl !== undefined && (
        <p>
          <ExternalLink href={error.statusPageUrl}>{t("client:maintenance.status_page")}</ExternalLink>
        </p>
      )}
      <div>
        <Button variant="primary" isPending={checking} onPress={() => void reload()}>
          {t("client:maintenance.reload")}
        </Button>
      </div>
    </StandalonePage>
  );
}
