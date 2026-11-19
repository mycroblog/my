// SPDX-License-Identifier: AGPL-3.0-only

// The staff's screens in the app's shell (ADR-0058 §2): the screen of an entry of an area at /<area>/<entry>, under
// its name, which reads its address's query and changes it; the column beside a report's screen, the queue's page the
// reader came from, compact, from the frame's `wide`; and which report a screen at an address is.

import type { Instance } from "@my/app-data";
import {
  ReportQueueBeside,
  StaffEntryScreen,
  staffHref,
  useStaffEntry,
  useStaffTitle,
  type StaffArea,
  type StaffSearch,
} from "@my/app-features";
import { FrameAside } from "@my/design-system";
import { useLocation, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { Around, ServerFooter } from "./around";
import { ScreenHeading } from "./heading";
import { SignedOut } from "./signed-out";

/** The query of an address as its members, each the last of its name. */
export function searchOfQuery(query: string): StaffSearch {
  return Object.fromEntries(new URLSearchParams(query));
}

/** The report the screen at `pathname` is (/moderation/reports/{id}); undefined elsewhere. */
export const reportAt = (pathname: string): string | undefined => {
  const id = /^\/moderation\/reports\/([^/]+)$/.exec(pathname)?.[1];
  return id === undefined ? undefined : decodeURIComponent(id);
};

function Entry({ area, entry }: { area: StaffArea; entry: string }) {
  const t = useStrings();
  const router = useRouter();
  const pathname = useLocation({ select: (l) => l.pathname });
  const query = useLocation({ select: (l) => l.searchStr });
  const title = useStaffTitle(area, entry);
  return (
    <>
      <ScreenHeading>{title ?? t(area === "moderation" ? "app.moderation" : "app.admin")}</ScreenHeading>
      <StaffEntryScreen
        area={area}
        entry={entry}
        search={searchOfQuery(query)}
        onSearch={(search) => {
          void router.navigate({ href: staffHref(pathname, search) });
        }}
      />
    </>
  );
}

/** The screen of the entry of `area` at `entry` for a signed-in reader; without a session, the ask to sign in. */
export function StaffEntryPage({ area, entry, signedIn }: { area: StaffArea; entry: string; signedIn: boolean }) {
  const t = useStrings();
  if (signedIn) return <Entry key={`${area}/${entry}`} area={area} entry={entry} />;
  return (
    <>
      <ScreenHeading>{t(area === "moderation" ? "app.moderation" : "app.admin")}</ScreenHeading>
      <SignedOut returnTo={`/${area}/${entry}`} />
    </>
  );
}

/**
 * The column beside the report of `id`: the queue's page the reader came from, the report marked, and the server's
 * pages; what is around the reader, as elsewhere, for a reader the queue is not open to.
 */
export function ReportAside({
  id,
  pathname,
  instance,
}: {
  id: string;
  pathname: string;
  instance: Instance | undefined;
}) {
  const t = useStrings();
  const query = useLocation({ select: (l) => l.searchStr });
  if (useStaffEntry("moderation", "reports") === undefined)
    return <Around signedIn pathname={pathname} instance={instance} />;
  return (
    <FrameAside label={t("app.report_queue")}>
      <ReportQueueBeside search={searchOfQuery(query)} current={id} />
      <ServerFooter instance={instance} />
    </FrameAside>
  );
}
