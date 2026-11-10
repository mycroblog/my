// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { instanceQueryOptions, KeywordFilters, keywordFiltersQueryOptions } from "../features/filters";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** The user's keyword filters (FR-209, ADR-0157). */
export const Route = createFileRoute("/settings/filters")({
  staticData: { title: "client:filters.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    Promise.all([
      context.queryClient.query({ ...keywordFiltersQueryOptions(context.api), staleTime: Infinity }),
      // The version of the server picks the version of the user manual that the form links.
      context.queryClient.query({ ...instanceQueryOptions(context.api), staleTime: Infinity }),
    ]),
  component: KeywordFiltersPage,
});

function KeywordFiltersPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:filters.title")}</SettingsHeading>
      <KeywordFilters />
    </>
  );
}
