// SPDX-License-Identifier: AGPL-3.0-only
// The settings feature (FR-002, FR-004, FR-005, FR-006, FR-007, FR-008, FR-118, FR-208, FR-009, FR-302, FR-303): the profile editor, the preferences, changing the password, passkeys, two-factor
// authentication, the automatic deletion of old posts, the lists of blocked and muted accounts and of follow requests, the archives of the user's data and deleting
// the account, the follows, blocks, mutes, and bookmarks as Mastodon's CSV files, the aliases and moving the account, displaying the app in the user's locale
// and time zone, the color theme of this browser, and the arrangement of the navigation's items on this browser. Routes and other features use it through this file only (web-client/README.md A3).
export { dataExportsQueryOptions } from "./api/account";
export { dataImportsQueryOptions } from "./api/importExport";
export { credentialsQueryOptions, preferencesQueryOptions } from "./api/me";
// The badges of the user's roles on their profile (ADR-0071).
export { badgeChoicesQueryOptions } from "./api/badges";
export { BadgeSettings } from "./components/BadgeSettings";
export { blocksQueryOptions, followRequestsQueryOptions, mutesQueryOptions } from "./api/relationships";
export { BlockedAccounts, FollowRequests, MutedAccounts } from "./components/AccountLists";
// The domains the user mutes (FR-304).
export { domainMutesQueryOptions } from "./api/domainMutes";
export { DomainMutes } from "./components/DomainMutes";
export { CsvExportsSection, ImportExport } from "./components/ImportExport";
export { DataExportsSection, OwnAccount } from "./components/OwnAccount";
export { PreferencesForm } from "./components/PreferencesForm";
// The notification settings: in the app, Web Push on this device, email, and the sound of this device (FR-210, ADR-0109).
export { notificationSettingsQueryOptions } from "./api/notificationSettings";
export { instanceQueryOptions } from "./api/me";
export { NotificationSettings } from "./components/NotificationSettings";
export { ProfileForm } from "./components/ProfileForm";
export { SettingsMenu } from "./components/SettingsMenu";
// The navigation's items as the user arranges them on this browser, which the app's navigation follows.
export { navigationEntryIcons } from "./components/navigationEntryIcons";
export {
  type NavigationEntryId,
  navigationEntryLabels,
  tabBarLimit,
  useNavigationEntries,
} from "./hooks/useNavigationSettings";
// The automatic deletion of the user's own old posts (FR-118).
export { autoDeletePolicyQueryOptions } from "./api/autoDelete";
export { AutoDeleteSettings } from "./components/AutoDeleteSettings";
// Two-factor authentication with an authenticator app (FR-005).
export { twoFactorQueryOptions } from "./api/twoFactor";
export { TwoFactorSettings } from "./components/TwoFactorSettings";
// Changing the password (FR-002), and removing it or setting it again for a user with passkeys (FR-004).
export { passwordStatusQueryOptions } from "./api/password";
export { PasswordSettings } from "./components/PasswordSettings";
// The security of the account in one glance on the settings page: the password, two-factor, passkeys, linked accounts.
export { SecuritySummary } from "./components/SecuritySummary";
// Passkeys, which sign in instead of the password (FR-004).
export { passkeysQueryOptions } from "./api/passkeys";
export { PasskeySettings } from "./components/PasskeySettings";
// The accounts at the login providers that the administrators set up, which sign in and confirm operations (FR-010).
export { linkedIdentitiesQueryOptions } from "./api/linkedIdentities";
export { LinkedAccountSettings } from "./components/LinkedAccountSettings";
// The user's own OAuth apps: registering, secrets, and tokens issued to the user (ADR-0090).
export { oauthAppsQueryOptions } from "./api/oauthApps";
export { OAuthAppSettings } from "./components/OAuthAppSettings";
// The apps the user authorized, and revoking them (ADR-0090 §5).
export { authorizedAppsQueryOptions } from "./api/authorizedApps";
export { AuthorizedApps } from "./components/AuthorizedApps";
// The aliases and moving the account to another account (FR-009, ADR-0089).
export { aliasesQueryOptions } from "./api/aliases";
export { AliasSettings } from "./components/AliasSettings";
export { AccountMoveSettings } from "./components/AccountMoveSettings";
export { useUserDisplaySettings } from "./hooks/useUserDisplaySettings";
// Uploading an image and waiting for the server to process it, which the admin screen of custom emoji shares.
export { acceptedImageTypes, type MediaAttachment, MediaProcessingError, uploadImage } from "./api/media";
// Asking before leaving a form with unsaved changes (web-client/09-app-shell.md §9), which the admin screens' forms share.
export { useUnsavedChangesGuard } from "./hooks/useUnsavedChangesGuard";
// The user's own invites (FR-011, ADR-0108 §5).
export { ownInvitesQueryOptions } from "./api/invites";
export { OwnInvites } from "./components/OwnInvites";
