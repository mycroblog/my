// SPDX-License-Identifier: AGPL-3.0-only
// The domain block list in Mastodon's CSV through the whole app (web-client/README.md T4): an operator with manage_domains
// saves the actions in effect as a CSV, and imports a CSV that Mastodon exported, previewing it and clearing the
// rows not wanted, with MSW playing the admin API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { domainBlockCsvHeader, importDomainBlocks, previewDomainBlocks } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_domains"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type ImportRequest = Schemas["ImportDomainBlocksRequest"];

/** Plays the import of the server, as the default handlers do, and records the requests and the fetches of the list. */
function csvServer() {
  const previews: string[] = [];
  const imports: ImportRequest[] = [];
  const lists: string[] = [];
  let actions: Schemas["AdminDomainAction"][] = [];
  server.use(
    http.get(`${apiBase}/admin/domain_actions`, () => {
      lists.push("domain_actions");
      return HttpResponse.json({ items: actions });
    }),
    http.post(`${apiBase}/admin/domain_actions/import/preview`, async ({ request }) => {
      const body = (await request.json()) as Schemas["PreviewDomainBlockImportRequest"];
      previews.push(body.csv);
      return HttpResponse.json(previewDomainBlocks(body.csv));
    }),
    http.post(`${apiBase}/admin/domain_actions/import`, async ({ request }) => {
      const body = (await request.json()) as ImportRequest;
      imports.push(body);
      const result = importDomainBlocks(body);
      actions = [...result.issued, ...actions];
      return HttpResponse.json(result);
    }),
  );
  return { previews, imports, lists };
}

const csv = [
  domainBlockCsvHeader,
  "noisy.example,silence,true,false,Harassment,false",
  "calm.example,noop,false,false,,false",
  "media.example,noop,true,true,,true",
  "odd.example,ban,false,false,,false",
].join("\n");

async function openSection() {
  const view = await renderApp({ path: "/moderation/domain-actions" });
  const section = within(await screen.findByRole("region", { name: "Mastodon's CSV" }));
  return { ...view, section };
}

describe("an import of a domain block CSV", () => {
  test("previews the rows, imports the chosen ones with a note, and says what it did", async () => {
    const api = csvServer();
    const { user, section } = await openSection();

    await user.click(section.getByRole("textbox", { name: "CSV" }));
    await user.paste(csv);
    await user.click(section.getByRole("button", { name: "Preview" }));

    expect(
      await section.findByText("2 of 4 rows can be imported. Clear the rows you do not want."),
    ).toBeInTheDocument();
    expect(api.previews).toEqual([csv]);
    const rows = section.getByRole("list", { name: "Rows of the CSV" });
    const noisy = within(rows).getByRole("checkbox", { name: "noisy.example" });
    expect(noisy).toBeChecked();
    expect(noisy).toHaveAccessibleDescription("Line 2. Takes: Limit and Reject media");
    expect(within(rows).getByRole("checkbox", { name: "media.example" })).toHaveAccessibleDescription(
      "Line 4. Takes: Reject media and Reject reports",
    );
    expect(within(rows).getByText("Line 3. No action")).toBeInTheDocument();
    expect(within(rows).getByText("Line 5. Not a valid row")).toBeInTheDocument();
    // The domain of a row that takes no action is as written in the CSV.
    expect(within(rows).getByText("odd.example")).toHaveAttribute("dir", "auto");

    await user.click(within(rows).getByRole("checkbox", { name: "media.example" }));
    await user.type(section.getByRole("textbox", { name: "Private note" }), "From mastodon.example.");
    api.lists.length = 0;
    await user.click(section.getByRole("button", { name: "Import 1 row" }));

    expect(await section.findByText("2 actions were taken. No row was skipped.")).toBeInTheDocument();
    expect(api.imports).toEqual([
      {
        blocks: [
          {
            domain: "noisy.example",
            severity: "silence",
            reject_media: true,
            reject_reports: false,
            obfuscate: false,
            public_comment: "Harassment",
          },
        ],
        private_note: "From mastodon.example.",
      },
    ]);
    // The list of actions was fetched again before the result was shown.
    expect(api.lists).toContain("domain_actions");
    // The preview gives way to the result, with focus on the heading of the import.
    expect(section.queryByRole("list", { name: "Rows of the CSV" })).not.toBeInTheDocument();
    await waitFor(() => {
      expect(section.getByRole("heading", { level: 3, name: "Import a CSV" })).toHaveFocus();
    });
    expect(section.getByRole("textbox", { name: "CSV" })).toHaveValue("");
  });

  test("asks before importing rows that suspend a domain", async () => {
    const api = csvServer();
    const { user, section } = await openSection();

    await user.click(section.getByRole("textbox", { name: "CSV" }));
    await user.paste(
      [domainBlockCsvHeader, "spam.example,suspend,false,false,,false", "bad.example,suspend,false,true,,false"].join(
        "\n",
      ),
    );
    await user.click(section.getByRole("button", { name: "Preview" }));
    await user.click(await section.findByRole("button", { name: "Import 2 rows" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Suspend 2 domains?" });
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(api.imports).toEqual([]);

    await user.click(section.getByRole("button", { name: "Import 2 rows" }));
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Import and suspend" }),
    );

    expect(await section.findByText("3 actions were taken. No row was skipped.")).toBeInTheDocument();
    expect(api.imports.map((request) => request.blocks.map((block) => block.domain))).toEqual([
      ["spam.example", "bad.example"],
    ]);
  });

  test("reads the CSV from a file", async () => {
    const api = csvServer();
    const { user, section } = await openSection();

    const button = section.getByRole("button", { name: "Choose a CSV file" });
    expect(button).toHaveAccessibleDescription("Its text replaces the CSV above.");
    // eslint-disable-next-line testing-library/no-node-access -- the hidden file input has no role.
    const input = button.parentElement?.querySelector<HTMLInputElement>('input[type="file"]');
    if (input === null || input === undefined) throw new Error("no file input");
    await user.upload(input, new File(["spam.example\n"], "domain_blocks.csv", { type: "text/csv" }));

    expect(await section.findByText("Read domain_blocks.csv.")).toBeInTheDocument();
    expect(section.getByRole("textbox", { name: "CSV" })).toHaveValue("spam.example\n");
    await user.click(section.getByRole("button", { name: "Preview" }));
    expect(await section.findByRole("checkbox", { name: "spam.example" })).toHaveAccessibleDescription(
      "Line 1. Takes: Suspend",
    );
    expect(api.previews).toEqual(["spam.example\n"]);
  });

  test("needs a CSV, and a chosen row, before anything is sent", async () => {
    const api = csvServer();
    const { user, section } = await openSection();

    await user.click(section.getByRole("button", { name: "Preview" }));
    expect(await section.findByText("Paste a CSV, or choose a file")).toBeInTheDocument();
    expect(api.previews).toEqual([]);

    await user.click(section.getByRole("textbox", { name: "CSV" }));
    await user.paste("calm.example,noop");
    await user.click(section.getByRole("button", { name: "Preview" }));
    await user.click(await section.findByRole("button", { name: "Import 0 rows" }));

    expect(await section.findByRole("alert")).toHaveTextContent("Choose at least one row to import");
    expect(api.imports).toEqual([]);
  });

  test("a CSV the server refuses says why under the form, and keeps it", async () => {
    csvServer();
    server.use(
      http.post(`${apiBase}/admin/domain_actions/import/preview`, () =>
        problem(422, "about:blank", "Unprocessable Entity"),
      ),
    );
    const { user, section } = await openSection();

    await user.click(section.getByRole("textbox", { name: "CSV" }));
    await user.paste('a.example,"b"c');
    await user.click(section.getByRole("button", { name: "Preview" }));

    expect(await section.findByText("Could not read the CSV: Unprocessable Entity")).toHaveAttribute("role", "alert");
    expect(section.getByRole("textbox", { name: "CSV" })).toHaveValue('a.example,"b"c');
  });
});

describe("the export of the domain actions", () => {
  test("saves the CSV as a file and says so", async () => {
    let saved: Blob | undefined;
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      value: (blob: Blob) => {
        saved = blob;
        return "blob:domain-blocks";
      },
    });
    Object.defineProperty(URL, "revokeObjectURL", { configurable: true, value: () => undefined });
    const { user, section } = await openSection();

    await user.click(section.getByRole("button", { name: "Export CSV" }));

    expect(await section.findByText("Saved domain_blocks.csv.")).toBeInTheDocument();
    expect(await saved?.text()).toBe(`${domainBlockCsvHeader}\n`);
  });

  test("a failed export says why", async () => {
    server.use(
      http.get(`${apiBase}/admin/domain_actions/export`, () => problem(503, "about:blank", "Service Unavailable")),
    );
    const { user, section } = await openSection();

    await user.click(section.getByRole("button", { name: "Export CSV" }));

    expect(await section.findByRole("alert")).toHaveTextContent("Could not export the CSV: Service Unavailable");
  });
});
