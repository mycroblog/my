// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { NewRelay } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * Registering and joining a federation relay (ADR-0006), for manage_settings; joining opens the relay's page. The `_`
 * keeps the page out of the list's layout at `/admin/relays`.
 */
export const Route = createFileRoute("/admin/relays_/new")({
  staticData: { title: "client:admin.relays.new.title" },
  beforeLoad: requireSignIn,
  component: NewRelayPage,
  errorComponent: NewRelayError,
});

function NewRelayHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.relays.new.title")}</PageHeading>
      <Link href="/admin/relays" variant="standalone" size="sm">
        {t("client:admin.relays.back")}
      </Link>
    </Stack>
  );
}

function NewRelayPage() {
  return (
    <>
      <NewRelayHeading />
      <NewRelay />
    </>
  );
}

function NewRelayError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_settings" heading={t("client:admin.relays.new.title")} />;
}
