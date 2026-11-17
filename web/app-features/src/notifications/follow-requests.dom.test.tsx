// SPDX-License-Identifier: AGPL-3.0-only

// The follow requests to the reader as a feature: a request per account that asked, the newest first, each with
// Decline and Approve; and the answer the reader gave, which stands in the place of the request's buttons and takes
// the focus they leave, while the other requests keep theirs. That the server keeps the answer is the E2E tests'
// (web/e2e/specs/follow-request.spec.ts).

import { expect, test } from "bun:test";
import { screen, waitFor } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { FollowRequests, loadFollowRequests } from "./follow-requests";

const words = {
  en: { approve: "Approve", decline: "Decline", approved: "Approved", declined: "Declined" },
  ja: { approve: "承認", decline: "断る", approved: "承認しました", declined: "断りました" },
};

/** The follow requests to ren, a locked account: sam's, then marco's, then mio's, the newest; drawn in `locale`. */
async function shown(locale: keyof typeof words = "en") {
  const made = fakeData({ viewer: "ren", followRequests: { from: ["marco", "mio"] } });
  await loadFollowRequests(made.data);
  const user = renderFeature(made.data, <FollowRequests onBack={() => undefined} />, locale);
  return { ...made, user, w: words[locale] };
}

/** The links to the accounts that asked and the buttons under the screen's own Back, in the order they stand. */
const requests = () =>
  [...screen.getAllByRole("link"), ...screen.getAllByRole("button").slice(1)]
    .toSorted((a, b) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1))
    .map((element) => element.textContent);

test("a request per account that asked, the newest first, each with Decline before Approve", async () => {
  await shown();

  expect(requests()).toEqual([
    ...["Mio 澪", "Decline", "Approve"],
    ...["Marco", "Decline", "Approve"],
    ...["Sam Rivera", "Decline", "Approve"],
  ]);
});

for (const locale of ["en", "ja"] as const)
  for (const answer of ["approve", "decline"] as const)
    test(`${words.en[answer]} puts the answer in the place of the request's buttons, with the focus on it, and the other requests keep theirs (${locale})`, async () => {
      const { fake, user, w } = await shown(locale);
      const said = answer === "approve" ? w.approved : w.declined;
      const sent = fake.script(
        answer === "approve" ? "POST /follow_requests/{id}/accept" : "POST /follow_requests/{id}/reject",
      );
      const [newest] = screen.getAllByRole("button", { name: w[answer] });
      if (newest === undefined) throw new Error("test: no request waits");

      await user.click(newest);

      expect((await sent.arrived()).url.pathname).toContain(`/follow_requests/${fake.accountId("mio")}/`);
      await waitFor(() => {
        if (document.activeElement?.textContent !== said)
          throw new Error(`the focus is on "${document.activeElement?.textContent ?? ""}", not on the answer`);
      });
      expect(screen.queryAllByRole("button", { name: said })).toHaveLength(0);
      expect(requests()).toEqual([
        "Mio 澪",
        ...["Marco", w.decline, w.approve],
        ...["Sam Rivera", w.decline, w.approve],
      ]);
      // The answer stands where the buttons stood: after the account it answers, before the next request.
      const [mio, marco] = screen.getAllByRole("link");
      const place = (of: Element | undefined) =>
        of === undefined ? 0 : screen.getByText(said).compareDocumentPosition(of);
      expect(place(mio) & Node.DOCUMENT_POSITION_PRECEDING).not.toBe(0);
      expect(place(marco) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
    });
