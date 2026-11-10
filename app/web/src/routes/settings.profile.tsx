// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { featuredTagsQueryOptions } from "../features/hashtags";
import { ProfileForm, credentialsQueryOptions } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/**
 * Editing the signed-in user's profile (FR-006), saved together by the form's button. The badges and the supporter
 * list, saved as soon as they change, have their own page (`/settings/badges`; ADR-0110 P1).
 */
export const Route = createFileRoute("/settings/profile")({
  staticData: { title: "client:settings.profile.title" },
  beforeLoad: requireSignIn,
  // The profile and the hashtags it features, which the form saves with it (ADR-0006 §17).
  loader: ({ context }) =>
    Promise.all([
      context.queryClient.query({ ...credentialsQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...featuredTagsQueryOptions(context.api), staleTime: Infinity }),
    ]),
  component: ProfilePage,
});

function ProfilePage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.profile.title")}</SettingsHeading>
      <ProfileForm />
    </>
  );
}
