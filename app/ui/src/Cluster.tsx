// SPDX-License-Identifier: AGPL-3.0-only
// The horizontal layout of a screen: buttons, tags and short facts side by side, wrapping onto the next line when the
// row is full, with gaps from the spacing steps (web-client/16-look-states-and-wording.md G1). It renders the element the content needs (`as`).
import { createElement } from "react";
import { tv } from "tailwind-variants";
import {
  columnGaps,
  flex,
  type FlexLayoutProps,
  type LayoutElement,
  type PolymorphicProps,
  rowGaps,
  type Space,
  splitBox,
} from "./layout";

const cluster = tv({
  extend: flex,
  variants: {
    wrap: { true: "flex-wrap", false: "" },
    inline: { true: "inline-flex", false: "" },
    gap: columnGaps,
    rowGap: rowGaps,
  },
  defaultVariants: { wrap: true },
});

type ClusterOwnProps = FlexLayoutProps & {
  /** The gap between the children, in a spacing step, across and (unless `rowGap` says otherwise) between wrapped lines. */
  gap?: Space;
  /** The gap between wrapped lines, when it differs from `gap` (`gap="2" rowGap="0"` is `gap-x-2`). */
  rowGap?: Space;
  /** Wraps onto more lines when the row is full (the default). `false` keeps one line, for an avatar beside its text. */
  wrap?: boolean;
  /** Sits in the line of the text around it (`inline-flex`), like a label with a badge. */
  inline?: boolean;
};

export type ClusterProps<E extends LayoutElement = "div"> = PolymorphicProps<E, ClusterOwnProps>;

/** Children side by side in the reading direction (`flex flex-wrap`). */
export function Cluster<E extends LayoutElement = "div">(props: ClusterProps<E>) {
  const [boxProps, { as, align, justify, gap, rowGap, wrap, inline, ...rest }] = splitBox(props);
  return createElement(as ?? "div", {
    ...rest,
    className: cluster({ ...boxProps, align, justify, gap, rowGap: rowGap ?? gap, wrap, inline }),
  });
}
