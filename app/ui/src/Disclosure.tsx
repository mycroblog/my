// SPDX-License-Identifier: AGPL-3.0-only
// More detail on a summary, folded until the reader asks for it (ADR-0110 P1: the summary first, the details after):
// the native `details` and `summary` (WAI-ARIA APG, disclosure pattern), which the browser opens with Enter and Space,
// and which find-in-page opens too. The marker turns quietly, only under motion-safe (web-client/21-experience-principles.md §21.2).
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { SubmenuIcon } from "./Icons";

const disclosure = tv({
  slots: {
    base: "group min-w-0",
    summary: [
      "inline-flex cursor-pointer list-none items-center gap-1 rounded-md py-1 text-sm font-semibold text-accent",
      "hover:text-accent-hover focus-visible:focus-ring [&::-webkit-details-marker]:hidden",
      "[&>svg]:size-4 [&>svg]:shrink-0 group-open:[&>svg]:rotate-90 rtl:group-open:[&>svg]:-rotate-90",
      "motion-safe:[&>svg]:transition-transform motion-safe:[&>svg]:duration-fast motion-safe:[&>svg]:ease-standard",
    ],
    body: "flex flex-col gap-2 pt-2",
  },
});

export type DisclosureProps = {
  /** What opening it shows ("How the server says it"), always visible. */
  summary: ReactNode;
  children: ReactNode;
};

/** A summary line that opens to the details under it. */
export function Disclosure({ summary, children }: DisclosureProps) {
  const slots = disclosure();
  return (
    <details className={slots.base()}>
      <summary className={slots.summary()}>
        <SubmenuIcon />
        {summary}
      </summary>
      <div className={slots.body()}>{children}</div>
    </details>
  );
}
