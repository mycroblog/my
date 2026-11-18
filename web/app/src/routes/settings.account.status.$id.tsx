// SPDX-License-Identifier: AGPL-3.0-only

// One moderators' action on the reader's account, at /settings/account/status/{id}, the screen under Account status
// (ADR-0110 §5) where the action is read whole and appealed: a form saved as a whole; back goes to Account status.
// Leaving it with an appeal written and not sent is held back, whichever way the reader leaves (a link, the
// navigation, Back, closing the tab), until they say whether to: the screen asks, here the router waits. An address
// that asks about signing out over the screen is no leaving. Without a session it asks the reader to sign in.

import { loadModerationAction, ModerationActionScreen, useModerationActionTitle } from "@my/app-features";
import { createFileRoute, useBlocker, useRouter } from "@tanstack/react-router";
import { useCallback, useRef } from "react";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/settings/account/status/$id")({
  loader: ({ context }) => screenLoad(context, loadModerationAction),
  component: ModerationActionRoute,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

/** The action, for a signed-in reader, and the router held while an appeal is written and not sent. */
function Action({ id }: { id: string }) {
  const router = useRouter();
  // Read as the router asks, so that a way out right after the appeal is sent is not held by what was written.
  const unsaved = useRef(false);
  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) => unsaved.current && current.pathname !== next.pathname,
    enableBeforeUnload: () => unsaved.current,
    withResolver: true,
  });
  const onUnsaved = useCallback((now: boolean) => {
    unsaved.current = now;
  }, []);
  return (
    <ModerationActionScreen
      id={id}
      onBack={() => {
        void router.navigate({ to: "/settings/account/status" });
      }}
      onUnsaved={onUnsaved}
      leaving={blocker.status === "blocked" ? { onStay: blocker.reset, onLeave: blocker.proceed } : undefined}
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
