// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { RelayDetail, relaysQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/admin/relays`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * One federation relay (ADR-0006): its facts and the last 24 hours, leaving it, joining it again and deleting it, for
 * manage_settings. The API has no endpoint for one relay, so the page reads it from the list.
 */
export const Route = createFileRoute("/admin/relays_/$id")({
  staticData: { title: "client:admin.relays.detail.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_settings", () =>
      context.queryClient.query({ ...relaysQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: RelayPage,
  errorComponent: RelayError,
});

function RelayHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.relays.detail.title")}</PageHeading>
      <Link href="/admin/relays" variant="standalone" size="sm">
        {t("client:admin.relays.back")}
      </Link>
    </Stack>
  );
}

function RelayPage() {
  const { id } = Route.useParams();
  return (
    <>
      <RelayHeading />
      {/* Another relay is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <RelayDetail key={id} id={id} />
    </>
  );
}

function RelayError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_settings" heading={t("client:admin.relays.detail.title")} />;
}
