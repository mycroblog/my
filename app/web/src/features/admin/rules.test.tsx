// SPDX-License-Identifier: AGPL-3.0-only
// The rules screen through the whole app (web-client/README.md T4): an administrator with manage_rules adds a rule (one text
// and an optional hint, written in any language), edits it in a dialog, moves rules up and down with the keyboard-reachable buttons (focus follows the
// moved rule), and retires one after an alert dialog; a viewer without the permission sees why; with MSW playing
// the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { ruleFixture } from "./test/handlers";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type Rule = Schemas["AdminRule"];

/** U+2068 FIRST STRONG ISOLATE … U+2069 POP DIRECTIONAL ISOLATE: a rule inside a heading. */
function isolated(text: string): string {
  return `⁨${text}⁩`;
}

/** Plays the rules of the server: GET lists them in order, and each write changes them; records the writes. */
function rulesServer(initial: readonly Rule[]) {
  let rules = [...initial];
  const writes: { method: string; path: string; body: unknown }[] = [];
  const record = async (request: Request) => {
    const body: unknown =
      request.method === "POST" && request.url.endsWith("/retire") ? undefined : await request.clone().json();
    writes.push({ method: request.method, path: new URL(request.url).pathname.replace("/api/v1", ""), body });
  };
  server.use(
    http.get(`${apiBase}/admin/rules`, () => HttpResponse.json({ items: rules })),
    http.post(`${apiBase}/admin/rules`, async ({ request }) => {
      await record(request);
      const input = (await request.json()) as Schemas["InstanceRuleInput"];
      const rule = ruleFixture("0199f0e0-0000-7000-8000-0000000000f9", rules.length + 1, input);
      rules = [...rules, rule];
      return HttpResponse.json(rule);
    }),
    http.patch(`${apiBase}/admin/rules/:id`, async ({ request, params }) => {
      await record(request);
      const { text, hint } = (await request.json()) as Schemas["InstanceRuleInput"];
      rules = rules.map((rule) =>
        rule.id === params["id"]
          ? {
              id: rule.id,
              position: rule.position,
              created_at: rule.created_at,
              revised_at: rule.revised_at,
              text,
              ...(hint === undefined ? {} : { hint }),
            }
          : rule,
      );
      return HttpResponse.json(rules.find((rule) => rule.id === params["id"]));
    }),
    http.post(`${apiBase}/admin/rules/:id/move`, async ({ request, params }) => {
      await record(request);
      const { position } = (await request.json()) as Schemas["MoveInstanceRuleRequest"];
      const moved = rules.find((rule) => rule.id === params["id"]);
      if (moved === undefined) return new HttpResponse(null, { status: 404 });
      const others = rules.filter((rule) => rule.id !== moved.id);
      rules = [...others.slice(0, position - 1), moved, ...others.slice(position - 1)].map((rule, index) => ({
        ...rule,
        position: index + 1,
      }));
      return HttpResponse.json(rules[position - 1]);
    }),
    http.post(`${apiBase}/admin/rules/:id/retire`, async ({ request, params }) => {
      await record(request);
      const retired = rules.find((rule) => rule.id === params["id"]);
      rules = rules.filter((rule) => rule.id !== params["id"]);
      return HttpResponse.json({ ...retired, retired_at: "2026-10-17T00:00:00Z" });
    }),
  );
  return { writes };
}

const first = ruleFixture("0199f0e0-0000-7000-8000-0000000000a1", 1, { text: "Be kind", hint: "No harassment." });
const second = ruleFixture("0199f0e0-0000-7000-8000-0000000000a2", 2, { text: "No spam" });

describe("the rules", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_rules"]));
  });

  test("lists the rules in order with their hints", async () => {
    rulesServer([first, second]);
    await renderApp({ path: "/admin/rules" });

    expect(await screen.findByRole("heading", { level: 1, name: "Rules" })).toBeInTheDocument();
    const list = screen.getByRole("region", { name: "Rules in effect" });
    const headings = within(list).getAllByRole("heading", { level: 3 });
    expect(headings.map((heading) => heading.textContent)).toEqual([
      `Rule 1: ${isolated("Be kind")}`,
      `Rule 2: ${isolated("No spam")}`,
    ]);
    // The administrator's hint, in a language the app does not know.
    const hint = within(list).getByText("No harassment.");
    expect(hint).toHaveAttribute("dir", "auto");
    expect(hint).toHaveAttribute("lang", "");
    // The first rule cannot move up nor the last down.
    expect(within(list).queryByRole("button", { name: "Move up: Be kind" })).not.toBeInTheDocument();
    expect(within(list).queryByRole("button", { name: "Move down: No spam" })).not.toBeInTheDocument();
  });

  test("adds a rule with a hint, one field each", async () => {
    const rules = rulesServer([first]);
    const { user } = await renderApp({ path: "/admin/rules" });
    const form = await screen.findByRole("region", { name: "Add a rule" });

    await user.type(within(form).getByRole("textbox", { name: "Rule" }), "作者を明記する");
    await user.type(within(form).getByRole("textbox", { name: "Hint" }), "Link the source.");
    await user.click(within(form).getByRole("button", { name: "Add the rule" }));

    expect(
      await within(form).findByText("Added the rule after the others. The audit log records the change."),
    ).toBeInTheDocument();
    expect(rules.writes).toEqual([
      { method: "POST", path: "/admin/rules", body: { text: "作者を明記する", hint: "Link the source." } },
    ]);
    expect(
      within(screen.getByRole("region", { name: "Rules in effect" })).getByRole("heading", {
        name: `Rule 2: ${isolated("作者を明記する")}`,
      }),
    ).toBeInTheDocument();
    expect(within(form).getByRole("textbox", { name: "Rule" })).toHaveValue("");
  });

  test("asks for a rule's text before sending it", async () => {
    const rules = rulesServer([]);
    const { user } = await renderApp({ path: "/admin/rules" });
    const form = await screen.findByRole("region", { name: "Add a rule" });
    expect(screen.getByText("No rules yet")).toBeInTheDocument();

    await user.click(within(form).getByRole("button", { name: "Add the rule" }));
    expect(await within(form).findByRole("textbox", { name: "Rule" })).toHaveAccessibleDescription(/Enter the rule\./);

    await user.type(within(form).getByRole("textbox", { name: "Hint" }), "Only a hint");
    await user.click(within(form).getByRole("button", { name: "Add the rule" }));
    expect(within(form).getByRole("textbox", { name: "Rule" })).toHaveAccessibleDescription(/Enter the rule\./);
    expect(rules.writes).toEqual([]);
  });

  test("moves a rule down, and focus follows it", async () => {
    const rules = rulesServer([first, second]);
    const { user } = await renderApp({ path: "/admin/rules" });
    await user.click(await screen.findByRole("button", { name: "Move down: Be kind" }));

    const moved = await screen.findByRole("heading", { level: 3, name: `Rule 2: ${isolated("Be kind")}` });
    await waitFor(() => {
      expect(moved).toHaveFocus();
    });
    expect(screen.getByText(/^Moved “Be kind” to place 2\./)).toBeInTheDocument();
    expect(rules.writes).toEqual([{ method: "POST", path: `/admin/rules/${first.id}/move`, body: { position: 2 } }]);
  });

  test("edits a rule's text in a dialog", async () => {
    const rules = rulesServer([first, second]);
    const { user } = await renderApp({ path: "/admin/rules" });
    await user.click(await screen.findByRole("button", { name: "Edit: No spam" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit the rule" });
    const text = within(dialog).getByRole("textbox", { name: "Rule" });
    expect(text).toHaveValue("No spam");

    await user.clear(text);
    await user.type(text, "No spam or ads");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Saved “No spam or ads”. The audit log records the change.")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(rules.writes).toEqual([
      { method: "PATCH", path: `/admin/rules/${second.id}`, body: { text: "No spam or ads" } },
    ]);
  });

  test("retires a rule after asking", async () => {
    const rules = rulesServer([first, second]);
    const { user } = await renderApp({ path: "/admin/rules" });
    await user.click(await screen.findByRole("button", { name: "Retire: Be kind" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Retire “Be kind”?" });

    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(rules.writes).toEqual([]);

    await user.click(screen.getByRole("button", { name: "Retire: Be kind" }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Retire" }));

    expect(await screen.findByText("Retired “Be kind”.")).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByRole("heading", { name: `Rule 1: ${isolated("Be kind")}` })).not.toBeInTheDocument();
    });
    expect(rules.writes).toEqual([{ method: "POST", path: `/admin/rules/${first.id}/retire`, body: undefined }]);
  });

  test("is in the administration menu", async () => {
    rulesServer([]);
    const { user } = await renderApp({ path: "/admin" });
    await user.click(
      await within(await screen.findByRole("navigation", { name: "Admin and moderation" })).findByRole("link", {
        name: "Rules",
      }),
    );

    expect(await screen.findByRole("heading", { level: 1, name: "Rules" })).toBeInTheDocument();
  });
});

test("a viewer without manage_rules sees why instead of the rules, and nothing is read", async () => {
  server.use(signedInHandler, operatorHandler(["manage_settings"]));
  let reads = 0;
  server.use(
    http.get(`${apiBase}/admin/rules`, () => {
      reads++;
      return HttpResponse.json({ items: [] });
    }),
  );
  await renderApp({ path: "/admin/rules" });

  expect(await screen.findByRole("heading", { level: 1, name: "Rules" })).toBeInTheDocument();
  expect(await screen.findByText(/manage_rules/)).toBeInTheDocument();
  expect(reads).toBe(0);
});
