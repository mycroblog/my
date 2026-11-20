// SPDX-License-Identifier: AGPL-3.0-only

// The closure of the service, at /admin/closure (FR-808, FR-809, ADR-0088): where it stands and what can be done now.
// With none announced, the way to announce it. Announced, its state, dates and the operator's message, a warning where
// the notice is short, then changing the dates and withdrawing it (asked first), and the way to check and close the
// server, offered once the closing date has passed: from then on it stands first, the state saying the date passed,
// since closing is the next step. Closed, its dates and message, then how far the `Delete` of every local account on
// the other servers has gone (the deliveries done of all, and apart those that failed), read again every few seconds
// until every delivery has ended.

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
  LinkButton,
  Meter,
  ModalPresence,
  Row,
  SettingsNote,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  StateMark,
  type Fact,
} from "@my/design-system";
import { useState, type ReactNode } from "react";
import { Titled, useStaffStrings, type T } from "../parts";
import { Caution, ScreenNotice } from "../settings/parts";
import { useClosureMoment, usePassed } from "./passed";

/** The address of the announcement's form. */
const announcementPath = "/admin/closure/announcement";
/** The address of the check and the closure. */
const closePath = "/admin/closure/close";

/** Reads the closure, for the route's loader. */
export const loadClosure = (data: AppData): Promise<void> => loadAdminClosure(data);

/**
 * The facts of a closure: where an announced one stands (its closing date passed, `due`) and when it was announced,
 * or when a closed one closed; its dates; and the operator's message.
 */
function closureFacts(t: T, moment: (iso: string) => string, closure: AdminClosure, due: boolean): Fact[] {
  const closed = closureClosed(closure.state);
  return [
    ...(closed
      ? []
      : [
          {
            term: t("staff_closure.fact.state"),
            value: (
              <StateMark state={due ? "pending" : "active"}>
                {t("staff_closure.state", { state: due ? "due" : closure.state })}
              </StateMark>
            ),
          },
        ]),
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

/** How far the closure's deliveries have gone: those done of all, the accounts and the work left, then those that failed. */
function Progress({ closure }: { closure: AdminClosure }) {
  const { t } = useStaffStrings();
  const progress = closure.progress;
  const done = closure.state === "delivery_completed";
  if (progress === undefined)
    return (
      <SettingsSection heading={t("staff_closure.progress")}>
        <SettingsNote instead>{t(done ? "staff_closure.progress.done" : "staff_closure.progress.going")}</SettingsNote>
      </SettingsSection>
    );
  const { succeeded, pending, permanent_failure: refused, skipped, dead } = progress.deliveries;
  const all = succeeded + pending + refused + skipped + dead;
  const count = (n: number) => t("staff_closure.check.count", { count: n });
  return (
    <SettingsSection heading={t("staff_closure.progress")}>
      <Meter
        value={all === 0 ? 0 : succeeded / all}
        label={t("staff_closure.progress.meter")}
        caption={t("staff_closure.progress.delivered", { succeeded, all })}
      />
      <FactList
        label={t("staff_closure.progress")}
        items={[
          { term: t("staff_closure.progress.accounts"), value: count(progress.accounts) },
          { term: t("staff_closure.progress.pending"), value: count(pending) },
          { term: t("staff_closure.progress.fanouts"), value: count(progress.fanouts_pending) },
        ]}
      />
      <Titled title={t("staff_closure.progress.undelivered")}>
        <FactList
          label={t("staff_closure.progress.undelivered")}
          items={[
            { term: t("staff_closure.progress.refused"), value: count(refused) },
            { term: t("staff_closure.progress.skipped"), value: count(skipped) },
            { term: t("staff_closure.progress.dead"), value: count(dead) },
          ]}
        />
      </Titled>
      <SettingsNote instead>{t(done ? "staff_closure.progress.done" : "staff_closure.progress.going")}</SettingsNote>
    </SettingsSection>
  );
}

/** An announced closure: its facts and actions, and the way to close the server, first once its date has passed. */
function Announced({ closure }: { closure: AdminClosure }) {
  const { t } = useStaffStrings();
  const moment = useClosureMoment();
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
  const close = (
    <SettingsSection key="close" heading={t("staff_closure.close")}>
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
  );
  const announcement = (
    <SettingsSection key="announcement" heading={t("staff_closure.announcement")}>
      <FactList label={t("staff_closure.announcement")} items={closureFacts(t, moment, closure, due)} />
      {closure.short_notice && !due && <Caution>{t("staff_closure.short_notice")}</Caution>}
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
  );
  return (
    <>
      {due ? [close, announcement] : [announcement, close]}
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
  const moment = useClosureMoment();
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
            <FactList label={t("staff_closure.closed")} items={closureFacts(t, moment, closure, true)} />
          </SettingsSection>
          <Progress closure={closure} />
        </>
      )}
    </SettingsScreen>
  );
}
