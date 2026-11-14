// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { focusOnLoad } from "./focus";

test("the page moves the focus to the element that asks for it when it opens", () => {
  const focused: string[] = [];
  const asked: string[] = [];
  focusOnLoad({
    querySelector: (selector: string) => {
      asked.push(selector);
      return { focus: () => focused.push("alert") } as unknown as Element;
    },
  } as Pick<ParentNode, "querySelector">);
  expect(asked).toEqual(["[data-focus-on-load]"]);
  expect(focused).toEqual(["alert"]);
});

test("a page without a refusal leaves the focus where it is", () => {
  expect(() => {
    focusOnLoad({ querySelector: () => null } as Pick<ParentNode, "querySelector">);
  }).not.toThrow();
});
