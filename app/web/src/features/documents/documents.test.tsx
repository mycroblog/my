// SPDX-License-Identifier: AGPL-3.0-only
// The links to the server's terms of service and privacy policy through the whole app (web-client/README.md T4, #489):
// on the sign-in page, in the settings, and in the phones' menu sheet, only for the documents the server
// publishes (GET /instance); a link opens the document's information page in the same tab (#1037).
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import { instanceFixture } from "../../test/fixtures";
import { apiBase, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

const termsUrl = "http://localhost:3000/about/terms";
const privacyUrl = "http://localhost:3000/about/privacy";
const documentHtml = "<h2>Accounts</h2><p>Be kind.</p><h2>Content</h2><p>Be honest.</p>";

function publish(documents: { terms_url?: string; privacy_policy_url?: string }) {
  server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, ...documents })));
}

describe("the links to the server's documents", () => {
  test("are in the footer of signed-out pages and open the page in the same tab", async () => {
    publish({ terms_url: termsUrl, privacy_policy_url: privacyUrl });
    await renderApp({ path: "/local" });

    const links = await within(await screen.findByRole("contentinfo")).findByRole("navigation", {
      name: "Terms and policies",
    });
    expect(
      within(links)
        .getAllByRole("link")
        .map((link) => [link.textContent, link.getAttribute("href")]),
    ).toEqual([
      ["Terms of service", "/about/terms"],
      ["Privacy policy", "/about/privacy"],
    ]);
    expect(within(links).getByRole("link", { name: "Privacy policy" })).not.toHaveAttribute("target");
  });

  test("leave out a document the server does not publish, and are not there without any", async () => {
    publish({ privacy_policy_url: privacyUrl });
    const view = await renderApp({ path: "/local" });
    const links = await within(await screen.findByRole("contentinfo")).findByRole("navigation", {
      name: "Terms and policies",
    });
    expect(
      within(links)
        .getAllByRole("link")
        .map((link) => link.textContent),
    ).toEqual(["Privacy policy"]);
    view.unmount();

    publish({});
    const { queryClient } = await renderApp({ path: "/local" });
    // The links would be there once the instance has loaded.
    await waitFor(() => {
      expect(queryClient.getQueryCache().find({ queryKey: ["get", "/instance"] })?.state.status).toBe("success");
    });
    expect(screen.queryByRole("navigation", { name: "Terms and policies" })).not.toBeInTheDocument();
  });

  test("are a section of the settings", async () => {
    server.use(signedInHandler);
    publish({ terms_url: termsUrl });
    await renderApp({ path: "/settings" });

    const section = await screen.findByRole("region", { name: "Terms and policies" });
    const links = within(section).getByRole("navigation", { name: "Terms and policies" });
    expect(within(links).getByRole("link", { name: "Terms of service" })).toHaveAttribute("href", "/about/terms");
  });

  test("are in the menu sheet under its items", async () => {
    server.use(signedInHandler);
    publish({ terms_url: termsUrl, privacy_policy_url: privacyUrl });
    const { user } = await renderApp({ path: "/home" });
    await screen.findByRole("heading", { level: 1, name: "Home" });

    await user.click(within(screen.getByRole("navigation", { name: "Main" })).getByRole("button", { name: "Menu" }));
    const sheet = await screen.findByRole("dialog", { name: "Menu" });
    const links = within(sheet).getByRole("navigation", { name: "Terms and policies" });
    expect(
      within(links)
        .getAllByRole("link")
        .map((link) => link.textContent),
    ).toEqual(["Terms of service", "Privacy policy"]);
  });
});

describe("the pages of the documents", () => {
  test.each([
    ["terms", "/about/terms", "Terms of service"],
    ["privacy_policy", "/about/privacy", "Privacy policy"],
    ["commerce_disclosure", "/about/commerce", "Commerce disclosures"],
  ])("show the %s with a table of contents, without signing in", async (kind, path, name) => {
    server.use(
      http.get(`${apiBase}/instance/documents/${kind}`, () =>
        HttpResponse.json({ kind, html: documentHtml, revised_at: "2026-10-12T08:00:00Z" }),
      ),
    );
    await renderApp({ path });

    expect(await screen.findByRole("heading", { level: 1, name })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { level: 2, name: "Accounts" })).toHaveAttribute("id");
    // eslint-disable-next-line testing-library/no-node-access -- the body is a plain div with no role.
    const body = screen.getByText("Be kind.").parentElement;
    expect(body).toHaveAttribute("lang", "");
    expect(body).toHaveAttribute("dir", "auto");
    expect(screen.getByText(/Last revised October 12, 2026/u)).toBeInTheDocument();
    const contents = screen.getAllByRole("navigation", { name: "On this page" })[0] as HTMLElement;
    expect(
      within(contents)
        .getAllByRole("link")
        .map((link) => link.textContent),
    ).toEqual(["Accounts", "Content"]);
  });

  test("show the not-found page for a document the server has not published", async () => {
    server.use(
      http.get(`${apiBase}/instance/documents/terms`, () =>
        HttpResponse.json(
          { type: "about:blank", title: "Not found", status: 404 },
          { status: 404, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    await renderApp({ path: "/about/terms" });

    expect(await screen.findByRole("heading", { level: 1, name: "Terms of service" })).toBeInTheDocument();
    expect(await screen.findByText("Not found")).toBeInTheDocument();
  });
});
