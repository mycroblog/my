// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { NewLoginProvider } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * Setting up an external login provider (ADR-0106), for manage_login_providers; setting it up opens its page, which
 * shows the callback URL to register at the provider. The `_` keeps the page out of the list's layout at
 * `/admin/login-providers`.
 */
export const Route = createFileRoute("/admin/login-providers_/new")({
  staticData: { title: "client:admin.login_providers.new.title" },
  beforeLoad: requireSignIn,
  component: NewLoginProviderPage,
  errorComponent: NewLoginProviderError,
});

function NewLoginProviderPage() {
  const t = useT();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>{t("client:admin.login_providers.new.title")}</PageHeading>
        <Link href="/admin/login-providers" variant="standalone" size="sm">
          {t("client:admin.login_providers.back")}
        </Link>
      </Stack>
      <NewLoginProvider />
    </>
  );
}

function NewLoginProviderError(props: ErrorComponentProps) {
  const t = useT();
  return (
    <AdminError {...props} permission="manage_login_providers" heading={t("client:admin.login_providers.new.title")} />
  );
}
