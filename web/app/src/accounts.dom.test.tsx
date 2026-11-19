// SPDX-License-Identifier: AGPL-3.0-only

// The moderation of accounts (#3260, FR-703, FR-714), on the fake API, as what follows a press: the search's words
// follow its address when the address changes from outside (Back), and words the server refuses for what the search
// finds by are said under the field, with nothing to try again; and an action on posts sends, and counts, only the posts
// the report still lists. What each screen shows at rest is the stories' (accounts.stories.tsx).

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp } from "./test/app";
import { fakeApp } from "./test/server";

/** The searchbox of the accounts, by what it finds by. */
const searchbox = (name: string) => main().findByRole<HTMLInputElement>("searchbox", { name });

test("the search's words follow its address when Back changes it, and the screen does not bring the last words back", async () => {
  const app = await renderApp("/moderation/accounts?q=kai", { viewer: "marco" });
  const box = await searchbox("Search by username");
  expect(box.value).toBe("kai");

  await app.user.type(box, "z");
  await waitFor(() => {
    if (!app.path().includes("q=kaiz")) throw new Error(`test: the address is ${app.path()}`);
  });

  await app.back();

  await waitFor(() => {
    expect(box.value).toBe("kai");
  });
  // Past the time the search waits for the reader to stop writing, the address is still the one Back went to.
  await new Promise((done) => setTimeout(done, 600));
  expect(app.path()).toContain("q=kai");
  expect(app.path()).not.toContain("q=kaiz");
});

test("words the server refuses for what the search finds by are said under the field, with no Try again", async () => {
  await renderApp("/moderation/accounts?by=ip&q=kai", { viewer: "marco" });

  const box = await searchbox("Search by IP address");
  const said = await main().findByText("This is not an IP address. Write one such as 192.0.2.1.");
  expect(box.getAttribute("aria-describedby")?.split(" ")).toContain(said.closest("[id]")?.id ?? "none");
  expect(main().queryByRole("button", { name: "Try again" })).toBeNull();
});

test("an action on posts sends and counts only the posts the report still lists, not one deleted since it was chosen", async () => {
  const on = fakeApp({ viewer: "marco" });
  const reportId = on.fake.reports.id("deals");
  // The report lists a second post of the account, which is deleted once both were chosen.
  const twice = on.fake.script("GET /admin/reports/{id}", {
    change: (report) => {
      const [first] = report.statuses;
      if (first === undefined) return report;
      const second = { status: { ...first.status, id: `${first.status.id}9` } };
      return { ...report, statuses: [first, second], status_ids: [...report.status_ids, second.status.id] };
    },
  });
  const account = on.fake.accountId("quickdeals");
  const app = await renderApp(`/moderation/accounts/${account}?report=${reportId}`, { on });

  const kind = await main().findByRole<HTMLSelectElement>("combobox", { name: "Action" });
  await waitFor(() => {
    if (
      !within(kind)
        .queryAllByRole("option")
        .some((o) => o.textContent === "Posts deleted")
    )
      throw new Error("test: the actions on posts are not offered yet");
  });
  await app.user.selectOptions(kind, "Posts deleted");
  const posts = await main().findAllByRole("checkbox", { name: /^Post \d+:/ });
  expect(posts).toHaveLength(2);
  for (const post of posts) await app.user.click(post);

  twice.end();
  await app.data.queryClient.invalidateQueries({ queryKey: ["get", "/admin/reports/{id}"] });
  await waitFor(() => {
    if (main().queryAllByRole("checkbox", { name: /^Post \d+:/ }).length !== 1)
      throw new Error("test: the deleted post is still listed");
  });

  const taken = app.fake.script("POST /admin/accounts/{id}/actions");
  await app.user.click(main().getByRole("button", { name: "Take this action" }));
  const dialog = await screen.findByRole("alertdialog", { name: "Delete 1 post?" });
  await app.user.click(within(dialog).getByRole("button", { name: "Delete posts" }));

  expect((await taken.arrived()).body.status_ids).toHaveLength(1);
});
