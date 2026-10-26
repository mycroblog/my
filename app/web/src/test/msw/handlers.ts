// SPDX-License-Identifier: AGPL-3.0-only
// The modules of the default MSW handlers, one line each (web-client/README.md §7): every module exports
// `handlers`, one successful response per client API operation in api/openapi.yaml that it owns. A feature
// keeps its handlers and the fixtures only it uses in features/<x>/test/handlers.ts; the fixtures that
// several features use are in src/test/fixtures.ts. Each operation has one default handler, so the order of
// the modules does not matter (handlers.test.ts checks it). A test that needs another answer overrides a
// handler with `server.use()`.
export * as about from "../../features/about/test/handlers";
export * as admin from "../../features/admin/test/handlers";
export * as announcements from "../../features/announcements/test/handlers";
export * as compose from "../../features/compose/test/handlers";
export * as costs from "../../features/costs/test/handlers";
export * as core from "./http";
export * as customEmojis from "../../features/custom-emojis/test/handlers";
export * as emoji from "../../features/emoji/test/handlers";
export * as moderation from "../../features/moderation/test/handlers";
export * as notifications from "../../features/notifications/test/handlers";
export * as profile from "../../features/profile/test/handlers";
export * as report from "../../features/report/test/handlers";
export * as session from "../../features/session/test/handlers";
export * as settings from "../../features/settings/test/handlers";
export * as status from "../../features/status/test/handlers";
export * as streaming from "./streaming";
export * as supporters from "../../features/supporters/test/handlers";
export * as timeline from "../../features/timeline/test/handlers";
export * as voice from "../../features/voice/test/handlers";
