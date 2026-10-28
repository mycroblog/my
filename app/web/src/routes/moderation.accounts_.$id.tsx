// SPDX-License-Identifier: AGPL-3.0-only
import { Card, Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { AccountDetailView, adminAccountQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the search's layout at `/moderation/accounts`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/** One account as the moderators see it, and the moderation actions on it (FR-714, FR-703, FR-713), for manage_accounts. */
export const Route = createFileRoute("/moderation/accounts_/$id")({
  staticData: { title: "client:admin.account.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_accounts", () =>
      context.queryClient.query({ ...adminAccountQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: AccountPage,
  errorComponent: AccountError,
});

function AccountHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.account.title")}</PageHeading>
      <Link href="/moderation/accounts" variant="standalone" size="sm">
        {t("client:admin.account.back")}
      </Link>
    </Stack>
  );
}

function AccountPage() {
  const { id } = Route.useParams();
  return (
    <>
      <AccountHeading />
      {/* Another account is another page: nothing of the previous one carries over, its private data least of all (web-client/README.md R10). */}
      <Card>
        <AccountDetailView key={id} id={id} />
      </Card>
    </>
  );
}

function AccountError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_accounts" heading={t("client:admin.account.title")} />;
}
