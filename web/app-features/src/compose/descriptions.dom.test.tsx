// SPDX-License-Identifier: AGPL-3.0-only

// The composer with an image that has no description where one is required (#3199): Post sends nothing, and the
// image's description says who requires it, the server or the reader's own settings, so that the reader knows where
// the rule can be changed. Which sentence each reason gives is the model's (model.test.ts), and who requires what is
// the data's (app-data, preferences.test.ts).

import { expect, test } from "bun:test";
import { defaultComposeSettings, instanceQuery, preferencesQuery, type MediaAttachment } from "@my/app-data";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { ComposerView } from "./composer";
import { emptyDraft, type Draft } from "./model";
import { useComposer } from "./use-composer";

const none = () => undefined;

/** A post of some words and one image the server holds, which nobody described. */
const initial: Draft = {
  ...emptyDraft({ kind: "new" }, defaultComposeSettings, "k"),
  text: "A walk.",
  attachments: [
    {
      key: "a1",
      state: "ready",
      media: { id: "m-a1", type: "image", state: "ready", url: "/a1.png" } as unknown as MediaAttachment,
      description: "",
    },
  ],
};

/** The composer of that post on a server of `options`, once the app read the instance and the reader's preferences. */
async function composing(options: FakeOptions) {
  const { fake, data } = fakeData(options);
  await data.queryClient.query(instanceQuery(data));
  await data.queryClient.query(preferencesQuery(data));
  function Composing() {
    const c = useComposer(initial, { onPosted: none, onKept: none });
    return <ComposerView c={c} layer={false} onClose={none} onDismiss={none} />;
  }
  const user = renderFeature(data, <Composing />);
  const dialog = within(screen.getByRole("dialog", { name: "New post" }));
  // The text's editor is a chunk of its own: the composer is whole once it came.
  await dialog.findByRole("textbox", { name: "Text" });
  return { fake, user, dialog };
}

/** What describes `field`, part by part. */
const described = (field: Element) =>
  (field.getAttribute("aria-describedby") ?? "").split(" ").map((id) => document.getElementById(id)?.textContent);

const reasons = [
  [
    "the server",
    { requiresDescriptions: true },
    "This server requires a description (alt text) for every image. Add one to each image before posting.",
  ],
  [
    "the reader's own settings",
    { preferences: { require_media_description: true } },
    "Your settings require a description (alt text) for every image. Add one to each image before posting.",
  ],
] as const;

for (const [who, options, why] of reasons)
  test(`where ${who} require a description, Post sends nothing and the image's description says who requires it`, async () => {
    const { fake, user, dialog } = await composing(options);
    const sent = fake.script("POST /statuses");
    const description = dialog.getByRole("textbox", { name: "Description" });
    expect(described(description)).not.toContain(why);

    await user.click(dialog.getByRole("button", { name: "Post" }));

    await waitFor(() => {
      expect(described(description)).toContain(why);
    });
    expect(description.getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByRole("dialog", { name: "New post" })).toBeDefined();
    expect(sent.seen).toHaveLength(0);
  });
