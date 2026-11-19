// SPDX-License-Identifier: AGPL-3.0-only

// A page of Server settings, at /admin/settings/{topic} (ADR-0058 §1): a form saved as a whole, under the entry
// Server settings; back goes to the entry. Leaving it with changes that are not saved is held back until the reader
// says whether to (unsaved-guard.ts). Without a session it asks the reader to sign in.

import { AdminSettingsTopicScreen, loadAdminSettingsTopic, useAdminSettingsTopicTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import icon from "../../../brand/icon.svg";
import logoDark from "../../../brand/Wordmark-DarkBg.svg";
import logo from "../../../brand/Wordmark-Primary-Color.svg";
import { adminSubLoad, AdminSubHeading, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/settings/$topic")({
  loader: ({ context, params }) =>
    adminSubLoad(context, "manage_settings", (data) => loadAdminSettingsTopic(data, params.topic)),
  component: AdminSettingsTopicPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

/** The built-in images of the brand, which the icon and logo page shows while the server has none of its own. */
const builtIn = { icon, logo, logo_dark: logoDark };

function AdminSettingsTopicPage() {
  const { data } = Route.useRouteContext();
  const { topic } = Route.useParams();
  return (
    <AdminSubPage
      key={topic}
      entry="settings"
      signedIn={data !== null}
      returnTo={`/admin/settings/${topic}`}
      heading={<Heading />}
    >
      {(sub) => <AdminSettingsTopicScreen topic={topic} builtIn={builtIn} {...sub} />}
    </AdminSubPage>
  );
}

/** The heading, which names what the screen shows once it is read. */
function Heading() {
  const { topic } = Route.useParams();
  return <AdminSubHeading entry="settings" title={useAdminSettingsTopicTitle(topic)} />;
}
