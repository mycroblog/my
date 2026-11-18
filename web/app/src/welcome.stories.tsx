// SPDX-License-Identifier: AGPL-3.0-only

// The welcome step of a new account, at /welcome (routes/welcome.tsx), on the fake server as a reader with neither a
// display name nor an avatar: the whole app at the step's address, a page of its own outside the frame; and the states
// that follow what the reader writes or presses, which no address leads to and the step is opened in (its specimen),
// the page alone on a fake server of its own: a name written, an image picked, a name the server refused, an image it
// could not use, the save on its way, and the page while the account is read. A refusal is the fake's own: the story
// saves what it starts with on its fake, in the story's language, and opens the step in what the save rejected with,
// so that it shows what the step makes of the server's answer and none of the story's words. An account that is not
// new goes on to the home timeline, which is the route's to do and no story.

import { AppDataProvider, createAppData, updateProfile } from "@my/app-data";
import type { WelcomeSpecimen } from "@my/app-features";
import { useLocale } from "@my/design-system";
import { images } from "@my/design-system/samples";
import { createFakeApi, fakeHost, fakeOrigin } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useState } from "react";
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

/** What a specimen starts with, and how its fake answers. */
type Opened = {
  start?: NonNullable<WelcomeSpecimen["start"]>;
  /** The save is on its way. */
  sending?: boolean;
  /** The fake refuses the save of what the specimen starts with, and the step opens in that refusal. */
  refused?: boolean;
  /** The fake refuses every upload, as a file it cannot read as an image. */
  uploads?: "refused";
  /** The fake never answers the account, so that the page stays as it is while it reads. */
  reading?: boolean;
};

/**
 * The page alone, opened as `opened` says, on a fake server of its own as a new account reads it, which answers in
 * the story's language.
 */
function Specimen({ opened }: { opened: Opened }) {
  const { start = {}, sending = false, refused = false, uploads, reading = false } = opened;
  const { locale } = useLocale();
  const [data] = useState(() => {
    const fake = createFakeApi({ ...newAccount, ...(uploads === undefined ? {} : { uploads }) });
    const fetch = (request: Request) =>
      reading && new URL(request.url).pathname.endsWith("/accounts/verify_credentials")
        ? new Promise<Response>(() => undefined)
        : fake.serve(request);
    return createAppData(
      createClient({ server: fakeOrigin, retry: { disabled: true }, fetch, acceptLanguage: () => locale }),
    );
  });
  // What the fake rejects the save of the start with, as the step's own save would hear it.
  const [refusal, setRefusal] = useState<{ error: unknown }>();
  const { displayName, avatar } = start;
  useEffect(() => {
    if (!refused) return;
    let gone = false;
    updateProfile(data, {
      ...(displayName === undefined ? {} : { display_name: displayName }),
      ...(avatar === undefined ? {} : { avatar: avatar.file }),
    }).then(
      () => undefined,
      (error: unknown) => {
        if (!gone) setRefusal({ error });
      },
    );
    return () => {
      gone = true;
    };
  }, [data, refused, displayName, avatar]);
  if (refused && refusal === undefined) return null;
  const phase: WelcomeSpecimen["phase"] =
    refusal !== undefined ? { kind: "refused", error: refusal.error } : sending ? { kind: "sending" } : undefined;
  return (
    <AppDataProvider data={data}>
      <WelcomePage serverName={fakeHost} onHome={() => undefined} specimen={{ start, phase }} />
    </AppDataProvider>
  );
}

/** A story of the step opened as `how` says. */
const opened = (how: Opened): Story => ({
  render: (_, { id }) => <Specimen key={id} opened={how} />,
});

/** An image as picked from the device, shown from an address of the story's own. */
const picked = { file: new File([], "avatar.svg", { type: "image/svg+xml" }), url: images.dusk };

/** A display name written: the avatar's initial is the name's. */
export const NameWritten: Story = opened({ start: { displayName: "Yuki Tanaka" } });

/** An image picked: it shows in the avatar's circle as it will show, with Replace and Remove. */
export const PicturePicked: Story = opened({ start: { displayName: "Yuki Tanaka", avatar: picked } });

/**
 * A name the server refused, one with a tab in it: the rule a display name follows under the field, and before
 * Continue that nothing was saved, with the ways on.
 */
export const NameRefused: Story = opened({ start: { displayName: "Yuki\tTanaka" }, refused: true });

/**
 * An image the server could not read: its words under the avatar with the other way on, removing it, and before
 * Continue that nothing was saved, the name neither.
 */
export const PictureRefused: Story = opened({
  start: { displayName: "Yuki Tanaka", avatar: picked },
  refused: true,
  uploads: "refused",
});

/** The save on its way: Continue is busy, and nothing can be changed meanwhile. */
export const Saving: Story = opened({ start: { displayName: "Yuki Tanaka", avatar: picked }, sending: true });

/** The page while the account is read: the server's mark, and that it loads. */
export const Loading: Story = opened({ reading: true });
