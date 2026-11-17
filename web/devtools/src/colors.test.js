// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { colorsInCSS, namedColor, rawColors } from "./colors.js";

test("rawColors finds the hex colours, the colour functions and the named colours of arbitrary values", () => {
  /** @type {[string, string[]][]} */
  const cases = [
    ["#fff", ["#fff"]],
    ["#FFFFFF80", ["#FFFFFF80"]],
    ["1px solid #ccc", ["#ccc"]],
    ["bg-[#ff6b6b] text-[#111]/50", ["#ff6b6b", "#111"]],
    ["shadow-[0_0_0_1px_#0008]", ["#0008"]],
    ["rgb(0 0 0 / 50%)", ["rgb()"]],
    ["0 0 0 1px rgba(0, 0, 0, 0.2), 0 1px hsl(0 0% 0%)", ["rgba()", "hsl()"]],
    ["border-[oklch(0.7_0.1_200)]", ["oklch()"]],
    ["color(display-p3 1 0 0)", ["color()"]],
    ["color-mix(in srgb, #fff 50%, lab(50 0 0))", ["#fff", "lab()"]],
    ["text-[red] [color:rebeccapurple] fill-[color:White]", ["red", "rebeccapurple", "White"]],
  ];
  for (const [text, colors] of cases) expect([text, rawColors(text)]).toEqual([text, colors]);
});

test("rawColors leaves the tokens, what is no colour of its own and what only looks like one", () => {
  for (const text of [
    "bg-surface text-muted-text border-spinner-track",
    "var(--color-surface)",
    "bg-[var(--color-surface)] text-[length:var(--text-title)]",
    "color-mix(in srgb, var(--color-text) 50%, transparent)",
    "currentColor",
    "transparent",
    "#main",
    "#",
    "/tags#add",
    "https://my.example/page#fed",
    "&#8203;",
    "#12345",
    "calc(1px * var(--my-direction))",
    "text-red",
    "red",
  ])
    expect([text, rawColors(text)]).toEqual([text, []]);
});

test("namedColor finds the named colours among the words of a colour's value", () => {
  expect(namedColor("red")).toEqual(["red"]);
  expect(namedColor("1px solid DarkSlateGray")).toEqual(["DarkSlateGray"]);
  expect(namedColor("linear-gradient(to bottom, white, transparent)")).toEqual(["white"]);
  for (const value of [
    "currentColor",
    "transparent",
    "inherit",
    "none",
    "var(--color-red)",
    "url(white.png)",
    "1px solid",
  ])
    expect([value, namedColor(value)]).toEqual([value, []]);
});

test("colorsInCSS reads the declarations of a CSS file, not its comments or its selectors", () => {
  const source = `/* color: #fff; background: red */
#fed, .tan:hover { color: var(--color-text); background: transparent; }
.a {
  color: #111;
  background: White;
  border-inline-start: 1px solid rgb(0 0 0 / 10%);
  box-shadow: 0 0 0 1px hsl(0 0% 0%), 0 1px black;
  --mine: oklch(0.7 0.1 200);
  --other: tomato;
  font-family: tan, sans-serif;
  content: "red";
  outline-color: currentColor;
  background-image: url(red.png);
}
@media (min-width: 40rem) { .b { fill:#3f6f86 } }
`;
  expect(colorsInCSS("a.css", source).map((f) => [f.line, f.color])).toEqual([
    [4, "#111"],
    [5, "White"],
    [6, "rgb()"],
    [7, "hsl()"],
    [7, "black"],
    [8, "oklch()"],
    [9, "tomato"],
    [15, "#3f6f86"],
  ]);
});
