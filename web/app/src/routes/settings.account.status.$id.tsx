// SPDX-License-Identifier: AGPL-3.0-only

// One moderators' action on the reader's account, at /settings/account/status/{id}, the screen under Account status
// (ADR-0110 §5) where the action is read whole and appealed: a form saved as a whole; back goes to Account status.
// Leaving it with an appeal written and not sent is held back, whichever way the reader leaves (a link, the
// navigation, Back, closing the tab), until they say whether to: the screen asks, here the router waits
// (unsaved-guard.ts). An address that asks about signing out over the screen is no leaving, and a session that is
// ending holds nothing. Without a session it asks the reader to sign in.

import { loadModerationAction, ModerationActionScreen, useModerationActionTitle } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";
import { useUnsavedGuard } from "../unsaved-guard";

export const Route = createFileRoute("/settings/account/status/$id")({
  loader: ({ context }) => screenLoad(context, loadModerationAction),
  component: ModerationActionRoute,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

/** The action, for a signed-in reader, and the router held while an appeal is written and not sent. */
function Action({ id }: { id: string }) {
  const router = useRouter();
  const { onUnsaved, leaving } = useUnsavedGuard();
  return (
    <ModerationActionScreen
      id={id}
      onBack={() => {
        void router.navigate({ to: "/settings/account/status" });
      }}
      onUnsaved={onUnsaved}
      leaving={leaving}
    />
  );
}

function ModerationActionRoute() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const title = useModerationActionTitle();
  return (
    <>
      <ScreenHeading>{title}</ScreenHeading>
      {data === null ? <SignedOut /> : <Action key={id} id={id} />}
    </>
  );
}
