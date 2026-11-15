// SPDX-License-Identifier: AGPL-3.0-only

// The timeline of a hashtag, at /tags/{tag}, reached from a hashtag in a post (FR-203): no place of the navigation, so
// back goes where the reader came from, or home when the hashtag was the first screen. A visitor reads it too, as the
// server shows a hashtag's public posts to everyone; following it and the actions of its posts that need an account
// lead them to the app's sign-in, which comes back to the hashtag or the post (ADR-0090 §16).

import { loadTagTimeline, TagTimeline } from "@my/app-features";
import { createFileRoute, useCanGoBack, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { signInReturningTo } from "../sign-in";

export const Route = createFileRoute("/tags/$tag")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadTagTimeline(data, params.tag)),
  component: TagScreen,
  // Back on a hashtag, the reader is where they left its timeline.
  staticData: { scroll: "restore" },
});

function TagScreen() {
  const { data } = Route.useRouteContext();
  const { tag } = Route.useParams();
  const router = useRouter();
  const canGoBack = useCanGoBack();
  const t = useStrings();
  const signIn = (returnTo: string) => {
    void router.navigate({ href: signInReturningTo(returnTo) });
  };
  return (
    <>
      <ScreenHeading>{t("app.tag", { name: tag })}</ScreenHeading>
      <TagTimeline
        key={tag}
        name={tag}
        signIn={data === null ? signIn : undefined}
        onBack={() => {
          if (canGoBack) router.history.back();
          else void router.navigate({ to: "/" });
        }}
      />
    </>
  );
}
