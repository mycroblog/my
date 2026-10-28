// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { useId } from "react";
import { z } from "zod";
import { AccountSearch, maxSearchLength } from "../features/search";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/** `q`: the text of the last search, which the field starts with when the page is opened again (web-client/README.md S3). */
const searchSchema = z.object({
  q: z.string().max(maxSearchLength).optional().catch(undefined),
});

/** Finds an account by its handle or URL and opens its profile (FR-305). */
export const Route = createFileRoute("/search")({
  staticData: { title: "client:search.title", hideHeading: true },
  validateSearch: searchSchema,
  component: SearchPage,
});

function SearchPage() {
  const t = useT();
  const headingId = useId();
  const { q } = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading id={headingId}>{t("client:search.title")}</PageHeading>
      <AccountSearch
        headingId={headingId}
        initialQuery={q ?? ""}
        onFound={(acct, query) => {
          // This page keeps the search, so that going back from the profile shows it again.
          void navigate({ search: { q: query }, replace: true }).then(() =>
            navigate({ to: "/@{$acct}", params: { acct } }),
          );
        }}
      />
    </>
  );
}
