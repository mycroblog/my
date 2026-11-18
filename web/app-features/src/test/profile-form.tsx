// SPDX-License-Identifier: AGPL-3.0-only

// Edit profile for its tests (#3200), on either DOM simulation: the form drawn on a fake server once its route loaded
// what it opens with, what it told its owner, the parts a test finds on it, and the browser's file chooser, which no
// simulation opens.

import { instanceQuery, viewerQuery } from "@my/app-data";
import { fakeOrigin, type FakeOptions } from "@my/e2e/fake";
import { screen, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { onTestFinished, spyOn } from "bun:test";
import { loadProfileEdit, ProfileEdit, type ProfileEditProps } from "../settings/profile-edit";
import { fakeData, renderFeature, type FakeData } from "./feature";

/** The form on its server, the reader's hands, and what the form told its owner, in order. */
export type DrawnForm = FakeData & {
  user: UserEvent;
  /** `back` for the way back pressed, `unsaved:true|false` for each report of unsaved changes, `stay`, `leave`. */
  told: string[];
};

/**
 * Draws Edit profile on a server of `options`, in the main landmark as the shell places it, once the shell read the
 * instance and the reader's account and the route loaded the form, in `locale`; `leaving` draws it as the app does
 * while the reader is asked whether to leave with unsaved changes. The server's answers reach the form when they
 * come, not inside an act() of the test, so React is told so until the test ends, and a test waits for what it
 * asserts.
 */
export async function profileForm(
  options: FakeOptions = {},
  { locale = "en", leaving = false }: { locale?: string; leaving?: boolean } = {},
): Promise<DrawnForm> {
  const environment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
  const wrapped = environment.IS_REACT_ACT_ENVIRONMENT;
  environment.IS_REACT_ACT_ENVIRONMENT = false;
  onTestFinished(() => {
    if (wrapped === undefined) delete environment.IS_REACT_ACT_ENVIRONMENT;
    else environment.IS_REACT_ACT_ENVIRONMENT = wrapped;
  });
  runtimeSignals();
  const on = fakeData(options);
  const { data } = on;
  await data.queryClient.query(instanceQuery(data));
  await data.queryClient.query(viewerQuery(data));
  await loadProfileEdit(data);
  const told: string[] = [];
  const asked: ProfileEditProps["leaving"] = {
    onStay: () => told.push("stay"),
    onLeave: () => told.push("leave"),
  };
  const user = renderFeature(
    data,
    <main>
      <ProfileEdit
        onBack={() => told.push("back")}
        onUnsaved={(unsaved) => told.push(`unsaved:${String(unsaved)}`)}
        leaving={leaving ? asked : undefined}
      />
    </main>,
    locale,
  );
  await within(screen.getByRole("main")).findAllByRole("textbox");
  return { ...on, user, told };
}

/**
 * On jsdom, gives the app's data abort signals the runtime's `Request` takes, until the test ends: jsdom puts its own
 * `AbortController` in the runtime's place, and a request made with its signal is refused before the fake sees it. The
 * signal given is the runtime's and never aborts: no test of the form gives a request up. On happy-dom, which keeps
 * the runtime's, nothing changes.
 */
export function runtimeSignals(): void {
  const Signal = new Request(fakeOrigin).signal.constructor as typeof AbortSignal;
  if (Signal === globalThis.AbortSignal) return;
  const simulated = globalThis.AbortController;
  globalThis.AbortController = class {
    readonly signal = Signal.any([]);
    abort(): void {
      // Nothing to give up: see above.
    }
  };
  onTestFinished(() => {
    globalThis.AbortController = simulated;
  });
}

/** A file of an image to pick: a PNG by its name, whose bytes no test reads. */
export const imageFile = (name: string): File =>
  new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])], name, { type: "image/png" });

/** Presses `button`, which opens the file chooser, and picks `file` in it, as a browser does once the reader chose. */
export async function chooseFile(user: UserEvent, button: HTMLElement, file: File): Promise<void> {
  const opened: HTMLInputElement[] = [];
  const click = spyOn(HTMLInputElement.prototype, "click").mockImplementation(function (this: HTMLInputElement) {
    if (this.type === "file") opened.push(this);
  });
  try {
    await user.click(button);
  } finally {
    click.mockRestore();
  }
  const [chooser] = opened;
  if (chooser === undefined) throw new Error("test: the press opened no file chooser");
  await user.upload(chooser, file);
}

/** The group of the image named `name`, in the form's language. */
export const image = (name: string) => within(screen.getByRole("group", { name }));
/** The addresses of the pictures the image's group shows: the picture it has, or the one picked. */
export const pictures = (name: string) =>
  image(name)
    .queryAllByRole<HTMLImageElement>("presentation", { hidden: true })
    .map((picture) => picture.src);
/** The names of the buttons of the image's group, in their order. */
export const imageButtons = (name: string) =>
  image(name)
    .getAllByRole("button")
    .map((button) => button.textContent);
export const nameField = () => screen.getByRole<HTMLInputElement>("textbox", { name: "Display name" });
export const bioField = () => screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Bio" });
export const saveButton = () => screen.getByRole<HTMLButtonElement>("button", { name: "Save" });
/** The text field `part` of the pair numbered `n` (from 1). */
export const pairField = (n: number, part: "Label" | "Content") =>
  within(screen.getByRole("group", { name: `Field ${String(n)}` })).getByRole<HTMLInputElement>("textbox", {
    name: part,
  });
/** The pairs of the form in order, each as what its two fields hold. */
export const pairs = () =>
  screen.queryAllByRole("group", { name: /^Field \d+$/ }).map((group) =>
    within(group)
      .getAllByRole<HTMLInputElement>("textbox")
      .map((input) => input.value),
  );
/** Puts `text` in `field` in place of what it holds, as one paste: no key decides anything here. */
export async function rewrite(user: UserEvent, field: HTMLElement, text: string): Promise<void> {
  await user.clear(field);
  if (text === "") return;
  await user.click(field);
  await user.paste(text);
}
/** What the alerts of the page say. */
export const alerts = () => screen.queryAllByRole("alert").map((alert) => alert.textContent);
/** What describes `element`, part by part. */
export const describedBy = (element: HTMLElement) =>
  (element.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .filter((id) => id !== "")
    .map((id) => document.getElementById(id)?.textContent ?? "");
/**
 * The text fields of the form that say their value is not valid, each by its name, a pair's after its legend
 * ("Field 2: Label").
 */
export const invalid = () =>
  [
    ["Display name", nameField()] as const,
    ["Bio", bioField()] as const,
    ...screen.queryAllByRole("group", { name: /^Field \d+$/ }).flatMap((group) =>
      within(group)
        .getAllByRole("textbox")
        .map(
          (input) =>
            [`${group.getAttribute("aria-label") ?? ""}: ${input.getAttribute("aria-label") ?? ""}`, input] as const,
        ),
    ),
  ]
    .filter(([, input]) => input.getAttribute("aria-invalid") === "true")
    .map(([name]) => name);
