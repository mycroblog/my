// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { adminCustomEmojiQueryOptions, CustomEmojiDetail } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/admin/custom-emojis`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * One custom emoji (ADR-0051), local or remote: its facts, and changing, replacing the image of, enabling, disabling,
 * deleting or copying it, for manage_custom_emojis.
 */
export const Route = createFileRoute("/admin/custom-emojis_/$id")({
  staticData: { title: "client:admin.custom_emojis.detail.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_custom_emojis", () =>
      context.queryClient.query({ ...adminCustomEmojiQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: CustomEmojiPage,
  errorComponent: CustomEmojiError,
});

function CustomEmojiHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.custom_emojis.detail.title")}</PageHeading>
      <Link href="/admin/custom-emojis" variant="standalone" size="sm">
        {t("client:admin.custom_emojis.back")}
      </Link>
    </Stack>
  );
}

function CustomEmojiPage() {
  const { id } = Route.useParams();
  return (
    <>
      <CustomEmojiHeading />
      {/* Another emoji is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <CustomEmojiDetail key={id} id={id} />
    </>
  );
}

function CustomEmojiError(props: ErrorComponentProps) {
  const t = useT();
  return (
    <AdminError {...props} permission="manage_custom_emojis" heading={t("client:admin.custom_emojis.detail.title")} />
  );
}
