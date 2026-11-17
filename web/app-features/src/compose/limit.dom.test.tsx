// SPDX-License-Identifier: AGPL-3.0-only

// The composer over the server's limit of a post's text and content warning together: the count says by how much, and
// Post sends nothing and says why on the text. How the limit is counted is the model's (model.test.ts), the mark over
// the excess a test in a browser (web/e2e/catalog/specs/browser/composer.spec.ts), and that a post at the limit is
// the server's too an E2E test (web/e2e/specs/editor.spec.ts).

import { expect, test } from "bun:test";
import { defaultComposeSettings } from "@my/app-data";
import { screen, within } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { ComposerView } from "./composer";
import { emptyDraft, type Draft } from "./model";
import { useComposer } from "./use-composer";

const none = () => undefined;

/** The composer's view of `initial` in the flow of the page. */
function Composing({ initial }: { initial: Draft }) {
  const c = useComposer(initial, { onPosted: none, onKept: none });
  return <ComposerView c={c} layer={false} onClose={none} onDismiss={none} />;
}

test("a post over the limit of its text and content warning is held back: the count says by how much, Post sends nothing, and the text says why", async () => {
  // 500 at most, by code point: a custom emoji's shortcode counts its characters, an astral emoji one.
  const cw = "注意 😀";
  const head = "Limit :teacup: 😀 日本語 ";
  const text = `${head}${"a".repeat(500 - Array.from(cw).length - Array.from(head).length)}xyz`;
  const { fake, data } = fakeData();
  const sent = fake.script("POST /statuses");
  const user = renderFeature(
    data,
    <Composing initial={{ ...emptyDraft({ kind: "new" }, defaultComposeSettings, "k"), text, cw }} />,
  );
  const dialog = within(screen.getByRole("dialog", { name: "New post" }));
  const box = await dialog.findByRole("textbox", { name: "Text" });
  expect(dialog.getByText("3 characters over")).toBeDefined();

  await user.click(dialog.getByRole("button", { name: "Post" }));

  const why = "The text and content warning must be at most 500 characters.";
  expect(await dialog.findByText(why)).toBeDefined();
  expect(box.getAttribute("aria-invalid")).toBe("true");
  const described = (box.getAttribute("aria-describedby") ?? "")
    .split(/\s+/)
    .map((id) => document.getElementById(id)?.textContent ?? "");
  expect(described).toContain(why);
  expect(screen.getByRole("dialog", { name: "New post" })).toBeDefined();
  expect(sent.seen).toHaveLength(0);
});
