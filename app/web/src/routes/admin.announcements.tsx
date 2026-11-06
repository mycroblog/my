// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { type AnnouncementFilter, AnnouncementList, adminAnnouncementsQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** `state`: the list; omitted, or anything unknown, lists every announcement (web-client/README.md S3). */
const searchSchema = z.object({
  state: z.enum(["draft", "scheduled", "showing", "ended", "withdrawn"]).optional().catch(undefined),
});

/** The server's announcements to every local user (FR-726), newest first, a page at a time, for manage_announcements. */
export const Route = createFileRoute("/admin/announcements")({
  staticData: { title: "client:admin.announcements.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search: { state } }): { filter: AnnouncementFilter } => ({ filter: state ?? "all" }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps: { filter } }) =>
    loadWithPermission(context, "manage_announcements", () =>
      context.queryClient.infiniteQuery({
        ...adminAnnouncementsQueryOptions(context.api, filter),
        staleTime: Infinity,
      }),
    ),
  component: AnnouncementsPage,
  errorComponent: AnnouncementsError,
});

function AnnouncementsPage() {
  const t = useT();
  const { filter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.announcements.title")}</PageHeading>
      <AnnouncementList
        filter={filter}
        onFilterChange={(next) => {
          void navigate({ search: { state: next === "all" ? undefined : next } });
        }}
      />
    </>
  );
}

function AnnouncementsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_announcements" heading={t("client:admin.announcements.title")} />;
}
