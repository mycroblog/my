// SPDX-License-Identifier: AGPL-3.0-only
// The result of the last action, where the person acted (web-client/16-look-states-and-wording.md G24,
// 21-experience-principles.md P2): a success is a check and its words in a `role="status"` region, a failure a warning
// and its words in a `role="alert"` region. Both regions are in the page, empty, from the first render, so that a screen
// reader reads the result when it arrives (a live region announces only what changes after it is there). Shared by the
// parts that show a result beside their action (`FormActionBar`, `ListSection`), so that a result looks the same in both.
import { tv } from "tailwind-variants";
import { CheckIcon, WarningIcon } from "./Icons";

/** The result of the last action, already translated. */
export type ActionResultValue = { tone: "success" | "danger"; message: string };

const actionResult = tv({
  slots: {
    message: [
      "flex items-start gap-2 text-sm font-medium wrap-anywhere",
      "motion-safe:transition-[opacity,translate] motion-safe:duration-normal motion-safe:ease-enter",
      "motion-safe:starting:translate-y-1 motion-safe:starting:opacity-0",
    ],
    icon: "mt-0.5 shrink-0 [&>svg]:size-4",
  },
  variants: {
    tone: {
      success: { message: "text-success" },
      danger: { message: "text-danger" },
    },
  },
});

/**
 * The two live regions of a result, and the result in the one of its tone. The regions stay in the accessibility tree
 * while empty (not display: none), or a screen reader would not read what arrives in them.
 */
export function ActionResult({ result }: { result: ActionResultValue | undefined }) {
  const message = (tone: ActionResultValue["tone"]) => {
    if (result?.tone !== tone) return null;
    const slots = actionResult({ tone });
    return (
      // A new result enters again (keyed by its words).
      <p key={result.message} className={slots.message()}>
        <span aria-hidden="true" className={slots.icon()}>
          {tone === "success" ? <CheckIcon /> : <WarningIcon filled />}
        </span>
        <span>{result.message}</span>
      </p>
    );
  };
  return (
    <>
      <div role="status">{message("success")}</div>
      <div role="alert">{message("danger")}</div>
    </>
  );
}
