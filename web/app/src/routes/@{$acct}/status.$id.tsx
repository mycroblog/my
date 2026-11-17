// SPDX-License-Identifier: AGPL-3.0-only

// A post with its thread, at /@user/status/id: the server's address of a post (ADR-0018), under its author's, which
// its HTML shell serves with the post's link preview. Back goes where the reader came from, or home when the post was
// the first screen. Without a session it asks the reader to sign in.

import { loadThread, Thread } from "@my/app-features";
import { createFileRoute, useCanGoBack, useRouter } from "@tanstack/react-router";
import { useStrings } from "../../locale";
import { screenLoad } from "../../screen-load";
import { ScreenHeading } from "../../shell/heading";
import { SignedOut } from "../../shell/signed-out";

export const Route = createFileRoute("/@{$acct}/status/$id")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadThread(data, params.id)),
  component: PostScreen,
  // Back on a post, the reader is where they left its thread.
  staticData: { scroll: "restore" },
});

function PostScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const router = useRouter();
  const canGoBack = useCanGoBack();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.post")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <Thread
          key={id}
          id={id}
          onBack={() => {
            if (canGoBack) router.history.back();
            else void router.navigate({ to: "/" });
          }}
        />
      )}
    </>
  );
}
