// SPDX-License-Identifier: AGPL-3.0-only
import type { ComponentPropsWithRef } from "react";
import { AboutIcon } from "./Icons";

export type CalloutProps = Omit<ComponentPropsWithRef<"div">, "className" | "style">;

/**
 * A tinted box with an info icon in front, for a note and the one action it offers ("Your device is in another
 * time zone": "Use the device's"). The note is text; the action is a button after it.
 */
export function Callout({ children, ...props }: CalloutProps) {
  return (
    <div
      data-helper="callout"
      {...props}
      className="flex items-start gap-3 rounded-lg bg-surface-sunken px-4 py-3 text-fg"
    >
      <span className="mt-px shrink-0 text-fg-muted [&>svg]:size-5">
        <AboutIcon />
      </span>
      <div className="flex min-w-0 flex-col items-start gap-2">{children}</div>
    </div>
  );
}
