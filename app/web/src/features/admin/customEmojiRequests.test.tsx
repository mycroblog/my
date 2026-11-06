// SPDX-License-Identifier: AGPL-3.0-only
// The requests page of the custom emoji screen through the whole app (web-client/README.md T4, ADR-0070 §2, §9): an
// administrator with manage_custom_emojis sees how many requests await review on the link to the page, approves one, rejects one with a
// reason, is told why one cannot be approved, and lists those in another state; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { adminCustomEmojiFixture, adminCustomEmojiRequestFixture } from "./test/handlers";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
  server.use(signedInHandler, operatorHandler(["manage_custom_emojis"]));
});

describe("the requests page of the custom emoji screen", () => {
  test("counts the pending requests on the link to the page, and lists them with their requesters", async () => {
    server.use(
      http.get(`${apiBase}/admin/custom_emoji_requests`, () =>
        HttpResponse.json({
          items: [
            adminCustomEmojiRequestFixture("r1", "blobcat"),
            adminCustomEmojiRequestFixture("r2", "blobfox", { kind: "image", emoji_id: "e2" }),
          ],
        }),
      ),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis" });
    const pages = await screen.findByRole("navigation", { name: "Custom emoji pages" });
    const link = within(pages).getByRole("link", { name: /^Requests/ });
    expect(await within(link).findByText("2 requests await review")).toBeInTheDocument();
    await user.click(link);

    const region = await screen.findByRole("region", { name: "Requests for custom emoji" });
    expect(
      within(screen.getByRole("navigation", { name: "Custom emoji pages" })).getByRole("link", {
        name: /^Requests/,
      }),
    ).toHaveAttribute("aria-current", "page");
    expect(await within(region).findByRole("rowheader", { name: /^:blobcat:/ })).toBeInTheDocument();
    expect(within(region).getByText("A new image")).toBeInTheDocument();
    expect(within(region).getAllByRole("link", { name: "@bob" })[0]).toHaveAttribute("href", "/@bob");
  });

  test("approves a request, which leaves the pending ones", async () => {
    let approved = "";
    server.use(
      http.get(`${apiBase}/admin/custom_emoji_requests`, () =>
        HttpResponse.json({
          items: approved === "" ? [adminCustomEmojiRequestFixture("r1", "blobcat")] : [],
        }),
      ),
      http.post(`${apiBase}/admin/custom_emoji_requests/:id/approve`, ({ params }) => {
        approved = String(params["id"]);
        return HttpResponse.json(
          adminCustomEmojiRequestFixture(approved, "blobcat", {
            state: "approved",
            emoji_id: "e1",
            decided_at: "2026-10-22T00:00:00Z",
          }),
        );
      }),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis/requests" });
    await user.click(await screen.findByRole("button", { name: "Approve the request for ⁦:blobcat:⁩" }));

    const region = screen.getByRole("region", { name: "Requests for custom emoji" });
    expect(await within(region).findByText("The request for ⁦:blobcat:⁩ is approved.")).toBeInTheDocument();
    expect(approved).toBe("r1");
    expect(within(region).queryByRole("rowheader", { name: /^:blobcat:/ })).not.toBeInTheDocument();
  });

  test("rejects a request with a reason after asking", async () => {
    let sent: Schemas["RejectCustomEmojiRequestRequest"] | undefined;
    server.use(
      http.get(`${apiBase}/admin/custom_emoji_requests`, () =>
        HttpResponse.json({
          items: sent === undefined ? [adminCustomEmojiRequestFixture("r1", "blobcat")] : [],
        }),
      ),
      http.post(`${apiBase}/admin/custom_emoji_requests/:id/reject`, async ({ request }) => {
        sent = (await request.json()) as Schemas["RejectCustomEmojiRequestRequest"];
        return HttpResponse.json(
          adminCustomEmojiRequestFixture("r1", "blobcat", {
            state: "rejected",
            reason: sent.reason ?? "",
            decided_at: "2026-10-22T00:00:00Z",
          }),
        );
      }),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis/requests" });
    await user.click(await screen.findByRole("button", { name: "Reject the request for ⁦:blobcat:⁩" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: "Reject the request for ⁦:blobcat:⁩?",
    });
    await user.type(within(dialog).getByRole("textbox", { name: "Reason" }), "Too close to an existing emoji.");
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));

    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(sent).toEqual({ reason: "Too close to an existing emoji." });
    expect(screen.getByRole("region", { name: "Requests for custom emoji" })).toHaveTextContent(
      "The request for ⁦:blobcat:⁩ is rejected.",
    );
  });

  test("shows why a request cannot be approved, and keeps it", async () => {
    server.use(
      http.get(`${apiBase}/admin/custom_emoji_requests`, () =>
        HttpResponse.json({ items: [adminCustomEmojiRequestFixture("r1", "blobcat")] }),
      ),
      http.post(`${apiBase}/admin/custom_emoji_requests/:id/approve`, () =>
        HttpResponse.json(
          {
            type: "/api/v1/problems/custom-emoji-request-conflict",
            title: "The custom emoji request cannot be decided as it is.",
            status: 409,
            detail: "The requester can no longer create custom emoji. Reject the request instead.",
            reason: "requester_cannot_create",
          },
          { status: 409, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis/requests" });
    await user.click(await screen.findByRole("button", { name: "Approve the request for ⁦:blobcat:⁩" }));

    const region = screen.getByRole("region", { name: "Requests for custom emoji" });
    expect(await within(region).findByRole("alert")).toHaveTextContent(
      "Could not decide the request: The requester can no longer create custom emoji. Reject the request instead.",
    );
    expect(screen.getByRole("rowheader", { name: /^:blobcat:/ })).toBeInTheDocument();
  });

  test("lists the requests in another state from the URL, with the reason of a rejection", async () => {
    const states: (string | null)[] = [];
    server.use(
      http.get(`${apiBase}/admin/custom_emoji_requests`, ({ request }) => {
        const state = new URL(request.url).searchParams.get("state");
        states.push(state);
        return HttpResponse.json({
          items:
            state === "rejected"
              ? [
                  adminCustomEmojiRequestFixture("r1", "blobcat", {
                    state: "rejected",
                    reason: "No thanks.",
                  }),
                ]
              : [],
        });
      }),
    );
    await renderApp({ path: "/admin/custom-emojis/requests?state=rejected" });

    expect(await screen.findByText("No thanks.")).toBeInTheDocument();
    expect(states).toContain("rejected");
    expect(screen.queryByRole("button", { name: /Approve/ })).not.toBeInTheDocument();
  });

  test("names on an emoji's page who made it, or that their account is deleted", async () => {
    server.use(
      http.get(`${apiBase}/admin/custom_emojis/e2`, () =>
        HttpResponse.json(adminCustomEmojiFixture("e2", "blobfox", { created_by_deleted: true })),
      ),
    );
    await renderApp({ path: "/admin/custom-emojis/e2" });

    expect(await screen.findByText("A deleted account")).toBeInTheDocument();
  });

  test("names the command line on the page of an emoji registered there", async () => {
    server.use(
      http.get(`${apiBase}/admin/custom_emojis/e1`, () => HttpResponse.json(adminCustomEmojiFixture("e1", "blobcat"))),
    );
    await renderApp({ path: "/admin/custom-emojis/e1" });

    expect(await screen.findByText("The command line")).toBeInTheDocument();
  });
});
