// SPDX-License-Identifier: AGPL-3.0-only

// What the server says across every screen (ADR-0088 §2, §15, §20, ADR-0028 §11, §12): at the top of the main column,
// the closure while it is announced, which the reader cannot dismiss, then the reader offline or the server down, with
// the status page kept from the last answer; and in the dock, the toast of a newer version of the app, or of one the
// server needs, never while the reader is in a call.

import { useServerActions, useServerState, useStatusPageUrl, type Instance } from "@my/app-data";
import { FrameBanner, FrameBanners, Toast, useLocale } from "@my/design-system";
import { useStrings } from "../locale";

/** The notices at the top of the main column, from the top: the closure, then the reader offline or the server down. */
export function ServerBanners({ instance }: { instance: Instance | undefined }) {
  const t = useStrings();
  const { locale } = useLocale();
  const { connection } = useServerState();
  const statusPage = useStatusPageUrl();
  const closure = instance?.closure?.state === "announced" ? instance.closure : undefined;
  if (closure === undefined && connection === "online") return null;
  const date = (iso: string) => new Intl.DateTimeFormat(locale, { dateStyle: "long" }).format(new Date(iso));
  const status =
    statusPage === undefined ? [] : [{ href: statusPage, label: t("app.status_page"), newTab: t("app.new_tab") }];
  return (
    <FrameBanners label={t("app.notices")}>
      {closure !== undefined && (
        <FrameBanner
          icon="caution"
          strong
          links={[
            { href: "/settings/export", label: t("app.closure_export") },
            { href: "/settings/move", label: t("app.closure_move") },
          ]}
        >
          {t("app.closure_notice", { closes: date(closure.closes_at), until: date(closure.export_until) })}
        </FrameBanner>
      )}
      {connection === "offline" && (
        <FrameBanner icon="offline" links={status}>
          {t("app.offline")}
        </FrameBanner>
      )}
      {connection === "outage" && (
        <FrameBanner icon="error" strong links={status}>
          {t("app.outage")}
        </FrameBanner>
      )}
    </FrameBanners>
  );
}

/**
 * The toast of an update: a newer version ready, which the reader may close until the app loads again, or one the
 * server needs, which stays. Update loads the app again. None while the reader is in a call (`inCall`).
 */
export function UpdateToast({ inCall }: { inCall: boolean }) {
  const t = useStrings();
  const { update } = useServerState();
  const { dismissUpdate } = useServerActions();
  if (inCall || update === "none") return null;
  const reload = () => {
    location.reload();
  };
  return update === "ready" ? (
    <Toast action={t("app.update")} onAction={reload} closeLabel={t("app.toast_close")} onClose={dismissUpdate}>
      {t("app.update_ready")}
    </Toast>
  ) : (
    <Toast action={t("app.update")} onAction={reload}>
      {t("app.update_required")}
    </Toast>
  );
}
