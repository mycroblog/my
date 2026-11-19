// SPDX-License-Identifier: AGPL-3.0-only

// Translation, at /admin/translation: the entry's screen, a form saved as a whole under how translation is used this
// month, given a route of its own (over /admin/$entry) so that leaving it with changes not saved is held back until
// the reader says whether to, as on every other form of the settings. Without a session it asks the reader to sign in.

import { loadTranslationSettings, TranslationSettingsScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/translation")({
  loader: ({ context }) => adminSubLoad(context, "manage_settings", loadTranslationSettings),
  component: TranslationPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function TranslationPage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="translation" signedIn={data !== null} returnTo="/admin/translation">
      {(sub) => <TranslationSettingsScreen {...sub} />}
    </AdminSubPage>
  );
}
