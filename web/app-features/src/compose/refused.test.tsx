// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { defaultComposeSettings, type MediaAttachment } from "@my/app-data";
import { render, server } from "../test/server";
import { ComposerView } from "./composer";
import { emptyDraft, type Draft } from "./model";
import { useComposer, type Phase } from "./use-composer";

const none = () => undefined;
const title = "This could not be done.";
const detail = "This value is not valid. Check what you entered.";

/** The composer's view of `fields` in `phase`, as a story's specimen opens it. */
function composer(fields: Partial<Draft>, phase: Phase) {
  const initial = { ...emptyDraft({ kind: "new" }, defaultComposeSettings, "k"), text: "when?", ...fields };
  function Composing() {
    const c = useComposer(initial, { onPosted: none, onKept: none }, phase);
    return <ComposerView c={c} layer={false} onClose={none} onDismiss={none} />;
  }
  return render(server({}).data, <Composing />);
}

/** The opening tag of each element of `html` marked invalid. */
const invalid = (html: string) => html.match(/<[a-z]+[^>]*aria-invalid="true"[^>]*>/g) ?? [];

/** The text of the element of `html` whose ID is `id`. */
const textOf = (html: string, id: string) =>
  new RegExp(`<([a-z]+)\\b[^>]*\\bid="${id}"[^>]*>(.*?)</\\1>`).exec(html)?.[2]?.replace(/<[^>]+>/g, "") ?? "";

/** The text of the form's alert. */
const alertOf = (html: string) => /role="alert"[^>]*>(.*?)<\/(?:p|div)>/.exec(html)?.[1]?.replace(/<[^>]+>/g, "") ?? "";

const failed = (input: string): Phase => ({ kind: "failed", reason: title, field: { input, text: detail } });

test("a poll's option refused stands invalid, described by what is wrong, and the alert says the title alone", () => {
  const html = composer(
    { poll: { options: ["Sat", "Sun"], duration: 3600, multiple: false, hideTotals: false } },
    failed("option:1"),
  );
  const marked = invalid(html);
  expect(marked).toHaveLength(1);
  expect(marked[0]).toContain('value="Sun"');
  const described = /aria-describedby="([^"]+)"/.exec(marked[0] ?? "")?.[1] ?? "";
  expect(described.split(" ").map((id) => textOf(html, id))).toContain(detail);
  expect(alertOf(html)).toBe(title);
});

test("an image's description refused stands invalid under that image, described by what is wrong", () => {
  const ready = (key: string, description: string) => ({
    key,
    state: "ready" as const,
    media: { id: `m-${key}`, type: "image", state: "ready", url: `/${key}.png` } as unknown as MediaAttachment,
    description,
  });
  const html = composer({ attachments: [ready("a1", "a hill"), ready("a2", "a river")] }, failed("media:a2"));
  const marked = invalid(html);
  expect(marked).toHaveLength(1);
  const described = /aria-describedby="([^"]+)"/.exec(marked[0] ?? "")?.[1] ?? "";
  expect(described.split(" ").map((id) => textOf(html, id))).toContain(detail);
  // The second image's description, after the first's.
  expect(html.indexOf(marked[0] ?? "")).toBeGreaterThan(html.indexOf(">a hill<"));
  expect(alertOf(html)).toBe(title);
});

test("a refusal of no input says the server's words in the alert, and marks no input", () => {
  const html = composer({}, { kind: "failed", reason: "The post was refused." });
  expect(invalid(html)).toEqual([]);
  expect(alertOf(html)).toBe("The post was refused.");
});
