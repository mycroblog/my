// SPDX-License-Identifier: AGPL-3.0-only
// The aliases and moving the account through the whole app (web-client/README.md T4, FR-009, ADR-0089): adding and
// removing aliases with their limit and errors, the move behind a dialog that says what moves and what stays and asks
// for the password, the moved state with the cancellation, with MSW playing the API.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { accountFixture, type Schemas, viewerFixture } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type Alias = Schemas["AccountAlias"];

const old: Alias = {
  uri: "https://old.example/users/alice",
  acct: "alice@old.example",
  created_at: "2026-10-01T09:00:00Z",
};
const carol = accountFixture(7, "Carol", "new.example");

/** Plays the aliases the server keeps; returns what the app sent. */
function aliasesServer(initial: Alias[]) {
  let aliases = [...initial];
  const added: string[] = [];
  const removed: string[] = [];
  server.use(
    http.get(`${apiBase}/accounts/aliases`, () => HttpResponse.json({ aliases })),
    http.post(`${apiBase}/accounts/aliases`, async ({ request }) => {
      const { acct } = (await request.json()) as Schemas["AddAccountAliasRequest"];
      added.push(acct);
      if (acct === "nobody@nowhere.example")
        return problem(422, "/api/v1/problems/invalid-alias", "That account cannot be an alias.");
      aliases = [
        ...aliases,
        {
          uri: `https://${acct.split("@")[1] ?? ""}/users/${acct.split("@")[0] ?? ""}`,
          acct,
          created_at: "2026-10-20T09:00:00Z",
        },
      ];
      return HttpResponse.json({ aliases });
    }),
    http.post(`${apiBase}/accounts/aliases/remove`, async ({ request }) => {
      const { uri } = (await request.json()) as Schemas["RemoveAccountAliasRequest"];
      removed.push(uri);
      aliases = aliases.filter((alias) => alias.uri !== uri);
      return HttpResponse.json({ aliases });
    }),
  );
  return { added, removed };
}

/** Plays the viewer, which names the account it moved to once moved; returns what the move calls received. */
function moveServer({ moved = false }: { moved?: boolean } = {}) {
  let movedTo: Schemas["Account"] | undefined = moved ? carol : undefined;
  const moves: Schemas["MoveOwnAccountRequest"][] = [];
  let cancellations = 0;
  server.use(
    http.get(`${apiBase}/accounts/verify_credentials`, () =>
      HttpResponse.json({ ...viewerFixture, ...(movedTo === undefined ? {} : { moved: movedTo }) }),
    ),
    http.get(`${apiBase}/accounts/password`, () => HttpResponse.json({ has_password: true })),
    http.get(`${apiBase}/accounts/lookup`, ({ request }) => {
      const acct = new URL(request.url).searchParams.get("acct");
      return acct === "@carol@new.example" ? HttpResponse.json(carol) : problem(404, "about:blank", "Not Found");
    }),
    http.post(`${apiBase}/accounts/move`, async ({ request }) => {
      const body = (await request.json()) as Schemas["MoveOwnAccountRequest"];
      moves.push(body);
      if (body.password !== "secret") return problem(422, "/api/v1/problems/wrong-password", "The password is wrong.");
      movedTo = carol;
      return new HttpResponse(null, { status: 204 });
    }),
    http.post(`${apiBase}/accounts/move/cancel`, () => {
      cancellations++;
      movedTo = undefined;
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return { moves, cancellations: () => cancellations };
}

describe("the aliases", () => {
  test("opens from the settings, lists the aliases, and adds one", async () => {
    const api = aliasesServer([old]);
    const { user } = await renderApp({ path: "/settings" });

    await user.click(await screen.findByRole("link", { name: "Aliases" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Aliases" })).toBeInTheDocument();
    const list = screen.getByRole("region", { name: "Your aliases" });
    expect(within(list).getByText("@alice@old.example")).toHaveAttribute("dir", "ltr");
    expect(within(list).getByText("https://old.example/users/alice")).toBeInTheDocument();

    const field = screen.getByRole("textbox", { name: /Account to move from/ });
    expect(field).toHaveAccessibleDescription(/up to 5 aliases/);
    await user.type(field, "bob@other.example");
    await user.click(screen.getByRole("button", { name: "Add alias" }));

    expect(await within(list).findByText("@bob@other.example")).toBeInTheDocument();
    expect(within(screen.getByRole("main")).getByRole("status")).toHaveTextContent(
      "Added the alias ⁨@bob@other.example⁩.",
    );
    expect(api.added).toEqual(["bob@other.example"]);
    expect(field).toHaveValue("");
  });

  test("says why an alias cannot be added and keeps what was typed", async () => {
    aliasesServer([]);
    const { user } = await renderApp({ path: "/settings/aliases" });

    expect(await screen.findByText("You have no aliases.")).toBeInTheDocument();
    const field = screen.getByRole("textbox", { name: /Account to move from/ });
    await user.type(field, "nobody@nowhere.example");
    await user.click(screen.getByRole("button", { name: "Add alias" }));

    expect(await within(screen.getByRole("main")).findByRole("alert")).toHaveTextContent(
      "The alias could not be added: That account cannot be an alias.",
    );
    expect(field).toHaveValue("nobody@nowhere.example");
  });

  test("removes an alias from its row, and announces it", async () => {
    const api = aliasesServer([old]);
    const { user } = await renderApp({ path: "/settings/aliases" });

    await user.click(await screen.findByRole("button", { name: "Remove the alias ⁨@alice@old.example⁩" }));

    expect(await screen.findByText("You have no aliases.")).toBeInTheDocument();
    expect(api.removed).toEqual([old.uri]);
  });

  test("replaces the form with a note at the most aliases", async () => {
    aliasesServer(
      ["a", "b", "c", "d", "e"].map((name, index) => ({
        uri: `https://x.example/users/${name}`,
        acct: `${name}@x.example`,
        created_at: `2026-10-0${String(index + 1)}T00:00:00Z`,
      })),
    );
    await renderApp({ path: "/settings/aliases" });

    expect(
      await screen.findByText("You have 5 aliases, the most there can be. Remove one to add another."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add alias" })).not.toBeInTheDocument();
  });
});

describe("moving the account", () => {
  test("looks the new account up, asks in a dialog what moves and what stays, and moves after the password", async () => {
    const api = moveServer();
    const { user } = await renderApp({ path: "/settings" });

    await user.click(await screen.findByRole("link", { name: "Move account" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Move account" })).toBeInTheDocument();
    expect(screen.getByText("https://my.example/@alice")).toHaveAttribute("dir", "ltr");

    await user.type(screen.getByRole("textbox", { name: /New account/ }), "@carol@new.example");
    await user.click(screen.getByRole("button", { name: "Continue" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Move to ⁨@carol@new.example⁩?" });
    expect(dialog).toHaveAccessibleDescription(/Enter your password to confirm\. Your followers follow/);
    expect(within(dialog).getByText("What moves")).toBeInTheDocument();
    expect(within(dialog).getByText("Your followers")).toBeInTheDocument();
    expect(within(dialog).getByText("What stays here")).toBeInTheDocument();
    expect(
      within(dialog).getByText("Your posts, the accounts you follow, your blocks, mutes, and bookmarks"),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByRole("link", { name: "Export your follows, blocks, mutes, and bookmarks as CSV files" }),
    ).toHaveAttribute("href", "/settings/import-export");
    // Focus starts on the password, not on the button that moves.
    const password = within(dialog).getByLabelText(/Password/);
    expect(password).toHaveFocus();

    await user.type(password, "wrong");
    await user.click(within(dialog).getByRole("button", { name: "Move account" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("The password is wrong.");

    await user.clear(password);
    await user.type(password, "secret");
    await user.click(within(dialog).getByRole("button", { name: "Move account" }));

    expect(await screen.findByRole("heading", { level: 2, name: "Your account has moved" })).toBeInTheDocument();
    expect(within(screen.getByRole("main")).getByRole("status")).toHaveTextContent(
      "Your account moved to ⁨@carol@new.example⁩.",
    );
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(api.moves).toEqual([
      { target_account_id: carol.id, password: "wrong" },
      { target_account_id: carol.id, password: "secret" },
    ]);
  });

  test("says that the new account was not found", async () => {
    moveServer();
    const { user } = await renderApp({ path: "/settings/move" });

    await user.type(await screen.findByRole("textbox", { name: /New account/ }), "@nobody@nowhere.example");
    await user.click(screen.getByRole("button", { name: "Continue" }));

    expect(await within(screen.getByRole("main")).findByRole("alert")).toHaveTextContent(
      "That account could not be found:",
    );
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  test("closes the dialog with Cancel and moves nothing", async () => {
    const api = moveServer();
    const { user } = await renderApp({ path: "/settings/move" });

    await user.type(await screen.findByRole("textbox", { name: /New account/ }), "@carol@new.example");
    await user.click(screen.getByRole("button", { name: "Continue" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(api.moves).toEqual([]);
  });

  test("shows where a moved account moved to, and cancels the move after asking", async () => {
    const api = moveServer({ moved: true });
    const { user } = await renderApp({ path: "/settings/move" });

    expect(await screen.findByRole("heading", { level: 2, name: "Your account has moved" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open ⁨@carol@new.example⁩" })).toHaveAttribute(
      "href",
      "/@carol@new.example",
    );
    expect(screen.queryByRole("textbox", { name: /New account/ })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel the move…" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Cancel the move?" });
    await user.click(within(dialog).getByRole("button", { name: "Cancel the move" }));

    expect(await screen.findByRole("textbox", { name: /New account/ })).toBeInTheDocument();
    expect(within(screen.getByRole("main")).getByRole("status")).toHaveTextContent("The move is cancelled.");
    expect(api.cancellations()).toBe(1);
  });
});
