// SPDX-License-Identifier: AGPL-3.0-only
import { Stack } from "@my/ui";
import { createFileRoute } from "@tanstack/react-router";
import { BadgeSettings, badgeChoicesQueryOptions } from "../features/settings";
import { SupporterListingSettings, supporterListingQueryOptions } from "../features/supporters";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/**
 * What the profile shows besides what the profile form edits: the badges of the user's roles (ADR-0071) and, for a
 * member of a supporter plan, whether to be on the supporter list (ADR-0071 §4). Every choice here is saved as soon as
 * it changes, so the page has no save button (ADR-0110 P1: one way of saving per page).
 */
export const Route = createFileRoute("/settings/badges")({
  staticData: { title: "client:settings.badges.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    Promise.all([
      context.queryClient.query({
        ...badgeChoicesQueryOptions(context.api),
        staleTime: Infinity,
      }),
      // A user who is not a member of a plan gets a 404, which only leaves the section out.
      context.queryClient
        .query({
          ...supporterListingQueryOptions(context.api),
          staleTime: Infinity,
        })
        .catch(() => undefined),
    ]),
  component: BadgesPage,
});

function BadgesPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.badges.title")}</SettingsHeading>
      <Stack gap="8">
        <BadgeSettings />
        <SupporterListingSettings />
      </Stack>
    </>
  );
}
