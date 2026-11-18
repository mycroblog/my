// SPDX-License-Identifier: AGPL-3.0-only

// Delete old posts automatically, at /settings/account/auto-delete, the screen under the settings' entry Account and
// data (ADR-0110 §5): a form saved as a whole, where the notification of the server's maximum leads; back goes to the
// entry. Leaving it with changes that are not saved is held back, whichever way the reader leaves (a link, the
// navigation, Back, closing the tab), until they say whether to: the screen asks, here the router waits. An address
// that asks about signing out over the screen is no leaving. Without a session it asks the reader to sign in.

import { AccountAutoDelete, loadAccountAutoDelete, useAccountAutoDeleteTitle } from "@my/app-features";
import { createFileRoute, useBlocker, useRouter } from "@tanstack/react-router";
import { useState } from "react";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/settings/account/auto-delete")({
  loader: ({ context }) => screenLoad(context, loadAccountAutoDelete),
  component: AccountAutoDeleteRoute,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

/** The form, for a signed-in reader, and the router held while it has changes that are not saved. */
function Edit() {
  const router = useRouter();
  const [unsaved, setUnsaved] = useState(false);
  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) => unsaved && current.pathname !== next.pathname,
    enableBeforeUnload: () => unsaved,
    withResolver: true,
  });
  return (
    <AccountAutoDelete
      onBack={() => {
        void router.navigate({ to: "/settings/$entry", params: { entry: "account" } });
      }}
      onUnsaved={setUnsaved}
      leaving={blocker.status === "blocked" ? { onStay: blocker.reset, onLeave: blocker.proceed } : undefined}
    />
  );
}

function AccountAutoDeleteRoute() {
  const { data } = Route.useRouteContext();
  const title = useAccountAutoDeleteTitle();
  return (
    <>
      <ScreenHeading>{title}</ScreenHeading>
      {data === null ? <SignedOut /> : <Edit />}
    </>
  );
}
