// SPDX-License-Identifier: AGPL-3.0-only
import {
  AccountStatusIcon,
  AliasIcon,
  ArchiveIcon,
  AuthorizedAppIcon,
  AutoDeleteIcon,
  BadgeIcon,
  BlockIcon,
  Box,
  CustomEmojiIcon,
  DeveloperIcon,
  DisplayIcon,
  DomainMuteIcon,
  ExternalLink,
  FollowRequestIcon,
  ImportExportIcon,
  InviteIcon,
  LinkedAccountIcon,
  MoveAccountIcon,
  MuteIcon,
  NavMenu,
  NotificationsIcon,
  PasskeyIcon,
  PasswordIcon,
  ProfileIcon,
  SupporterIcon,
  TwoFactorIcon,
  VoiceIcon,
} from "@my/ui";
import type { ComponentType } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useStatusPageUrl } from "../../../lib/statusPage";
import { type Permission, useViewerPermissions } from "../../moderation";
import { useSupporterEntry } from "../../supporters";
import { useVoiceAvailable } from "../../voice";
import { useFollowRequestCount } from "../hooks/useFollowRequestCount";
import { useOwnInvitesEntry } from "../hooks/useOwnInvites";

/**
 * One page of the settings; a page with a `permission` (a member permission, ADR-0070 §1) is listed only for a viewer
 * whose roles have it, and the page of the supporter plans only while the program is enabled, outside the Tauri app
 * (ADR-0072 §14), the voice chat history only while the server has voice chat on (ADR-0100 §13), and the user's
 * invites only while they may create invites (ADR-0108 §5).
 */
type SettingsPage = {
  href: string;
  label: ClientMessageKey;
  icon: ComponentType;
  permission?: Permission;
  supporters?: true;
  voice?: true;
  invites?: true;
};

/**
 * The pages of the settings in the groups of the menu, in their order (ADR-0110 §5): every page has an icon, and a
 * group has at most seven pages (the test setup checks both on every menu a test renders).
 */
const groups: readonly { id: string; heading: ClientMessageKey; pages: readonly SettingsPage[] }[] = [
  {
    id: "profile",
    heading: "client:settings.menu.group.profile",
    pages: [
      { href: "/settings/profile", label: "client:settings.profile.title", icon: ProfileIcon },
      { href: "/settings/badges", label: "client:settings.badges.title", icon: BadgeIcon },
      {
        href: "/settings/custom-emojis",
        label: "client:custom_emojis.title",
        icon: CustomEmojiIcon,
        permission: "create_custom_emojis",
      },
      { href: "/settings/invites", label: "client:settings.invites.title", icon: InviteIcon, invites: true },
      { href: "/settings/supporter", label: "client:supporter.title", icon: SupporterIcon, supporters: true },
    ],
  },
  {
    id: "display",
    heading: "client:settings.menu.group.display",
    pages: [
      { href: "/settings/preferences", label: "client:settings.preferences.title", icon: DisplayIcon },
      { href: "/settings/notifications", label: "client:settings.notifications.title", icon: NotificationsIcon },
      { href: "/settings/voice", label: "client:voice.history.title", icon: VoiceIcon, voice: true },
    ],
  },
  {
    id: "relationships",
    heading: "client:settings.menu.group.relationships",
    pages: [
      { href: "/settings/follow-requests", label: "client:settings.follow_requests.title", icon: FollowRequestIcon },
      { href: "/settings/blocks", label: "client:settings.blocks.title", icon: BlockIcon },
      { href: "/settings/mutes", label: "client:settings.mutes.title", icon: MuteIcon },
      { href: "/settings/domain-mutes", label: "client:settings.domain_mutes.title", icon: DomainMuteIcon },
    ],
  },
  {
    id: "security",
    heading: "client:settings.menu.group.security",
    pages: [
      { href: "/settings/password", label: "client:settings.password.title", icon: PasswordIcon },
      { href: "/settings/two-factor", label: "client:settings.two_factor.title", icon: TwoFactorIcon },
      { href: "/settings/passkeys", label: "client:settings.passkeys.title", icon: PasskeyIcon },
      { href: "/settings/linked-accounts", label: "client:settings.linked_accounts.title", icon: LinkedAccountIcon },
    ],
  },
  {
    id: "apps",
    heading: "client:settings.menu.group.apps",
    pages: [
      { href: "/settings/authorized-apps", label: "client:settings.authorized_apps.title", icon: AuthorizedAppIcon },
      { href: "/settings/apps", label: "client:settings.apps.title", icon: DeveloperIcon },
    ],
  },
  {
    id: "account",
    heading: "client:settings.menu.group.account",
    pages: [
      { href: "/settings/account-status", label: "client:account_status.title", icon: AccountStatusIcon },
      { href: "/settings/aliases", label: "client:account_move.aliases.title", icon: AliasIcon },
      { href: "/settings/move", label: "client:account_move.move.title", icon: MoveAccountIcon },
      { href: "/settings/import-export", label: "client:settings.import_export.title", icon: ImportExportIcon },
      { href: "/settings/auto-delete", label: "client:settings.auto_delete.title", icon: AutoDeleteIcon },
      { href: "/settings/account", label: "client:settings.account.title", icon: ArchiveIcon },
    ],
  },
];

/**
 * The links to the pages of the settings, in named groups (`NavMenu`); the page the user is on is current, and the
 * follow requests show how many await the user's approval. Below
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
  const followRequests = useFollowRequestCount();
  const shown = (page: SettingsPage) =>
    (page.permission === undefined || permissions.includes(page.permission)) &&
    (page.supporters === undefined || supporters) &&
    (page.voice === undefined || voice) &&
    (page.invites === undefined || invites);
  return (
    <>
      <NavMenu
        groups={groups.map((group) => ({
          id: group.id,
          heading: t(group.heading),
          items: group.pages.filter(shown).map(({ href, label, icon: Icon }) => {
            const name = t(label);
            // What awaits the user there: the follow requests to approve, none at zero.
            const count = href === "/settings/follow-requests" ? followRequests : 0;
            const waiting = t("client:settings.follow_requests.waiting", { count });
            return {
              href,
              icon: <Icon />,
              label: name,
              current: pathname === href,
              ...(count > 0 && { badge: { count, label: waiting, name: `${name} ${waiting}` } }),
            };
          }),
        }))}
      />
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
