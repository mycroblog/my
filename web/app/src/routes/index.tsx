// SPDX-License-Identifier: AGPL-3.0-only

// The home screen, at /: the home timeline of the signed-in reader, under a line that counts the unread announcements
// where no column beside it holds them (ADR-0125 §4). A visitor without a session sees the operator's profile on a
// one-person server (ADR-0152 §7); on any other server, the landing page takes the place of the shell (__root.tsx).

import { useInstance } from "@my/app-data";
import { HomeTimeline, loadHomeTimeline, Profile } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { UnreadAnnouncements } from "../shell/around";
import { ScreenHeading } from "../shell/heading";
import { ProfileHeading } from "../shell/profile-heading";

export const Route = createFileRoute("/")({
  loader: ({ context }) => (context.data === null ? undefined : loadHomeTimeline(context.data)),
  component: Home,
  // Back on the home timeline, the reader is where they left it.
  staticData: { scroll: "restore" },
});

/** The first page of a visitor of a one-person server: its operator's profile (ADR-0152 §7). */
function Visitor() {
  const operator = useInstance()?.single_user_account;
  if (operator === undefined) return null;
  return (
    <>
      <ProfileHeading acct={operator.acct} />
      <Profile acct={operator.acct} />
    </>
  );
}

function Home() {
  const { data } = Route.useRouteContext();
  const t = useStrings();
  if (data === null) return <Visitor />;
  return (
    <>
      <ScreenHeading>{t("app.home")}</ScreenHeading>
      <UnreadAnnouncements />
      <HomeTimeline />
    </>
  );
}
