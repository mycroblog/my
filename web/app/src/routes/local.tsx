// SPDX-License-Identifier: AGPL-3.0-only

// The local timeline, at /local, a place of the navigation: the public posts of this server's people (ADR-0110 §11).

import { loadPublicTimeline } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { PublicTimelineScreen } from "../shell/public-timeline";

export const Route = createFileRoute("/local")({
  loader: ({ context }) => screenLoad(context, (data) => loadPublicTimeline(data, "local")),
  component: LocalScreen,
  // Back on the timeline, the reader is where they left it.
  staticData: { scroll: "restore" },
});

function LocalScreen() {
  return <PublicTimelineScreen kind="local" signedIn={Route.useRouteContext().data !== null} />;
}
