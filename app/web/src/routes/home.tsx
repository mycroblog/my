// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useId } from "react";
import { Announcements } from "../features/announcements";
import { FirstRun } from "../features/onboarding";
import {
  forgetSavedHomeTimeline,
  HomeTimeline,
  joinWaitingStatuses,
  restoreSavedHomeTimeline,
  TimelineSkeleton,
  timelineQueryOptions,
} from "../features/timeline";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useSessionUnreachable } from "../lib/auth/SessionProvider";
import { NetworkError } from "../lib/errors";
import { useT } from "../lib/i18n";
import { useOnReconnect } from "../lib/network";
import { PageHeading } from "./-layout/PageHeading";

/** The home timeline (FR-201): the signed-in user's posts and those of the accounts they follow. */
export const Route = createFileRoute("/home")({
  // A feed to add to: the shell floats the new-post button over this page (web-client/16-look-states-and-wording.md G4).
  staticData: { title: "client:home.title", hideHeading: "wide", composeButton: true },
  // The home timeline needs a session; without one the browser goes to sign in first, and comes back here
  // (web-client/10-api-client-and-auth.md §10). While the server cannot be reached to resume it (a page opened offline), the page opens
  // and shows the posts kept on this browser (FR-604, web-client/11-pwa.md §11); once the server answers that there is
  // no session, they are forgotten.
  beforeLoad: async (args) => {
    await args.context.session.restore();
    if (args.context.session.status === "signed-in" || args.context.session.unreachable) return;
    forgetSavedHomeTimeline();
    await requireSignIn(args);
  },
  loader: async ({ context: { queryClient, api, session } }) => {
    // Past the guard without a session: the server cannot be reached to resume it.
    if (session.status !== "signed-in") return { saved: restoreSavedHomeTimeline(queryClient, api) };
    try {
      // Cached pages that no write has marked stale are shown at once (and refetched in the background once
      // old); marked or missing ones are fetched and waited for (web-client/README.md S7).
      await queryClient.infiniteQuery({ ...timelineQueryOptions(api, { kind: "home" }), staleTime: Infinity });
    } catch (error) {
      // Offline, the posts loaded last (cached, or kept on this browser) are shown instead of the error.
      if (!(error instanceof NetworkError) || !restoreSavedHomeTimeline(queryClient, api)) throw error;
    }
    // Opening the timeline from another page: the news that waited for the reader are on it from the first render.
    joinWaitingStatuses(queryClient, api, { kind: "home" });
    return { saved: false };
  },
  component: HomePage,
  pendingComponent: HomePending,
});

function HomePage() {
  const t = useT();
  const headingId = useId();
  const router = useRouter();
  const { saved } = Route.useLoaderData();
  const unreachable = useSessionUnreachable();
  // Showing the kept posts: back online, the loader resumes the session and loads the timeline again (web-client/16-look-states-and-wording.md G17).
  useOnReconnect(() => {
    void router.invalidate();
  }, saved && unreachable);
  return (
    <>
      <PageHeading id={headingId}>{t("client:home.title")}</PageHeading>
      {/* The server's announcements to every user (FR-726); not while the page shows the posts kept on this browser. */}
      {!saved && <Announcements />}
      {/* The first steps of a new user and the accounts suggested to follow (web-client/screens/home-first-run.md). */}
      {!saved && (
        <FirstRun
          onDismissed={() => {
            document.getElementById(headingId)?.focus();
          }}
        />
      )}
      {/* Mounted again once the session resumes: the user change replaced the kept posts' query (lib/auth/userData). */}
      <HomeTimeline key={saved ? "saved" : "loaded"} aria-labelledby={headingId} saved={saved} />
    </>
  );
}

function HomePending() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:home.title")}</PageHeading>
      <TimelineSkeleton />
    </>
  );
}
