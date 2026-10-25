// SPDX-License-Identifier: AGPL-3.0-only
// The notice of the server's closure (FR-808, FR-809, ADR-0088 §2), kept at the top of every screen that has the
// app's layout, the sign-in page and the server's information page among them, and not closable: while the closure is
// announced it says when the server closes, until when the data can still be exported afterwards and that the
// account can move, with the operators' message and the way to the export; after the closure it says so to a visitor
// who is not signed in and until when signing in still takes the data out. Nothing is shown when no closure is announced.
import { Link, NoticeBanner } from "@my/ui";
import { useIntl } from "react-intl";
import { hasClosed, isAnnounced, useClosure } from "../../../lib/closure";
import { useT } from "../../../lib/i18n";
import { useInstanceName } from "../../../lib/instanceName";
import { dataExportPath } from "../../../lib/paths";
import { useViewer } from "../../session";

/** The format of the dates of the notice. */
const dateFormat = { dateStyle: "long" } as const;

export function ClosureBanner() {
  const t = useT();
  const intl = useIntl();
  const instance = useInstanceName();
  const closure = useClosure();
  const signedIn = useViewer().status === "signed-in";
  if (isAnnounced(closure)) {
    return (
      <NoticeBanner
        label={t("client:closure.label")}
        headline={t("client:closure.announced.headline", { instance, date: intl.formatDate(closure.closes_at, dateFormat) })}
        actions={
          <Link href={dataExportPath} variant="standalone">
            {t("client:closure.announced.export")}
          </Link>
        }
      >
        <p>{t("client:closure.announced.export_until", { date: intl.formatDate(closure.export_until, dateFormat) })}</p>
        <p>{t("client:closure.announced.move")}</p>
        <OperatorMessage message={closure.message} />
      </NoticeBanner>
    );
  }
  // A signed-in user has the closed screen instead (ClosedScreen), except the staff in their own screens, who know.
  if (hasClosed(closure) && !signedIn) {
    return (
      <NoticeBanner label={t("client:closure.label")} headline={t("client:closure.closed.headline", { instance })}>
        <p>{t("client:closure.closed.sign_in", { until: intl.formatDate(closure.export_until, dateFormat) })}</p>
        <OperatorMessage message={closure.message} />
      </NoticeBanner>
    );
  }
  return null;
}

/**
 * The operators' own message, as plain text in the language they wrote it in (lang="" rather than the app's, in the
 * direction of its own text; web-client/README.md Y10), after a label that says whose it is.
 */
export function OperatorMessage({ message }: { message: string | undefined }) {
  const t = useT();
  if (message === undefined || message === "") return null;
  return (
    <p>
      <span>{t("client:closure.message_from")}</span> <span dir="auto" lang="">{message}</span>
    </p>
  );
}
