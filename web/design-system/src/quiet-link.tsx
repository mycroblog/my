// SPDX-License-Identifier: AGPL-3.0-only

// A link that reads as the text around it: no underline, the ring on keyboard focus, and the color of its text, the
// page's or muted. For a figure or a name in a line of facts that leads to its list; a link inside a sentence is a
// `TextLink`.

import type { ReactNode } from "react";
import { Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { usePreviewedLink } from "./button";
import { Box } from "./layout";
import { useWordsLook } from "./press-options";

const quiet = tv({ base: "rounded-control no-underline outline-hidden data-focus-visible:shadow-focus" });

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
  const look = useWordsLook("bare");
  return (
    <Box as={<Link href={href} className={`${quiet()} ${look}`} render={previewed} />} tone={tone}>
      {children}
    </Box>
  );
}
