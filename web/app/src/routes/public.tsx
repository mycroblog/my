// SPDX-License-Identifier: AGPL-3.0-only

// The federated timeline, at /public, a place of the navigation: the public posts this server knows from every server
// (ADR-0110 §11).

import { loadPublicTimeline } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { PublicTimelineScreen } from "../shell/public-timeline";

export const Route = createFileRoute("/public")({
  loader: ({ context }) => screenLoad(context, (data) => loadPublicTimeline(data, "federated")),
  component: FederatedScreen,
  // Back on the timeline, the reader is where they left it.
  staticData: { scroll: "restore" },
});

function FederatedScreen() {
  return <PublicTimelineScreen kind="federated" signedIn={Route.useRouteContext().data !== null} />;
}
