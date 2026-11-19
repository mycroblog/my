// SPDX-License-Identifier: AGPL-3.0-only

// The check before closing the server, and the closure, at /admin/closure/close (FR-809, ADR-0088): what closing
// would end, the deliveries of `Delete` it sends (every local account to every known inbox), the supporter
// subscriptions it cancels at once, and what should be dealt with first (account moves not ended, accounts under a
// retention hold), each a warning while it is not none; then, last and apart, Close the server, which cannot be
// undone and is confirmed by typing the server's domain. It is offered only once an announced closing date has
// passed, as the server refuses it before; once closed, the closure's screen shows the progress.

import {
  loadClosureCheck,
  closureStateRefused,
  useClosureCheck,
  useAdminClosureWrites,
  useInstance,
  type AdminClosure,
  type AppData,
  type ClosureCheck,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  ConfirmDialog,
  FactList,
  ModalPresence,
  SettingsNote,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
} from "@my/design-system";
import { useState, type ReactNode } from "react";
import { useMoment, useStaffStrings } from "../parts";
import { Irreversible, ScreenNotice, type AdminSubProps } from "../settings/parts";

/** The address of the closure. */
const adminClosurePath = "/admin/closure";

/** Reads the closure and what closing would end, for the route's loader. */
export const loadClosureClose = (data: AppData): Promise<void> => loadClosureCheck(data);

/** The way back to the closure. */
function BackToClosure({ go }: { go: (href: string) => void }) {
  const { t } = useStaffStrings();
  return (
    <BackButton
      onPress={() => {
        go(adminClosurePath);
      }}
    >
      {t("staff.entry.closure")}
    </BackButton>
  );
}

/** Why the server cannot be closed now, in a sentence; none once it can. */
function notYet(closure: AdminClosure, now: number): "none" | "early" | "closed" | undefined {
  if (closure.state === "none") return "none";
  if (closure.state !== "announced") return "closed";
  return closure.closes_at !== undefined && Date.parse(closure.closes_at) <= now ? undefined : "early";
}

/** The check and the closure, once both are read. */
function Close({
  closure,
  check,
  go,
}: {
  closure: AdminClosure;
  check: ClosureCheck;
  go: (href: string) => void;
}): ReactNode {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const writes = useAdminClosureWrites();
  const domain = useInstance()?.domain;
  const [now] = useState(() => Date.now());
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [said, setSaid] = useState<string | undefined>(undefined);
  const why = notYet(closure, now);
  const close = () => {
    if (domain === undefined) return;
    setBusy(true);
    setSaid(undefined);
    writes.close(domain).then(
      () => {
        setBusy(false);
        setAsking(false);
        go(adminClosurePath);
      },
      (error: unknown) => {
        setBusy(false);
        setAsking(false);
        setSaid(t(closureStateRefused(error) ? "staff_closure.close.refused" : "staff_settings.act_failed"));
      },
    );
  };
  return (
    <>
      <SettingsScreen sub title={t("staff_closure.close")} back={<BackToClosure go={go} />}>
        <SettingsSection heading={t("staff_closure.check.heading")}>
          <FactList
            label={t("staff_closure.check.heading")}
            items={[
              {
                term: t("staff_closure.check.deletes"),
                value: t("staff_closure.check.deletes.value", {
                  deletes: check.estimated_deletes,
                  accounts: check.local_accounts,
                  inboxes: check.delivery_inboxes,
                }),
              },
              {
                term: t("staff_closure.check.subscriptions"),
                value: t("staff_closure.check.count", { count: check.live_supporter_subscriptions }),
              },
              {
                term: t("staff_closure.check.migrations"),
                value: t("staff_closure.check.count", { count: check.pending_migrations }),
              },
              {
                term: t("staff_closure.check.holds"),
                value: t("staff_closure.check.count", { count: check.retention_holds }),
              },
            ]}
          />
          {check.pending_migrations > 0 && (
            <SettingsNote instead>{t("staff_closure.check.migrations.warning")}</SettingsNote>
          )}
          {check.retention_holds > 0 && <SettingsNote instead>{t("staff_closure.check.holds.warning")}</SettingsNote>}
        </SettingsSection>
        <SettingsSection heading={t("staff_closure.close")}>
          {why === undefined ? (
            <>
              <Irreversible>{t("staff_closure.irreversible")}</Irreversible>
              <Box>
                <Button
                  size="timeline"
                  touch
                  isDisabled={busy || domain === undefined}
                  onPress={() => {
                    setAsking(true);
                  }}
                >
                  {t("staff_closure.close.press")}
                </Button>
              </Box>
              <SettingsResult>{said}</SettingsResult>
            </>
          ) : (
            <SettingsNote instead>
              {t("staff_closure.close.not_yet", {
                why,
                date: closure.closes_at === undefined ? "" : moment(closure.closes_at),
              })}
            </SettingsNote>
          )}
        </SettingsSection>
      </SettingsScreen>
      <ModalPresence>
        {asking && domain !== undefined && (
          <ConfirmDialog
            key="close"
            title={t("staff_closure.close.title", { domain })}
            cancel={t("staff_settings.ask.cancel")}
            confirm={t("staff_closure.close.press")}
            phrase={domain}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={close}
          >
            {t("staff_closure.close.text", {
              date: closure.export_until === undefined ? "" : moment(closure.export_until),
              deletes: check.estimated_deletes,
            })}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </>
  );
}

/** The check before closing and the closure, or that they load or failed. */
export function ClosureCloseScreen({ go }: AdminSubProps): ReactNode {
  const { t } = useStaffStrings();
  const read = useClosureCheck();
  if (read.closure === undefined || read.check === undefined)
    return (
      <SettingsScreen
        sub
        title={t("staff_closure.close")}
        back={<BackToClosure go={go} />}
        notice={
          <ScreenNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{ loading: t("staff_closure.loading"), failed: t("staff_closure.failed") }}
          />
        }
      />
    );
  return <Close closure={read.closure} check={read.check} go={go} />;
}

/** The title of the check before closing. */
export function useClosureCloseTitle(): string {
  return useStaffStrings().t("staff_closure.close");
}
