// SPDX-License-Identifier: AGPL-3.0-only

// A report as a moderator opens it, at /moderation/reports/{id} (ADR-0058 §2), under the queue of reports: its query
// is the queue's the reader came from (its filters and page), which its way back and the column beside it keep. A
// reader whose permissions do not open the queue reads that the address names no screen open to them, as the queue's
// own address says. Without a session it asks the reader to sign in.

import { loadReport, viewerQuery } from "@my/app-data";
import { reportQueuePath } from "@my/app-components";
import { ReportScreen, StaffMissing, staffHref, useReportTitle, useStaffEntry, useStaffFacts } from "@my/app-features";
import { createFileRoute, useLocation, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";
import { searchOfQuery } from "../shell/staff";

export const Route = createFileRoute("/moderation/reports/$id")({
  loader: ({ context, params }) =>
    screenLoad(context, async (data) => {
      const viewer = await data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined);
      if (viewer?.permissions.includes("manage_reports") === true) await loadReport(data, params.id);
    }),
  component: ReportPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Report({ id }: { id: string }) {
  const t = useStrings();
  const router = useRouter();
  const search = searchOfQuery(useLocation({ select: (l) => l.searchStr }));
  const title = useReportTitle(id);
  return (
    <>
      <ScreenHeading>{title ?? t("app.report")}</ScreenHeading>
      <ReportScreen
        key={id}
        id={id}
        search={search}
        onBack={() => {
          void router.navigate({ href: staffHref(reportQueuePath, search) });
        }}
      />
    </>
  );
}

/** The report, unless the reader's permissions turn out not to open the queue: then that the address names no screen open to them. */
function Opened({ id }: { id: string }) {
  const t = useStrings();
  const known = useStaffFacts().known;
  const queue = useStaffEntry("moderation", "reports");
  if (known && queue === undefined)
    return (
      <>
        <ScreenHeading>{t("app.moderation")}</ScreenHeading>
        <StaffMissing area="moderation" />
      </>
    );
  return <Report id={id} />;
}

function ReportPage() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const t = useStrings();
  if (data !== null) return <Opened id={id} />;
  return (
    <>
      <ScreenHeading>{t("app.report")}</ScreenHeading>
      <SignedOut returnTo={`/moderation/reports/${id}`} />
    </>
  );
}
