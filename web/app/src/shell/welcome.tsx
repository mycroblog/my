// SPDX-License-Identifier: AGPL-3.0-only

// The welcome step of a new account, a page of its own before the app, outside its frame, as the closed server's is:
// its address, where the server sends a new person through the app's sign-in (internal/interface/authpages,
// PathWelcome), and the step on the server's page, with its mark and the footer of a page of its own.

import { useInstance, useStatusPageUrl } from "@my/app-data";
import { Welcome, type WelcomeSpecimen } from "@my/app-features";
import { useRouterState } from "@tanstack/react-router";
import { PageFooter, pageServer, wordmark } from "./pages";

/** The address of the welcome step. */
export const welcomePath = "/welcome";

/**
 * Whether the router shows the welcome step (routes/welcome.tsx), by the matches it renders, not by its location: the
 * location changes as the step goes on to the home timeline, while the step is still on the page (screen.ts), and the
 * step stays outside the frame until the home timeline takes its place.
 */
export const useShowsWelcome = () =>
  useRouterState({ select: (state) => state.matches.at(-1)?.routeId === welcomePath });

/**
 * The welcome step on the page of the server, which `serverName` names until the server is read; `onHome` goes on to
 * the home timeline.
 */
export function WelcomePage({
  serverName,
  onHome,
  specimen,
}: {
  serverName: string;
  onHome: () => void;
  specimen?: WelcomeSpecimen | undefined;
}) {
  const instance = useInstance();
  const statusPage = useStatusPageUrl();
  return (
    <Welcome
      server={pageServer(instance, serverName)}
      wordmark={wordmark}
      footer={<PageFooter statusPage={statusPage} />}
      onHome={onHome}
      specimen={specimen}
    />
  );
}
