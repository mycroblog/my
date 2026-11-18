// SPDX-License-Identifier: AGPL-3.0-only

// The screen of an entry of the settings, at /settings/{entry} (ADR-0110 §5): the entries are the features' list
// (@my/app-features, settings/entries.tsx), so a new one needs no route; back goes to the menu. An address that names
// no entry says so in the screen, under the settings' own name as its heading and the document's title, as every
// screen whose item is missing keeps its place's name. Without a session it asks the reader to sign in. A screen under
// an entry (/settings/security/password) is a route of its own, which this one gives way to.

import { loadSettingsEntry, SettingsEntryScreen, useSettingsTitle } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { appLanguages, useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { useSettingsAddress } from "../shell/settings";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/settings/$entry")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadSettingsEntry(data, params.entry)),
  component: EntryScreen,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

/** The entry's screen under its name, or under the settings' for an address that names none, for a signed-in reader. */
function Entry({ entry }: { entry: string }) {
  const router = useRouter();
  const t = useStrings();
  const title = useSettingsTitle(entry);
  const address = useSettingsAddress();
  return (
    <>
      <ScreenHeading>{title ?? t("app.settings")}</ScreenHeading>
      <SettingsEntryScreen
        {...address}
        entry={entry}
        languages={appLanguages}
        onBack={() => {
          void router.navigate({ to: "/settings" });
        }}
      />
    </>
  );
}

function EntryScreen() {
  const { data } = Route.useRouteContext();
  const { entry } = Route.useParams();
  const t = useStrings();
  if (data !== null) return <Entry key={entry} entry={entry} />;
  return (
    <>
      <ScreenHeading>{t("app.settings")}</ScreenHeading>
      <SignedOut />
    </>
  );
}
