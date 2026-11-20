// SPDX-License-Identifier: AGPL-3.0-only

// What a test reads of the settings' Apps screens (#3206) besides what ./security.ts reads of any settings screen: the
// apps the fake server holds, the fields of an app's form, a value shown to copy and the reader's clipboard, what a
// screen says of what it did, and the sentences several of the screens say, each pinned by the test of the screen
// that owns it.

import type { AuthorizedApp, OAuthApp } from "@my/app-data";
import type { FakeApi } from "@my/e2e/fake";
import { screen, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { onTestFinished } from "bun:test";
import { heldBy, words } from "./security";

/** The apps the reader authorized, as the fake server lists them: its own web client first. */
export const authorizedApps = async (fake: FakeApi): Promise<AuthorizedApp[]> =>
  (await heldBy<{ items: AuthorizedApp[] }>(fake, "/authorized_apps")).items;

/** The apps the reader registered, as the fake server lists them, oldest first. */
export const ownApps = async (fake: FakeApi): Promise<OAuthApp[]> =>
  (await heldBy<{ items: OAuthApp[] }>(fake, "/oauth_apps")).items;

/** The app at `index` of `apps`; a test that names one the fake does not hold fails here. */
export function appAt<T>(apps: readonly T[], index: number): T {
  const app = apps[index];
  if (app === undefined) throw new Error(`test: the fake holds no app at ${String(index)}`);
  return app;
}

/** What a screen says while what it shows is on its way. */
export const loading = "Loading your settings…";
/** What a screen says when what it shows could not be read, with the way to read it again. */
export const notLoaded = /^Your settings could not be loaded\.\s*Try again$/;
/** What a screen says of an address that names no app of the reader's, with the way to the entry. */
export const notAmong = /^This app is not among your apps\. It may have been revoked or deleted\.\s*See your apps$/;
/** What the reader's own apps say on a server whose administrators do not let users register apps. */
export const registrationOff =
  "The server's administrators have turned off app registration. Apps already registered keep working.";
/** What stands with a secret or a token, which the server shows once. */
export const notShownAgain = "Copy it now and keep it somewhere safe: it is not shown again.";
/** What the submit bar says of a form the server refused for one of its fields. */
export const notDone = "This could not be done.";

/** The fields of an app's form, by their labels, and what describes each at rest. */
export const hints = {
  Name: "Shown on the page where people allow the app. Up to 100 characters.",
  Website: "Optional. An https or http address, shown on the same page.",
  "Redirect URIs":
    "The addresses the server may send the browser back to after someone allows the app. One per line, up to 10: an https address, an address with a scheme of your own such as com.example.app:/callback, or http://127.0.0.1 on any port.",
  Scopes:
    "The most the app may ask for, separated by spaces, such as read write:statuses. An app cannot have admin, admin:read, admin:write or write:supporters.",
} as const;

/** A field of an app's form, by its label. */
export type AppFieldName = keyof typeof hints;

/** The field labelled `name`: a line of text, or the lines of the redirect URIs. */
export const field = (name: AppFieldName) =>
  screen.getByRole<HTMLInputElement | HTMLTextAreaElement>("textbox", { name });

/** Writes `text` in `box` in place of what it holds, as one paste: no key decides anything in these fields. */
export async function write(user: UserEvent, box: HTMLElement, text: string): Promise<void> {
  await user.clear(box);
  if (text === "") return;
  await user.click(box);
  await user.paste(text);
}

/** The values the list named `name` shows to copy (a client ID, a secret, a token), or the facts it states. */
export const listed = (name: string): string[] =>
  screen.queryAllByRole("list", { name }).flatMap((list) => within(list).getAllByRole("listitem").map(words));

/** The one value the list named `name` shows; a test that reads a list of another length fails here. */
export function shown(name: string): string {
  const [value, ...others] = listed(name);
  if (value === undefined || others.length > 0)
    throw new Error(`test: the list "${name}" shows ${String(listed(name).length)} values`);
  return value;
}

/** What the status regions say, the empty ones left out: an empty status is a place kept for what it will say. */
export const said = (): string[] =>
  screen
    .queryAllByRole("status")
    .map(words)
    .filter((text) => text !== "");

/** What the alerts say. */
export const alerts = (): string[] => screen.queryAllByRole("alert").map(words);

/** The buttons of the screen, by their words, in their order. */
export const buttons = (): string[] => screen.queryAllByRole("button").map(words);

/** Whether `first` stands before `second` in the page, as it is read and as Tab goes. */
export const before = (first: Element, second: Element): boolean =>
  (first.compareDocumentPosition(second) & first.DOCUMENT_POSITION_FOLLOWING) !== 0;

/**
 * The clipboard the reader's hands brought to the simulation, reached as a stub reaches it: only @my/platform reads
 * the platform's globals in the app.
 */
export function clipboard(): Clipboard {
  const browser = Reflect.get(globalThis, "navigator") as object;
  return Reflect.get(browser, "clipboard") as Clipboard;
}

/**
 * Makes the clipboard refuse what is written to it, until the test ends. The refusal is the clipboard's own property,
 * over its prototype's method: deleting it gives the clipboard back as it was, so that a later test of the process
 * finds the method as the DOM defines it.
 */
export function refusingClipboard(): void {
  const held = clipboard();
  Object.defineProperty(held, "writeText", {
    configurable: true,
    value: () => Promise.reject(new DOMException("test: the clipboard refused", "NotAllowedError")),
  });
  onTestFinished(() => {
    Reflect.deleteProperty(held, "writeText");
  });
}
