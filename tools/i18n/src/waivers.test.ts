// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test } from "bun:test";
import { addedWaivers, CAP, loadWaivers, reasonProblem, type Waiver, waiverErrors, waiverKey } from "./waivers";

const reason =
  "The document title separates the page from the server name in a browser tab, where the title bar shows no layout.";
const waiver = (over: Partial<Waiver> = {}): Waiver => ({
  id: "server:auth.document_title",
  locale: "en",
  rule: "dash",
  reason,
  ...over,
});

test("the committed waivers hold", () => {
  expect(() => loadWaivers()).not.toThrow();
});

test("a reason is refused when it is short or boilerplate", () => {
  expect(reasonProblem(reason)).toBeUndefined();
  expect(reasonProblem("")).toMatch(/at least 40/);
  expect(reasonProblem("intentional")).toMatch(/at least 40/);
  expect(reasonProblem("False positive, this is intentional and by design, not a bug.")).toMatch(/boilerplate/);
  expect(reasonProblem("OK OK OK OK OK OK OK OK OK OK OK OK OK OK OK OK")).toMatch(/boilerplate/);
  expect(reasonProblem("needed needed needed needed needed needed needed")).toMatch(/boilerplate/);
});

test("a waiver of a finding the message has passes; one without a reason, of a blocking rule, or of nothing is refused", () => {
  const found = new Set([waiverKey(waiver()), waiverKey(waiver({ locale: "ja" }))]);
  expect(waiverErrors([waiver()], found)).toEqual([]);
  expect(waiverErrors([waiver({ reason: "" })], found).join("\n")).toMatch(/at least 40/);
  expect(waiverErrors([waiver({ rule: "emoji" })], found).join("\n")).toMatch(/emoji is not waivable/);
  expect(waiverErrors([waiver({ id: "server:auth.other" })], found).join("\n")).toMatch(/waives nothing/);
  expect(waiverErrors([waiver({ locale: "de" })], found).join("\n")).toMatch(/en and ja only/);
  expect(waiverErrors([waiver(), waiver()], found).join("\n")).toMatch(/given twice/);
  expect(waiverErrors([waiver(), waiver({ locale: "ja" })], found).join("\n")).toMatch(/the same reason as/);
  expect(waiverErrors([{ id: "server:x" } as Waiver], found).join("\n")).toMatch(/a waiver is/);
});

test("more than CAP waivers added in one change are refused", () => {
  const many = Array.from({ length: CAP + 1 }, (_, i) => waiver({ id: `server:k${i}` }));
  expect(addedWaivers(many.slice(0, CAP), []).refusal).toBeUndefined();
  expect(addedWaivers(many, []).refusal).toMatch(new RegExp(`more than ${CAP}`));
  expect(addedWaivers(many, many.slice(0, 1)).refusal).toBeUndefined();
});
