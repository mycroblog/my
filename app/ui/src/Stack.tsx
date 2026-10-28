// SPDX-License-Identifier: AGPL-3.0-only
// The vertical layout of a screen: blocks, fields and lines one above another, with a gap from the spacing steps
// (web-client/16-look-states-and-wording.md G1) or rules between them (a list, G3). It renders the element the content needs (`as`), so a
// screen keeps its sections, lists and forms without a wrapper and without classes of its own.
import { createElement } from "react";
import { tv } from "tailwind-variants";
import {
  flex,
  type FlexLayoutProps,
  gaps,
  type LayoutElement,
  type PolymorphicProps,
  type Space,
  splitBox,
} from "./layout";

const stack = tv({
  extend: flex,
  base: "flex-col",
  variants: {
    gap: gaps,
    divided: { true: "divide-y divide-border" },
    marker: {
      disc: "list-disc ps-6",
      decimal: "list-decimal ps-6",
      discInside: "list-inside list-disc",
      decimalInside: "list-inside list-decimal",
    },
    rowAt: { sm: "@sm:flex-row @sm:items-center", md: "@md:flex-row @md:items-center" },
  },
});

type StackOwnProps = FlexLayoutProps & {
  /** The gap between the children, in a spacing step; none by default. Use the step of the role (README "Spacing"). */
  gap?: Space;
  /** Rules between the children (`divide-y divide-border`): the rows of a list, which then take no gap (web-client/16-look-states-and-wording.md G3). */
  divided?: boolean;
  /** List markers for a `ul` / `ol` of running text: `disc` and `decimal` hang in `ps-6`, `discInside` and `decimalInside` sit inside the text. */
  marker?: "disc" | "decimal" | "discInside" | "decimalInside";
  /** Turns into a row with centered items from this container width up (an ancestor needs `container`). */
  rowAt?: "sm" | "md";
};

export type StackProps<E extends LayoutElement = "div"> = PolymorphicProps<E, StackOwnProps>;

/** Children one above another (`flex flex-col`). */
export function Stack<E extends LayoutElement = "div">(props: StackProps<E>) {
  const [boxProps, { as, align, justify, gap, divided, marker, rowAt, ...rest }] = splitBox(props);
  return createElement(as ?? "div", {
    ...rest,
    className: stack({ ...boxProps, align, justify, gap, divided, marker, rowAt }),
  });
}
