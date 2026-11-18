// SPDX-License-Identifier: AGPL-3.0-only

// What a test reads of the settings' screens of how an account signs in (Security and the screens under it), drawn
// alone (./settings.tsx) or in the app (web/app): the sections by their titles, the rows of their lists, the dates as
// the screens write them, the dialogs over the screen; what the fake server holds; and a browser with passkeys, which
// no simulation has.

import type { AppData } from "@my/app-data";
import { fakeOrigin, type FakeApi } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { onTestFinished } from "bun:test";
import type { Given } from "./settings";

/** Waits until the app's data has no read on its way. */
export const atRest = ({ data }: { data: AppData }) =>
  waitFor(() => {
    if (data.queryClient.isFetching() > 0) throw new Error("test: a read is still on its way");
  });

/** What the fake server holds at `path` of the client API, as it answers a read of it. */
export async function heldBy<T>(fake: FakeApi, path: string): Promise<T> {
  const answer = await fake.serve(new Request(`${fakeOrigin}/api/v1${path}`));
  return (await answer.json()) as T;
}

/**
 * Removes what the fake server holds at `path` of the client API past the app, as another browser of the reader's
 * does: the app's own removal of it is then answered 404.
 */
export async function removedElsewhere(fake: FakeApi, path: string): Promise<void> {
  const answer = await fake.serve(new Request(`${fakeOrigin}/api/v1${path}`, { method: "DELETE" }));
  if (answer.status !== 204) throw new Error(`test: the fake answered ${String(answer.status)} to DELETE ${path}`);
}

/**
 * A server whose login providers take no new link (`allow_link` is false for each): an account linked before is
 * still listed, to unlink, and no provider is offered to link.
 */
export const noNewLinks: Given = {
  before: ({ fake }) => {
    fake.script("GET /linked_identities", {
      change: (body) => ({ ...body, providers: body.providers.map((p) => ({ ...p, allow_link: false })) }),
    });
  },
};

/** What `element` says as a reader reads its line: each run of spaces, breaking or not, as one space. */
export const words = (element: Element): string => element.textContent.replace(/\s+/g, " ").trim();

/**
 * What matches the words of an element that says `parts` in their order and nothing else: whether a space stands
 * between two of them depends on the markup around them, which no reader hears.
 */
export const inOrder = (...parts: string[]): RegExp =>
  new RegExp(`^${parts.map((part) => part.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")).join("\\s*")}$`);

/** What has the focus, as a reader's tools name it: its role and its words; "nothing" when it is the page itself. */
export const focused = (): string => {
  const element = document.activeElement;
  if (element === null || element === document.body) return "nothing";
  const role =
    element.getAttribute("role") ?? (/^H[1-6]$/.test(element.tagName) ? "heading" : element.tagName.toLowerCase());
  return `${role}: ${words(element)}`;
};

/** Waits until the focus is on `target`: an element, or what finds one that may not be on the screen yet. */
export const focusOn = (target: Element | (() => Element | undefined)) =>
  waitFor(() => {
    const element = typeof target === "function" ? target() : target;
    if (element === undefined || document.activeElement !== element)
      throw new Error(`test: the focus is on ${focused()}`);
  });

/** What describes `control`, part by part: its hint, then what is wrong with its value. */
export const describedBy = (control: HTMLElement): string[] =>
  (control.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .filter((id) => id !== "")
    .map((id) => {
      const part = document.getElementById(id);
      return part === null ? "" : words(part);
    });

/** The year of `iso`, where a date carries it: any year but this one. */
const yearOf = (iso: string) =>
  new Date(iso).getFullYear() === new Date().getFullYear() ? {} : ({ year: "numeric" } as const);

/** The day of `iso` as the settings write it in English ("Nov 15", "Dec 31, 2025"), each space as one. */
export const dayOf = (iso: string): string =>
  new Intl.DateTimeFormat("en", { month: "short", day: "numeric", ...yearOf(iso) })
    .format(new Date(iso))
    .replace(/\s+/g, " ");

/** The day and the hour of `iso` as the settings write them in English ("Nov 18, 6 AM"), each space as one. */
export const hourOf = (iso: string): string =>
  new Intl.DateTimeFormat("en", { month: "short", day: "numeric", hour: "numeric", ...yearOf(iso) })
    .format(new Date(iso))
    .replace(/\s+/g, " ");

/** The screen's own part, the main landmark: in the app, the shell and the settings' menu stand around it. */
const page = () => within(screen.getByRole("main"));

/** The title of a section as a reader hears it: its heading's words, with the count beside them ("Passkeys 3"). */
const titleOf = (region: HTMLElement) =>
  (region.getAttribute("aria-labelledby") ?? "")
    .split(" ")
    .map((id) => document.getElementById(id))
    .map((title) => (title === null ? "" : words(title)))
    .join(" ");

/** The titles of the screen's sections, in their order. */
export const sections = (): string[] => page().queryAllByRole("region").map(titleOf);

/** The section titled `name`, whatever count stands beside its title. */
export function section(name: string): HTMLElement {
  const [found, ...others] = page()
    .queryAllByRole("region")
    .filter((region) => titleOf(region).replace(/ \d+$/, "") === name);
  if (found === undefined || others.length > 0)
    throw new Error(`test: ${String(others.length + (found === undefined ? 0 : 1))} sections are titled "${name}"`);
  return found;
}

/** A row of a settings list: what it says (its name, then its lines), where it leads, and the buttons it offers. */
export type Row = { says: string; leads: string | null; offers: string[] };

/** A row that says `says`, and leads or offers what `more` names. */
export const row = (says: string, more: Partial<Omit<Row, "says">> = {}): Row => ({
  says,
  leads: null,
  offers: [],
  ...more,
});

/** The rows of the list named `name`, in their order. */
export const rows = (name: string): Row[] =>
  page()
    .queryAllByRole("list", { name })
    .flatMap((list) =>
      within(list)
        .getAllByRole("listitem")
        .map((item) => {
          // A row's buttons stand in a group named by the row, after what the row says.
          const offered = within(item)
            .queryAllByRole("group")
            .map((group) => group.textContent)
            .join("");
          const [link] = within(item).queryAllByRole("link");
          return {
            says: item.textContent
              .slice(0, item.textContent.length - offered.length)
              .replace(/\s+/g, " ")
              .trim(),
            leads: link?.getAttribute("href") ?? null,
            offers: within(item)
              .queryAllByRole("button")
              .map((button) => button.textContent),
          };
        }),
    );

/** The button `button` of the row named `name`, in the section titled `title`: a row's buttons stand in a group it names. */
export const offered = (title: string, name: string, button: string): HTMLElement =>
  within(within(section(title)).getByRole("group", { name })).getByRole("button", { name: button });

/** The dialogs open over the screen, of either kind. */
export const dialogs = (): HTMLElement[] => [
  ...screen.queryAllByRole("dialog"),
  ...screen.queryAllByRole("alertdialog"),
];

/** Waits until no dialog is open. */
export const closed = () =>
  waitFor(() => {
    if (dialogs().length > 0) throw new Error("test: a dialog is still open");
  });

/** The dialog that asks to confirm it's the reader, once it is open. */
export const proving = () => screen.findByRole("dialog", { name: "Confirm it's you" });

/** The buttons of `dialog`, by their names, in their order. */
export const buttonsOf = (dialog: HTMLElement): string[] =>
  within(dialog)
    .getAllByRole("button")
    .map((button) => button.getAttribute("aria-label") ?? button.textContent);

/** How the stubbed browser's passkey prompt ends: with a credential, or as the browser refuses it. */
type PasskeyAnswer = "credential" | "cancelled" | "exists";

/** What the stubbed browser did: the prompts it showed, and how the next one ends. */
export type PasskeyBrowser = { created: number; asked: number; answer: PasskeyAnswer };

const refusals = { cancelled: "NotAllowedError", exists: "InvalidStateError" } as const;

/**
 * Gives the test a browser with passkeys (WebAuthn), which no simulation has, until the test ends: its prompts answer
 * at once, with a credential or as `answer` says. Only @my/platform reads the platform's globals in the app, so the
 * stub reaches the navigator as a stub does.
 */
export function passkeyBrowser(): PasskeyBrowser {
  const seen: PasskeyBrowser = { created: 0, asked: 0, answer: "credential" };
  const bytes = (...values: number[]) => new Uint8Array(values).buffer;
  const answerOf = (response: Record<string, unknown>) =>
    seen.answer === "credential"
      ? Promise.resolve({
          id: "AQID",
          rawId: bytes(1, 2, 3),
          type: "public-key",
          authenticatorAttachment: "platform",
          response,
          getClientExtensionResults: () => ({}),
        })
      : Promise.reject(new DOMException("test: the browser refused", refusals[seen.answer]));
  const browser = Reflect.get(globalThis, "navigator") as object;
  Object.defineProperty(globalThis, "PublicKeyCredential", { configurable: true, value: {} });
  Object.defineProperty(browser, "credentials", {
    configurable: true,
    value: {
      create: () => {
        seen.created += 1;
        return answerOf({
          clientDataJSON: bytes(1, 2, 3),
          attestationObject: bytes(4, 5, 6),
          getTransports: () => ["internal"],
        });
      },
      get: () => {
        seen.asked += 1;
        return answerOf({
          clientDataJSON: bytes(1, 2, 3),
          authenticatorData: bytes(7, 8),
          signature: bytes(9),
          userHandle: bytes(0x75),
        });
      },
    },
  });
  onTestFinished(() => {
    Reflect.deleteProperty(globalThis, "PublicKeyCredential");
    Reflect.deleteProperty(browser, "credentials");
  });
  return seen;
}
