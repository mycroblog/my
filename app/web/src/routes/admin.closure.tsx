// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { adminClosureCheckQueryOptions, adminClosureQueryOptions, ServiceClosureSettings } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The service closure (FR-808, FR-809, ADR-0088): announcing it, changing and withdrawing the announcement, checking
 * and confirming the closing, and following the progress of its deliveries, for manage_service_closure.
 */
export const Route = createFileRoute("/admin/closure")({
  staticData: { title: "client:admin.closure.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_service_closure", async () => {
      const closure = await context.queryClient.query({
        ...adminClosureQueryOptions(context.api),
        staleTime: Infinity,
      });
      // The check stands before the confirmation of the closing, which only an announced closure has.
      if (closure.state === "announced")
        await context.queryClient.query({ ...adminClosureCheckQueryOptions(context.api), staleTime: Infinity });
    }),
  component: ClosurePage,
  errorComponent: ClosureError,
});

function ClosurePage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.closure.title")}</PageHeading>
      <ServiceClosureSettings />
    </>
  );
}

function ClosureError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_service_closure" heading={t("client:admin.closure.title")} />;
}
