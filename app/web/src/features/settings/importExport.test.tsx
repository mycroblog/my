// SPDX-License-Identifier: AGPL-3.0-only
// The import and export page through the whole app (web-client/README.md T4): downloading the CSV files, checking a file and
// importing it, and the progress of the imports, with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { dataImportFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

afterEach(() => {
  vi.restoreAllMocks();
});

/** The hidden file input of the import form's FileTrigger, which user-event uploads through as the file picker would. */
function fileInput(): HTMLInputElement {
  const region = screen.getByRole("region", { name: "Import" });
  // eslint-disable-next-line testing-library/no-node-access -- FileTrigger's input is hidden and has no role.
  const input = region.querySelector<HTMLInputElement>('input[type="file"]');
  if (input === null) throw new Error("no file input");
  return input;
}

describe("exporting", () => {
  test("opens from the settings and downloads a CSV file with Mastodon's name", async () => {
    Object.defineProperty(URL, "createObjectURL", { configurable: true, value: () => "blob:csv" });
    Object.defineProperty(URL, "revokeObjectURL", { configurable: true, value: () => undefined });
    const saved: { href: string; download: string }[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
      saved.push({ href: this.href, download: this.download });
    });
    const kinds: string[] = [];
    server.use(
      http.get(`${apiBase}/csv_exports/:kind`, ({ params }) => {
        kinds.push(String(params["kind"]));
        return new HttpResponse("bob@remote.example\n", { headers: { "Content-Type": "text/csv" } });
      }),
    );
    const { user } = await renderApp({ path: "/settings" });

    await user.click(await screen.findByRole("link", { name: "Import and export" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Import and export" })).toBeInTheDocument();
    const section = screen.getByRole("region", { name: "Export" });
    await user.click(within(section).getByRole("button", { name: "Download Blocked accounts as CSV" }));

    await waitFor(() => {
      expect(saved).toEqual([{ href: "blob:csv", download: "blocked_accounts.csv" }]);
    });
    expect(kinds).toEqual(["blocks"]);
  });
});

describe("importing", () => {
  test("checks a file, shows what it would do, starts the import, and shows its progress", async () => {
    const previews: Schemas["DataImportRequest"][] = [];
    const starts: Schemas["DataImportRequest"][] = [];
    let items: Schemas["DataImport"][] = [];
    server.use(
      http.get(`${apiBase}/imports`, () => HttpResponse.json({ items })),
      http.post(`${apiBase}/imports/preview`, async ({ request }) => {
        previews.push((await request.json()) as Schemas["DataImportRequest"]);
        return HttpResponse.json({
          entries: 2,
          present: 1,
          removals: 3,
          skipped: [{ line: 3, value: "not an address", reason: "invalid" }],
        } satisfies Schemas["DataImportPreview"]);
      }),
      http.post(`${apiBase}/imports`, async ({ request }) => {
        starts.push((await request.json()) as Schemas["DataImportRequest"]);
        const started = dataImportFixture({ kind: "mutes", mode: "overwrite", total: 5 });
        items = [started];
        return HttpResponse.json(started, { status: 202 });
      }),
    );
    const { user } = await renderApp({ path: "/settings/import-export" });
    const section = await screen.findByRole("region", { name: "Import" });
    expect(within(section).getByRole("button", { name: "Check the file" })).toBeDisabled();
    expect(screen.getByRole("heading", { level: 3, name: "You have no imports" })).toBeInTheDocument();

    await user.click(within(section).getByRole("radio", { name: "Muted accounts" }));
    await user.click(within(section).getByRole("radio", { name: /Overwrite/ }));
    const csv = "Account address,Hide notifications\nnoisy@remote.example,true\nnot an address\n";
    await user.upload(fileInput(), new File([csv], "muted_accounts.csv", { type: "text/csv" }));
    expect(await within(section).findByText("Chosen file: muted_accounts.csv")).toBeInTheDocument();
    await user.click(within(section).getByRole("button", { name: "Check the file" }));

    const preview = await within(section).findByRole("region", { name: "What the import does" });
    // The entries, those already on the list, and what the overwrite removes.
    expect(
      within(preview)
        .getAllByRole("term")
        .map((term) => term.textContent),
    ).toEqual(["Entries in the file", "Already on your list", "To be removed"]);
    expect(
      within(preview)
        .getAllByRole("definition")
        .map((value) => value.textContent),
    ).toEqual(["2", "1", "3"]);
    expect(within(preview).getByText("Line 3: not an address")).toBeInTheDocument();
    expect(within(preview).getByText("Not an account address or a post URL")).toBeInTheDocument();
    expect(previews).toEqual([{ kind: "mutes", mode: "overwrite", text: csv }]);

    await user.click(within(preview).getByRole("button", { name: "Start the import" }));

    await waitFor(() => {
      expect(within(section).getByRole("status")).toHaveTextContent(
        "The import has started with 5 entries. Its progress appears below.",
      );
    });
    expect(starts).toEqual([{ kind: "mutes", mode: "overwrite", text: csv }]);
    // The form is ready for another file.
    expect(within(section).getByText("No file chosen")).toBeInTheDocument();
    expect(within(section).queryByRole("region", { name: "What the import does" })).not.toBeInTheDocument();
    const item = screen.getByRole("article", { name: "Muted accounts, Oct 21, 2026, 9:30 AM" });
    expect(within(item).getByText("In progress")).toBeInTheDocument();
    expect(within(item).getByText("0 of 5")).toBeInTheDocument();
  });

  test("changing the kind asks for another check, and a refused import says why", async () => {
    server.use(
      http.post(`${apiBase}/imports`, () =>
        problem(409, "/api/v1/problems/import-in-progress", "Another import is in progress. Wait until it finishes."),
      ),
    );
    const { user } = await renderApp({ path: "/settings/import-export" });
    const section = await screen.findByRole("region", { name: "Import" });

    await user.upload(fileInput(), new File(["bob@remote.example\n"], "following_accounts.csv", { type: "text/csv" }));
    await within(section).findByText("Chosen file: following_accounts.csv");
    await user.click(within(section).getByRole("button", { name: "Check the file" }));
    await within(section).findByRole("region", { name: "What the import does" });

    await user.click(within(section).getByRole("radio", { name: "Bookmarks" }));
    expect(within(section).queryByRole("region", { name: "What the import does" })).not.toBeInTheDocument();

    await user.click(within(section).getByRole("button", { name: "Check the file" }));
    const preview = await within(section).findByRole("region", { name: "What the import does" });
    await user.click(within(preview).getByRole("button", { name: "Start the import" }));

    expect(await within(section).findByRole("alert")).toHaveTextContent(
      "Another import is in progress. Wait until it finishes.",
    );
  });

  test("lists the finished imports with what could not be imported", async () => {
    server.use(
      http.get(`${apiBase}/imports`, () =>
        HttpResponse.json({
          items: [dataImportFixture({ state: "finished", total: 1200, processed: 1200, failed: 3 })],
        }),
      ),
    );
    await renderApp({ path: "/settings/import-export" });

    const item = await screen.findByRole("article", { name: "Follows, Oct 21, 2026, 9:30 AM" });
    expect(
      within(item)
        .getAllByRole("definition")
        .map((value) => value.textContent),
    ).toEqual(["Finished", "Merge", "1,200 of 1,200", "3"]);
    expect(within(item).getAllByRole("term").at(-1)).toHaveTextContent("Could not be imported");
  });
});
