// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { AccountDetailView, adminAccountQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the search's layout at `/moderation/accounts`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/** `status`: the post of the account that the page starts from (a post's menu leads here with it). Anything else is dropped. */
const searchSchema = z.object({ status: z.string().min(1).max(64).optional().catch(undefined) });

/**
 * The review page of one account as the moderators see it: its summary, the moderation action to take on it and the
 * evidence (FR-714, FR-703, FR-713), for manage_accounts. Its other pages are /actions and /manage.
 */
export const Route = createFileRoute("/moderation/accounts_/$id")({
  staticData: { title: "client:admin.account.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  validateSearch: searchSchema,
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
  const { status } = Route.useSearch();
  return (
    <>
      <AccountHeading />
      {/* Another account is another page: nothing of the previous one carries over, its private data least of all (web-client/README.md R10). */}
      <AccountDetailView key={id} id={id} statusId={status} />
    </>
  );
}

function AccountError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_accounts" heading={t("client:admin.account.title")} />;
}
