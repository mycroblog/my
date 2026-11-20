// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { checkFrame } from "./check";
import { frameFile, renderFrame } from "./frame";

test("the committed layout is up to date (scripts/task.sh mail:frame rewrites it)", async () => {
  expect(await Bun.file(frameFile).text()).toBe(renderFrame());
});

test("the layout keeps the rules of an email's HTML", () => {
  expect(checkFrame(renderFrame())).toEqual([]);
});

test("the layout keeps Go's template syntax and leaves no marker", () => {
  const frame = renderFrame();
  expect(frame).toContain('<html lang="{{.Lang}}" dir="{{$.Dir}}">');
  expect(frame).toContain("{{range .Paragraphs}}");
  expect(frame).toContain("{{with .Action}}{{if .URL}}");
  expect(frame).toContain("{{comment `<!--[if mso]>");
  expect(frame).not.toContain("<template");
  expect(frame).not.toContain("&quot;");
  expect(frame).not.toMatch(/#[0-9a-f]{6}/i);
});
