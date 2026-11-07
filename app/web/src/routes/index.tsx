// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps, redirect } from "@tanstack/react-router";
import { instanceQueryOptions, Landing } from "../features/landing";
import { useInstanceName } from "../lib/instanceName";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";

/**
 * The root address: the landing page for people who are not signed in (on a one-person server, the operator's
 * profile, where it sends them), and the home timeline for signed-in users, whom it sends there (the timelines are named by their own paths, web-client/README.md S3). The server's shell of this
 * address carries the instance's link preview (ADR-0028).
 */
export const Route = createFileRoute("/")({
  loader: async ({ context: { queryClient, api, session } }) => {
    await session.restore();
    // While the server cannot be reached to resume the session (a page opened offline), the home timeline shows
    // the posts kept on this browser (web-client/11-pwa.md §11).
    if (session.status === "signed-in" || session.unreachable) {
      // eslint-disable-next-line typescript/only-throw-error -- TanStack Router's redirects are thrown objects.
      throw redirect({ to: "/home", replace: true });
    }
    // The instance tells what to show; the page still offers to sign in without it, so its failure does not fail
    // the page (the shell, which reads the same query, handles it). Cached data is shown at once (web-client/README.md S7).
    const instance = await queryClient
      .query({ ...instanceQueryOptions(api), staleTime: Infinity })
      .catch(() => undefined);
    // A one-person server is its operator's home: its front page is the operator's profile, whose shell offers to
    // sign in and links the server's information and documents (ADR-0152 §7).
    const operator = instance?.single_user === true ? instance.single_user_account : undefined;
    if (operator !== undefined) {
      // eslint-disable-next-line typescript/only-throw-error -- TanStack Router's redirects are thrown objects.
      throw redirect({ to: "/@{$acct}", params: { acct: operator.acct }, replace: true });
    }
  },
  staticData: { reloadSafe: true, wideMain: true },
  component: LandingPage,
  errorComponent: LandingError,
});

function LandingPage() {
  return <Landing heading={<LandingHeading />} />;
}

/** The page's heading: the instance's name, the title of its masthead (and so the document's title). */
function LandingHeading() {
  const instance = useInstanceName();
  return (
    <PageHeading display>
      <span dir="auto">{instance}</span>
    </PageHeading>
  );
}

function LandingError(props: ErrorComponentProps) {
  const instance = useInstanceName();
  return <RouteError {...props} heading={instance} />;
}
