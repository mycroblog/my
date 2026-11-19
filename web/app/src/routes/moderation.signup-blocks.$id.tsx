// SPDX-License-Identifier: AGPL-3.0-only

// A sign-up block, at /moderation/signup-blocks/{id} (ADR-0058 §2): all it holds, and Lift while it is in effect.
// Without a session it asks the reader to sign in.

import { loadSignupBlock } from "@my/app-data";
import { loadUnderEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SignupBlockRoute } from "../shell/staff-accounts";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/signup-blocks/$id")({
  loader: ({ context, params }) =>
    screenLoad(context, (data) => loadUnderEntry(data, "signup-blocks", (d) => loadSignupBlock(d, params.id))),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return data === null ? <StaffSignedOut returnTo={`/moderation/signup-blocks/${id}`} /> : <SignupBlockRoute id={id} />;
}
