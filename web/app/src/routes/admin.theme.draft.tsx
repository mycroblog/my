// SPDX-License-Identifier: AGPL-3.0-only

// The draft of the instance theme, at /admin/theme/draft (ADR-0059): a form saved as a whole, under the entry Theme;
// back goes to the entry. Leaving it with changes that are not saved is held back until the reader says whether to
// (unsaved-guard.ts). Without a session it asks the reader to sign in.

import { loadTheme, ThemeDraftScreen, useThemeDraftTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubHeading, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/theme/draft")({
  loader: ({ context }) => adminSubLoad(context, "manage_settings", loadTheme),
  component: ThemeDraftPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function ThemeDraftPage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="theme" signedIn={data !== null} returnTo="/admin/theme/draft" heading={<Heading />}>
      {(sub) => <ThemeDraftScreen {...sub} />}
    </AdminSubPage>
  );
}

function Heading() {
  return <AdminSubHeading entry="theme" title={useThemeDraftTitle()} />;
}
