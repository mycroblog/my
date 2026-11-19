// SPDX-License-Identifier: AGPL-3.0-only

// The appeals of refused posts, at /moderation/hook-decisions/appeals (ADR-0058 §2, ADR-0147): pending first, filtered
// by their state, which the address keeps. Without a session it asks the reader to sign in.

import { loadModeratedHookAppeals } from "@my/app-data";
import { loadUnderEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { searchOfQuery } from "../shell/staff";
import { screenLoad } from "../screen-load";
import { HookAppealsRoute } from "../shell/staff-accounts";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/hook-decisions/appeals/")({
  loaderDeps: ({ search }) => ({ search: search as Record<string, unknown> }),
  loader: ({ context, location }) => {
    const search = searchOfQuery(location.searchStr);
    return screenLoad(context, (data) =>
      loadUnderEntry(data, "hook-decisions", (d) =>
        loadModeratedHookAppeals(
          d,
          search["state"] === "any"
            ? undefined
            : search["state"] === "upheld" || search["state"] === "overturned"
              ? search["state"]
              : "pending",
          search["cursor"],
        ),
      ),
    );
  },
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  return data === null ? <StaffSignedOut returnTo="/moderation/hook-decisions/appeals" /> : <HookAppealsRoute />;
}
