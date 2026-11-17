// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { bothDirections, findIn, logicalOf } from "./direction";

test("logicalOf names the logical utility for each physical side", () => {
  const cases: [string, string][] = [
    ["ml-2", "ms-2"],
    ["mr-auto", "me-auto"],
    ["pl-disclosure-start", "ps-disclosure-start"],
    ["pr-[3px]", "pe-[3px]"],
    ["scroll-ml-4", "scroll-ms-4"],
    ["scroll-pr-4", "scroll-pe-4"],
    ["left-0", "inset-s-0"],
    ["right-1/2", "inset-e-1/2"],
    ["text-left", "text-start"],
    ["text-right", "text-end"],
    ["float-left", "float-start"],
    ["clear-right", "clear-end"],
    ["rounded-l", "rounded-s"],
    ["rounded-r-lg", "rounded-e-lg"],
    ["rounded-tl-md", "rounded-ss-md"],
    ["rounded-tr", "rounded-se"],
    ["rounded-bl", "rounded-es"],
    ["rounded-br-control", "rounded-ee-control"],
    ["border-l", "border-s"],
    ["border-r-2", "border-e-2"],
    ["border-l-muted-text", "border-s-muted-text"],
  ];
  for (const [physical, logical] of cases) expect([physical, logicalOf(physical)]).toEqual([physical, logical]);
});

test("logicalOf keeps the variants, the negative and the important mark", () => {
  expect(logicalOf("-ml-2")).toBe("-ms-2");
  expect(logicalOf("!pl-4")).toBe("!ps-4");
  expect(logicalOf("pl-4!")).toBe("ps-4!");
  expect(logicalOf("md:hover:mr-1")).toBe("md:hover:me-1");
  expect(logicalOf("max-[40rem]:[&>button]:pl-2")).toBe("max-[40rem]:[&>button]:ps-2");
});

test("logicalOf names the logical declaration of an arbitrary property", () => {
  expect(logicalOf("[margin-left:0]")).toBe("[margin-inline-start:0]");
  expect(logicalOf("hover:[padding-right:var(--gap)]")).toBe("hover:[padding-inline-end:var(--gap)]");
  expect(logicalOf("[text-align:left]")).toBe("[text-align:start]");
  expect(logicalOf("[left:calc(100%_-_1rem)]")).toBe("[inset-inline-start:calc(100%_-_1rem)]");
  for (const c of ["[margin-inline:0]", "[text-align:center]", "[word-break:auto-phrase]", "[&_p]:m-0"])
    expect([c, logicalOf(c)]).toEqual([c, null]);
});

test("bothDirections writes a utility without a logical form for both directions", () => {
  const cases: [string, string][] = [
    ["bg-left", "ltr:bg-left rtl:bg-right"],
    ["bg-right-top", "ltr:bg-right-top rtl:bg-left-top"],
    ["object-bottom-left", "ltr:object-bottom-left rtl:object-bottom-right"],
    ["origin-left", "ltr:origin-left rtl:origin-right"],
    ["origin-top-right", "ltr:origin-top-right rtl:origin-top-left"],
    ["bg-linear-to-r", "ltr:bg-linear-to-r rtl:bg-linear-to-l"],
    ["bg-gradient-to-bl", "ltr:bg-gradient-to-bl rtl:bg-gradient-to-br"],
    ["mask-l-from-50%", "ltr:mask-l-from-50% rtl:mask-r-from-50%"],
    ["touch-pan-left", "ltr:touch-pan-left rtl:touch-pan-right"],
    ["translate-x-press", "ltr:translate-x-press rtl:-translate-x-press"],
    ["-translate-x-1/2", "ltr:-translate-x-1/2 rtl:translate-x-1/2"],
    ["data-selected:translate-x-full!", "data-selected:ltr:translate-x-full! data-selected:rtl:-translate-x-full!"],
    ["!-translate-x-[3px]", "ltr:!-translate-x-[3px] rtl:!translate-x-[3px]"],
  ];
  for (const [physical, both] of cases) expect([physical, bothDirections(physical)]).toEqual([physical, both]);
});

test("bothDirections leaves what stands under ltr: or rtl:, what moves nothing and what turns with the page", () => {
  for (const c of [
    "ltr:bg-left",
    "rtl:bg-right",
    "rtl:-translate-x-full",
    "hover:ltr:translate-x-press",
    "translate-x-0",
    "translate-x-[calc(var(--shift)*var(--my-direction))]",
    "translate-y-press",
    "-translate-y-1/2",
    "bg-top",
    "bg-center",
    "origin-center",
    "object-cover",
    "bg-linear-to-t",
    "mask-t-from-50%",
    "touch-pan-x",
    "rtl:-scale-x-100",
  ])
    expect([c, bothDirections(c)]).toEqual([c, null]);
});

test("logicalOf leaves the logical utilities and look-alikes alone", () => {
  for (const c of [
    "ms-2",
    "pe-4",
    "px-2",
    "inset-s-0",
    "text-start",
    "rounded-lg",
    "rounded-s",
    "border-red-500",
    "border-lime-500",
    "rtl:-scale-x-100",
    "[&>button]:px-disclosure-padding",
    "left:",
    "margin-left:",
  ])
    expect([c, logicalOf(c)]).toEqual([c, null]);
});

test("findIn reads the code of a TSX file, not its comments, with the line of each class", () => {
  const source = `// pl-2 in a comment
/* text-left in a block */
export const A = () => <p className="flex ml-2 items-center">Don't</p>;
export const b = \`pr-\${1}\`;
`;
  expect(findIn("a.tsx", source)).toEqual([
    { file: "a.tsx", line: 3, physical: "ml-2", logical: "ms-2" },
    { file: "a.tsx", line: 4, physical: "pr-$", logical: "pe-$" },
  ]);
});

test("findIn reads a .ts file as TypeScript, where a generic arrow function is not a tag", () => {
  const source = `const first = <T>(items: T[]): T | undefined => items[0];
export const c = "pl-1";
`;
  expect(findIn("a.ts", source)).toEqual([{ file: "a.ts", line: 2, physical: "pl-1", logical: "ps-1" }]);
});

test("findIn reads a CSS file's utilities and physical declarations, not its comments or selectors", () => {
  const source = `/* rounded-l; margin-left: 0 */
.a { @apply rounded-l-md text-right; }
.b:hover {
  margin-left: 0;
  padding-right: 1px;
  scroll-margin-left: 2px;
  scroll-padding-right: 2px;
  left: 0;
  right: auto !important;
  border-left: 1px solid;
  border-right-width: 2px;
  border-left-color: red;
  border-right-style: dotted;
  border-top-left-radius: 1px;
  border-top-right-radius: 1px;
  border-bottom-left-radius: 1px;
  border-bottom-right-radius: 1px;
  text-align: left;
  float: right;
  clear: left !important;
  margin-inline-start: 0;
  text-align: center;
  inset-inline-end: 0;
}
@media (min-width: 40rem) { .c { text-align:right } }
`;
  expect(findIn("a.css", source).map((f) => [f.line, f.physical, f.logical])).toEqual([
    [2, "rounded-l-md", "rounded-s-md"],
    [2, "text-right", "text-end"],
    [4, "margin-left", "margin-inline-start"],
    [5, "padding-right", "padding-inline-end"],
    [6, "scroll-margin-left", "scroll-margin-inline-start"],
    [7, "scroll-padding-right", "scroll-padding-inline-end"],
    [8, "left", "inset-inline-start"],
    [9, "right", "inset-inline-end"],
    [10, "border-left", "border-inline-start"],
    [11, "border-right-width", "border-inline-end-width"],
    [12, "border-left-color", "border-inline-start-color"],
    [13, "border-right-style", "border-inline-end-style"],
    [14, "border-top-left-radius", "border-start-start-radius"],
    [15, "border-top-right-radius", "border-start-end-radius"],
    [16, "border-bottom-left-radius", "border-end-start-radius"],
    [17, "border-bottom-right-radius", "border-end-end-radius"],
    [18, "text-align: left", "text-align: start"],
    [19, "float: right", "float: inline-end"],
    [20, "clear: left", "clear: inline-start"],
    [25, "text-align: right", "text-align: end"],
  ]);
});

test("findIn reads the physical keys of a TSX file's style objects, and left or right only in style={{…}}", () => {
  const source = `const box = { left: 1, right: 2 };
const quiet = { marginLeft: 4, paddingRight: 2, borderTopLeftRadius: 3, textAlign: "left", float: "right" };
const fine = { marginInlineStart: 4, textAlign: "center", float: "none" };
export const A = (on: boolean) => (
  <p style={{ position: "fixed", left: on ? 1 : 2, top: 0, right: 3 }}>{box.left}</p>
);
export { quiet, fine };
`;
  expect(findIn("a.tsx", source).map((f) => [f.line, f.physical, f.logical])).toEqual([
    [2, "marginLeft", "marginInlineStart"],
    [2, "paddingRight", "paddingInlineEnd"],
    [2, "borderTopLeftRadius", "borderStartStartRadius"],
    [2, 'textAlign: "left"', 'textAlign: "start"'],
    [2, 'float: "right"', 'float: "inline-end"'],
    [5, "left", "insetInlineStart"],
    [5, "right", "insetInlineEnd"],
  ]);
});

test("findIn reads left and right in a style object that a style names, and in one typed CSSProperties", () => {
  const source = `import type { CSSProperties } from "react";
const box = { left: 1, right: 2 };
const pinned = { position: "fixed", left: 0 } as const;
const base = { top: 0 };
const moved = { ...base, right: box.right };
const typed: CSSProperties = {
  right: 4,
};
const fine = { insetInlineStart: box.left, top: 0 };
export const A = ({ on }: { on: boolean }) => (
  <p style={on ? pinned : moved}>
    <span style={fine}>{box.left}</span>
  </p>
);
export { typed };
`;
  expect(findIn("a.tsx", source).map((f) => [f.line, f.physical, f.logical])).toEqual([
    [3, "left", "insetInlineStart"],
    [5, "right", "insetInlineEnd"],
  ]);
  const typedOnly = `import type { CSSProperties } from "react";
const box = { left: 1 };
export const typed: CSSProperties = {
  left: box.left,
};
`;
  expect(findIn("a.ts", typedOnly).map((f) => [f.line, f.physical, f.logical])).toEqual([
    [4, "left", "insetInlineStart"],
  ]);
});

test("findIn reads a utility without a logical form and a translateX() that does not turn with the page", () => {
  const source = `export const a = "bg-left ltr:origin-left rtl:origin-right translate-x-0 hover:translate-x-press";
export const b = { transform: "translateX(4px)" };
const along = (sign: number) => \`translateX(calc(\${sign} * var(--shift) * var(--my-direction)))\`;
export const c = { transform: along(1), still: "translateX(0)", up: "translateY(4px)" };
`;
  expect(findIn("a.ts", source).map((f) => [f.line, f.physical, f.logical])).toEqual([
    [1, "bg-left", "ltr:bg-left rtl:bg-right"],
    [1, "hover:translate-x-press", "hover:ltr:translate-x-press hover:rtl:-translate-x-press"],
    [2, "translateX(4px)", "translateX(calc(4px * var(--my-direction)))"],
  ]);
  const css = `.a { @apply origin-left; transform: translateX(-50%); }
.b { transform: translateX(calc(1rem * var(--my-direction))) translateY(2px); }
`;
  expect(findIn("a.css", css).map((f) => [f.line, f.physical, f.logical])).toEqual([
    [1, "origin-left", "ltr:origin-left rtl:origin-right"],
    [1, "translateX(-50%)", "translateX(calc(-50% * var(--my-direction)))"],
  ]);
});
