// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { useId } from "react";
import { joinWaitingStatuses, PublicTimeline, TimelineSkeleton, timelineQueryOptions } from "../features/timeline";
import { SignInPrompt } from "../features/session";
import { ApiError } from "../lib/errors";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";
import { HomeLink } from "./-layout/HomeLink";

/**
 * The local timeline (FR-202): the public posts of this server's accounts, and their local posts for a signed-in
 * user. The navigation offers it, at every width, as an item of its own while the instance serves it.
 */
export const Route = createFileRoute("/local")({
  // A feed to add to: the shell floats the new-post button over this page (web-client/16-look-states-and-wording.md G4).
  staticData: { title: "client:local.title", hideHeading: "wide", composeButton: true },
  loader: async ({ context: { queryClient, api } }) => {
    // Cached pages that no write has marked stale are shown at once (and refetched in the background once
    // old); marked or missing ones are fetched and waited for (web-client/README.md S7).
    await queryClient.infiniteQuery({ ...timelineQueryOptions(api, { kind: "local" }), staleTime: Infinity });
    // New posts that waited behind the button when the reader left the timeline are on it when it opens again.
    joinWaitingStatuses(queryClient, api, { kind: "local" });
  },
  component: LocalPage,
  pendingComponent: LocalPending,
  errorComponent: LocalError,
});

function LocalPage() {
  const t = useT();
  const headingId = useId();
  return (
    <>
      <PageHeading id={headingId}>{t("client:local.title")}</PageHeading>
      <PublicTimeline timeline={{ kind: "local" }} aria-labelledby={headingId} />
    </>
  );
}

function LocalPending() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:local.title")}</PageHeading>
      <TimelineSkeleton />
    </>
  );
}

/**
 * A 404 is the instance setting that turns the local timeline off, and a 401 the one that keeps it for
 * signed-in users (api/openapi.yaml, getLocalTimeline), reached from an old link, a bookmark, or a shared address: it says
 * so, with the way to sign in for the latter, instead of offering to try again.
 */
function LocalError(props: ErrorComponentProps) {
  const t = useT();
  if (props.error instanceof ApiError && props.error.status === 401) {
    return (
      <>
        <PageHeading visuallyHidden={false}>{t("client:local.title")}</PageHeading>
        <SignInPrompt
          heading={t("client:local.sign_in_required.title")}
          description={t("client:local.sign_in_required.body")}
          returnTo="/local"
        />
      </>
    );
  }
  if (!(props.error instanceof ApiError && props.error.status === 404)) return <RouteError {...props} />;
  return (
    <>
      <PageHeading visuallyHidden={false}>{t("client:local.disabled.title")}</PageHeading>
      <p>{t("client:local.disabled.body")}</p>
      <HomeLink />
    </>
  );
}
