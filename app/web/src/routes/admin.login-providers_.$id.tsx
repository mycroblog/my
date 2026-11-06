// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { LoginProviderDetail, loginProviderQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/admin/login-providers`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * One external login provider (ADR-0106): its callback URL, changing its settings, giving it a new client secret,
 * enabling, disabling and deleting it, for manage_login_providers.
 */
export const Route = createFileRoute("/admin/login-providers_/$id")({
  staticData: { title: "client:admin.login_providers.detail.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_login_providers", () =>
      context.queryClient.query({ ...loginProviderQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: LoginProviderPage,
  errorComponent: LoginProviderError,
});

function LoginProviderHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.login_providers.detail.title")}</PageHeading>
      <Link href="/admin/login-providers" variant="standalone" size="sm">
        {t("client:admin.login_providers.back")}
      </Link>
    </Stack>
  );
}

function LoginProviderPage() {
  const { id } = Route.useParams();
  return (
    <>
      <LoginProviderHeading />
      {/* Another provider is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <LoginProviderDetail key={id} id={id} />
    </>
  );
}

function LoginProviderError(props: ErrorComponentProps) {
  const t = useT();
  return (
    <AdminError
      {...props}
      permission="manage_login_providers"
      heading={t("client:admin.login_providers.detail.title")}
    />
  );
}
