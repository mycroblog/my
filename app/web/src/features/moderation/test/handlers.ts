// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the moderation feature (web-client/README.md §7). On the user's side: the actions on the
// user's own account (none by default) and appealing one. On the staff's side: the health of remote servers, the
// domain actions, the sign-up blocks, the audit log, the registrations awaiting approval, the reports, the
// appeals, the accounts and their roles, all empty and every server available by default. The default viewer
// has no permissions, so the staff screens' tests start with `server.use(operatorHandler(...))` (test/msw/http.ts).
import { http, HttpResponse } from "msw";
import {
  otherAccountFixture,
  remoteAccountFixture,
  type Schemas,
  statusFixture,
  viewerFixture,
} from "../../../test/fixtures";
import { apiBase, withViewer } from "../../../test/msw/http";

/** A moderation action on the viewer's account: a suspension in effect, taken on 16 October 2026, appealable. */
export function ownActionFixture(overrides: Partial<Schemas["ModerationAction"]> = {}): Schemas["ModerationAction"] {
  return {
    id: "0192f0c4-7a8e-7000-8000-0000000007a1",
    kind: "suspend",
    kind_label: "Suspended",
    reason_category: "violation",
    reason_category_label: "Rule violation",
    public_reason: "Repeated spam in replies.",
    status_ids: [],
    state: "active",
    issued_at: "2026-10-16T09:30:00Z",
    expires_at: "2026-11-15T09:30:00Z",
    appealable: true,
    appeal_deadline: "2027-04-14T09:30:00Z",
    ...overrides,
  };
}

/** The health of a server without failures, as GET /admin/instances/{domain} answers for it. */
export function availableHealth(domain: string): Schemas["AdminInstanceHealth"] {
  return { domain, state: "available", failure_days: 0 };
}

/** A domain action on `domain` in effect since 2026-10-10, as the server answers it; `overrides` replace its fields. */
export function domainActionFixture(
  overrides: Partial<Schemas["AdminDomainAction"]> = {},
): Schemas["AdminDomainAction"] {
  return {
    id: "0192f0c4-7a8e-7000-8000-0000000000a1",
    domain: "spam.example",
    kind: "limit",
    obfuscate: false,
    state: "active",
    issued_at: "2026-10-10T08:00:00Z",
    ...overrides,
  };
}

/** The action that POST /admin/domain_actions records for `body`, taken at 2026-10-17T00:00:00Z. */
export function newDomainAction(body: Schemas["CreateDomainActionRequest"]): Schemas["AdminDomainAction"] {
  const issuedAt = "2026-10-17T00:00:00Z";
  return {
    ...domainActionFixture({
      id: "0192f0c4-7a8e-7000-8000-0000000000af",
      domain: body.domain,
      kind: body.kind,
      obfuscate: body.obfuscate,
      issued_at: issuedAt,
    }),
    ...(body.public_comment === undefined ? {} : { public_comment: body.public_comment }),
    ...(body.private_note === undefined ? {} : { private_note: body.private_note }),
    ...(body.expires_in === undefined
      ? {}
      : { expires_at: new Date(Date.parse(issuedAt) + body.expires_in * 1000).toISOString() }),
  };
}

/** The header of Mastodon's domain block CSV. */
export const domainBlockCsvHeader = "#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate";

/** The kinds of action a block stands for, as the server issues them: the severity first, then the rejections. */
function domainBlockKinds(block: Schemas["DomainBlock"]): Schemas["AdminDomainActionKind"][] {
  const severity: Schemas["AdminDomainActionKind"][] =
    block.severity === "suspend" ? ["suspend"] : block.severity === "silence" ? ["limit"] : [];
  return [
    ...severity,
    ...(block.reject_media ? (["reject_media"] as const) : []),
    ...(block.reject_reports ? (["reject_reports"] as const) : []),
  ];
}

/**
 * The preview of a CSV of simple rows (no quoted field), as POST /admin/domain_actions/import/preview answers it with
 * no action in effect: every row with a severity of Mastodon's is importable, unless it takes no action.
 */
export function previewDomainBlocks(csv: string): Schemas["DomainBlockImportPreview"] {
  const rows: Schemas["DomainBlockImportRow"][] = [];
  csv.split("\n").forEach((line, index) => {
    const [domain = "", severity = "suspend", media = "", reports = "", comment = "", obfuscate = ""] = line
      .split(",")
      .map((field) => field.trim());
    if (domain === "" || domain === "#domain") return;
    const row = { line: index + 1, written_domain: domain };
    if (severity !== "suspend" && severity !== "silence" && severity !== "noop") {
      rows.push({ ...row, status: "invalid", issue: [] });
      return;
    }
    const block: Schemas["DomainBlock"] = {
      domain,
      severity,
      reject_media: media === "true",
      reject_reports: reports === "true",
      obfuscate: obfuscate === "true",
      ...(comment === "" ? {} : { public_comment: comment }),
    };
    const issue = domainBlockKinds(block);
    rows.push({ ...row, block, status: issue.length === 0 ? "no_action" : "importable", issue });
  });
  return { rows };
}

/** What POST /admin/domain_actions/import does for `body`: every action of every block is taken. */
export function importDomainBlocks(body: Schemas["ImportDomainBlocksRequest"]): Schemas["DomainBlockImportResult"] {
  const issued = body.blocks.flatMap((block) =>
    domainBlockKinds(block).map((kind) =>
      newDomainAction({
        domain: block.domain,
        kind,
        obfuscate: block.obfuscate,
        ...(block.public_comment === undefined ? {} : { public_comment: block.public_comment }),
        ...(body.private_note === undefined ? {} : { private_note: body.private_note }),
      }),
    ),
  );
  return { issued, skipped: [] };
}

/** A sign-up block on the email domain `mail.example` in effect since 2026-10-10, as the server answers it; `overrides` replace its fields. */
export function signupBlockFixture(overrides: Partial<Schemas["AdminSignupBlock"]> = {}): Schemas["AdminSignupBlock"] {
  return {
    id: "0192f0c4-7a8e-7000-8000-0000000001a1",
    kind: "email_domain",
    value: "mail.example",
    severity: "reject",
    state: "active",
    created_at: "2026-10-10T08:00:00Z",
    ...overrides,
  };
}

/** The block that POST /admin/signup_blocks records for `body`, added at 2026-10-17T00:00:00Z; the value is kept as sent. */
export function newSignupBlock(body: Schemas["CreateSignupBlockRequest"]): Schemas["AdminSignupBlock"] {
  const createdAt = "2026-10-17T00:00:00Z";
  return {
    ...signupBlockFixture({
      id: "0192f0c4-7a8e-7000-8000-0000000001af",
      kind: body.kind,
      value: body.value,
      severity: body.severity,
      created_at: createdAt,
    }),
    ...(body.comment === undefined ? {} : { comment: body.comment }),
    ...(body.expires_in === undefined
      ? {}
      : { expires_at: new Date(Date.parse(createdAt) + body.expires_in * 1000).toISOString() }),
  };
}

/** A usable invite for one sign-up, created at 2026-10-16 and expiring a week later, as the server answers it; `overrides` replace its fields. */
export function inviteFixture(overrides: Partial<Schemas["AdminInvite"]> = {}): Schemas["AdminInvite"] {
  const code = overrides.code ?? "ABCDEFGHJKLMNPQR";
  return {
    id: "0192f0c4-7a8e-7000-8000-0000000006a1",
    code,
    url: `https://my.example/auth/sign_up?invite=${code}`,
    max_uses: 1,
    uses: 0,
    state: "usable",
    creator_kind: "staff",
    created_by_deleted: false,
    expires_at: "2026-10-23T00:00:00Z",
    created_at: "2026-10-16T00:00:00Z",
    ...overrides,
  };
}

/** The invite that POST /admin/invites creates for `body`, at 2026-10-17T00:00:00Z. */
export function newInvite(body: Schemas["CreateInviteRequest"]): Schemas["AdminInvite"] {
  const createdAt = "2026-10-17T00:00:00Z";
  const {
    max_uses: _,
    expires_at: __,
    ...invite
  } = inviteFixture({ id: "0192f0c4-7a8e-7000-8000-0000000006af", code: "NEWCODE234567ABC", created_at: createdAt });
  return {
    ...invite,
    ...(body.max_uses === undefined ? {} : { max_uses: body.max_uses }),
    ...(body.expires_in === undefined
      ? {}
      : { expires_at: new Date(Date.parse(createdAt) + body.expires_in * 1000).toISOString() }),
  };
}

/** A registration of `bob` awaiting approval since 2026-10-16, as the server lists it; `overrides` replace its fields. */
export function registrationFixture(
  overrides: Partial<Schemas["AdminRegistration"]> = {},
): Schemas["AdminRegistration"] {
  return {
    user_id: "0192f0c4-7a8e-7000-8000-0000000000b1",
    account_id: "0192f0c4-7a8e-7000-8000-0000000000c1",
    username: "bob",
    state: "pending",
    since: "2026-10-16T09:00:00Z",
    email: "bob@example.com",
    email_verified: true,
    reason: "I would like to post about birds.",
    submitted_at: "2026-10-16T09:00:00Z",
    ...overrides,
  };
}

/** GET /admin/counts answering `counts`, the counts that the viewer's permissions allow. */
export function countsHandler(counts: Schemas["AdminCounts"]) {
  return http.get(`${apiBase}/admin/counts`, ({ request }) => withViewer(request, () => HttpResponse.json(counts)));
}

/**
 * An open report by Carol of another server on Bob, filed on 2026-10-16 with one post and no notes, as the server
 * answers it; `overrides` replace its fields.
 */
export function reportFixture(overrides: Partial<Schemas["AdminReportDetail"]> = {}): Schemas["AdminReportDetail"] {
  const statusIds = overrides.status_ids ?? ["0192f0c4-7a8e-7000-8000-0000000000b1"];
  return {
    id: "0192f0c4-7a8e-7000-8000-0000000c0001",
    category: "spam",
    comment: "Posts the same link again and again.",
    forwarded: false,
    filed_at: "2026-10-16T09:00:00Z",
    state: "open",
    state_changed_at: "2026-10-16T09:00:00Z",
    reporter_account_id: remoteAccountFixture.id,
    reporter_account: remoteAccountFixture,
    target_account_id: otherAccountFixture.id,
    target_account: otherAccountFixture,
    status_ids: statusIds,
    rule_ids: [],
    statuses: statusIds.map((id) => ({ status: statusFixture(id) })),
    notes: [],
    target_history: { reports_count: 0, actions_count: 0 },
    ...overrides,
  };
}

/** The action that POST /admin/accounts/{id}/actions records for `body` on account `accountId`, taken at 2026-10-17T00:00:00Z. */
export function newAccountAction(
  accountId: string,
  body: Schemas["CreateAccountActionRequest"],
): Schemas["AdminAccountAction"] {
  const issuedAt = "2026-10-17T00:00:00Z";
  return {
    id: "0192f0c4-7a8e-7000-8000-0000000d0001",
    account_id: accountId,
    kind: body.kind,
    reason_category: body.reason_category,
    issuer: "moderator",
    issued_by_account_id: viewerFixture.id,
    notify_target: body.notify_target,
    state: "active",
    issued_at: issuedAt,
    ...(body.public_reason === undefined ? {} : { public_reason: body.public_reason }),
    ...(body.private_note === undefined ? {} : { private_note: body.private_note }),
    ...(body.report_id === undefined ? {} : { report_id: body.report_id }),
    ...(body.status_ids === undefined ? {} : { status_ids: body.status_ids }),
    ...(body.expires_in === undefined
      ? {}
      : { expires_at: new Date(Date.parse(issuedAt) + body.expires_in * 1000).toISOString() }),
  };
}

/** `account` as the moderators see it: active, without restrictions, local when it is on this server. */
export function adminAccountFixture(
  account: Schemas["Account"],
  overrides: Partial<Schemas["AdminAccount"]> = {},
): Schemas["AdminAccount"] {
  const local = !account.acct.includes("@");
  return {
    id: account.id,
    account,
    local,
    state: "active",
    restrictions: [],
    ...(local ? { registration_state: "approved" } : {}),
    ...overrides,
  };
}

/**
 * The details of `account` as GET /admin/accounts/{id} answers them: a local one registered on 2026-10-01 with
 * a verified email address, no two-factor authentication, and no roles, actions or reports; `overrides` replace its fields.
 */
export function adminAccountDetailFixture(
  account: Schemas["Account"],
  overrides: Partial<Schemas["AdminAccountDetail"]> = {},
): Schemas["AdminAccountDetail"] {
  const summary = adminAccountFixture(account);
  return {
    ...summary,
    ...(summary.local
      ? {
          registration: { submitted_at: "2026-10-01T00:00:00Z", state_changed_at: "2026-10-01T01:00:00Z" },
          email_verified: true,
          two_factor_enabled: false,
        }
      : {}),
    roles: [],
    actions: [],
    reports: [],
    ...overrides,
  };
}

/** What POST /admin/accounts/{id}/reveal answers by default: an email address, no IP addresses kept. */
export const privateDataFixture: Schemas["AdminAccountPrivateData"] = {
  email: "bob@example.com",
  email_verified: true,
  sign_ins: [],
  same_ip_accounts: [],
};

/** A moderation action `kind` on Bob in effect since 2026-10-10, as the server answers it; `overrides` replace its fields. */
export function accountActionFixture(
  overrides: Partial<Schemas["AdminAccountAction"]> = {},
): Schemas["AdminAccountAction"] {
  return {
    id: "0192f0c4-7a8e-7000-8000-0000000d00a1",
    account_id: otherAccountFixture.id,
    kind: "limit",
    reason_category: "spam",
    issuer: "moderator",
    issued_by_account_id: viewerFixture.id,
    notify_target: true,
    state: "active",
    issued_at: "2026-10-10T08:00:00Z",
    ...overrides,
  };
}

/**
 * A pending appeal by Bob, made on 2026-10-16, of an active suspension that a moderator took on 2026-10-15, as
 * the server answers it; `overrides` replace its fields.
 */
export function appealFixture(overrides: Partial<Schemas["AdminAppeal"]> = {}): Schemas["AdminAppeal"] {
  return {
    id: "0192f0c4-7a8e-7000-8000-0000000f0001",
    account_id: otherAccountFixture.id,
    text: "I posted the link once, to my own blog.",
    state: "pending",
    submitted_at: "2026-10-16T09:00:00Z",
    action: {
      id: "0192f0c4-7a8e-7000-8000-0000000d0101",
      account_id: otherAccountFixture.id,
      kind: "suspend",
      reason_category: "spam",
      public_reason: "Repeated advertising.",
      private_note: "Third report this week.",
      issuer: "moderator",
      issued_by_account_id: viewerFixture.id,
      notify_target: true,
      state: "active",
      issued_at: "2026-10-15T09:00:00Z",
    },
    ...overrides,
  };
}

/** The appeal `appeal` after `decision` at 2026-10-17T00:00:00Z by the viewer; an overturn lifts an active action. */
export function decidedAppeal(
  appeal: Schemas["AdminAppeal"],
  decision: "uphold" | "overturn",
  reason: string | undefined,
): Schemas["AdminAppeal"] {
  const decidedAt = "2026-10-17T00:00:00Z";
  const action =
    decision === "overturn" && appeal.action.state === "active"
      ? { ...appeal.action, state: "lifted" as const, lifted_at: decidedAt }
      : appeal.action;
  return {
    ...appeal,
    state: decision === "uphold" ? "upheld" : "overturned",
    decided_at: decidedAt,
    decided_by_account_id: viewerFixture.id,
    action,
    ...(reason === undefined ? {} : { decision_reason: reason }),
  };
}

/** The roles that every instance has (GET /admin/roles), highest first: admin with every permission, and moderator. */
export const adminRoleFixture: Schemas["AdminRole"] = {
  id: "0192f0c4-7a8e-7000-8000-000000000a01",
  key: "admin",
  position: 1000,
  permissions: [
    "manage_reports",
    "manage_accounts",
    "manage_domains",
    "manage_rules",
    "manage_settings",
    "manage_roles",
    "manage_invites",
    "view_audit",
    "manage_jobs",
    "manage_login_recovery",
    "manage_custom_emojis",
    "manage_webhooks",
    "manage_announcements",
    "manage_finance",
    "manage_supporters",
  ],
};
export const moderatorRoleFixture: Schemas["AdminRole"] = {
  id: "0192f0c4-7a8e-7000-8000-000000000a02",
  key: "moderator",
  position: 100,
  permissions: ["manage_reports", "manage_accounts", "view_audit"],
};
export const rolesFixture: Schemas["AdminRole"][] = [adminRoleFixture, moderatorRoleFixture];

/** Whether a path segment is an ID of the API (a UUID), not the name of another operation under the same path. */
function isId(segment: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(segment);
}

export const handlers = [
  // operationIds listOwnModerationActions, appealModerationAction.
  http.get(`${apiBase}/moderation/actions`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] })),
  ),
  http.post(`${apiBase}/moderation/actions/:id/appeal`, async ({ request, params }) => {
    const { text } = (await request.clone().json()) as Schemas["AppealModerationActionRequest"];
    return withViewer(request, () =>
      HttpResponse.json(
        ownActionFixture({
          id: String(params["id"]),
          appealable: false,
          appeal: {
            id: "0192f0c4-7a8e-7000-8000-0000000007b1",
            text,
            state: "pending",
            submitted_at: "2026-10-17T00:00:00Z",
          },
        }),
      ),
    );
  }),
  // operationId getAdminCounts: nothing awaits the moderators.
  countsHandler({}),
  // operationIds listInstanceHealth, getInstanceHealth, listInstanceAvailabilityEvents, markInstanceAvailable.
  http.get(`${apiBase}/admin/instances`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
  http.get(`${apiBase}/admin/instances/:domain`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(availableHealth(String(params["domain"])))),
  ),
  http.get(`${apiBase}/admin/instances/:domain/availability_events`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] })),
  ),
  http.post(`${apiBase}/admin/instances/:domain/mark_available`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(availableHealth(String(params["domain"])))),
  ),
  // operationIds refetchStatus, refetchAccountStatuses, refetchInstanceStatuses: every refetch is queued.
  http.post(`${apiBase}/admin/statuses/:id/refetch`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.post(`${apiBase}/admin/accounts/:id/refetch_statuses`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.post(`${apiBase}/admin/instances/:domain/refetch_statuses`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  // operationIds listDomainActions, createDomainAction, getDomainAction, liftDomainAction. A new action keeps the
  // domain as sent (the real server converts U-labels to A-labels).
  http.get(`${apiBase}/admin/domain_actions`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] })),
  ),
  http.post(`${apiBase}/admin/domain_actions`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["CreateDomainActionRequest"];
    return withViewer(request, () => HttpResponse.json(newDomainAction(body)));
  }),
  // operationIds exportDomainBlocks, previewDomainBlockImport, importDomainBlocks, before the routes of one action
  // (whose `:id` would match them): no action is exported; every row of a CSV is imported, as written.
  http.get(`${apiBase}/admin/domain_actions/export`, ({ request }) =>
    withViewer(
      request,
      () => new HttpResponse(`${domainBlockCsvHeader}\n`, { headers: { "Content-Type": "text/csv" } }),
    ),
  ),
  http.post(`${apiBase}/admin/domain_actions/import/preview`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["PreviewDomainBlockImportRequest"];
    return withViewer(request, () => HttpResponse.json(previewDomainBlocks(body.csv)));
  }),
  http.post(`${apiBase}/admin/domain_actions/import`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["ImportDomainBlocksRequest"];
    return withViewer(request, () => HttpResponse.json(importDomainBlocks(body)));
  }),
  http.get(`${apiBase}/admin/domain_actions/:id`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(domainActionFixture({ id: String(params["id"]) }))),
  ),
  http.post(`${apiBase}/admin/domain_actions/:id/lift`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json(
        domainActionFixture({ id: String(params["id"]), state: "lifted", lifted_at: "2026-10-17T00:00:00Z" }),
      ),
    ),
  ),
  // operationIds listSignupBlocks, createSignupBlock, getSignupBlock, liftSignupBlock. A new block keeps the value
  // as sent (the real server keeps it in canonical form).
  http.get(`${apiBase}/admin/signup_blocks`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] })),
  ),
  http.post(`${apiBase}/admin/signup_blocks`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["CreateSignupBlockRequest"];
    return withViewer(request, () => HttpResponse.json(newSignupBlock(body)));
  }),
  http.get(`${apiBase}/admin/signup_blocks/:id`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(signupBlockFixture({ id: String(params["id"]) }))),
  ),
  http.post(`${apiBase}/admin/signup_blocks/:id/lift`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json(
        signupBlockFixture({ id: String(params["id"]), state: "lifted", lifted_at: "2026-10-17T00:00:00Z" }),
      ),
    ),
  ),
  // operationId importSignupBlocks: every value of the list is added.
  http.post(`${apiBase}/admin/signup_blocks/import`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["ImportSignupBlocksRequest"];
    const added = body.text.split("\n").filter((line) => line.trim() !== "" && !line.trim().startsWith("#")).length;
    return withViewer(request, () =>
      HttpResponse.json({ added, skipped: [] } satisfies Schemas["SignupBlockImportResult"]),
    );
  }),
  // operationIds listInvites, createInvite, revokeInvite: no invites; a new one is created as asked.
  http.get(`${apiBase}/admin/invites`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
  http.post(`${apiBase}/admin/invites`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["CreateInviteRequest"];
    return withViewer(request, () => HttpResponse.json(newInvite(body)));
  }),
  http.post(`${apiBase}/admin/invites/:id/revoke`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(inviteFixture({ id: String(params["id"]), state: "revoked" }))),
  ),
  // operationId listAuditLog.
  http.get(`${apiBase}/admin/audit_log`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
  // operationIds listPendingRegistrations, approveRegistration, rejectRegistration.
  http.get(`${apiBase}/admin/registrations`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] })),
  ),
  http.post(`${apiBase}/admin/registrations/:userId/approve`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json(
        registrationFixture({ user_id: String(params["userId"]), state: "approved", since: "2026-10-17T00:00:00Z" }),
      ),
    ),
  ),
  http.post(`${apiBase}/admin/registrations/:userId/reject`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json(
        registrationFixture({ user_id: String(params["userId"]), state: "rejected", since: "2026-10-17T00:00:00Z" }),
      ),
    ),
  ),
  // operationIds listReports, getReport, assignReportToSelf, unassignReport, resolveReport, addReportNote, reopenReport.
  http.get(`${apiBase}/admin/reports`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
  http.get(`${apiBase}/admin/reports/:id`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(reportFixture({ id: String(params["id"]) }))),
  ),
  http.post(`${apiBase}/admin/reports/:id/assign_to_self`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json(reportFixture({ id: String(params["id"]), assignee_account_id: viewerFixture.id })),
    ),
  ),
  http.post(`${apiBase}/admin/reports/:id/unassign`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(reportFixture({ id: String(params["id"]) }))),
  ),
  http.post(`${apiBase}/admin/reports/:id/resolve`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["ResolveReportRequest"];
    return withViewer(request, () =>
      HttpResponse.json(
        reportFixture({
          id: String(params["id"]),
          state: "resolved",
          outcome: body.outcome,
          state_changed_at: "2026-10-17T00:00:00Z",
        }),
      ),
    );
  }),
  http.post(`${apiBase}/admin/reports/:id/notes`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["CreateReportNoteRequest"];
    return withViewer(request, () =>
      HttpResponse.json(
        reportFixture({
          id: String(params["id"]),
          notes: [
            {
              id: "0192f0c4-7a8e-7000-8000-0000000e0001",
              author_account_id: viewerFixture.id,
              body: body.body,
              created_at: "2026-10-17T00:00:00Z",
            },
          ],
        }),
      ),
    );
  }),
  http.post(`${apiBase}/admin/reports/:id/reopen`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(reportFixture({ id: String(params["id"]) }))),
  ),
  // operationIds listAppeals, getAppeal, upholdAppeal, overturnAppeal.
  http.get(`${apiBase}/admin/appeals`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
  http.get(`${apiBase}/admin/appeals/:id`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(appealFixture({ id: String(params["id"]) }))),
  ),
  ...(["uphold", "overturn"] as const).map((decision) =>
    http.post(`${apiBase}/admin/appeals/:id/${decision}`, async ({ request, params }) => {
      const body = (await request.clone().json()) as Schemas["DecideAppealRequest"];
      return withViewer(request, () =>
        HttpResponse.json(decidedAppeal(appealFixture({ id: String(params["id"]) }), decision, body.public_reason)),
      );
    }),
  ),
  // operationIds listAccountActions, createAccountAction.
  http.get(`${apiBase}/admin/accounts/:id/actions`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] })),
  ),
  http.post(`${apiBase}/admin/accounts/:id/actions`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["CreateAccountActionRequest"];
    return withViewer(request, () => HttpResponse.json(newAccountAction(String(params["id"]), body)));
  }),
  // operationIds listAdminAccounts, getAdminAccount, revealAdminAccount: no accounts found; Bob, local, for any
  // ID; no private data but an email address.
  http.get(`${apiBase}/admin/accounts`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
  http.get(`${apiBase}/admin/accounts/:id`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json(adminAccountDetailFixture({ ...otherAccountFixture, id: String(params["id"]) })),
    ),
  ),
  http.post(`${apiBase}/admin/accounts/:id/reveal`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(privateDataFixture)),
  ),
  // operationId disableAdminAccountTwoFactor.
  http.post(`${apiBase}/admin/accounts/:id/two_factor/disable`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  // operationIds listRoles, listAccountRoles, grantAccountRole, revokeAccountRole: the instance's two roles; the
  // viewer holds admin and every other account none; a change answers the role granted, or none after a revoke.
  http.get(`${apiBase}/admin/roles`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: rolesFixture })),
  ),
  http.get(`${apiBase}/admin/accounts/:id/roles`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json({ items: params["id"] === viewerFixture.id ? [adminRoleFixture] : [] }),
    ),
  ),
  http.put(`${apiBase}/admin/accounts/:id/roles/:role`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json({ items: rolesFixture.filter((role) => role.key === params["role"]) })),
  ),
  http.delete(`${apiBase}/admin/accounts/:id/roles/:role`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] })),
  ),
  // operationIds getAccountAction, liftAccountAction.
  http.get(`${apiBase}/admin/account_actions/:id`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(accountActionFixture({ id: String(params["id"]) }))),
  ),
  http.post(`${apiBase}/admin/account_actions/:id/lift`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["LiftAccountActionRequest"];
    return withViewer(request, () =>
      HttpResponse.json(
        accountActionFixture({
          id: String(params["id"]),
          state: "lifted",
          lifted_at: "2026-10-17T00:00:00Z",
          ...(body.reason === undefined ? {} : { lift_reason: body.reason }),
        }),
      ),
    );
  }),
  // operationId getAccount: the moderators of the reports, Bob for any other ID. The path also matches the
  // operations under /accounts with a name (lookup, relationships…), which go on to their own handlers.
  http.get(`${apiBase}/accounts/:id`, ({ request, params }) => {
    const id = String(params["id"]);
    if (!isId(id)) return undefined;
    return withViewer(request, () =>
      HttpResponse.json(id === viewerFixture.id ? viewerFixture : { ...otherAccountFixture, id }),
    );
  }),
];
