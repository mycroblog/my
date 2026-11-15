// SPDX-License-Identifier: AGPL-3.0-only

// The screen of a public timeline (ADR-0110 §11): its heading and its column, for whoever the instance shows it to. A
// visitor's post actions that need an account lead to the app's sign-in, which comes back to the post (ADR-0090 §16);
// a visitor it is not shown to is asked to sign in, and a timeline the instance does not offer says so.

import { useInstance, type PublicTimelineKind } from "@my/app-data";
import { PublicTimeline } from "@my/app-features";
import { FeedNotice } from "@my/design-system";
import { useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { signInReturningTo } from "../sign-in";
import { ScreenHeading } from "./heading";
import { SignedOut } from "./signed-out";

export function PublicTimelineScreen({ kind, signedIn }: { kind: PublicTimelineKind; signedIn: boolean }) {
  const t = useStrings();
  const router = useRouter();
  const audience = useInstance()?.timelines[kind];
  const signIn = (returnTo: string) => {
    void router.navigate({ href: signInReturningTo(returnTo) });
  };
  let body = <PublicTimeline kind={kind} signIn={signedIn ? undefined : signIn} />;
  if (audience === "off") body = <FeedNotice>{t("app.timeline_off")}</FeedNotice>;
  else if (!signedIn && audience === "signed_in") body = <SignedOut />;
  return (
    <>
      <ScreenHeading>{kind === "local" ? t("app.local") : t("app.federated")}</ScreenHeading>
      {body}
    </>
  );
}
