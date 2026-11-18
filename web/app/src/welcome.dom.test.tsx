// SPDX-License-Identifier: AGPL-3.0-only

// The welcome step of a new account (#3209), at /welcome, as the app places it: a page of its own, outside the frame,
// that asks for a display name and an avatar, both optional, saves what was given, and goes on to the home timeline,
// or skips; an account that is not new goes on to the home timeline at once, and a visitor is asked to sign in. A new
// account is one with neither a display name nor an avatar (the fake's `profile: { start: "empty" }`) on a device it
// has not passed the step on.

import { writeDeviceValue } from "@my/platform";
import { screen, waitFor, within } from "@testing-library/react";
import { describe, expect, onTestFinished, test } from "bun:test";
import { describedBy, focusOn } from "./test/account-data";
import { main, renderApp, words, type TestApp } from "./test/app";
import { chooseFiles, imageFile } from "./test/files";
import { fakeApp } from "./test/server";

const welcome = "/welcome";

/** A reader with nothing on their profile yet, as an account is after signing up. */
const newAccount = { profile: { start: "empty" } } as const;

/** The device's key of the accounts that passed the step, which a test starts and ends without. */
const passedKey = "my:welcome-passed";

/** Forgets on this device, now and when the test ends, which accounts passed the step. */
function freshDevice() {
  writeDeviceValue(passedKey, null);
  onTestFinished(() => {
    writeDeviceValue(passedKey, null);
  });
}

const nameField = () => main().getByRole<HTMLInputElement>("textbox", { name: "Display name" });
const avatar = () => main().getByRole("group", { name: "Avatar" });
const continueButton = () => main().getByRole("button", { name: "Continue" });
const skipButton = () => main().getByRole("button", { name: "Skip for now" });

/** The step opened as the new account `options` names arrives at it, with what it would send counted. */
async function step(options: Parameters<typeof fakeApp>[0] = newAccount) {
  freshDevice();
  const on = fakeApp(options);
  const saved = on.fake.script("PATCH /accounts/update_credentials");
  const uploaded = on.fake.script("POST /media");
  const app = await renderApp(welcome, { on });
  await main().findByRole("textbox", { name: "Display name" });
  return { app, saved, uploaded };
}

/** Waits until the app is at the home timeline, with its heading drawn. */
async function home(app: TestApp) {
  await waitFor(() => {
    if (app.path() !== "/") throw new Error(`test: the app is at ${app.path()}`);
  });
  return main().findByRole("heading", { level: 1, name: "Home" });
}

describe("a new account", () => {
  test("is welcomed to the server on a page of its own, asked for a display name and an avatar, with Continue and Skip for now", async () => {
    await step();

    expect(main().getByRole("heading", { level: 1 }).textContent).toBe("Welcome to mycroblog demo");
    // The app's navigation is not there: the step is no screen of the frame.
    expect(screen.queryAllByRole("navigation", { name: "Main" })).toHaveLength(0);
    expect(screen.queryAllByRole("link", { name: "Home" })).toHaveLength(0);
    expect(nameField().value).toBe("");
    expect(within(avatar()).getAllByRole("button").map(words)).toEqual(["Choose an image"]);
    expect(
      main()
        .getAllByRole("button")
        .map(words)
        .filter((name) => name !== "Choose an image"),
    ).toEqual(["Continue", "Skip for now"]);
  });

  test("Continue with a name saves it alone, goes on to the home timeline with the focus on its heading, and the step is passed", async () => {
    const { app, saved, uploaded } = await step();
    await app.user.click(nameField());
    await app.user.paste("Yuki Tanaka");

    await app.user.click(continueButton());

    const heading = await home(app);
    await focusOn(() => heading);
    expect(saved.seen.map((s) => s.body)).toEqual([{ display_name: "Yuki Tanaka" }]);
    expect(uploaded.seen).toHaveLength(0);
  });

  test("Continue with an image uploads it, then saves the name with the avatar's upload", async () => {
    const { app, saved, uploaded } = await step();
    await app.user.click(nameField());
    await app.user.paste("Yuki Tanaka");
    await chooseFiles(app, within(avatar()).getByRole("button", { name: "Choose an image" }), imageFile("me.png"));

    await app.user.click(continueButton());

    await home(app);
    expect(uploaded.seen).toHaveLength(1);
    const [body, ...more] = saved.seen.map((s) => s.body);
    expect(more).toEqual([]);
    expect(body?.display_name).toBe("Yuki Tanaka");
    expect(typeof body?.avatar_media_id).toBe("string");
  });

  for (const [what, press] of [
    ["Continue with nothing given", continueButton],
    ["Skip for now", skipButton],
  ] as const)
    test(`${what} sends nothing and goes on to the home timeline`, async () => {
      const { app, saved, uploaded } = await step();

      await app.user.click(press());

      await home(app);
      expect(saved.seen).toHaveLength(0);
      expect(uploaded.seen).toHaveLength(0);
    });

  test("Skip for now with a name written sends nothing", async () => {
    const { app, saved } = await step();
    await app.user.click(nameField());
    await app.user.paste("Yuki Tanaka");

    await app.user.click(skipButton());

    await home(app);
    expect(saved.seen).toHaveLength(0);
  });

  test("once passed on this device, the step is not asked again: /welcome goes on to the home timeline", async () => {
    const { app } = await step();
    await app.user.click(skipButton());
    await home(app);

    await app.navigate(welcome);

    await home(app);
  });

  test("a name the server refuses is said under the field, which takes the focus, with one alert that nothing was saved", async () => {
    const { app } = await step({ ...newAccount, profile: { start: "empty", save: "name" } });
    await app.user.click(nameField());
    await app.user.paste("Yuki Tanaka");

    await app.user.click(continueButton());

    await waitFor(() => {
      if (describedBy(nameField()).length === 0) throw new Error("test: nothing describes the field");
    });
    expect(nameField().getAttribute("aria-invalid")).toBe("true");
    await focusOn(nameField);
    expect(main().getAllByRole("alert").map(words)).toEqual([
      "Nothing was saved. Fix what is marked, or skip for now.",
    ]);
    expect(app.path()).toBe(welcome);
  });

  test("an image the server cannot use is said under the avatar in its words, with the way on, and the focus on the avatar's button", async () => {
    const { app } = await step({ ...newAccount, uploads: "refused" });
    await chooseFiles(app, within(avatar()).getByRole("button", { name: "Choose an image" }), imageFile("me.png"));

    await app.user.click(continueButton());

    await waitFor(() => {
      if (!describedBy(avatar()).some((part) => part.endsWith(" Or remove the image and continue.")))
        throw new Error(`test: the avatar is described by ${JSON.stringify(describedBy(avatar()))}`);
    });
    // The focus is on a button of the avatar's.
    await waitFor(() => {
      const on = document.activeElement;
      if (
        on === null ||
        !within(avatar())
          .queryAllByRole("button")
          .includes(on as HTMLElement)
      )
        throw new Error(`test: the focus is on ${words(on ?? document.body).slice(0, 80)}`);
    });
    expect(main().getAllByRole("alert").map(words)).toEqual([
      "Nothing was saved. Fix what is marked, or skip for now.",
    ]);
    expect(app.path()).toBe(welcome);
  });

  test("while the save is on its way, Continue is busy and Skip for now is disabled", async () => {
    const { app } = await step({ ...newAccount, profile: { start: "empty", save: "waits" } });
    await app.user.click(nameField());
    await app.user.paste("Yuki Tanaka");

    await app.user.click(continueButton());

    await waitFor(() => {
      if (continueButton().getAttribute("aria-busy") !== "true") throw new Error("test: Continue is not busy");
    });
    expect(skipButton().hasAttribute("disabled") || skipButton().getAttribute("aria-disabled") === "true").toBe(true);
  });

  test("a name over 30 characters is not sent", async () => {
    const { app, saved } = await step();
    await app.user.click(nameField());
    await app.user.paste("A name of thirty-one characters");

    await app.user.click(continueButton());

    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(saved.seen).toHaveLength(0);
    expect(app.path()).toBe(welcome);
  });
});

test("an account that is not new goes on to the home timeline in the step's place", async () => {
  freshDevice();
  const app = await renderApp("/lists");

  await app.navigate(welcome);

  await home(app);
  await app.back();
  expect(app.path()).toBe("/lists");
});

test("a visitor is asked to sign in, and back to the step", async () => {
  await renderApp(welcome, { visitor: true });

  const asked = main().getByRole("status");
  expect(asked.textContent).toMatch(/^Sign in to see this screen\.\s*Sign in$/);
  expect(within(asked).getByRole("link", { name: "Sign in" }).getAttribute("href")).toBe("/login?return_to=%2Fwelcome");
});
