// SPDX-License-Identifier: AGPL-3.0-only

// A link that reads as the text around it: no underline at rest, the ring on keyboard focus, and the color of its text,
// the page's or muted; hovered and pressed, it takes the underline of words in a line (`wordPress`). For a figure or a name in a line of facts that leads to its list; a link inside a sentence is a
// `TextLink`.

import type { ReactNode } from "react";
import { Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { usePreviewedLink, wordPress } from "./button";
import { Box } from "./layout";

const quiet = tv({
  base: "rounded-control no-underline underline-offset-[0.15em] outline-hidden data-focus-visible:shadow-focus",
});

/** A link to `href` with the color of `tone` and no underline. */
export function QuietLink({
  href,
  tone = "text",
  children,
}: {
  href: string;
  tone?: "text" | "muted";
  children: ReactNode;
}) {
  const previewed = usePreviewedLink();
  return (
    <Box as={<Link href={href} className={`${quiet()} ${wordPress}`} render={previewed} />} tone={tone}>
      {children}
    </Box>
  );
}
