// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import {
  instanceQueryOptions,
  NotificationSettings,
  notificationSettingsQueryOptions,
  preferencesQueryOptions,
} from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/**
 * The notification settings (FR-210, ADR-0109 §1): which notifications arrive in the app, by Web Push on this device,
 * by email, and with a sound on this device. The server lists the rows the viewer gets; the preferences say whether
 * emails reach the viewer's address.
 */
export const Route = createFileRoute("/settings/notifications")({
  staticData: { title: "client:settings.notifications.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    Promise.all([
      context.queryClient.query({ ...notificationSettingsQueryOptions(context.api), staleTime: Infinity }),
      context.queryClient.query({ ...preferencesQueryOptions(context.api), staleTime: Infinity }),
      // Whether the server sends email and Web Push decides the columns, so the table does not grow them after it
      // shows; a failure is the shell's to show, and the table goes without those columns.
      context.queryClient.query({ ...instanceQueryOptions(context.api), staleTime: Infinity }).catch(() => undefined),
    ]),
  component: NotificationSettingsPage,
});

function NotificationSettingsPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.notifications.title")}</SettingsHeading>
      <NotificationSettings />
    </>
  );
}
