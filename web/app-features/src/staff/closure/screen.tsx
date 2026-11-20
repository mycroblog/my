// SPDX-License-Identifier: AGPL-3.0-only

// The closure of the service, at /admin/closure (FR-808, FR-809, ADR-0088): where it stands and what can be done now.
// With none announced, the way to announce it; announced, its dates and the operator's message, a warning where the
// notice is short, then changing the dates and withdrawing it (asked first), and, last and apart, the way to check
// and close the server once the closing date has passed. Closed, what remains (the users sign in and export their
// data until the deadline, and nothing else) and the progress of the `Delete` of every local account on the other
// servers, read again every few seconds until every delivery has ended.

import {
  closureClosed,
  loadAdminClosure,
  useAdminClosure,
  useAdminClosureWrites,
  type AdminClosure,
  type AppData,
} from "@my/app-data";
import {
  Box,
  Button,
  ConfirmDialog,
  FactList,
  FigureTiles,
  LinkButton,
  ModalPresence,
  Row,
  SettingsNote,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  type Fact,
} from "@my/design-system";
import { useState, type ReactNode } from "react";
import { useMoment, useStaffStrings, type T } from "../parts";
import { ScreenNotice } from "../settings/parts";
import { usePassed } from "./passed";

/** The address of the announcement's form. */
const announcementPath = "/admin/closure/announcement";
/** The address of the check and the closure. */
const closePath = "/admin/closure/close";

/** Reads the closure, for the route's loader. */
export const loadClosure = (data: AppData): Promise<void> => loadAdminClosure(data);

/** The facts of an announced or closed closure: where it stands, its dates, and the operator's message. */
function closureFacts(t: T, moment: (iso: string) => string, closure: AdminClosure): Fact[] {
  const closed = closureClosed(closure.state);
  return [
    { term: t("staff_closure.fact.state"), value: t("staff_closure.state", { state: closure.state }) },
    ...(closure.closes_at === undefined
      ? []
      : [
          {
            term: t(closed ? "staff_closure.fact.closed" : "staff_closure.fact.closes"),
            value: moment(closed && closure.changed_at !== undefined ? closure.changed_at : closure.closes_at),
          },
        ]),
    ...(closure.export_until === undefined
      ? []
      : [{ term: t("staff_closure.fact.export_until"), value: moment(closure.export_until) }]),
    ...(closed || closure.announced_at === undefined
      ? []
      : [{ term: t("staff_closure.fact.announced"), value: moment(closure.announced_at) }]),
    ...(closure.message === undefined ? [] : [{ term: t("staff_closure.fact.message"), value: closure.message }]),
  ];
}

/** The progress of the closure's deliveries, each count a figure. */
function Progress({ closure }: { closure: AdminClosure }) {
  const { t, locale } = useStaffStrings();
  const progress = closure.progress;
  const n = (count: number) => new Intl.NumberFormat(locale).format(count);
  return (
    <SettingsSection heading={t("staff_closure.progress")}>
      {progress !== undefined && (
        <FigureTiles
          label={t("staff_closure.progress")}
          items={[
            { figure: n(progress.accounts), label: t("staff_closure.progress.accounts") },
            { figure: n(progress.fanouts_pending), label: t("staff_closure.progress.fanouts") },
            { figure: n(progress.deliveries.succeeded), label: t("staff_closure.progress.succeeded") },
            { figure: n(progress.deliveries.pending), label: t("staff_closure.progress.pending") },
            { figure: n(progress.deliveries.permanent_failure), label: t("staff_closure.progress.refused") },
            { figure: n(progress.deliveries.skipped), label: t("staff_closure.progress.skipped") },
            { figure: n(progress.deliveries.dead), label: t("staff_closure.progress.dead") },
          ]}
        />
      )}
      <SettingsNote instead>
        {t(closure.state === "delivery_completed" ? "staff_closure.progress.done" : "staff_closure.progress.going")}
      </SettingsNote>
    </SettingsSection>
  );
}

/** An announced closure: its facts and actions, then the way to close the server. */
function Announced({ closure }: { closure: AdminClosure }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const writes = useAdminClosureWrites();
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [said, setSaid] = useState<string | undefined>(undefined);
  const due = usePassed(closure.closes_at);
  const withdraw = () => {
    setBusy(true);
    setSaid(undefined);
    writes.withdraw().then(
      () => {
        setBusy(false);
        setAsking(false);
      },
      () => {
        setBusy(false);
        setAsking(false);
        setSaid(t("staff_settings.act_failed"));
      },
    );
  };
  return (
    <>
      <SettingsSection heading={t("staff_closure.announcement")}>
        <FactList label={t("staff_closure.announcement")} items={closureFacts(t, moment, closure)} />
        {closure.short_notice && <SettingsNote instead>{t("staff_closure.short_notice")}</SettingsNote>}
        <Row wrap gap="entry-caption">
          <LinkButton size="timeline" href={announcementPath}>
            {t("staff_closure.change")}
          </LinkButton>
          <Button
            size="timeline"
            touch
            isDisabled={busy}
            onPress={() => {
              setAsking(true);
            }}
          >
            {t("staff_closure.withdraw")}
          </Button>
        </Row>
        <SettingsResult>{said}</SettingsResult>
      </SettingsSection>
      <SettingsSection heading={t("staff_closure.close")}>
        {due ? (
          <Box>
            <LinkButton size="timeline" href={closePath}>
              {t("staff_closure.check")}
            </LinkButton>
          </Box>
        ) : (
          <SettingsNote instead>
            {t("staff_closure.close.wait", { date: closure.closes_at === undefined ? "" : moment(closure.closes_at) })}
          </SettingsNote>
        )}
      </SettingsSection>
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            key="withdraw"
            title={t("staff_closure.withdraw.title")}
            cancel={t("staff_settings.ask.cancel")}
            confirm={t("staff_closure.withdraw")}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={withdraw}
          >
            {t("staff_closure.withdraw.text")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </>
  );
}

/** The closure of the service: where it stands and what can be done now, or that it loads or failed. */
export function AdminClosureScreen(): ReactNode {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const read = useAdminClosure();
  const title = t("staff.entry.closure");
  const closure = read.closure;
  if (closure === undefined)
    return (
      <SettingsScreen
        sub
        icon="closure"
        title={title}
        notice={
          <ScreenNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{ loading: t("staff_closure.loading"), failed: t("staff_closure.failed") }}
          />
        }
      />
    );
  return (
    <SettingsScreen sub icon="closure" title={title}>
      {closure.state === "none" ? (
        <SettingsSection heading={t("staff_closure.announcement")}>
          <SettingsNote instead>{t("staff_closure.none")}</SettingsNote>
          <Box>
            <LinkButton kind="primary" size="timeline" href={announcementPath}>
              {t("staff_closure.announce")}
            </LinkButton>
          </Box>
        </SettingsSection>
      ) : closure.state === "announced" ? (
        <Announced closure={closure} />
      ) : (
        <>
          <SettingsSection heading={t("staff_closure.closed")}>
            <FactList label={t("staff_closure.closed")} items={closureFacts(t, moment, closure)} />
            <SettingsNote instead>{t("staff_closure.closed.remains")}</SettingsNote>
          </SettingsSection>
          <Progress closure={closure} />
        </>
      )}
    </SettingsScreen>
  );
}
