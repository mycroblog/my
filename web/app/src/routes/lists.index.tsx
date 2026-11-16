// SPDX-License-Identifier: AGPL-3.0-only

// The reader's lists, at /lists, a place of the menu (ADR-0013 §15): each list a row that opens its timeline, and New
// list, which `?create` (New list of a profile's Add to list) opens at once. Without a session it asks the reader to
// sign in.

import { ListIndex, loadLists } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/lists/")({
  validateSearch: (search: Record<string, unknown>): { create?: true } =>
    search["create"] === undefined || search["create"] === false ? {} : { create: true },
  loader: ({ context }) => screenLoad(context, loadLists),
  component: ListsScreen,
  // A place of the navigation opens at its top.
  staticData: { scroll: "top" },
});

function ListsScreen() {
  const { data } = Route.useRouteContext();
  const { create } = Route.useSearch();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.lists")}</ScreenHeading>
      {data === null ? <SignedOut /> : <ListIndex create={create === true} />}
    </>
  );
}
