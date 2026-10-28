// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { AdminOverview } from "../features/admin";
import { viewerQueryOptions } from "../features/session";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The server administration overview that the navigation's Administration entry opens (ADR-0058,
 * screens/admin.md): how the server runs. The loader reads the viewer's account, whose permissions decide what it
 * shows.
 */
export const Route = createFileRoute("/admin/")({
  staticData: { title: "client:admin.menu.title" },
  beforeLoad: requireSignIn,
  loader: async ({ context: { queryClient, api } }) => {
    await queryClient.query({ ...viewerQueryOptions(api), staleTime: Infinity });
  },
  component: AdminPage,
});

function AdminPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.menu.title")}</PageHeading>
      <AdminOverview />
    </>
  );
}
