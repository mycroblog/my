// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { AnnouncementDetail, adminAnnouncementQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/admin/announcements`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/** One announcement as the users see it, its state, and publishing, editing and withdrawing it (FR-726), for manage_announcements. */
export const Route = createFileRoute("/admin/announcements_/$id")({
  staticData: { title: "client:admin.announcements.detail.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_announcements", () =>
      context.queryClient.query({ ...adminAnnouncementQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: AnnouncementPage,
  errorComponent: AnnouncementError,
});

function AnnouncementHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.announcements.detail.title")}</PageHeading>
      <Link href="/admin/announcements" variant="standalone" size="sm">
        {t("client:admin.announcements.back")}
      </Link>
    </Stack>
  );
}

function AnnouncementPage() {
  const { id } = Route.useParams();
  return (
    <>
      <AnnouncementHeading />
      {/* Another announcement is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <AnnouncementDetail key={id} id={id} />
    </>
  );
}

function AnnouncementError(props: ErrorComponentProps) {
  const t = useT();
  return (
    <AdminError {...props} permission="manage_announcements" heading={t("client:admin.announcements.detail.title")} />
  );
}
