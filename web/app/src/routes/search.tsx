// SPDX-License-Identifier: AGPL-3.0-only

// Search, at /search, a place of the navigation (FR-305–FR-307, FR-309): the words in `?q`, so that back returns to
// what was found. Without a session it asks the reader to sign in, as the API searches only for one.

import { Search } from "@my/app-features";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useCallback } from "react";
import { useStrings } from "../locale";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/search")({
  validateSearch: (search: Record<string, unknown>): { q?: string } =>
    typeof search["q"] === "string" && search["q"] !== "" ? { q: search["q"] } : {},
  component: SearchScreen,
  // A place of the navigation opens at its top.
  staticData: { scroll: "top" },
});

function SearchScreen() {
  const { data } = Route.useRouteContext();
  const { q = "" } = Route.useSearch();
  const navigate = useNavigate({ from: "/search" });
  const t = useStrings();
  // The words the reader settled on replace the address's, so that back leaves the search rather than each word.
  const onQuery = useCallback(
    (next: string) => {
      void navigate({ search: next === "" ? {} : { q: next }, replace: true });
    },
    [navigate],
  );
  return (
    <>
      <ScreenHeading>{t("app.search")}</ScreenHeading>
      {data === null ? <SignedOut /> : <Search q={q} onQuery={onQuery} />}
    </>
  );
}
