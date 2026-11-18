// SPDX-License-Identifier: AGPL-3.0-only

// The welcome step of a new account, at /welcome (routes/welcome.tsx), on the fake server as a reader with neither a
// display name nor an avatar: the whole app at the step's address, a page of its own outside the frame; and the states
// that follow what the reader writes or presses, which no address leads to and the step is opened in (its specimen),
// the page alone on a fake server of its own: a name written, an image picked, a name the server refused, an image it
// could not use, the save on its way, and the page while the account is read. An account that is not new goes on to
// the home timeline, which is the route's to do and no story.

import { AppDataProvider, createAppData } from "@my/app-data";
import type { WelcomeSpecimen } from "@my/app-features";
import { images } from "@my/design-system/samples";
import { createFakeApi, fakeHost, fakeOrigin } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { screen, screenMeta } from "./app.samples";
import { WelcomePage, welcomePath } from "./shell/welcome";

/** A reader with nothing on their profile yet, as an account is after signing up. */
const newAccount = { profile: { start: "empty" } } as const;

/** The device's key of the accounts that passed the step (app-features' welcome/device.ts). */
const passedKey = "my:welcome-passed";

const meta = {
  title: "Welcome",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: welcomePath }) },
  // A story starts on a device no account passed the step on, and leaves it so.
  beforeEach: () => {
    localStorage.removeItem(passedKey);
    return () => {
      localStorage.removeItem(passedKey);
    };
  },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** /welcome as a new account arrives at it: the two fields, empty, Continue and Skip for now. */
export const New: Story = { parameters: screen({ at: welcomePath, server: newAccount }) };

/**
 * The page alone, opened in `specimen`, on a fake server of its own as a new account reads it; `reading` never
 * answers the account, so that the page stays as it is while it reads.
 */
function Specimen({ specimen, reading = false }: { specimen: WelcomeSpecimen; reading?: boolean }) {
  const [data] = useState(() => {
    const fake = createFakeApi(newAccount);
    const fetch = (request: Request) =>
      reading && new URL(request.url).pathname.endsWith("/accounts/verify_credentials")
        ? new Promise<Response>(() => undefined)
        : fake.serve(request);
    return createAppData(createClient({ server: fakeOrigin, retry: { disabled: true }, fetch }));
  });
  return (
    <AppDataProvider data={data}>
      <WelcomePage serverName={fakeHost} onHome={() => undefined} specimen={specimen} />
    </AppDataProvider>
  );
}

/** A story of the step opened in `specimen`. */
const opened = (specimen: WelcomeSpecimen, reading = false): Story => ({
  render: (_, { id }) => <Specimen key={id} specimen={specimen} reading={reading} />,
});

/** An image as picked from the device, shown from an address of the story's own. */
const picked = { file: new File([], "avatar.svg", { type: "image/svg+xml" }), url: images.dusk };

/** A display name written: the avatar's initial is the name's. */
export const NameWritten: Story = opened({ start: { displayName: "Yuki Tanaka" } });

/** An image picked: it shows in the avatar's circle as it will show, with Replace and Remove. */
export const PicturePicked: Story = opened({ start: { displayName: "Yuki Tanaka", avatar: picked } });

/** A name the server refused: what is wrong under the field, and that nothing was saved before Continue. */
export const NameRefused: Story = opened({
  start: { displayName: "Yuki Tanaka" },
  phase: {
    kind: "refused",
    refused: {
      reason: "This could not be done.",
      field: { input: "name", text: "This value is not valid. Check what you entered." },
    },
  },
});

/** An image the server could not use: the server's words under the avatar, and that nothing was saved before Continue. */
export const PictureRefused: Story = opened({
  start: { displayName: "Yuki Tanaka", avatar: picked },
  phase: {
    kind: "refused",
    refused: {
      reason: undefined,
      field: { input: "avatar", text: "This file is not an image this server can use." },
    },
  },
});

/** The save on its way: Continue is busy, and nothing can be changed meanwhile. */
export const Saving: Story = opened({
  start: { displayName: "Yuki Tanaka", avatar: picked },
  phase: { kind: "sending" },
});

/** The page while the account is read: the server's mark, and that it loads. */
export const Loading: Story = opened({}, true);
