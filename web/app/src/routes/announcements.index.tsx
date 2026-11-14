// SPDX-License-Identifier: AGPL-3.0-only

// The server's announcements, at /announcements, a place of the menu (ADR-0125 §3, §4): every one that showed, newest
// first, with the unread marked. Without a session it asks the reader to sign in.

import { AnnouncementList, loadAnnouncements } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/announcements/")({
  loader: ({ context }) => screenLoad(context, loadAnnouncements),
  component: AnnouncementsScreen,
  // A place of the navigation opens at its top, where the newest announcements are.
  staticData: { scroll: "top" },
});

function AnnouncementsScreen() {
  const { data } = Route.useRouteContext();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.announcements")}</ScreenHeading>
      {data === null ? <SignedOut /> : <AnnouncementList />}
    </>
  );
}
