// SPDX-License-Identifier: AGPL-3.0-only

// Where the focus goes once a screen under Apps shows what the server shows once (#3206), on jsdom: the form or the
// screen the reader pressed in gives way to the view, and what takes the focus there is no control (the title of the
// summary that says an app is registered, a token is issued, or a new secret is ready; and, back from the new secret,
// the title of the app's screen); happy-dom gives the focus to any element a script asks it for, focusable or not.
// What each view shows is pinned on happy-dom (register.dom.test.tsx, token.dom.test.tsx, app.dom.test.tsx), where
// the focus on a field or a button of a refused form is pinned too.

import { expect, test } from "bun:test";
import { screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { field, write } from "../../test/apps";
import { atRest, closed, focused, focusOn, offered } from "../../test/security";
import { requestsOnJsdom, settingsOn, type addressProps } from "../../test/settings";
import { OwnApp } from "./app";
import { AppRegister } from "./register";
import { AppToken } from "./token";

/** The title named `name`, once it is on the screen. */
const title = (name: string) => () => screen.queryAllByRole("heading", { name })[0];

/** The screen of the reader's first app, the confidential Morning bot, that `make` builds, with its `shows` drawn. */
async function ofMorningBot(make: (id: string, address: ReturnType<typeof addressProps>) => ReactNode, shows: string) {
  requestsOnJsdom();
  let id = "";
  const on = await settingsOn({}, (address) => make(id, address), {
    before: ({ fake }) => {
      id = fake.appId("own", 0);
    },
  });
  await screen.findByRole("region", { name: shows });
  await atRest(on);
  return on;
}

test("an app that was registered takes the focus to the title that says so", async () => {
  requestsOnJsdom();
  const on = await settingsOn({}, (address) => <AppRegister {...address} />);
  await screen.findByRole("textbox", { name: "Name" });
  await atRest(on);
  await write(on.user, field("Name"), "Reading list");
  await write(on.user, field("Redirect URIs"), "https://reading.example/callback");
  await write(on.user, field("Scopes"), "read");

  await on.user.click(screen.getByRole("button", { name: "Register an app" }));

  await focusOn(title("“Reading list” is registered"));
  expect(focused()).toBe("heading: “Reading list” is registered");
  // The title of a section of a screen under an entry.
  expect(screen.getByRole("heading", { level: 3, name: "“Reading list” is registered" })).not.toBeNull();
});

test("a token that was issued takes the focus to the title that says so", async () => {
  const { user } = await ofMorningBot((id, address) => <AppToken id={id} {...address} />, "Token for your own account");

  await user.click(screen.getByRole("button", { name: "Issue token" }));

  await focusOn(title("The token is issued"));
  expect(focused()).toBe("heading: The token is issued");
});

test("a new secret takes the focus to the title that says it is ready, which keeps it once the confirmation is gone; Finish then gives the focus to the title of the app's screen", async () => {
  const { user } = await ofMorningBot((id, address) => <OwnApp id={id} {...address} />, "Credentials");

  await user.click(offered("Credentials", "Client secret", "Make a new secret"));
  const asking = await screen.findByRole("alertdialog", { name: "Make a new secret for Morning bot?" });
  await user.click(within(asking).getByRole("button", { name: "Make a new secret" }));

  await closed();
  await focusOn(title("The new secret is ready"));
  // A confirmation gives the focus back a frame after it left, to the button that opened it: that button is gone
  // with the screen, and the title keeps the focus past that frame.
  await new Promise((frames) => setTimeout(frames, 50));
  expect(focused()).toBe("heading: The new secret is ready");

  await user.click(screen.getByRole("button", { name: "Finish" }));

  await focusOn(title("Morning bot"));
  expect(focused()).toBe("heading: Morning bot");
  // The app's screen is back, with its sections under the title.
  expect(screen.getAllByRole("region", { name: "Credentials" })).toHaveLength(1);
});
