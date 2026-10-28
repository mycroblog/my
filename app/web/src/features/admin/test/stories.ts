// SPDX-License-Identifier: AGPL-3.0-only
// What the screen stories of the server administration (ADR-0112 §1, `Screens/Admin/*`) share: the signed-in
// administrator whose roles grant every permission of the admin screens, and a viewer without the screen's permission.
import type { Meta } from "@storybook/react-vite";
import type { RequestHandler } from "msw";
import type { Schemas } from "../../../test/fixtures";
import {
  answering,
  answeringWithErrorPage,
  failingOn,
  loadingOn,
  renderScreen,
  screenLoader,
  settled,
  viewerWith,
} from "../../../test/stories";

/** Every permission of the server administration and moderation screens. */
export const everyPermission: Schemas["Permission"][] = [
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
  "view_finance",
  "manage_supporters",
  "create_custom_emojis",
  "manage_oauth_apps",
  "manage_service_closure",
  "start_voice_rooms",
  "manage_errors",
  "manage_login_providers",
  "create_invites",
];

/** The signed-in viewer: the server's administrator, with every permission. */
export const administrator = viewerWith({ display_name: "Alice the administrator", permissions: everyPermission });

/**
 * The meta of an admin screen story besides its title, which Storybook's indexer reads from the meta's literal: the
 * app at `path`, signed in as the administrator, with the screen's `handlers` answering before the default ones, and
 * played until the screen has loaded.
 */
export function adminScreen(path: string, handlers: RequestHandler[] = []) {
  return {
    render: renderScreen,
    loaders: [screenLoader],
    parameters: { screen: { path, signedIn: true, handlers: [administrator, ...handlers] } },
    play: settled,
  } satisfies Meta;
}

type Method = "get" | "post" | "put" | "patch" | "delete";

/** A story whose server answers the administrator with `handlers` before the default ones, in place of the meta's. */
export function adminAnswering(...handlers: RequestHandler[]) {
  return answering(administrator, ...handlers);
}

/** loadingOn for the administrator: the request `method path` never answers, the others as by default. */
export function adminLoadingOn(method: Method, path: string, ...handlers: RequestHandler[]) {
  const story = loadingOn(method, path);
  return { ...story, ...adminAnswering(...story.parameters.screen.handlers, ...handlers) };
}

/** answeringWithErrorPage for the administrator: the screen shows its error or not-found page on purpose. */
export function adminAnsweringWithErrorPage(...handlers: RequestHandler[]) {
  return answeringWithErrorPage(administrator, ...handlers);
}

/** failingOn for the administrator: the request `method path` fails with a 500, the others as by default. */
export function adminFailingOn(method: Method, path: string, ...handlers: RequestHandler[]) {
  return adminAnsweringWithErrorPage(...failingOn(method, path).parameters.screen.handlers, ...handlers);
}

/**
 * A story whose viewer is a moderator without the screen's permission: the screen says why it is empty, and asks
 * the server for nothing of it. Its handlers replace the meta's.
 */
export const withoutPermission = {
  parameters: {
    screen: { handlers: [viewerWith({ display_name: "Marco", permissions: ["manage_reports", "manage_accounts"] })] },
  },
};
