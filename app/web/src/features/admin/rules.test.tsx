// SPDX-License-Identifier: AGPL-3.0-only
// The rules screen through the whole app (web-client/README.md T4): an administrator with manage_rules lists the rules
// in effect and the retired ones, moves rules up and down with the keyboard-reachable buttons of the list (focus follows
// the moved rule), adds a rule on its page (one text and an optional hint, written in any language), and edits and
// retires one on the rule's page; a viewer without the permission sees why; with MSW playing the API.
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

/** Plays the rules of the server: GET lists them in order, and each write changes them; records the writes. */
function rulesServer(initial: readonly Rule[], initialRetired: readonly Rule[] = []) {
  let rules = [...initial];
  let retired = [...initialRetired];
  const writes: { method: string; path: string; body: unknown }[] = [];
  const record = async (request: Request) => {
    const body: unknown =
      request.method === "POST" && request.url.endsWith("/retire") ? undefined : await request.clone().json();
    writes.push({ method: request.method, path: new URL(request.url).pathname.replace("/api/v1", ""), body });
  };
  server.use(
    http.get(`${apiBase}/admin/rules`, ({ request }) =>
      HttpResponse.json({
        items: new URL(request.url).searchParams.get("include_retired") === "true" ? [...rules, ...retired] : rules,
      }),
    ),
    http.get(`${apiBase}/admin/rules/:id`, ({ params }) => {
      const rule = [...rules, ...retired].find((other) => other.id === params["id"]);
      return rule === undefined ? new HttpResponse(null, { status: 404 }) : HttpResponse.json(rule);
    }),
    http.post(`${apiBase}/admin/rules`, async ({ request }) => {
      await record(request);
      const input = (await request.json()) as Schemas["InstanceRuleInput"];
      const rule = ruleFixture("0199f0e0-0000-7000-8000-0000000000f9", rules.length + 1, input);
      rules = [...rules, rule];
      return HttpResponse.json(rule);
    }),
    http.put(`${apiBase}/admin/rules/:id`, async ({ request, params }) => {
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
      const rule = rules.find((other) => other.id === params["id"]);
      if (rule === undefined) return new HttpResponse(null, { status: 404 });
      const done = { ...rule, retired_at: "2026-10-17T00:00:00Z" };
      rules = rules.filter((other) => other.id !== rule.id);
      retired = [...retired, done];
      return HttpResponse.json(done);
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

  test("lists the rules in effect in order, each a link to its page", async () => {
    rulesServer([first, second]);
    await renderApp({ path: "/admin/rules" });

    expect(await screen.findByRole("heading", { level: 1, name: "Rules" })).toBeInTheDocument();
    const table = screen.getByRole("table", { name: "Rules in effect" });
    const names = within(table).getAllByRole("rowheader");
    expect(names.map((name) => name.textContent)).toEqual(["Be kind", "No spam"]);
    const link = within(names[0] as HTMLElement).getByRole("link", { name: "Be kind" });
    expect(link).toHaveAttribute("href", `/admin/rules/${first.id}`);
    // The administrator's text, in a language the app does not know.
    expect(within(link).getByText("Be kind")).toHaveAttribute("lang", "");
    // The first rule cannot move up nor the last down.
    expect(within(table).getByRole("button", { name: "Move up: Be kind" })).toBeDisabled();
    expect(within(table).getByRole("button", { name: "Move down: Be kind" })).toBeEnabled();
    expect(within(table).getByRole("button", { name: "Move down: No spam" })).toBeDisabled();
    // No form and no dialog on the list: adding is its own page.
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add a rule" })).toHaveAttribute("href", "/admin/rules/new");
  });

  test("lists the retired rules under their filter, in the URL", async () => {
    const retired = ruleFixture("0199f0e0-0000-7000-8000-0000000000a3", 3, {
      text: "No ads",
      retired_at: "2026-10-17T00:00:00Z",
    });
    rulesServer([first], [retired]);
    const { user, router } = await renderApp({ path: "/admin/rules" });

    await user.click(await screen.findByRole("button", { name: /Rules to show/ }));
    await user.click(await screen.findByRole("option", { name: "Retired" }));

    const table = await screen.findByRole("table", { name: "Retired rules" });
    expect(
      within(table)
        .getAllByRole("rowheader")
        .map((name) => name.textContent),
    ).toEqual(["No ads"]);
    expect(router.state.location.search).toEqual({ filter: "retired" });
  });

  test("moves a rule down, and focus follows it", async () => {
    const rules = rulesServer([first, second]);
    const { user } = await renderApp({ path: "/admin/rules" });
    await user.click(await screen.findByRole("button", { name: "Move down: Be kind" }));

    await waitFor(() => {
      const names = within(screen.getByRole("table", { name: "Rules in effect" })).getAllByRole("rowheader");
      expect(names.map((name) => name.textContent)).toEqual(["No spam", "Be kind"]);
      expect(names[1]).toHaveFocus();
    });
    expect(screen.getByText(/^Moved “Be kind” to place 2\./)).toBeInTheDocument();
    expect(rules.writes).toEqual([{ method: "POST", path: `/admin/rules/${first.id}/move`, body: { position: 2 } }]);
  });

  test("adds a rule with a hint on its page, then opens the rule's page", async () => {
    const rules = rulesServer([first]);
    const { user, router } = await renderApp({ path: "/admin/rules/new" });

    expect(await screen.findByRole("heading", { level: 1, name: "Add a rule" })).toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "Rule" }), "作者を明記する");
    await user.type(screen.getByRole("textbox", { name: "Hint" }), "Link the source.");
    await user.click(screen.getByRole("button", { name: "Add the rule" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Rule" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/admin/rules/0199f0e0-0000-7000-8000-0000000000f9");
    expect(
      await screen.findByText("Added the rule “作者を明記する”. The audit log records the change."),
    ).toBeInTheDocument();
    expect(rules.writes).toEqual([
      { method: "POST", path: "/admin/rules", body: { text: "作者を明記する", hint: "Link the source." } },
    ]);
  });

  test("asks for a rule's text before sending it", async () => {
    const rules = rulesServer([]);
    const { user } = await renderApp({ path: "/admin/rules/new" });
    const text = await screen.findByRole("textbox", { name: "Rule" });

    await user.click(screen.getByRole("button", { name: "Add the rule" }));
    expect(text).toHaveAccessibleDescription(/Enter the rule\./);

    await user.type(screen.getByRole("textbox", { name: "Hint" }), "Only a hint");
    await user.click(screen.getByRole("button", { name: "Add the rule" }));
    expect(text).toHaveAccessibleDescription(/Enter the rule\./);
    expect(rules.writes).toEqual([]);
  });

  test("shows that there are no rules yet", async () => {
    rulesServer([]);
    await renderApp({ path: "/admin/rules" });

    expect(await screen.findByText("No rules yet")).toBeInTheDocument();
  });

  test("edits a rule's text on its page, with one save", async () => {
    const rules = rulesServer([first, second]);
    const { user } = await renderApp({ path: `/admin/rules/${second.id}` });
    const facts = await screen.findByRole("region", { name: "About this rule" });
    expect(within(facts).getByText("No hint")).toBeInTheDocument();
    const form = screen.getByRole("region", { name: "Edit the rule" });
    const text = within(form).getByRole("textbox", { name: "Rule" });
    expect(text).toHaveValue("No spam");
    expect(screen.getAllByRole("button", { name: "Save" })).toHaveLength(1);

    await user.clear(text);
    await user.type(text, "No spam or ads");
    await user.click(within(form).getByRole("button", { name: "Save" }));

    expect(
      await within(form).findByText("Saved “No spam or ads”. The audit log records the change."),
    ).toBeInTheDocument();
    expect(rules.writes).toEqual([
      { method: "PUT", path: `/admin/rules/${second.id}`, body: { text: "No spam or ads" } },
    ]);
    expect(await within(facts).findByText("No spam or ads")).toBeInTheDocument();
  });

  test("retires a rule on its page after asking, and focus moves to its facts", async () => {
    const rules = rulesServer([first, second]);
    const { user } = await renderApp({ path: `/admin/rules/${first.id}` });
    const region = await screen.findByRole("region", { name: "Retire the rule" });
    await user.click(within(region).getByRole("button", { name: "Retire" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Retire “Be kind”?" });

    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(rules.writes).toEqual([]);

    await user.click(within(region).getByRole("button", { name: "Retire" }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Retire" }));

    const facts = screen.getByRole("region", { name: "About this rule" });
    expect(await within(facts).findByText("Retired “Be kind”.")).toBeInTheDocument();
    await waitFor(() => {
      expect(within(facts).getByRole("heading", { name: "About this rule" })).toHaveFocus();
    });
    // A retired rule can no longer change.
    expect(screen.queryByRole("region", { name: "Edit the rule" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Retire the rule" })).not.toBeInTheDocument();
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
