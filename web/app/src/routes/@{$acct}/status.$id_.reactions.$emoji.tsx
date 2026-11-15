// SPDX-License-Identifier: AGPL-3.0-only

// Who reacted to a post with one emoji, at /@user/status/id/reactions/emoji (the emoji a reaction's name: a Unicode
// emoji, or a custom one's shortcode), which the rows under a post on its own page lead to. Back goes to the post, or
// to where the reader came from. Without a session it asks the reader to sign in.

import { loadReactors, ReactorList } from "@my/app-features";
import { createFileRoute, useCanGoBack, useRouter } from "@tanstack/react-router";
import { useStrings } from "../../locale";
import { screenLoad } from "../../screen-load";
import { ScreenHeading } from "../../shell/heading";
import { SignedOut } from "../../shell/signed-out";

export const Route = createFileRoute("/@{$acct}/status/$id_/reactions/$emoji")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadReactors(data, params.id, params.emoji)),
  component: ReactorsScreen,
  staticData: { scroll: "top" },
});

function ReactorsScreen() {
  const { data } = Route.useRouteContext();
  const { acct, id, emoji } = Route.useParams();
  const router = useRouter();
  const canGoBack = useCanGoBack();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.reactors")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <ReactorList
          key={`${id} ${emoji}`}
          id={id}
          emoji={emoji}
          onBack={() => {
            if (canGoBack) router.history.back();
            else void router.navigate({ to: "/@{$acct}/status/$id", params: { acct, id } });
          }}
        />
      )}
    </>
  );
}
