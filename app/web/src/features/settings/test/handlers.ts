// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the settings feature (web-client/README.md §7): the user's own profile, its badges, and
// preferences, the automatic deletion of old posts, passkeys, the user's own OAuth apps, two-factor authentication, the lists of blocks, mutes, muted domains, and follow requests, the archives of the user's
// data, the CSV exports and imports, deleting the account, and the user's own invites. Unblocking and unmuting are the account
// actions of the profile feature's handlers.
import { http, HttpResponse } from "msw";
import { preferencesFixture, relationshipFixture, type Schemas, viewerFixture } from "../../../test/fixtures";
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

/** The options of a confirmation with a passkey, which test/webauthn's browser answers. */
const passkeyConfirmationFixture = {
  options: { challenge: "Y29uZmlybQ", rpId: "social.example", userVerification: "required" },
  confirmation_token: "conf-1",
  expires_at: "2026-10-20T00:05:00Z",
};

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
