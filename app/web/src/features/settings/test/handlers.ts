// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the settings feature (web-client/README.md §7): the user's own profile, its badges, and
// preferences, the automatic deletion of old posts and its estimate, passkeys, the user's own OAuth apps, two-factor authentication, the lists of blocks, mutes, muted domains, and follow requests, the archives of the user's
// data, the CSV exports and imports, deleting the account, and the user's own invites. Unblocking and unmuting are the account
// actions of the profile feature's handlers.
import { http, HttpResponse } from "msw";
import {
  preferencesFixture,
  relationshipFixture,
  type Schemas,
  viewerFixture,
  viewerStatusFixture,
} from "../../../test/fixtures";
import { apiBase, problem, withViewer } from "../../../test/msw/http";

/** The VAPID public key of an instance that sends Web Push (Instance.push_server_key): the example key of RFC 8291. */
export const pushServerKey = "BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8";

/** Where the viewer's two-factor authentication stands by default: off. */
export const twoFactorOffFixture: Schemas["TwoFactor"] = { enabled: false };

/** A new secret as POST /two_factor/totp/setup answers it: a 29-module QR code with one dark row. */
export const totpSetupFixture: Schemas["TotpSetup"] = {
  secret: "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP",
  uri: "otpauth://totp/social.example:alice@social.example?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&issuer=social.example",
  qr_code: { size: 29, path: "M4 4h7v1h-7z" },
  setup_token: "setup-token",
  expires_at: "2026-10-20T00:15:00Z",
};

/** The recovery codes that turning two-factor authentication on, or replacing them, answers by default. */
export const recoveryCodesFixture: Schemas["RecoveryCodes"] = {
  recovery_codes: ["abcd-efgh-ijkl-mnop", "qrst-uvwx-yz23-4567"],
};

/** The viewer's automatic deletion of old posts by default: the server's default policy, disabled. */
export const autoDeletePolicyFixture: Schemas["AutoDeletePolicy"] = {
  enabled: false,
  min_age_days: 30,
  keep_pinned: true,
  keep_bookmarked: true,
  keep_favourited: true,
  keep_media: false,
  keep_polls: false,
  keep_visibilities: ["direct"],
  keep_min_boosts: 0,
  keep_min_favourites: 0,
};

/** One of the viewer's posts that the fake estimate of the automatic deletion counts. */
type AutoDeleteCandidate = {
  id: string;
  ageDays: number;
  visibility: Schemas["Visibility"];
  pinned?: boolean;
  bookmarked?: boolean;
  favourited?: boolean;
  media?: boolean;
  poll?: boolean;
  boosts?: number;
  favourites?: number;
};

/** The viewer's posts of the fake estimate, oldest first: one for each condition of a policy, and a fresh one. */
const autoDeleteCandidates: readonly AutoDeleteCandidate[] = [
  { id: "0192f0c4-7a8e-7000-8000-00000000ad01", ageDays: 400, visibility: "public" },
  { id: "0192f0c4-7a8e-7000-8000-00000000ad02", ageDays: 300, visibility: "public", pinned: true },
  { id: "0192f0c4-7a8e-7000-8000-00000000ad03", ageDays: 200, visibility: "unlisted", bookmarked: true },
  { id: "0192f0c4-7a8e-7000-8000-00000000ad04", ageDays: 120, visibility: "public", favourited: true },
  { id: "0192f0c4-7a8e-7000-8000-00000000ad05", ageDays: 100, visibility: "public", media: true },
  { id: "0192f0c4-7a8e-7000-8000-00000000ad06", ageDays: 80, visibility: "local", poll: true },
  { id: "0192f0c4-7a8e-7000-8000-00000000ad07", ageDays: 60, visibility: "private" },
  { id: "0192f0c4-7a8e-7000-8000-00000000ad08", ageDays: 45, visibility: "direct" },
  { id: "0192f0c4-7a8e-7000-8000-00000000ad09", ageDays: 40, visibility: "public", boosts: 5 },
  { id: "0192f0c4-7a8e-7000-8000-00000000ad0a", ageDays: 35, visibility: "public", favourites: 10 },
  { id: "0192f0c4-7a8e-7000-8000-00000000ad0b", ageDays: 1, visibility: "public" },
];

/** The time the fake estimate counts the ages of posts from. */
const autoDeleteNow = Date.parse("2026-10-20T00:00:00Z");

/**
 * What POST /preferences/auto_delete/preview answers for `policy` over the fake posts, as the server counts it: the
 * posts older than the age that no exception keeps, the oldest three of them as examples.
 */
export function autoDeletePreviewOf(policy: Schemas["AutoDeletePolicy"]): Schemas["AutoDeletePreview"] {
  const thresholdKeeps = (threshold: number, count = 0) => threshold > 0 && count >= threshold;
  const deleted = autoDeleteCandidates.filter(
    (post) =>
      post.ageDays > policy.min_age_days &&
      !(policy.keep_pinned && post.pinned === true) &&
      !(policy.keep_bookmarked && post.bookmarked === true) &&
      !(policy.keep_favourited && post.favourited === true) &&
      !(policy.keep_media && post.media === true) &&
      !(policy.keep_polls && post.poll === true) &&
      !policy.keep_visibilities.includes(post.visibility) &&
      !thresholdKeeps(policy.keep_min_boosts, post.boosts) &&
      !thresholdKeeps(policy.keep_min_favourites, post.favourites),
  );
  return {
    count: deleted.length,
    more: false,
    examples: deleted.slice(0, 3).map((post) =>
      viewerStatusFixture({
        id: post.id,
        uri: `https://my.example/ap/statuses/${post.id}`,
        url: `https://my.example/@alice/status/${post.id}`,
        content: `<p>A post from ${String(post.ageDays)} days ago</p>`,
        created_at: new Date(autoDeleteNow - post.ageDays * 86_400_000).toISOString(),
        visibility: post.visibility,
      }),
    ),
  };
}

/** The options of a confirmation with a passkey, which test/webauthn's browser answers. */
const passkeyConfirmationFixture = {
  options: { challenge: "Y29uZmlybQ", rpId: "social.example", userVerification: "required" },
  confirmation_token: "conf-1",
  expires_at: "2026-10-20T00:05:00Z",
};

type NotificationRow = Schemas["NotificationSettingRow"];

/** A row of the notification settings that the user chooses in the app and by email. */
function notificationRow(
  group: Schemas["NotificationGroup"],
  section: NotificationRow["section"],
  types: Schemas["NotificationType"][],
  { inApp = true, email = false, inAppFixed = false, emailFixed = false } = {},
): NotificationRow {
  return {
    group,
    section,
    types,
    in_app: { enabled: inApp, fixed: inAppFixed },
    email: { enabled: email, fixed: emailFixed },
  };
}

/** The ordinary rows at their defaults (in the app on, email off), in the server's order. */
export const ordinaryNotificationRows: NotificationRow[] = [
  notificationRow("mention", "ordinary", ["mention"]),
  notificationRow("follow", "ordinary", ["follow"]),
  notificationRow("follow_request", "ordinary", ["follow_request"]),
  notificationRow("follow_accepted", "ordinary", ["follow_accepted"]),
  notificationRow("boost", "ordinary", ["boost"]),
  notificationRow("favourite", "ordinary", ["favourite"]),
  notificationRow("reaction", "ordinary", ["reaction"]),
  notificationRow("quote", "ordinary", ["quote"]),
  notificationRow("poll_closed", "ordinary", ["poll_closed"]),
  notificationRow("account_moved", "ordinary", ["account_moved"]),
  notificationRow("voice", "ordinary", ["voice_room_started", "voice_room_invited"]),
  // The new posts of the accounts that the user subscribes to: never by email, so the row has no email channel.
  { group: "status", section: "ordinary", types: ["status"], in_app: { enabled: true, fixed: false } },
];

/** The moderator rows of a holder of `manage_reports` and `manage_accounts`, at their defaults (both on). */
export const moderatorNotificationRows: NotificationRow[] = [
  notificationRow("admin_report", "moderator", ["admin_report"], { email: true }),
  notificationRow("admin_sign_up", "moderator", ["admin_sign_up"], { email: true }),
  notificationRow("admin_appeal", "moderator", ["admin_appeal"], { email: true }),
  notificationRow("admin_account_moved", "moderator", ["admin_account_moved"], { email: true }),
];

/** The instance's notices: always in the app but for `announcement`; `service_closure` and `supporter` always by email too. */
export const instanceNotificationRows: NotificationRow[] = [
  notificationRow(
    "moderation",
    "instance",
    ["moderation_notice", "report_resolved", "appeal_decided", "relationships_severed", "custom_emoji_request_decided"],
    { inAppFixed: true },
  ),
  notificationRow("service_closure", "instance", ["service_closure"], {
    email: true,
    inAppFixed: true,
    emailFixed: true,
  }),
  // The administrators' announcements: in the app on by default and changeable, never by email (ADR-0125 §2).
  { group: "announcement", section: "instance", types: ["announcement"], in_app: { enabled: true, fixed: false } },
];

/** The supporter row, while the supporter program is enabled. */
export const supporterNotificationRow: NotificationRow = notificationRow(
  "supporter",
  "instance",
  ["supporter_payment_failed", "supporter_subscription_ended", "supporter_plan_retired", "supporter_service_closure"],
  { email: true, inAppFixed: true, emailFixed: true },
);

/** The notification settings of a user without moderator permissions, at the defaults. */
export const notificationSettingsFixture: Schemas["NotificationSettings"] = {
  rows: [...ordinaryNotificationRows, ...instanceNotificationRows],
};

/** `settings` with `changes`, as the server saves them. */
export function applyNotificationSettingChanges(
  settings: Schemas["NotificationSettings"],
  changes: Schemas["NotificationSettingsUpdate"]["changes"],
): Schemas["NotificationSettings"] {
  return {
    rows: settings.rows.map((row) =>
      changes.reduce(
        (changed, change) =>
          change.group === row.group
            ? { ...changed, [change.channel]: { ...changed[change.channel], enabled: change.enabled } }
            : changed,
        row,
      ),
    ),
  };
}

/** The ID the default POST /invites handler gives a new invite. */
const createdInviteId = "0192f0c4-7a8e-7000-8000-0000000009a1";

export const handlers = [
  // operationIds listBadgeChoices, updateBadgeChoice: the viewer holds no role with a badge by default; the tests of
  // the badges play the server themselves.
  http.get(`${apiBase}/preferences/badges`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["BadgeChoiceList"])),
  ),
  http.put(`${apiBase}/preferences/badges/:roleId`, ({ request }) =>
    withViewer(request, () => problem(404, "about:blank", "Not Found")),
  ),
  // operationIds getAutoDeletePolicy, updateAutoDeletePolicy: the server answers the policy it saved.
  http.get(`${apiBase}/preferences/auto_delete`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(autoDeletePolicyFixture)),
  ),
  http.put(`${apiBase}/preferences/auto_delete`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["AutoDeletePolicy"];
    return withViewer(request, () => HttpResponse.json(body));
  }),
  // operationId previewAutoDeletePolicy: the estimate over the fake posts of autoDeletePreviewOf.
  http.post(`${apiBase}/preferences/auto_delete/preview`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["AutoDeletePolicy"];
    return withViewer(request, () => HttpResponse.json(autoDeletePreviewOf(body)));
  }),
  // operationId listPasskeys: no passkeys; the tests that add or remove one play the server themselves.
  http.get(`${apiBase}/passkeys`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
  // operationId listLinkedIdentities: no linked accounts and no login providers; the tests that link, unlink, or
  // confirm with one play the server themselves.
  http.get(`${apiBase}/linked_identities`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [], providers: [] } satisfies Schemas["LinkedIdentityList"])),
  ),
  // operationId listOAuthApps: no apps; the tests that register, change or delete one play the server themselves.
  http.get(`${apiBase}/oauth_apps`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
  // operationId listAuthorizedApps: no apps; the tests that revoke one play the server themselves.
  http.get(`${apiBase}/authorized_apps`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
  // operationId beginPasskeyConfirmation: the options of a confirmation with a passkey.
  http.post(`${apiBase}/passkeys/confirmation`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(passkeyConfirmationFixture satisfies Schemas["PasskeyConfirmation"])),
  ),
  // operationIds getTwoFactor, setUpTotp, enableTotp, regenerateRecoveryCodes, disableTotp.
  http.get(`${apiBase}/two_factor`, ({ request }) => withViewer(request, () => HttpResponse.json(twoFactorOffFixture))),
  http.post(`${apiBase}/two_factor/totp/setup`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(totpSetupFixture)),
  ),
  http.post(`${apiBase}/two_factor/totp`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(recoveryCodesFixture)),
  ),
  http.post(`${apiBase}/two_factor/recovery_codes`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(recoveryCodesFixture)),
  ),
  http.post(`${apiBase}/two_factor/totp/disable`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(twoFactorOffFixture)),
  ),
  // operationIds updateAccountCredentials, updatePreferences; getPreferences is with the compose handlers.
  http.patch(`${apiBase}/accounts/update_credentials`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(viewerFixture)),
  ),
  http.patch(`${apiBase}/preferences`, async ({ request }) => {
    const change = (await request.clone().json()) as Schemas["UpdatePreferencesRequest"];
    return withViewer(request, () => HttpResponse.json({ ...preferencesFixture, ...change }));
  }),
  // operationIds getNotificationSettings, updateNotificationSettings: a user without moderator permissions at the
  // defaults; a change answers the defaults with it.
  http.get(`${apiBase}/preferences/notifications`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(notificationSettingsFixture)),
  ),
  http.patch(`${apiBase}/preferences/notifications`, async ({ request }) => {
    const { changes } = (await request.clone().json()) as Schemas["NotificationSettingsUpdate"];
    return withViewer(request, () =>
      HttpResponse.json(applyNotificationSettingChanges(notificationSettingsFixture, changes)),
    );
  }),
  // operationIds getPushSubscription, putPushSubscription, deletePushSubscription: the grant has no subscription by default.
  http.get(`${apiBase}/push/subscription`, ({ request }) =>
    withViewer(request, () => problem(404, "about:blank", "Not Found")),
  ),
  http.put(`${apiBase}/push/subscription`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["PushSubscriptionRequest"];
    return withViewer(request, () =>
      HttpResponse.json({
        id: "01900000-0000-7000-8000-0000000000f1",
        endpoint: body.endpoint,
        alerts: body.alerts,
        server_key: pushServerKey,
      }),
    );
  }),
  http.delete(`${apiBase}/push/subscription`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  // operationIds listBlocks, listMutes, listFollowRequests, acceptFollowRequest, rejectFollowRequest: empty lists by default.
  http.get(`${apiBase}/blocks`, ({ request }) => withViewer(request, () => HttpResponse.json({ accounts: [] }))),
  http.get(`${apiBase}/mutes`, ({ request }) => withViewer(request, () => HttpResponse.json({ mutes: [] }))),
  // operationIds listDomainMutes, muteDomain, unmuteDomain: no muted domains by default; muting answers the domain in lower case.
  http.get(`${apiBase}/domain_mutes`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ domain_mutes: [] })),
  ),
  http.post(`${apiBase}/domain_mutes`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["MuteDomainRequest"];
    return withViewer(request, () =>
      HttpResponse.json({
        domain: body.domain.toLowerCase(),
        created_at: "2026-10-21T09:30:00Z",
      } satisfies Schemas["DomainMute"]),
    );
  }),
  http.delete(`${apiBase}/domain_mutes/:domain`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.get(`${apiBase}/follow_requests`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ accounts: [] })),
  ),
  http.post(`${apiBase}/follow_requests/:id/authorize`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json({ ...relationshipFixture(String(params["id"])), followed_by: true })),
  ),
  http.post(`${apiBase}/follow_requests/:id/reject`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(relationshipFixture(String(params["id"])))),
  ),
  // operationIds listDataExports, requestDataExport, downloadDataExport, deleteOwnAccount: no archives by default.
  http.get(`${apiBase}/exports`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
  http.post(`${apiBase}/exports`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(dataExportFixture(), { status: 202 })),
  ),
  http.get(`${apiBase}/exports/:id/archive`, ({ request }) =>
    withViewer(
      request,
      () =>
        new HttpResponse(new Blob(["PK"], { type: "application/zip" }), {
          headers: { "Content-Type": "application/zip" },
        }),
    ),
  ),
  http.post(`${apiBase}/accounts/delete`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  // operationIds getPasswordStatus, changePassword, removePassword: the user has a password, which changes and goes.
  http.get(`${apiBase}/accounts/password`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ has_password: true } satisfies Schemas["PasswordStatus"])),
  ),
  http.post(`${apiBase}/accounts/password`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.post(`${apiBase}/accounts/password/remove`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  // operationIds exportCsv, listDataImports, previewDataImport, startDataImport: one blocked account, no imports, and a file of
  // one entry.
  http.get(`${apiBase}/csv_exports/:kind`, ({ request }) =>
    withViewer(request, () => new HttpResponse("bob@remote.example\n", { headers: { "Content-Type": "text/csv" } })),
  ),
  http.get(`${apiBase}/imports`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
  http.post(`${apiBase}/imports/preview`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({ entries: 1, present: 0, removals: 0, skipped: [] } satisfies Schemas["DataImportPreview"]),
    ),
  ),
  http.post(`${apiBase}/imports`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(dataImportFixture(), { status: 202 })),
  ),
  // operationIds listOwnInvites, createOwnInvite, revokeOwnInvite: no invites, and the viewer may not create one by
  // default, so the settings do not list the page; the tests of the page answer `can_create` themselves.
  http.get(`${apiBase}/invites`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(ownInvitePageFixture([], { can_create: false }))),
  ),
  http.post(`${apiBase}/invites`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["CreateOwnInviteRequest"];
    return withViewer(request, () =>
      HttpResponse.json(
        ownInviteFixture(createdInviteId, "NEWINVITECODE234", {
          max_uses: body.max_uses ?? 1,
          expires_at: "2026-10-23T00:00:00Z",
        }),
      ),
    );
  }),
  http.post(`${apiBase}/invites/:id/revoke`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json(ownInviteFixture(String(params["id"]), "ABCDEFGHJKLMNPQR", { state: "revoked" })),
    ),
  ),
];

/** An invite of the user's own with `code`, usable, for one sign-up within a week from 2026-10-16, with `overrides`. */
export function ownInviteFixture(
  id: string,
  code: string,
  overrides: Partial<Schemas["OwnInvite"]> = {},
): Schemas["OwnInvite"] {
  return {
    id,
    code,
    url: `https://my.example/auth/sign_up?invite=${code}`,
    max_uses: 1,
    uses: 0,
    state: "usable",
    expires_at: "2026-10-23T00:00:00Z",
    created_at: "2026-10-16T00:00:00Z",
    ...overrides,
  };
}

/**
 * A page of the user's invites: `items`, the defaults of the instance settings as the caps, and a usable count of the
 * usable `items`; `overrides` replace the rest.
 */
export function ownInvitePageFixture(
  items: Schemas["OwnInvite"][],
  overrides: Partial<Omit<Schemas["OwnInvitePage"], "items">> = {},
): Schemas["OwnInvitePage"] {
  return {
    items,
    can_create: true,
    usable: items.filter((invite) => invite.state === "usable").length,
    limits: {
      max_active: 5,
      max_uses: 10,
      max_expires_in: 30 * 86_400,
      default_max_uses: 1,
      default_expires_in: 7 * 86_400,
    },
    ...overrides,
  };
}

/** An import of a CSV file; following, merged, and in progress unless `change` says otherwise. */
export function dataImportFixture(change: Partial<Schemas["DataImport"]> = {}): Schemas["DataImport"] {
  return {
    id: "0192f0c4-7a8e-7000-8000-0000000009a1",
    kind: "following",
    mode: "merge",
    state: "in_progress",
    created_at: "2026-10-21T09:30:00Z",
    total: 1,
    processed: 0,
    failed: 0,
    ...change,
  };
}

/** An archive of the user's data; pending unless `change` says otherwise. */
export function dataExportFixture(change: Partial<Schemas["DataExport"]> = {}): Schemas["DataExport"] {
  return {
    id: "0192f0c4-7a8e-7000-8000-0000000008e1",
    state: "pending",
    requested_at: "2026-10-16T09:30:00Z",
    ...change,
  };
}
