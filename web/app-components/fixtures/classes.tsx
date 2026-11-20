// SPDX-License-Identifier: AGPL-3.0-only
// Above the design system no element gets a class or a style, and no class is written otherwise (ADR-0025 §4): a
// part of this layer composes the parts of @my/design-system by their props.

import { Box, Stack } from "@my/design-system";
import "tailwind-variants/lite"; // error: classes tailwind-variants/lite
import type { ReactNode } from "react";

const given = { className: "text-caption" }; // error: <div> className

// A part of this layer's own takes no class either: what it would give one is a class written above the system.
const Own = ({ children }: { className?: string; style?: object; children?: ReactNode }) => <Box>{children}</Box>;

export const Classes = () => (
  <Stack gap="entry-line" surface="raised" radius="control" pad="embed">
    <Box as="p" text="caption" tone="muted">
      Composed
    </Box>
    <p className="text-caption">Written</p> {/* error: <p> className */}
    <span style={{ color: "inherit" }}>Styled</span> {/* error: <span> style */}
    <div {...given} />
    <Box className="text-caption" /> {/* error: <Box> className */}
    <Own className="text-caption" /> {/* error: <Own> className */}
    <Own style={{ color: "inherit" }} /> {/* error: <Own> style */}
  </Stack>
);

export function written(element: HTMLElement) {
  element.className = "text-caption"; // error: classes .className =
  element.classList.add("text-caption"); // error: classes classList.add()
  element.setAttribute("class", "text-caption"); // error: classes setAttribute("class")
  element.setAttribute("style", "color: inherit"); // error: classes setAttribute("style")
  element.setAttribute("title", "text-caption");
  return element.className;
}
