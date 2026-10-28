// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { LoginProviderSettings, loginProvidersQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The server's external login providers (ADR-0106): setting them up, changing, disabling and deleting them, for manage_login_providers. */
export const Route = createFileRoute("/admin/login-providers")({
  staticData: { title: "client:admin.login_providers.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_login_providers", () =>
      context.queryClient.query({ ...loginProvidersQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: LoginProvidersPage,
  errorComponent: LoginProvidersError,
});

function LoginProvidersPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.login_providers.title")}</PageHeading>
      <LoginProviderSettings />
    </>
  );
}

function LoginProvidersError(props: ErrorComponentProps) {
  const t = useT();
  return (
    <AdminError {...props} permission="manage_login_providers" heading={t("client:admin.login_providers.title")} />
  );
}
