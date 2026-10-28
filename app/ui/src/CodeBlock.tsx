// SPDX-License-Identifier: AGPL-3.0-only
import type { ComponentPropsWithRef } from "react";
import { tv } from "tailwind-variants";

const codeBlock = tv({
  base: "rounded-md bg-surface-sunken p-3 text-sm whitespace-pre-wrap wrap-anywhere",
});

export type CodeBlockProps = Omit<ComponentPropsWithRef<"pre">, "className" | "style">;

/** Preformatted code or data on a sunken surface, wrapping anywhere so it never scrolls sideways. Give it `dir="ltr"` for code. */
export function CodeBlock({ children, ...props }: CodeBlockProps) {
  return (
    <pre {...props} className={codeBlock()}>
      <code>{children}</code>
    </pre>
  );
}
