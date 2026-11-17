// SPDX-License-Identifier: AGPL-3.0-only

// A post the server refused, in the composer: the input it names stands invalid, described by what is wrong with it,
// and the form's alert says the refusal's title alone; a refusal of no input says the server's words in the alert.

import { expect, test } from "bun:test";
import { defaultComposeSettings, type MediaAttachment } from "@my/app-data";
import { screen, within } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { ComposerView } from "./composer";
import { emptyDraft, type Draft } from "./model";
import { useComposer, type Phase } from "./use-composer";

const none = () => undefined;
const title = "This could not be done.";
const detail = "This value is not valid. Check what you entered.";

/** The composer's view of `fields` in `phase`, as a story's specimen opens it: its dialog. */
function composer(fields: Partial<Draft>, phase: Phase) {
  const initial = { ...emptyDraft({ kind: "new" }, defaultComposeSettings, "k"), text: "when?", ...fields };
  function Composing() {
    const c = useComposer(initial, { onPosted: none, onKept: none }, phase);
    return <ComposerView c={c} layer={false} onClose={none} onDismiss={none} />;
  }
  renderFeature(fakeData().data, <Composing />);
  return within(screen.getByRole("dialog"));
}

/** The fields of the composer that are marked invalid. */
const invalid = (form: ReturnType<typeof composer>) =>
  form
    .queryAllByRole<HTMLInputElement | HTMLTextAreaElement>("textbox")
    .filter((field) => field.getAttribute("aria-invalid") === "true");

/** What describes `field`, part by part. */
const described = (field: Element) =>
  (field.getAttribute("aria-describedby") ?? "").split(" ").map((id) => document.getElementById(id)?.textContent);

const failed = (input: string): Phase => ({ kind: "failed", reason: title, field: { input, text: detail } });

test("a poll's option refused stands invalid, described by what is wrong, and the alert says the title alone", () => {
  const form = composer(
    { poll: { options: ["Sat", "Sun"], duration: 3600, multiple: false, hideTotals: false } },
    failed("option:1"),
  );

  const marked = invalid(form);
  expect(marked.map((field) => field.value)).toEqual(["Sun"]);
  expect(marked.flatMap(described)).toContain(detail);
  expect(form.getByRole("alert").textContent).toBe(title);
});

test("an image's description refused stands invalid under that image, described by what is wrong", () => {
  const ready = (key: string, description: string) => ({
    key,
    state: "ready" as const,
    media: { id: `m-${key}`, type: "image", state: "ready", url: `/${key}.png` } as unknown as MediaAttachment,
    description,
  });
  const form = composer({ attachments: [ready("a1", "a hill"), ready("a2", "a river")] }, failed("media:a2"));

  const marked = invalid(form);
  // The second image's description, after the first's.
  expect(marked.map((field) => field.value)).toEqual(["a river"]);
  expect(marked.flatMap(described)).toContain(detail);
  expect(form.getByRole("alert").textContent).toBe(title);
});

test("a refusal of no input says the server's words in the alert, and marks no input", () => {
  const form = composer({}, { kind: "failed", reason: "The post was refused." });

  expect(invalid(form)).toHaveLength(0);
  expect(form.getByRole("alert").textContent).toBe("The post was refused.");
});
