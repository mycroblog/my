// SPDX-License-Identifier: AGPL-3.0-only

// The reader's favourites, at /favourites, a place of the navigation beside the bookmarks (ADR-0110 §11): the posts
// they favourited, the newest favourite first. Without a session it asks the reader to sign in.

import { Favourites, loadFavourites } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/favourites")({
  loader: ({ context }) => screenLoad(context, loadFavourites),
  component: FavouritesScreen,
  // Back on the favourites, the reader is where they left them.
  staticData: { scroll: "restore" },
});

function FavouritesScreen() {
  const { data } = Route.useRouteContext();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.favourites")}</ScreenHeading>
      {data === null ? <SignedOut /> : <Favourites />}
    </>
  );
}
