// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { ListsView, listsQueryOptions } from "../features/lists";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/** The user's lists of the accounts they follow, and their members (web-client/screens/lists.md, ADR-0122). */
export const Route = createFileRoute("/lists")({
  staticData: { title: "client:lists.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) => context.queryClient.query({ ...listsQueryOptions(context.api), staleTime: Infinity }),
  component: ListsPage,
});

function ListsPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:lists.title")}</PageHeading>
      <ListsView />
    </>
  );
}
