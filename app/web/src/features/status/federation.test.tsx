// SPDX-License-Identifier: AGPL-3.0-only
// The actions on posts and the new-post form under the instance's federation direction (ADR-0076, FR-413), through
// the whole app (web-client/README.md T4): while the instance sends nothing out (`off`, `inbound`), the actions on a post of
// another server and a post that replies to or mentions an account of another server say that they do not reach
// it, and quoting a post of another server is not offered. The real router and QueryClient, MSW for the client API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { instanceFixture, remoteStatusFixture, type Schemas, statusFixture } from "../../test/fixtures";
import { apiBase, oauthPaths, tokenResponse } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Status } from "./api/status";

const bobsId = "0192f0c4-7a8e-7000-8000-0000000000f1";
const bobs: Status = { ...statusFixture(bobsId), content: "<p>Bob's thought</p>", quotable: true };
const carols: Status = { ...remoteStatusFixture, spoiler_text: "", media_attachments: [], quotable: true };
const carolsPath = `/@${carols.account.acct}/status/${carols.id}`;

const note = "Won't reach its server";
const composeNote = /^This won't reach other servers/;

/** Signs the viewer in, serves `statuses` by ID, and GET /instance with the federation direction `mode`. */
function serve(mode: Schemas["InstanceFederation"]["mode"], ...statuses: Status[]) {
  server.use(
    http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")),
    http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, federation: { mode } })),
    http.get(`${apiBase}/statuses/:id`, ({ params }) => {
      const found = statuses.find((s) => s.id === params["id"]);
      return HttpResponse.json(found ?? statusFixture(String(params["id"])));
    }),
  );
}

/** The action bar of the post the page is about. */
async function focusedPost() {
  return screen.findByRole("article", { name: /^Opened post/ });
}

beforeEach(() => {
  sessionStorage.clear();
});

describe("the actions on a post of another server", () => {
  test.each(["inbound", "off"] as const)(
    "say that they do not reach it, and offer no quote, while the direction is %s",
    async (mode) => {
      serve(mode, carols);
      await renderApp({ path: carolsPath });

      const post = await focusedPost();
      const text = await within(post).findByText(note);
      await waitFor(() => {
        expect(within(post).queryByRole("button", { name: "Quote" })).not.toBeInTheDocument();
      });
      expect(text).toBeVisible();
      expect(within(post).getByRole("button", { name: "Favourite" })).toHaveAccessibleDescription(note);
      expect(within(post).getByRole("button", { name: "Boost" })).toHaveAccessibleDescription(note);
      expect(within(post).getByRole("button", { name: "Reply" })).toHaveAccessibleDescription(note);
    },
  );

  test.each(["outbound", "both"] as const)(
    "say nothing more and offer the quote while the direction is %s",
    async (mode) => {
      serve(mode, carols);
      await renderApp({ path: carolsPath });

      const post = await focusedPost();
      expect(await within(post).findByRole("button", { name: "Quote" })).toBeEnabled();
      expect(within(post).queryByText(note)).not.toBeInTheDocument();
    },
  );

  test("leave the posts of this server as they are", async () => {
    serve("inbound", bobs);
    await renderApp({ path: `/@bob/status/${bobsId}` });

    const post = await focusedPost();
    expect(await within(post).findByRole("button", { name: "Quote" })).toBeEnabled();
    expect(within(post).queryByText(note)).not.toBeInTheDocument();
  });
});

describe("the new-post form", () => {
  test("says that a reply to a post of another server does not reach it", async () => {
    serve("inbound", carols);
    const { user } = await renderApp({ path: carolsPath });

    await user.click(await within(await focusedPost()).findByRole("button", { name: "Reply" }));
    const sheet = await screen.findByRole("dialog", { name: "New post" });
    expect(await within(sheet).findByText(composeNote)).toBeInTheDocument();
  });

  test("says nothing more of a reply while the instance sends out", async () => {
    serve("both", carols);
    const { user } = await renderApp({ path: carolsPath });

    await user.click(await within(await focusedPost()).findByRole("button", { name: "Reply" }));
    const sheet = await screen.findByRole("dialog", { name: "New post" });
    await within(sheet).findByText(/Replying to/);
    expect(within(sheet).queryByText(composeNote)).not.toBeInTheDocument();
  });

  test("says that a mention of an account of another server does not reach it, but not of one here", async () => {
    serve("off", bobs);
    const { user } = await renderApp({ path: `/@bob/status/${bobsId}` });

    await user.click(await within(await focusedPost()).findByRole("button", { name: "Reply" }));
    const sheet = await screen.findByRole("dialog", { name: "New post" });
    const text = within(sheet).getByRole("textbox", { name: "Your post" });
    await user.type(text, "Hi @bob@my.example");
    expect(within(sheet).queryByText(composeNote)).not.toBeInTheDocument();

    await user.type(text, " and @dave@elsewhere.example");
    expect(await within(sheet).findByText(composeNote)).toBeInTheDocument();
  });
});
