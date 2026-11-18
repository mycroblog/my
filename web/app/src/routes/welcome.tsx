// SPDX-License-Identifier: AGPL-3.0-only

// The welcome step of a new account, at /welcome (docs/operations/registration.md): the server sends a person who can
// use their account at once here through the app's sign-in, after signing up or confirming their email address
// (internal/interface/authpages, PathWelcome). A page of its own outside the frame (__root.tsx): it asks for a display
// name and an avatar, both optional, and goes on to the home timeline in its own place in the history, so that Back
// does not return to it. An account that is not new goes there at once. Without a session it asks the reader to sign
// in, in the frame as every screen that needs a session does, and the sign-in comes back here.

import { useInstance } from "@my/app-data";
import { useWelcomeTitle } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";
import { WelcomePage, welcomePath } from "../shell/welcome";

export const Route = createFileRoute("/welcome")({
  component: WelcomeScreen,
  // A page of its own opens at its top.
  staticData: { scroll: "top" },
});

/** What a visitor sees: the step's name, and that they sign in to see it. */
function Visitor({ serverName }: { serverName: string }) {
  const title = useWelcomeTitle(useInstance()?.name ?? serverName);
  return (
    <>
      <ScreenHeading>{title}</ScreenHeading>
      <SignedOut returnTo={welcomePath} />
    </>
  );
}

function WelcomeScreen() {
  const router = useRouter();
  const { data, config } = Route.useRouteContext();
  if (data === null) return <Visitor serverName={config.instanceName} />;
  return (
    <WelcomePage
      serverName={config.instanceName}
      onHome={() => {
        void router.navigate({ to: "/", replace: true });
      }}
    />
  );
}
