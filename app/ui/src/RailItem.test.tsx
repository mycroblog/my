// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, test } from "vitest";
import { NotificationsIcon } from "./Icons";
import { RailItem } from "./RailItem";

/**
 * The text a sighted user reads on the control, as axe's `label-content-name-mismatch` (WCAG 2.5.3) takes it: its text
 * content, aria-hidden digits included, without the generated content of CSS and without the text that is only for
 * assistive technology.
 */
function visibleText(element: HTMLElement): string {
  const copy = element.cloneNode(true) as HTMLElement;
  for (const hidden of copy.querySelectorAll(".sr-only")) hidden.remove();
  return copy.textContent.replace(/\s+/g, " ").trim();
}

describe("RailItem with a count", () => {
  test.each(["tab", "row", "section"] as const)(
    "%s: its accessible name starts with its visible text, which holds no digits of the count",
    (form) => {
      render(
        <RailItem
          href="#notifications"
          form={form}
          icon={<NotificationsIcon />}
          label="Notifications"
          badge={{ count: 3, label: "3 unread notifications", name: "Notifications, 3 unread" }}
        />,
      );

      const link = screen.getByRole("link", { name: "Notifications, 3 unread" });
      expect(visibleText(link)).toBe("Notifications");
      expect("Notifications, 3 unread".startsWith(visibleText(link))).toBe(true);
    },
  );
});
