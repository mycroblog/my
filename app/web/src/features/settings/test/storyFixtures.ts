// SPDX-License-Identifier: AGPL-3.0-only
// The data of the settings screen stories: lists of accounts with long, right-to-left and plain names, and the user's
// apps, archives, imports and invites in each of their states.
import {
  accountFixture,
  longDomainAccountFixture,
  longNameAccountFixture,
  rtlNameAccountFixture,
  type Schemas,
} from "../../../test/fixtures";
import { dataExportFixture, dataImportFixture, ownInviteFixture } from "./handlers";

/** Ten accounts: a long name, a right-to-left name, a long domain, and plain ones of this server and another. */
export const listedAccounts: Schemas["Account"][] = [
  longNameAccountFixture,
  rtlNameAccountFixture,
  longDomainAccountFixture,
  ...["Dana", "Eiji", "Fatima", "Gustav", "Hanna", "Ines", "Jun"].map((name, i) =>
    accountFixture(80 + i, name, i % 2 === 0 ? "my.example" : "social.example"),
  ),
];

/** The user's own OAuth apps: a confidential web app, a public native one, and a disabled one. */
export const ownApps: Schemas["OAuthApp"][] = [
  {
    id: "0192f0c4-7a8e-7000-8000-000000000aa1",
    client_id: "my-bread-diary-7f3a9c",
    name: "Bread diary",
    website: "https://bread-diary.example/",
    redirect_uris: ["https://bread-diary.example/oauth/callback"],
    scope: "read write",
    client_type: "confidential",
    disabled: false,
    created_at: "2026-09-01T10:00:00Z",
  },
  {
    id: "0192f0c4-7a8e-7000-8000-000000000aa2",
    client_id: "sourdough-starter-feeding-reminder-for-the-desktop-1b2c",
    name: "Sourdough starter feeding reminder for the desktop and the phone",
    redirect_uris: ["http://127.0.0.1/callback", "com.example.reminder:/oauth"],
    scope: "read",
    client_type: "public",
    disabled: false,
    created_at: "2026-10-01T10:00:00Z",
  },
  {
    id: "0192f0c4-7a8e-7000-8000-000000000aa3",
    client_id: "old-bot-0001",
    name: "Old bot",
    redirect_uris: ["https://bot.example/callback"],
    scope: "write",
    client_type: "confidential",
    disabled: true,
    created_at: "2026-05-01T10:00:00Z",
  },
];

/** The apps the user authorized: this server's app, one registered by a user, and two that registered themselves. */
export const authorizedApps: Schemas["AuthorizedApp"][] = [
  {
    id: "0192f0c4-7a8e-7000-8000-000000000ab1",
    client_id: "my-web",
    name: "my web",
    registration: "first_party",
    first_party: true,
    scope: "read write follow push",
    authorized_at: "2026-10-01T00:00:00Z",
    last_used_at: "2026-10-28T08:00:00Z",
  },
  {
    id: "0192f0c4-7a8e-7000-8000-000000000ab2",
    client_id: "my-bread-diary-7f3a9c",
    name: "Bread diary",
    website: "https://bread-diary.example/",
    registration: "user",
    registered_by: "bob",
    first_party: false,
    scope: "read write",
    authorized_at: "2026-09-02T00:00:00Z",
    last_used_at: "2026-10-20T08:00:00Z",
  },
  {
    id: "0192f0c4-7a8e-7000-8000-000000000ab3",
    client_id: "https://client.long-app-name.example/client-metadata.json",
    name: "A reader with a very long name that does not fit on one line of a phone",
    website: "https://client.long-app-name.example/",
    registration: "metadata_document",
    first_party: false,
    scope: "read",
    authorized_at: "2026-08-15T00:00:00Z",
  },
  {
    id: "0192f0c4-7a8e-7000-8000-000000000ab4",
    client_id: "dyn-4f1e",
    name: "Crossposter",
    registration: "dynamic",
    first_party: false,
    scope: "write",
    authorized_at: "2026-07-01T00:00:00Z",
    last_used_at: "2026-07-02T00:00:00Z",
  },
];

/** Archives of the user's data: one being made, one ready, and one that failed. */
export const dataExports: Schemas["DataExport"][] = [
  dataExportFixture({ id: "0192f0c4-7a8e-7000-8000-0000000008e3", requested_at: "2026-10-28T09:30:00Z" }),
  dataExportFixture({
    id: "0192f0c4-7a8e-7000-8000-0000000008e2",
    state: "ready",
    requested_at: "2026-10-20T09:30:00Z",
    completed_at: "2026-10-20T09:42:00Z",
    expires_at: "2026-10-27T09:42:00Z",
    file_size: 48_213_504,
  }),
  dataExportFixture({ state: "failed", requested_at: "2026-10-10T09:30:00Z" }),
];

/** Imports: one in progress, one finished with failures, and one finished cleanly. */
export const dataImports: Schemas["DataImport"][] = [
  dataImportFixture({ total: 420, processed: 180 }),
  dataImportFixture({
    id: "0192f0c4-7a8e-7000-8000-0000000009a2",
    kind: "blocks",
    mode: "overwrite",
    state: "finished",
    created_at: "2026-10-18T09:30:00Z",
    total: 35,
    processed: 35,
    failed: 3,
  }),
  dataImportFixture({
    id: "0192f0c4-7a8e-7000-8000-0000000009a3",
    kind: "domain_mutes",
    state: "finished",
    created_at: "2026-10-01T09:30:00Z",
    total: 4,
    processed: 4,
  }),
];

/** Invites: two usable, one used up, one expired, and one revoked. */
export const ownInvites: Schemas["OwnInvite"][] = [
  ownInviteFixture("0192f0c4-7a8e-7000-8000-0000000009b1", "ABCDEFGHJKLMNPQR"),
  ownInviteFixture("0192f0c4-7a8e-7000-8000-0000000009b2", "STUVWXYZ23456789", { max_uses: 10, uses: 4 }),
  ownInviteFixture("0192f0c4-7a8e-7000-8000-0000000009b3", "BCDEFGHJKLMNPQRS", { uses: 1, state: "exhausted" }),
  ownInviteFixture("0192f0c4-7a8e-7000-8000-0000000009b4", "CDEFGHJKLMNPQRST", {
    state: "expired",
    expires_at: "2026-10-09T00:00:00Z",
    created_at: "2026-10-02T00:00:00Z",
  }),
  ownInviteFixture("0192f0c4-7a8e-7000-8000-0000000009b5", "DEFGHJKLMNPQRSTU", { state: "revoked" }),
];
