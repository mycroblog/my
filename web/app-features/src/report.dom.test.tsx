// SPDX-License-Identifier: AGPL-3.0-only

// The report panel once sent: a block of the account, asked first, hides Mute, since a block covers it.

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { ReportDialog } from "./report";
import { fakeData, renderFeature } from "./test/feature";

test("once the report is sent and the account blocked from it, Mute is no longer offered", async () => {
  const { fake, data } = fakeData();
  const account = { id: fake.accountId("sam"), username: "sam", acct: "sam", domain: "demo.test" };
  const user = renderFeature(
    data,
    <ReportDialog subject={{ kind: "account", account }} onClose={() => undefined} layer={false} />,
  );

  await user.click(await screen.findByRole("radio", { name: /^Spam/ }));
  await user.click(screen.getByRole("button", { name: "Send report" }));
  const sent = within(await screen.findByRole("dialog", { name: "Report sent" }));
  expect(sent.getByRole("button", { name: /^Mute @sam/ })).not.toBeNull();

  await user.click(sent.getByRole("button", { name: /^Block @sam.*…$/ }));
  const asking = within(await screen.findByRole("alertdialog", { name: /^Block .*@sam.*\?$/ }));
  await user.click(asking.getByRole("button", { name: "Block" }));

  await waitFor(() => {
    expect(sent.getByText(/^You blocked .*@sam.*\.$/)).not.toBeNull();
  });
  expect(sent.queryByRole("button", { name: /^Mute/ })).toBeNull();
});
