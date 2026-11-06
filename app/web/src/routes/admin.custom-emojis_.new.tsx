// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { NewCustomEmoji } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * Registering a custom emoji of this server (ADR-0051), for manage_custom_emojis; once registered, its page opens. The
 * `_` keeps the page out of the list's layout at `/admin/custom-emojis`.
 */
export const Route = createFileRoute("/admin/custom-emojis_/new")({
  staticData: { title: "client:admin.custom_emojis.new.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) => loadWithPermission(context, "manage_custom_emojis", () => Promise.resolve()),
  component: NewCustomEmojiPage,
  errorComponent: NewCustomEmojiError,
});

function NewCustomEmojiPage() {
  const t = useT();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>{t("client:admin.custom_emojis.new.title")}</PageHeading>
        <Link href="/admin/custom-emojis" variant="standalone" size="sm">
          {t("client:admin.custom_emojis.back")}
        </Link>
      </Stack>
      <NewCustomEmoji />
    </>
  );
}

function NewCustomEmojiError(props: ErrorComponentProps) {
  const t = useT();
  return (
    <AdminError {...props} permission="manage_custom_emojis" heading={t("client:admin.custom_emojis.new.title")} />
  );
}
