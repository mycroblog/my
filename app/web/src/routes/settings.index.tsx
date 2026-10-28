// SPDX-License-Identifier: AGPL-3.0-only
import { Stack } from "@my/ui";
import { createFileRoute } from "@tanstack/react-router";
import { DocumentsSection } from "../features/documents";
import { AccountSection } from "../features/session";
import { SecuritySummary } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { StaffModeEntry } from "./-layout/StaffModeEntry";

/**
 * The settings, in the sections of their screen requirements (web-client/screens/settings.md): the security of the
 * account in one glance, with a link to each of its pages; for a viewer with staff permissions, the one entry into the
 * admin and moderation mode (ADR-0058); the signed-in account with signing out; and the links to the server's terms of
 * service and privacy policy. The menu of the settings pages is the frame's (routes/settings.tsx).
 */
export const Route = createFileRoute("/settings/")({
  staticData: { title: "client:settings.title", hideHeading: true },
  // Every setting is the user's own, so the page needs a session (web-client/10-api-client-and-auth.md §10).
  beforeLoad: requireSignIn,
  component: SettingsPage,
});

function SettingsPage() {
  return (
    <Stack gap="8">
      <SecuritySummary />
      <StaffModeEntry />
      <AccountSection />
      <DocumentsSection />
    </Stack>
  );
}
