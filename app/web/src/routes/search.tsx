// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { useId } from "react";
import { z } from "zod";
import { AccountSearch, maxSearchLength, SearchTabs, type SearchType } from "../features/search";
import { usePostSearchAvailable } from "../lib/fullTextSearch";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/**
 * `q`: the text of the last search, which the field starts with when the page is opened again (web-client/README.md
 * S3). `type`: which search it is; without it, the search of accounts, which is the only one on most servers.
 */
const searchSchema = z.object({
  q: z.string().max(maxSearchLength).optional().catch(undefined),
  type: z.enum(["posts"]).optional().catch(undefined),
});

/** Finds an account by its handle or URL and opens its profile (FR-305), and posts by their words where the server can (FR-309). */
export const Route = createFileRoute("/search")({
  staticData: { reloadSafe: true, title: "client:search.title", hideHeading: true },
  validateSearch: searchSchema,
  // Whether the page has tabs is known before it shows, so that the field typed in does not move into a tab. The
  // search itself runs only in the page: a loader would spend the rate limit of the search on a preload (R7).
  loader: async ({ context: { queryClient, api, session } }) => {
    await session.restore();
    await queryClient
      .query({ ...api.query.queryOptions("get", "/instance"), staleTime: Infinity })
      // The shell shows this failure; the page offers what every server has.
      .catch(() => undefined);
  },
  component: SearchPage,
});

function SearchPage() {
  const t = useT();
  const headingId = useId();
  const { q, type } = Route.useSearch();
  const navigate = Route.useNavigate();
  const postSearch = usePostSearchAvailable();
  // A post search's words are not a name: a server or a viewer without it starts from an empty field.
  const accounts = (
    <AccountSearch
      headingId={headingId}
      initialQuery={type === "posts" ? "" : (q ?? "")}
      onFound={(acct, query) => {
        // This page keeps the search, so that going back from the profile shows it again.
        void navigate({ search: { q: query }, replace: true }).then(() =>
          navigate({ to: "/@{$acct}", params: { acct } }),
        );
      }}
    />
  );
  return (
    <>
      <PageHeading id={headingId}>{t("client:search.title")}</PageHeading>
      {postSearch ? (
        <SearchTabs
          headingId={headingId}
          selected={type ?? "accounts"}
          onSelect={(next: SearchType) => {
            void navigate({ search: next === "posts" ? { type: "posts" } : {} });
          }}
          accounts={accounts}
          postQuery={q ?? ""}
          onPostSearch={(query) => {
            // The field keeps the focus and the text; the address holds the search for a reload and for going back.
            void navigate({ search: { type: "posts", q: query }, replace: true, resetScroll: false });
          }}
        />
      ) : (
        accounts
      )}
    </>
  );
}
