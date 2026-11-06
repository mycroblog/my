// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { NewAnnouncement } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * Writing an announcement to every local user (FR-726), as a draft or published at once, for manage_announcements;
 * writing it opens its page. The `_` keeps the page out of the list's layout at `/admin/announcements`.
 */
export const Route = createFileRoute("/admin/announcements_/new")({
  staticData: { title: "client:admin.announcements.new.title" },
  beforeLoad: requireSignIn,
  component: NewAnnouncementPage,
  errorComponent: NewAnnouncementError,
});

function NewAnnouncementHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.announcements.new.title")}</PageHeading>
      <Link href="/admin/announcements" variant="standalone" size="sm">
        {t("client:admin.announcements.back")}
      </Link>
    </Stack>
  );
}

function NewAnnouncementPage() {
  return (
    <>
      <NewAnnouncementHeading />
      <NewAnnouncement />
    </>
  );
}

function NewAnnouncementError(props: ErrorComponentProps) {
  const t = useT();
  return (
    <AdminError {...props} permission="manage_announcements" heading={t("client:admin.announcements.new.title")} />
  );
}
