// SPDX-License-Identifier: AGPL-3.0-only
// Equal columns that appear as the container widens: one column in a narrow container, `columns` from the container
// width `at` up (web-client/README.md D2). The facts of a record are a `DescriptionList`; this is for anything else in columns
// (fields of a form, short lists).
import { createElement } from "react";
import { tv } from "tailwind-variants";
import {
  box,
  type BoxProps,
  columnGaps,
  type LayoutElement,
  type PolymorphicProps,
  rowGaps,
  type Space,
  splitBox,
} from "./layout";

const grid = tv({
  extend: box,
  base: "grid grid-cols-1",
  variants: {
    columns: { 2: "", 3: "" },
    at: { sm: "", md: "", "2xl": "" },
    gap: columnGaps,
    rowGap: rowGaps,
  },
  compoundVariants: [
    { columns: 2, at: "sm", class: "@sm:grid-cols-2" },
    { columns: 3, at: "sm", class: "@sm:grid-cols-3" },
    { columns: 2, at: "md", class: "@md:grid-cols-2" },
    { columns: 3, at: "md", class: "@md:grid-cols-3" },
    { columns: 2, at: "2xl", class: "@2xl:grid-cols-2" },
    { columns: 3, at: "2xl", class: "@2xl:grid-cols-3" },
  ],
  defaultVariants: { columns: 2, at: "sm" },
});

type GridOwnProps = BoxProps & {
  /** The columns in a wide container. */
  columns?: 2 | 3;
  /** The container width from which the columns appear (an ancestor needs `container`). */
  at?: "sm" | "md" | "2xl";
  /** The gap between columns and (unless `rowGap` says otherwise) between rows, in a spacing step. */
  gap?: Space;
  /** The gap between rows, when it differs from `gap`. */
  rowGap?: Space;
};

export type GridProps<E extends LayoutElement = "div"> = PolymorphicProps<E, GridOwnProps>;

/** Children in equal columns (`grid grid-cols-1 @sm:grid-cols-2`). */
export function Grid<E extends LayoutElement = "div">(props: GridProps<E>) {
  const [boxProps, { as, columns, at, gap, rowGap, ...rest }] = splitBox(props);
  return createElement(as ?? "div", {
    ...rest,
    className: grid({ ...boxProps, columns, at, gap, rowGap: rowGap ?? gap }),
  });
}
