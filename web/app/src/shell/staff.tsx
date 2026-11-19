// SPDX-License-Identifier: AGPL-3.0-only

// The staff's screens in the app's shell (ADR-0058 §2): the screen of an entry of an area at /<area>/<entry>, under
// its name, which reads its address's query and changes it; a screen under an entry of the administration
// (/admin/rules/{id} and the like), held while it has changes not saved; the column beside a report's screen, the
// queue's page the reader came from, compact, from the frame's `wide`; and which report a screen at an address is.

import { viewerQuery, type AppData, type Instance, type Permission } from "@my/app-data";
import {
  ReportQueueBeside,
  StaffMissing,
  useStaffFacts,
  type AdminSubProps,
  StaffEntryScreen,
  staffHref,
  useStaffEntry,
  useStaffTitle,
  type StaffArea,
  type StaffSearch,
} from "@my/app-features";
import { FrameAside } from "@my/design-system";
import { useLocation, useRouter } from "@tanstack/react-router";
import type { ReactNode } from "react";
import type { RouterContext } from "../router";
import { screenLoad } from "../screen-load";
import { useUnsavedGuard } from "../unsaved-guard";
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
        go={(href) => {
          void router.navigate({ href });
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

/**
 * What the loader of a screen under an entry of the administration waits for: who the reader is, then, once their
 * permissions hold `permission`, what `load` reads.
 */
export function adminSubLoad(
  context: RouterContext,
  permission: Permission,
  load: (data: AppData) => Promise<void>,
): Promise<void> | undefined {
  return screenLoad(context, async (data) => {
    const viewer = await data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined);
    if (viewer?.permissions.includes(permission) === true) await load(data);
  });
}

type AdminSubPageProps = {
  /** The entry the screen is under, whose permission opens it. */
  readonly entry: string;
  /** The screen's heading, an `AdminSubHeading` that names what the screen shows; the entry's name without it. */
  readonly heading?: ReactNode;
  /** The feature's screen, given the way to go and the guard of its unsaved changes. */
  readonly children: (sub: AdminSubProps) => ReactNode;
};

/** The heading of a screen under the entry of `entry`: `title` once what it shows is read, the entry's name until then. */
export function AdminSubHeading({ entry, title }: { entry: string; title?: string | undefined }) {
  const t = useStrings();
  const entryTitle = useStaffTitle("admin", entry);
  return <ScreenHeading>{title ?? entryTitle ?? t("app.admin")}</ScreenHeading>;
}

/** The screen, held while it says it has changes not saved. */
function AdminSub({ entry, heading, children }: AdminSubPageProps) {
  const router = useRouter();
  const { onUnsaved, leaving } = useUnsavedGuard();
  return (
    <>
      {heading ?? <AdminSubHeading entry={entry} />}
      {children({
        go: (href) => {
          void router.navigate({ href });
        },
        onUnsaved,
        leaving,
      })}
    </>
  );
}

/** The screen, unless the reader's permissions turn out not to open its entry: then that the address names none. */
function AdminOpened(props: AdminSubPageProps) {
  const t = useStrings();
  const known = useStaffFacts().known;
  const open = useStaffEntry("admin", props.entry);
  if (known && open === undefined)
    return (
      <>
        <ScreenHeading>{t("app.admin")}</ScreenHeading>
        <StaffMissing area="admin" />
      </>
    );
  return <AdminSub {...props} />;
}

/** A screen under an entry of the administration at `returnTo`, for a signed-in reader; without a session, the ask to sign in. */
export function AdminSubPage({
  signedIn,
  returnTo,
  ...props
}: AdminSubPageProps & { readonly signedIn: boolean; readonly returnTo: string }) {
  const t = useStrings();
  if (signedIn) return <AdminOpened {...props} />;
  return (
    <>
      <ScreenHeading>{t("app.admin")}</ScreenHeading>
      <SignedOut returnTo={returnTo} />
    </>
  );
}
