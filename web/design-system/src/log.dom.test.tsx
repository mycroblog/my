// SPDX-License-Identifier: AGPL-3.0-only

// A log of a record (#3258), as assistive technology gets it: an ordered list named by its label, each entry its
// sentence with its time, the line about it, and Details, a disclosure's button whose state is read with it and whose
// panel stays out of the page while folded. How it stands at a width is its stories' (Logs).

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SentenceName } from "./list";
import { LogEntry, LogList } from "./log";

test("a log is an ordered list of its entries, each who did what to what with its time, and Details shows more", async () => {
  const user = userEvent.setup();
  render(
    <LogList label="Audit log">
      <LogEntry icon="moderation" date={new Date()} about="Reason: Spam." detail={<p>Resolves 41 reports.</p>}>
        <SentenceName>mika</SentenceName> suspended <SentenceName>@spam_bot@social.example</SentenceName>
      </LogEntry>
      <LogEntry icon="settings" date={new Date()}>
        <SentenceName>admin_sato</SentenceName> changed <SentenceName>Registrations</SentenceName>
      </LogEntry>
    </LogList>,
  );
  const log = screen.getByRole("list", { name: "Audit log" });
  expect(log.tagName).toBe("OL");
  const [first, second] = within(log).getAllByRole("listitem");
  expect(first?.textContent).toContain("mika suspended @spam_bot@social.example");
  expect(first?.querySelector("time")).not.toBeNull();
  expect(second && within(second).queryByRole("button")).toBeNull();

  const details = within(first as HTMLElement).getByRole("button", { name: "Details" });
  expect(details.getAttribute("aria-expanded")).toBe("false");
  expect(screen.getByText("Resolves 41 reports.").closest("[hidden]")).not.toBeNull();

  await user.click(details);

  expect(details.getAttribute("aria-expanded")).toBe("true");
  expect(screen.getByText("Resolves 41 reports.").closest("[hidden]")).toBeNull();
});
