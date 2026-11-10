// SPDX-License-Identifier: AGPL-3.0-only
// The keyword filters page through the whole app (web-client/README.md T4, web-client/screens/settings-filters.md): MSW
// plays the server's filters of the viewer.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { sectionHeadings } from "../../test/sections";
import { keywordFilterFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler);
});

/** Plays a server that keeps the filters oldest first, and records what was sent. */
function useFilters(initial: Schemas["KeywordFilter"][]) {
  let filters = [...initial];
  const sent: { method: string; body?: Schemas["KeywordFilterRequest"] }[] = [];
  let next = 1;
  server.use(
    http.get(`${apiBase}/filters`, () => HttpResponse.json({ items: filters })),
    http.post(`${apiBase}/filters`, async ({ request }) => {
      const body = (await request.json()) as Schemas["KeywordFilterRequest"];
      sent.push({ method: "POST", body });
      const filter = keywordFilterFixture(`new-${String(next)}`, body);
      next += 1;
      filters = [...filters, filter];
      return HttpResponse.json(filter, { status: 201 });
    }),
    http.patch(`${apiBase}/filters/:id`, async ({ request, params }) => {
      const body = (await request.json()) as Schemas["KeywordFilterRequest"];
      sent.push({ method: "PATCH", body });
      const filter = keywordFilterFixture(String(params["id"]), body);
      filters = filters.map((item) => (item.id === filter.id ? filter : item));
      return HttpResponse.json(filter);
    }),
    http.delete(`${apiBase}/filters/:id`, ({ params }) => {
      sent.push({ method: "DELETE" });
      filters = filters.filter((item) => item.id !== params["id"]);
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return sent;
}

const drama = keywordFilterFixture("f1", {
  title: "Drama",
  action: "warn",
  contexts: ["home", "public"],
  keywords: [
    { keyword: "spoiler", whole_word: true },
    { keyword: "ネタバレ", whole_word: false },
  ],
});

describe("keyword filters", () => {
  test("opens from the settings and adds a filter that applies everywhere and folds", async () => {
    const sent = useFilters([]);
    const { user } = await renderApp({ path: "/settings" });

    await user.click(await screen.findByRole("link", { name: "Keyword filters" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Keyword filters" })).toBeInTheDocument();
    // docs/architecture/web-client/screens/settings-filters.md §3
    expect(sectionHeadings()).toEqual(["Your filters"]);
    const list = screen.getByRole("region", { name: "Your filters" });
    expect(within(list).getByRole("heading", { level: 3, name: "You have no keyword filters" })).toBeInTheDocument();

    await user.click(within(list).getByRole("button", { name: "Add a filter" }));
    const form = within(list).getByRole("region", { name: "Add a filter" });
    const name = within(form).getByRole("textbox", { name: /Name/ });
    expect(name).toHaveFocus();
    await user.type(name, "Drama");
    await user.type(within(form).getByRole("textbox", { name: "Word 1" }), "spoiler");
    await user.click(within(form).getByRole("checkbox", { name: "Whole word" }));
    // The one line about whole words in languages without spaces, and the manual of the server's version (not a release).
    expect(
      within(form).getByText("In Japanese and Chinese, a whole word still matches inside other words."),
    ).toBeInTheDocument();
    expect(within(form).getByRole("link", { name: /How words match/ })).toHaveAttribute(
      "href",
      "https://mycroblog.dev/next/guide/keyword-filters/#how-words-match",
    );
    await user.click(within(form).getByRole("button", { name: "Add" }));

    expect(await within(list).findByRole("status")).toHaveTextContent("You added the filter ⁨Drama⁩.");
    expect(within(list).getByRole("button", { name: "Add a filter" })).toHaveFocus();
    expect(sent).toEqual([
      {
        method: "POST",
        body: {
          title: "Drama",
          action: "warn",
          contexts: ["home", "notifications", "public", "thread", "account"],
          keywords: [{ keyword: "spoiler", whole_word: true }],
        },
      },
    ]);
    const rows = await within(list).findAllByRole("listitem");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("Drama");
    expect(rows[0]).toHaveTextContent("Fold");
  });

  test("refuses a filter with nowhere to apply before it is sent", async () => {
    const sent = useFilters([]);
    const { user } = await renderApp({ path: "/settings/filters" });

    await user.click(await screen.findByRole("button", { name: "Add a filter" }));
    const form = screen.getByRole("region", { name: "Add a filter" });
    await user.type(within(form).getByRole("textbox", { name: /Name/ }), "Drama");
    await user.type(within(form).getByRole("textbox", { name: "Word 1" }), "spoiler");
    for (const place of [
      "Home and lists",
      "Notifications",
      "Local, federated, and hashtag timelines",
      "Threads",
      "Profiles",
    ]) {
      await user.click(within(form).getByRole("checkbox", { name: place }));
    }
    await user.click(within(form).getByRole("button", { name: "Add" }));

    expect(await within(form).findByText("Choose at least one place.")).toBeInTheDocument();
    expect(sent).toEqual([]);
  });

  test("adds a word to a filter in its row, keeps its end, and gives focus back to Edit", async () => {
    const sent = useFilters([{ ...drama, expires_at: "2027-01-01T00:00:00Z" }]);
    const { user } = await renderApp({ path: "/settings/filters" });

    await user.click(await screen.findByRole("button", { name: "Edit: ⁨Drama⁩" }));
    const form = screen.getByRole("region", { name: "Edit the filter ⁨Drama⁩" });
    expect(within(form).getByRole("textbox", { name: /Name/ })).toHaveValue("Drama");
    await user.click(within(form).getByRole("button", { name: "Add a word" }));
    await user.type(within(form).getByRole("textbox", { name: "Word 3" }), "finale");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Edit: ⁨Drama⁩" })).toHaveFocus();
    });
    expect(sent).toEqual([
      {
        method: "PATCH",
        body: {
          title: "Drama",
          action: "warn",
          contexts: ["home", "public"],
          keywords: [
            { keyword: "spoiler", whole_word: true },
            { keyword: "ネタバレ", whole_word: false },
            { keyword: "finale", whole_word: false },
          ],
          expires_at: "2027-01-01T00:00:00Z",
        },
      },
    ]);
    expect(await screen.findByText(/3 words/)).toBeInTheDocument();
  });

  test("deletes a filter after asking, and says so", async () => {
    const sent = useFilters([drama]);
    const { user } = await renderApp({ path: "/settings/filters" });

    await user.click(await screen.findByRole("button", { name: "Delete: ⁨Drama⁩" }));
    const dialog = screen.getByRole("alertdialog", { name: "Delete the filter ⁨Drama⁩?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByRole("heading", { level: 3, name: "You have no keyword filters" })).toBeInTheDocument();
    expect(sent).toEqual([{ method: "DELETE" }]);
  });

  test("an expired filter is marked, and the words beyond all the filters' limit say why they are not saved", async () => {
    useFilters([{ ...drama, expires_at: "2026-01-01T00:00:00Z", expired: true }]);
    server.use(
      http.patch(`${apiBase}/filters/:id`, () =>
        problem(422, "/api/v1/problems/too-many-filter-keywords", "Too many filter keywords"),
      ),
    );
    const { user } = await renderApp({ path: "/settings/filters" });

    const list = await screen.findByRole("region", { name: "Your filters" });
    const row = within(list).getAllByRole("listitem")[0] as HTMLElement;
    expect(within(row).getByText("Expired")).toBeInTheDocument();
    await user.click(within(row).getByRole("button", { name: "Edit: ⁨Drama⁩" }));
    const form = screen.getByRole("region", { name: "Edit the filter ⁨Drama⁩" });
    expect(within(form).getByRole("button", { name: /Ends/ })).toHaveTextContent("Keep: expired");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(
      await within(form).findByText(
        "Your filters have at most 1,000 words in all. Remove some words to save this filter.",
      ),
    ).toBeInTheDocument();
  });
});
