// SPDX-License-Identifier: AGPL-3.0-only

// An announcement's page, at /announcements/{id} (ADR-0125 §5), where the notification, the column beside the timeline
// and the list lead: its title, its text and when it shows; opening it marks it read. Without a session it asks the
// reader to sign in.

import { AnnouncementPage, loadAnnouncement, useAnnouncementTitle } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/announcements/$id")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadAnnouncement(data, params.id)),
  component: AnnouncementScreen,
  // A page of its own opens at its top.
  staticData: { scroll: "top" },
});

function Heading({ id }: { id: string }) {
  const t = useStrings();
  return <ScreenHeading>{useAnnouncementTitle(id) ?? t("app.announcements")}</ScreenHeading>;
}

function AnnouncementScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const router = useRouter();
  const t = useStrings();
  if (data === null)
    return (
      <>
        <ScreenHeading>{t("app.announcements")}</ScreenHeading>
        <SignedOut />
      </>
    );
  return (
    <>
      <Heading id={id} />
      <AnnouncementPage
        key={id}
        id={id}
        onBack={() => {
          void router.navigate({ to: "/announcements" });
        }}
      />
    </>
  );
}
