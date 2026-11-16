// SPDX-License-Identifier: AGPL-3.0-only

// Whom an account follows, at /@user/following (ADR-0018 §10): the tab of its follows on the screen of its lists,
// which the profile's count opens (shell/follows-screen.tsx).

import { loadFollows } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../../screen-load";
import { FollowsScreen } from "../../shell/follows-screen";

export const Route = createFileRoute("/@{$acct}/following")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadFollows(data, params.acct, "following")),
  component: Screen,
  staticData: { scroll: "top" },
});

function Screen() {
  const { data } = Route.useRouteContext();
  const { acct } = Route.useParams();
  return <FollowsScreen signedIn={data !== null} acct={acct} list="following" />;
}
