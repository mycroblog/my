// SPDX-License-Identifier: AGPL-3.0-only
// The sign-up blocks screen through the whole app (web-client/README.md T4): a moderator with manage_accounts lists the
// blocks on sign-ups, adds blocks on an email domain, an email address or an IP range, and lifts them, with MSW
// playing the admin API. The server keeps an email domain in IDNA A-labels; the screen shows it with its U-labels.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { newSignupBlock, signupBlockFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_accounts"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type SignupBlock = Schemas["AdminSignupBlock"];
type NewBlock = Schemas["CreateSignupBlockRequest"];

/** U+2066 LEFT-TO-RIGHT ISOLATE … U+2069 POP DIRECTIONAL ISOLATE: a value inside a sentence. */
function isolated(value: string): string {
  return `⁦${value}⁩`;
}

const hash = "5d41402abc4b2a76b9719d911017c592ae0b4f1c7ad8e2c6f5a1b0e9d8c7b6a5";
const idn = signupBlockFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000001a3",
  value: "xn--r8jz45g.jp",
  severity: "require_approval",
  comment: "Disposable addresses.",
  created_at: "2026-10-12T08:00:00Z",
  expires_at: "2026-11-11T08:00:00Z",
});
const range = signupBlockFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000001a2",
  kind: "ip_range",
  value: "192.0.2.0/24",
  moderation_action_id: "0192f0c4-7a8e-7000-8000-0000000d0000",
});
const address = signupBlockFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000001a0",
  kind: "email_hash",
  value: hash,
  state: "lifted",
  lifted_at: "2026-10-11T00:00:00Z",
});

/** Plays the sign-up blocks of the server: a list that new blocks join and lifts change; records the requests. */
function signupBlocksServer(initial: SignupBlock[]) {
  let blocks = initial;
  const lists: string[] = [];
  const created: NewBlock[] = [];
  const liftedIds: string[] = [];
  server.use(
    http.get(`${apiBase}/admin/signup_blocks`, ({ request }) => {
      const query = new URL(request.url).searchParams;
      lists.push(query.toString());
      return HttpResponse.json({
        items: blocks.filter((block) => query.get("active") !== "true" || block.state === "active"),
      });
    }),
    http.post(`${apiBase}/admin/signup_blocks`, async ({ request }) => {
      const body = (await request.json()) as NewBlock;
      created.push(body);
      // The server records U-labels as A-labels.
      const block = newSignupBlock({ ...body, value: body.value === "例え.jp" ? "xn--r8jz45g.jp" : body.value });
      blocks = [block, ...blocks];
      return HttpResponse.json(block);
    }),
    http.post(`${apiBase}/admin/signup_blocks/:id/lift`, ({ params }) => {
      const id = String(params["id"]);
      liftedIds.push(id);
      const block = blocks.find((item) => item.id === id);
      if (block === undefined) return problem(404, "about:blank", "Not Found");
      const next: SignupBlock = { ...block, state: "lifted", lifted_at: "2026-10-17T00:00:00Z" };
      blocks = blocks.map((item) => (item.id === id ? next : item));
      return HttpResponse.json(next);
    }),
  );
  return { lists, created, liftedIds };
}

/** The rows of the table of blocks, without its header row. */
function rows() {
  return within(screen.getByRole("table")).queryAllByRole("row").slice(1);
}

describe("the list of sign-up blocks", () => {
  test("shows the blocks in effect with both forms of a domain, and all of them under the other filter", async () => {
    const api = signupBlocksServer([idn, range, address]);
    const { user, router } = await renderApp({ path: "/moderation/signup-blocks" });

    expect(await screen.findByRole("heading", { level: 1, name: "Sign-up blocks" })).toBeInTheDocument();
    expect(
      within(await screen.findByRole("table"))
        .getAllByRole("rowheader")
        .map((header) => header.textContent),
    ).toEqual(["例え.jp (xn--r8jz45g.jp)", "192.0.2.0/24"]);
    const [first, second] = rows();
    if (first === undefined || second === undefined) throw new Error("the blocks are not listed");
    const details = within(first);
    expect(details.getByText("Email domain")).toBeInTheDocument();
    expect(details.getByText("Require approval")).toBeInTheDocument();
    expect(details.getByText("In effect")).toBeInTheDocument();
    expect(details.getByText(/^Oct 12, 2026/)).toBeInTheDocument();
    expect(details.getByText(/^Nov 11, 2026/)).toBeInTheDocument();
    expect(details.getByText("Added on its own")).toBeInTheDocument();
    expect(details.getByText("Disposable addresses.")).toHaveAttribute("dir", "auto");
    expect(
      details.getByRole("button", {
        name: `Lift the block on the Email domain ${isolated("例え.jp (xn--r8jz45g.jp)")}, added ${details.getByText(/^Oct 12, 2026/).textContent}`,
      }),
    ).toBeInTheDocument();
    expect(within(second).getByText("IP range")).toBeInTheDocument();
    expect(within(second).getByText("Refuse")).toBeInTheDocument();
    expect(within(second).getByText("With a ban")).toBeInTheDocument();
    expect(within(second).getByText("Until lifted")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Blocks to show/ }));
    await user.click(await screen.findByRole("option", { name: "All" }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ filter: "all" });
    });
    const all = within(screen.getByRole("table"));
    expect(await all.findByRole("rowheader", { name: hash })).toBeInTheDocument();
    expect(all.getByText("Lifted", { selector: "span" })).toBeInTheDocument();
    expect(rows()).toHaveLength(3);
    // A lifted block has no lift button.
    expect(all.getAllByRole("button", { name: /^Lift / })).toHaveLength(2);
    expect(api.lists).toEqual(["limit=40&active=true", "limit=40"]);
  });

  test("says when no block is in effect", async () => {
    signupBlocksServer([]);
    await renderApp({ path: "/moderation/signup-blocks" });

    expect(await screen.findByRole("heading", { level: 3, name: "No sign-up blocks in effect" })).toBeInTheDocument();
  });

  test("lifts a block: it leaves the blocks in effect, focus moves on, and the result stays on the page", async () => {
    const api = signupBlocksServer([idn, range]);
    const { user } = await renderApp({ path: "/moderation/signup-blocks" });

    await user.click(await screen.findByRole("button", { name: /^Lift the block on the Email domain/ }));

    await waitFor(() => {
      expect(screen.queryByRole("rowheader", { name: "例え.jp (xn--r8jz45g.jp)" })).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /^Lift the block on the IP range/ })).toHaveFocus();
    });
    expect(screen.getAllByRole("status").map((status) => status.textContent)).toContain(
      `The block on the Email domain ${isolated("例え.jp (xn--r8jz45g.jp)")} is lifted.`,
    );
    expect(api.liftedIds).toEqual([idn.id]);
  });

  test("a lifted block stays in the list of all of them, and focus moves to its heading", async () => {
    signupBlocksServer([range]);
    const { user } = await renderApp({ path: "/moderation/signup-blocks?filter=all" });

    await user.click(await screen.findByRole("button", { name: /^Lift the block on the IP range/ }));

    await waitFor(() => {
      expect(screen.getByRole("rowheader", { name: "192.0.2.0/24" })).toHaveFocus();
    });
    expect(within(screen.getByRole("table")).getByText("Lifted", { selector: "span" })).toBeInTheDocument();
  });

  test("a block that cannot be lifted stays, and the failure says why", async () => {
    signupBlocksServer([range]);
    server.use(
      http.post(`${apiBase}/admin/signup_blocks/:id/lift`, () => problem(503, "about:blank", "Service Unavailable")),
    );
    const { user } = await renderApp({ path: "/moderation/signup-blocks" });

    await user.click(await screen.findByRole("button", { name: /^Lift the block on the IP range/ }));

    expect(await screen.findByText("Could not lift the block: Service Unavailable")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Lift the block on the IP range/ })).toBeInTheDocument();
  });
});

describe("a new sign-up block", () => {
  test("blocks an email domain in non-Latin letters, recorded in A-labels and shown in both forms", async () => {
    const api = signupBlocksServer([]);
    const { user } = await renderApp({ path: "/moderation/signup-blocks" });
    const form = within(await screen.findByRole("region", { name: "Add a block" }));

    // The domain of the email address is the kind chosen first.
    expect(form.getByRole("radio", { name: "Email domain" })).toBeChecked();
    await user.type(form.getByRole("textbox", { name: "Domain" }), "例え.jp");
    await user.click(form.getByRole("radio", { name: "Refuse" }));
    await user.click(form.getByRole("button", { name: /Duration/ }));
    await user.click(await screen.findByRole("option", { name: "30 days" }));
    await user.type(form.getByRole("textbox", { name: "Comment" }), "Disposable addresses.");
    await user.click(form.getByRole("button", { name: "Add block" }));

    expect(
      await screen.findByText(`Refuse is in effect for the Email domain ${isolated("例え.jp (xn--r8jz45g.jp)")}.`),
    ).toBeInTheDocument();
    expect(api.created).toEqual([
      {
        kind: "email_domain",
        value: "例え.jp",
        severity: "reject",
        comment: "Disposable addresses.",
        expires_in: 30 * 86_400,
      },
    ]);
    expect(
      await within(screen.getByRole("table")).findByRole("rowheader", { name: "例え.jp (xn--r8jz45g.jp)" }),
    ).toBeInTheDocument();
    // The form is ready for the next block of the same kind.
    expect(form.getByRole("radio", { name: "Email domain" })).toBeChecked();
    expect(form.getByRole("textbox", { name: "Domain" })).toHaveValue("");
    expect(form.getByRole("radio", { name: "Refuse" })).not.toBeChecked();
    expect(form.getByRole("textbox", { name: "Comment" })).toHaveValue("");
  });

  test("blocks an IP range until it is lifted, with the field named for the kind", async () => {
    const api = signupBlocksServer([]);
    const { user } = await renderApp({ path: "/moderation/signup-blocks" });
    const form = within(await screen.findByRole("region", { name: "Add a block" }));

    await user.click(form.getByRole("radio", { name: "IP range" }));
    await user.type(form.getByRole("textbox", { name: "IP address or range" }), " 192.0.2.0/24 ");
    await user.click(form.getByRole("radio", { name: "Require approval" }));
    await user.click(form.getByRole("button", { name: "Add block" }));

    expect(
      await screen.findByText(`Require approval is in effect for the IP range ${isolated("192.0.2.0/24")}.`),
    ).toBeInTheDocument();
    expect(api.created).toEqual([{ kind: "ip_range", value: "192.0.2.0/24", severity: "require_approval" }]);
    expect(form.getByRole("radio", { name: "IP range" })).toBeChecked();
  });

  test("needs a value and an effect before anything is sent", async () => {
    const api = signupBlocksServer([]);
    const { user } = await renderApp({ path: "/moderation/signup-blocks" });
    const form = within(await screen.findByRole("region", { name: "Add a block" }));

    await user.click(form.getByRole("radio", { name: "Email address" }));
    await user.click(form.getByRole("button", { name: "Add block" }));

    expect(await screen.findByText("Enter an email address")).toBeInTheDocument();
    expect(form.getByRole("textbox", { name: "Email address" })).toBeInvalid();
    expect(screen.getByText("Choose an effect")).toBeInTheDocument();
    expect(api.created).toEqual([]);
  });

  test("a block the server refuses says why under the form, and keeps what was entered", async () => {
    signupBlocksServer([]);
    server.use(
      http.post(`${apiBase}/admin/signup_blocks`, () =>
        HttpResponse.json(
          {
            type: "about:blank",
            title: "Unprocessable Content",
            status: 422,
            detail: "value: not a valid CIDR prefix",
          },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/moderation/signup-blocks" });
    const form = within(await screen.findByRole("region", { name: "Add a block" }));

    await user.click(form.getByRole("radio", { name: "IP range" }));
    await user.type(form.getByRole("textbox", { name: "IP address or range" }), "192.0.2.0/99");
    await user.click(form.getByRole("radio", { name: "Refuse" }));
    await user.click(form.getByRole("button", { name: "Add block" }));

    expect(await screen.findByText("Could not add the block: value: not a valid CIDR prefix")).toHaveAttribute(
      "role",
      "alert",
    );
    expect(form.getByRole("textbox", { name: "IP address or range" })).toHaveValue("192.0.2.0/99");
    expect(form.getByRole("radio", { name: "Refuse" })).toBeChecked();
  });
});

describe("permissions", () => {
  test("the screen is in the moderation menu of a user with manage_accounts", async () => {
    await renderApp({ path: "/moderation" });

    const link = await within(await screen.findByRole("main")).findByRole("link", { name: "Sign-up blocks" });
    expect(link).toHaveAttribute("href", "/moderation/signup-blocks");
    expect(link).toHaveAccessibleDescription(
      "Email domains, email addresses and IP ranges whose sign-ups are refused or need approval.",
    );
  });

  test("a user without manage_accounts is told why, and nothing is asked of the server", async () => {
    server.use(operatorHandler(["manage_domains"]));
    const api = signupBlocksServer([range]);
    await renderApp({ path: "/moderation/signup-blocks" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
    expect(api.lists).toEqual([]);
  });
});
