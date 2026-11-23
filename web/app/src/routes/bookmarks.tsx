// SPDX-License-Identifier: AGPL-3.0-only

// The reader's bookmarks, at /bookmarks, a place of the navigation (ADR-0110 §11): the posts they bookmarked, the
// newest bookmark first. Without a session it asks the reader to sign in.

import { Bookmarks, loadBookmarks } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/bookmarks")({
  loader: ({ context }) => screenLoad(context, loadBookmarks),
  component: BookmarksScreen,
  // A place of the navigation opens at the newest; going back to the bookmarks, the reader is where they left it.
  staticData: { scroll: "back" },
});

function BookmarksScreen() {
  const { data } = Route.useRouteContext();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.bookmarks")}</ScreenHeading>
      {data === null ? <SignedOut /> : <Bookmarks />}
    </>
  );
}
