// SPDX-License-Identifier: AGPL-3.0-only
import { Box, ExternalLink, RailItem, Stack } from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useStatusPageUrl } from "../../../lib/statusPage";
import { type Permission, useViewerPermissions } from "../../moderation";
import { useSupporterEntry } from "../../supporters";
import { useVoiceAvailable } from "../../voice";
import { useOwnInvitesEntry } from "../hooks/useOwnInvites";

/**
 * The pages of the settings, in the order of the menu; a page with a `permission` (a member permission, ADR-0070 §1)
 * is listed only for a viewer whose roles have it, and the page of the supporter plans only while the program is
 * enabled, outside the Tauri app (ADR-0072 §14), the voice chat history only while the server has voice chat on
 * (ADR-0100 §13), and the user's invites only while they may create invites (ADR-0108 §5).
 */
const pages: readonly {
  href: string;
  label: ClientMessageKey;
  permission?: Permission;
  supporters?: true;
  voice?: true;
  invites?: true;
}[] = [
  { href: "/settings/profile", label: "client:settings.profile.title" },
  { href: "/settings/preferences", label: "client:settings.preferences.title" },
  { href: "/settings/notifications", label: "client:settings.notifications.title" },
  { href: "/settings/password", label: "client:settings.password.title" },
  { href: "/settings/two-factor", label: "client:settings.two_factor.title" },
  { href: "/settings/passkeys", label: "client:settings.passkeys.title" },
  { href: "/settings/linked-accounts", label: "client:settings.linked_accounts.title" },
  { href: "/settings/auto-delete", label: "client:settings.auto_delete.title" },
  { href: "/settings/follow-requests", label: "client:settings.follow_requests.title" },
  { href: "/settings/blocks", label: "client:settings.blocks.title" },
  { href: "/settings/mutes", label: "client:settings.mutes.title" },
  { href: "/settings/domain-mutes", label: "client:settings.domain_mutes.title" },
  { href: "/settings/import-export", label: "client:settings.import_export.title" },
  { href: "/settings/aliases", label: "client:account_move.aliases.title" },
  { href: "/settings/move", label: "client:account_move.move.title" },
  { href: "/settings/custom-emojis", label: "client:custom_emojis.title", permission: "create_custom_emojis" },
  { href: "/settings/invites", label: "client:settings.invites.title", invites: true },
  { href: "/settings/authorized-apps", label: "client:settings.authorized_apps.title" },
  { href: "/settings/apps", label: "client:settings.apps.title" },
  { href: "/settings/supporter", label: "client:supporter.title", supporters: true },
  { href: "/settings/voice", label: "client:voice.history.title", voice: true },
  { href: "/settings/account-status", label: "client:account_status.title" },
  { href: "/settings/account", label: "client:settings.account.title" },
];

/**
 * The links to the pages of the settings, as the items of the section menu; the page the user is on is current. Below
 * them, set apart by a line, the help: the link to the instance's status page (FR-810, ADR-0085 §6), which leaves the
 * app (web-client/README.md Y12); none when the instance has no status page.
 */
export function SettingsMenu({ pathname }: { pathname: string }) {
  const t = useT();
  const permissions = useViewerPermissions();
  const supporters = useSupporterEntry();
  const voice = useVoiceAvailable();
  const invites = useOwnInvitesEntry();
  const statusPageUrl = useStatusPageUrl();
  const shown = pages.filter(
    (page) =>
      (page.permission === undefined || permissions.includes(page.permission)) &&
      (page.supporters === undefined || supporters) &&
      (page.voice === undefined || voice) &&
      (page.invites === undefined || invites),
  );
  return (
    <>
      <Stack as="ul" gap="1">
        {shown.map((page) => (
          <li key={page.href}>
            <RailItem
              form="section"
              href={page.href}
              icon={null}
              label={t(page.label)}
              current={pathname === page.href}
            />
          </li>
        ))}
      </Stack>
      {statusPageUrl !== undefined && (
        <Box marginTop="3" border="top" paddingX="3" paddingTop="3">
          <ExternalLink href={statusPageUrl} variant="standalone" size="sm">
            {t("client:settings.menu.status_page")}
          </ExternalLink>
        </Box>
      )}
    </>
  );
}
