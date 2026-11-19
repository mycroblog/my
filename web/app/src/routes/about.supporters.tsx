// SPDX-License-Identifier: AGPL-3.0-only

// The supporters' page, at /about/supporters (ADR-0070 §13, ADR-0072 §14, ADR-0162), where the rail's funding meter
// and the menu lead: the plans and subscribing to one or changing to it, the way to tip the server, and the
// supporters who chose to be listed, each where the server has it for the reader. A visitor sees the plans, and the
// list where it is shown to everyone; subscribing, tipping and a list for signed-in readers ask them to sign in, which
// comes back here. The provider's checkout is a page the app leaves for.

import { supportersPagePath } from "@my/app-components";
import { loadSupporters, SupportersScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { signInReturningTo } from "../sign-in";

export const Route = createFileRoute("/about/supporters")({
  loader: ({ context }) => screenLoad(context, loadSupporters),
  component: SupportersPage,
  // A page of its own opens at its top.
  staticData: { scroll: "top" },
});

function SupportersPage() {
  const { data, leave } = Route.useRouteContext();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.supporters")}</ScreenHeading>
      <SupportersScreen signedIn={data !== null} signInHref={signInReturningTo(supportersPagePath)} leave={leave} />
    </>
  );
}
