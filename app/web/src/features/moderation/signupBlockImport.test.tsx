// SPDX-License-Identifier: AGPL-3.0-only
// The import of a list of sign-up blocks through the whole app (web-client/README.md T4): a moderator with
// manage_accounts pastes a list or reads it from a text file, sends it with one kind, effect, duration and
// comment, and learns how many blocks were added and which lines were skipped and why, with MSW playing the
// admin API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { signupBlockFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_accounts"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type ImportRequest = Schemas["ImportSignupBlocksRequest"];
type Skipped = Schemas["SignupBlockImportSkipped"];

/**
 * Plays the server of the import: it skips the lines `skip` names, adds the others as blocks in effect, and
 * records the requests and the fetches of the list of blocks.
 */
function importServer(skip: (value: string) => Skipped["reason"] | undefined = () => undefined) {
  const imports: ImportRequest[] = [];
  const fetches: string[] = [];
  let blocks: Schemas["AdminSignupBlock"][] = [];
  server.use(
    http.get(`${apiBase}/admin/signup_blocks`, () => {
      fetches.push("signup_blocks");
      return HttpResponse.json({ items: blocks });
    }),
    http.post(`${apiBase}/admin/signup_blocks/import`, async ({ request }) => {
      const body = (await request.json()) as ImportRequest;
      imports.push(body);
      const skipped: Skipped[] = [];
      let added = 0;
      body.text.split("\n").forEach((raw, index) => {
        const value = raw.trim();
        if (value === "" || value.startsWith("#")) return;
        const reason = skip(value);
        if (reason !== undefined) skipped.push({ line: index + 1, value, reason });
        else {
          added += 1;
          blocks = [
            signupBlockFixture({
              id: `0192f0c4-7a8e-7000-8000-${String(index).padStart(12, "0")}`,
              kind: body.kind,
              value,
              severity: body.severity,
            }),
            ...blocks,
          ];
        }
      });
      return HttpResponse.json({ added, skipped });
    }),
  );
  return { imports, fetches };
}

const list = [
  "# Disposable email domains",
  "spam.example",
  "not a domain",
  "",
  "spam.example",
  "mail.example",
  "old.example",
].join("\n");

function reasonOf(value: string): Skipped["reason"] | undefined {
  if (value === "not a domain") return "invalid";
  if (value === "spam.example") return undefined;
  if (value === "old.example") return "already_blocked";
  return undefined;
}

describe("an import of a list of sign-up blocks", () => {
  test("adds a pasted list with one effect, duration and comment, and shows the lines it skipped and why", async () => {
    // The second spam.example repeats the first one.
    let seen = false;
    const api = importServer((value) => {
      if (value === "spam.example") {
        if (seen) return "duplicate";
        seen = true;
      }
      return reasonOf(value);
    });
    const { user } = await renderApp({ path: "/moderation/signup-blocks" });
    const section = within(await screen.findByRole("region", { name: "Add from a list" }));

    expect(section.getByRole("radio", { name: "Email domain" })).toBeChecked();
    await user.click(section.getByRole("textbox", { name: "List" }));
    await user.paste(list);
    await user.click(section.getByRole("radio", { name: "Refuse" }));
    await user.click(section.getByRole("button", { name: /Duration/ }));
    await user.click(await screen.findByRole("option", { name: "90 days" }));
    await user.type(section.getByRole("textbox", { name: "Comment" }), "Public list of 2026-10.");
    api.fetches.length = 0;
    await user.click(section.getByRole("button", { name: "Add blocks" }));

    expect(
      await section.findByText("2 blocks were added: Email domain, Refuse. 3 lines were skipped."),
    ).toBeInTheDocument();
    expect(api.imports).toEqual([
      {
        kind: "email_domain",
        text: list,
        severity: "reject",
        comment: "Public list of 2026-10.",
        expires_in: 90 * 86_400,
      },
    ]);
    // The list of blocks was fetched again before the result was shown.
    expect(api.fetches).toContain("signup_blocks");

    const table = section.getByRole("table", { name: "Lines not added" });
    const rows = within(table).getAllByRole("row");
    expect(
      rows.map((row) =>
        within(row)
          .getAllByRole(row === rows[0] ? "columnheader" : "cell")
          .map((cell) => cell.textContent),
      ),
    ).toEqual([
      ["Line", "Value", "Reason"],
      ["3", "not a domain", "Not a valid value for the kind"],
      ["5", "spam.example", "Same as an earlier line"],
      ["7", "old.example", "Already blocked"],
    ]);
    // The value is as written in the list, in any script.
    expect(within(table).getByRole("cell", { name: "not a domain" })).toHaveAttribute("dir", "auto");
    // The count of each reason.
    expect(section.getAllByRole("term").map((term) => term.textContent)).toEqual([
      "Not a valid value for the kind",
      "Same as an earlier line",
      "Already blocked",
    ]);
    expect(section.getAllByRole("definition").map((definition) => definition.textContent)).toEqual([
      "1 line",
      "1 line",
      "1 line",
    ]);

    // The blocks are in the list below.
    expect(
      await within(screen.getByRole("table", { name: "In effect" })).findByRole("rowheader", { name: "mail.example" }),
    ).toBeInTheDocument();
    // The form is ready for the next list with the same settings.
    expect(section.getByRole("textbox", { name: "List" })).toHaveValue("");
    expect(section.getByRole("textbox", { name: "Comment" })).toHaveValue("");
    expect(section.getByRole("radio", { name: "Refuse" })).toBeChecked();
  });

  test("reads the list from a text file of IP ranges", async () => {
    const api = importServer();
    const { user } = await renderApp({ path: "/moderation/signup-blocks" });
    const section = within(await screen.findByRole("region", { name: "Add from a list" }));

    await user.click(section.getByRole("radio", { name: "IP range" }));
    const button = section.getByRole("button", { name: "Choose a text file" });
    expect(button).toHaveAccessibleDescription("A text or CSV file. Its text replaces the list above.");
    // FileTrigger's input is hidden; user-event uploads through it as the file picker would.
    // eslint-disable-next-line testing-library/no-node-access -- the hidden file input has no role.
    const input = button.parentElement?.querySelector<HTMLInputElement>('input[type="file"]');
    if (input === null || input === undefined) throw new Error("no file input");
    await user.upload(input, new File(["192.0.2.0/24\n198.51.100.7,abuse\n"], "ranges.csv", { type: "text/csv" }));

    expect(await section.findByText("Read ranges.csv: 2 values.")).toBeInTheDocument();
    expect(section.getByRole("textbox", { name: "List" })).toHaveValue("192.0.2.0/24\n198.51.100.7,abuse\n");
    await user.click(section.getByRole("radio", { name: "Require approval" }));
    await user.click(section.getByRole("button", { name: "Add blocks" }));

    expect(
      await section.findByText("2 blocks were added: IP range, Require approval. No line was skipped."),
    ).toBeInTheDocument();
    expect(api.imports).toEqual([
      { kind: "ip_range", text: "192.0.2.0/24\n198.51.100.7,abuse\n", severity: "require_approval" },
    ]);
    expect(section.queryByRole("table")).not.toBeInTheDocument();
  });

  test("refuses a file too large to send without reading it", async () => {
    const api = importServer();
    const { user } = await renderApp({ path: "/moderation/signup-blocks" });
    const section = within(await screen.findByRole("region", { name: "Add from a list" }));

    const button = section.getByRole("button", { name: "Choose a text file" });
    // eslint-disable-next-line testing-library/no-node-access -- the hidden file input has no role.
    const input = button.parentElement?.querySelector<HTMLInputElement>('input[type="file"]');
    if (input === null || input === undefined) throw new Error("no file input");
    await user.upload(input, new File(["a".repeat(1_000_001)], "huge.txt", { type: "text/plain" }));

    expect(await section.findByRole("alert")).toHaveTextContent(
      "huge.txt is larger than 1 MB. Split the list into smaller files.",
    );
    expect(section.getByRole("textbox", { name: "List" })).toHaveValue("");
    expect(api.imports).toEqual([]);
  });

  test("needs a list with a value, and an effect, before anything is sent", async () => {
    const api = importServer();
    const { user } = await renderApp({ path: "/moderation/signup-blocks" });
    const section = within(await screen.findByRole("region", { name: "Add from a list" }));

    await user.click(section.getByRole("textbox", { name: "List" }));
    await user.paste("# only a comment\n\n");
    await user.click(section.getByRole("button", { name: "Add blocks" }));

    expect(await section.findByText("Enter at least one value, or choose a file")).toBeInTheDocument();
    expect(section.getByRole("textbox", { name: "List" })).toBeInvalid();
    expect(section.getByText("Choose an effect")).toBeInTheDocument();
    expect(api.imports).toEqual([]);
  });

  test("a list of more values than one import holds is not sent", async () => {
    const api = importServer();
    const { user } = await renderApp({ path: "/moderation/signup-blocks" });
    const section = within(await screen.findByRole("region", { name: "Add from a list" }));

    await user.click(section.getByRole("textbox", { name: "List" }));
    await user.paste(Array.from({ length: 10_001 }, (_, i) => `d${String(i)}.example`).join("\n"));
    await user.click(section.getByRole("radio", { name: "Refuse" }));
    await user.click(section.getByRole("button", { name: "Add blocks" }));

    expect(
      await section.findByText("The list has 10,001 values. Enter 10,000 or fewer, and add the rest in another import"),
    ).toBeInTheDocument();
    expect(api.imports).toEqual([]);
  });

  test("shows the first skipped lines of a long list, and the rest on request with focus on the table's heading", async () => {
    importServer(() => "already_blocked");
    const { user } = await renderApp({ path: "/moderation/signup-blocks" });
    const section = within(await screen.findByRole("region", { name: "Add from a list" }));

    await user.click(section.getByRole("textbox", { name: "List" }));
    await user.paste(Array.from({ length: 150 }, (_, i) => `d${String(i)}.example`).join("\n"));
    await user.click(section.getByRole("radio", { name: "Refuse" }));
    await user.click(section.getByRole("button", { name: "Add blocks" }));

    expect(
      await section.findByText("No block was added: Email domain, Refuse. 150 lines were skipped."),
    ).toBeInTheDocument();
    const table = section.getByRole("table", { name: "Lines not added" });
    // A header row and the first 100 lines.
    expect(within(table).getAllByRole("row")).toHaveLength(101);
    expect(section.getByText("The first 100 of 150 lines are shown.")).toBeInTheDocument();
    await user.click(section.getByRole("button", { name: "Show all 150 lines" }));

    expect(within(table).getAllByRole("row")).toHaveLength(151);
    expect(section.queryByRole("button", { name: /^Show all/ })).not.toBeInTheDocument();
    await waitFor(() => {
      expect(section.getByRole("heading", { level: 3, name: "Lines not added" })).toHaveFocus();
    });
  });

  test("a list the server refuses says why under the form, and keeps what was entered", async () => {
    importServer();
    server.use(
      http.post(`${apiBase}/admin/signup_blocks/import`, () => problem(503, "about:blank", "Service Unavailable")),
    );
    const { user } = await renderApp({ path: "/moderation/signup-blocks" });
    const section = within(await screen.findByRole("region", { name: "Add from a list" }));

    await user.click(section.getByRole("textbox", { name: "List" }));
    await user.paste("spam.example");
    await user.click(section.getByRole("radio", { name: "Refuse" }));
    await user.click(section.getByRole("button", { name: "Add blocks" }));

    expect(await section.findByText("Could not add the blocks: Service Unavailable")).toHaveAttribute("role", "alert");
    expect(section.getByRole("textbox", { name: "List" })).toHaveValue("spam.example");
    expect(section.getByRole("radio", { name: "Refuse" })).toBeChecked();
  });
});
