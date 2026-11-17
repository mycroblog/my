// SPDX-License-Identifier: AGPL-3.0-only

// A list's timeline, at /lists/{id} (ADR-0013 §13, §15): back goes to the reader's lists. Without a session it asks the
// reader to sign in.

import { ListTimeline, loadListTimeline } from "@my/app-features";
import { useList } from "@my/app-data";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/lists/$id")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadListTimeline(data, params.id)),
  component: ListScreen,
  // Back on a list, the reader is where they left its timeline.
  staticData: { scroll: "restore" },
});

/** The h1 of a list's screen: its name once known, the word for a list until then. */
function ListHeading({ id }: { id: string }) {
  const t = useStrings();
  return <ScreenHeading>{useList(id).list?.name ?? t("app.list")}</ScreenHeading>;
}

function ListScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const router = useRouter();
  const t = useStrings();
  if (data === null)
    return (
      <>
        <ScreenHeading>{t("app.list")}</ScreenHeading>
        <SignedOut />
      </>
    );
  return (
    <>
      <ListHeading id={id} />
      <ListTimeline
        key={id}
        id={id}
        onBack={() => {
          void router.navigate({ to: "/lists" });
        }}
      />
    </>
  );
}
